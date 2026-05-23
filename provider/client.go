package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultBaseURL  = "https://api4.thetvdb.com/v4"
	defaultAPIKey   = "9bad61f9-16d5-468d-9c98-4c4038c13706"
	maxRetries      = 3
	maxResponseBody = 2 << 20 // 2 MB
)

// Client is an HTTP client for the TVDB v4 API.
type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
	token      string       // Bearer token from /login
	tokenMu    sync.RWMutex // protects token read/write
	refreshMu  sync.Mutex   // serialises re-auth attempts
	limiter    *rate.Limiter
}

// NewClient creates a TVDB API client with the given rate limit (requests per
// second). It uses the built-in project API key.
func NewClient(rateLimit int) *Client {
	if rateLimit <= 0 {
		rateLimit = 50
	}
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiKey:     defaultAPIKey,
		baseURL:    defaultBaseURL,
		limiter:    rate.NewLimiter(rate.Limit(rateLimit), rateLimit),
	}
}

// SetBaseURL overrides the API base URL. Used for testing.
func (c *Client) SetBaseURL(url string) {
	c.baseURL = url
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

// authenticate posts to /login with the API key and stores the bearer token.
func (c *Client) authenticate(ctx context.Context) error {
	body, err := json.Marshal(map[string]string{"apikey": c.apiKey})
	if err != nil {
		return fmt.Errorf("tvdb: marshal login body: %w", err)
	}
	reqURL := c.baseURL + "/login"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("tvdb: create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("tvdb: login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		return fmt.Errorf("tvdb: login failed HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var loginResp loginResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(&loginResp); err != nil {
		return fmt.Errorf("tvdb: decode login response: %w", err)
	}
	if loginResp.Data.Token == "" {
		return fmt.Errorf("tvdb: login returned empty token")
	}

	c.tokenMu.Lock()
	c.token = loginResp.Data.Token
	c.tokenMu.Unlock()

	return nil
}

// ensureToken calls authenticate if no token is set.
func (c *Client) ensureToken(ctx context.Context) error {
	c.tokenMu.RLock()
	hasToken := c.token != ""
	c.tokenMu.RUnlock()

	if hasToken {
		return nil
	}
	return c.refreshToken(ctx, "")
}

// refreshToken performs a serialised re-authentication with a double-check
// pattern.
func (c *Client) refreshToken(ctx context.Context, oldToken string) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	c.tokenMu.RLock()
	current := c.token
	c.tokenMu.RUnlock()

	if current != oldToken {
		return nil // already refreshed
	}

	return c.authenticate(ctx)
}

// getToken returns the current bearer token.
func (c *Client) getToken() string {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.token
}

// ---------------------------------------------------------------------------
// Core HTTP
// ---------------------------------------------------------------------------

// doGet executes a GET request against the TVDB API with rate limiting,
// Bearer token auth, automatic 401 refresh, and JSON decoding into dest.
func (c *Client) doGet(ctx context.Context, path string, dest any) error {
	if err := c.ensureToken(ctx); err != nil {
		return err
	}

	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}

	reqURL := c.baseURL + path
	authRetries := 0

	for attempt := 0; attempt <= maxRetries; attempt++ {
		tok := c.getToken()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return fmt.Errorf("tvdb: create request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("tvdb: request failed: %w", err)
		}

		// 401 Unauthorized — refresh token and retry once.
		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			authRetries++
			if authRetries > 1 {
				return fmt.Errorf("tvdb: authentication failed after token refresh")
			}
			if err := c.refreshToken(ctx, tok); err != nil {
				return fmt.Errorf("tvdb: token refresh failed: %w", err)
			}
			attempt--
			continue
		}

		// 429 Too Many Requests.
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			retryAfter := resp.Header.Get("Retry-After")
			if attempt < maxRetries {
				backoff := retryAfterOrDefault(resp, attempt)
				slog.Warn("tvdb: rate limited by API, backing off",
					"path", path,
					"attempt", attempt+1,
					"retry_after_header", retryAfter,
					"backoff", backoff.String(),
				)
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return ctx.Err()
				}
				continue
			}
			slog.Error("tvdb: rate limited after max retries",
				"path", path,
				"retries", maxRetries,
				"retry_after_header", retryAfter,
			)
			return fmt.Errorf("tvdb: rate limited after %d retries", maxRetries)
		}

		// 5xx — retry with exponential backoff.
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			if attempt < maxRetries {
				backoff := time.Duration(1<<attempt) * time.Second
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return ctx.Err()
				}
				continue
			}
			return fmt.Errorf("tvdb: server error %d after %d retries", resp.StatusCode, maxRetries)
		}

		// 4xx — client error, no retry.
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
			resp.Body.Close()
			var apiErr apiError
			if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Message != "" {
				return fmt.Errorf("tvdb: HTTP %d: %s", resp.StatusCode, apiErr.Message)
			}
			return fmt.Errorf("tvdb: HTTP %d", resp.StatusCode)
		}

		// 2xx — decode response.
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(dest)
		resp.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("tvdb: decode response: %w", decodeErr)
		}
		return nil
	}
	return fmt.Errorf("tvdb: max retries exceeded")
}

// retryAfterOrDefault parses the Retry-After header (seconds) or falls back
// to exponential backoff.
func retryAfterOrDefault(resp *http.Response, attempt int) time.Duration {
	if val := resp.Header.Get("Retry-After"); val != "" {
		if secs, err := strconv.Atoi(val); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return time.Duration(1<<attempt) * time.Second
}

// ---------------------------------------------------------------------------
// Public endpoint methods
// ---------------------------------------------------------------------------

// Search searches TVDB for entities matching the query.
func (c *Client) Search(ctx context.Context, query, mediaType string) ([]SearchResult, error) {
	path := "/search?query=" + url.QueryEscape(query)
	if mediaType != "" {
		path += "&type=" + url.QueryEscape(mediaType)
	}
	var resp apiResponse[[]SearchResult]
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// SearchByRemoteID looks up TVDB entities by an external ID (e.g. IMDb ID).
func (c *Client) SearchByRemoteID(ctx context.Context, remoteID string) ([]RemoteIDResult, error) {
	path := "/search/remoteid/" + url.PathEscape(remoteID)
	var resp apiResponse[[]RemoteIDResult]
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetPersonExtended fetches the extended record for a person.
func (c *Client) GetPersonExtended(ctx context.Context, id int) (*PeopleExtendedRecord, error) {
	path := fmt.Sprintf("/people/%d/extended?meta=translations", id)
	var resp apiResponse[PeopleExtendedRecord]
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetSeriesExtended fetches the extended record for a series, including
// translations (needed to resolve preferred-language titles and overviews).
func (c *Client) GetSeriesExtended(ctx context.Context, id int) (*SeriesExtendedRecord, error) {
	path := fmt.Sprintf("/series/%d/extended?meta=translations", id)
	var resp apiResponse[SeriesExtendedRecord]
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetMovieExtended fetches the extended record for a movie, including
// translations (needed because movie records don't have an inline overview).
func (c *Client) GetMovieExtended(ctx context.Context, id int) (*MovieExtendedRecord, error) {
	path := fmt.Sprintf("/movies/%d/extended?meta=translations", id)
	var resp apiResponse[MovieExtendedRecord]
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetSeasonExtended fetches the extended record for a season including
// episodes and translations.
func (c *Client) GetSeasonExtended(ctx context.Context, id int) (*SeasonExtendedRecord, error) {
	path := fmt.Sprintf("/seasons/%d/extended?meta=translations", id)
	var resp apiResponse[SeasonExtendedRecord]
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetSeriesTranslation fetches a single-language translation for a series.
// lang3 must be a 3-letter ISO 639-2 code (e.g. "jpn", "fra").
func (c *Client) GetSeriesTranslation(ctx context.Context, id int, lang3 string) (*TranslationRecord, error) {
	path := fmt.Sprintf("/series/%d/translations/%s", id, url.PathEscape(lang3))
	var resp apiResponse[TranslationRecord]
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetMovieTranslation fetches a single-language translation for a movie.
func (c *Client) GetMovieTranslation(ctx context.Context, id int, lang3 string) (*TranslationRecord, error) {
	path := fmt.Sprintf("/movies/%d/translations/%s", id, url.PathEscape(lang3))
	var resp apiResponse[TranslationRecord]
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetSeasonTranslation fetches a single-language translation for a season.
func (c *Client) GetSeasonTranslation(ctx context.Context, id int, lang3 string) (*TranslationRecord, error) {
	path := fmt.Sprintf("/seasons/%d/translations/%s", id, url.PathEscape(lang3))
	var resp apiResponse[TranslationRecord]
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetEpisodeTranslation fetches a single-language translation for an episode.
func (c *Client) GetEpisodeTranslation(ctx context.Context, id int, lang3 string) (*TranslationRecord, error) {
	path := fmt.Sprintf("/episodes/%d/translations/%s", id, url.PathEscape(lang3))
	var resp apiResponse[TranslationRecord]
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}
