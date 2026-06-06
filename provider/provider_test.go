package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Silo-Server/silo-plugin-tvdb/metadata"
)

func TestProviderSearchByTitleIncludesRemoteIDs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"token": "test-token",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/search":
			if r.URL.Query().Get("query") != "The Rookie: Feds" {
				t.Fatalf("query = %q, want The Rookie: Feds", r.URL.Query().Get("query"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": []map[string]any{{
					"name":     "The Rookie: Feds",
					"year":     "2022",
					"tvdb_id":  "420105",
					"overview": "A spinoff series.",
					"remote_ids": []map[string]any{
						{"type": 12, "id": "201992", "sourceName": "TheMovieDB.com"},
						{"type": 2, "id": "tt18076310", "sourceName": "IMDB"},
					},
				}},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	results, err := p.Search(context.Background(), metadata.SearchQuery{
		Title:       "The Rookie: Feds",
		ContentType: "series",
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	ids := results[0].ProviderIDs
	if ids["tvdb"] != "420105" || ids["tmdb"] != "201992" || ids["imdb"] != "tt18076310" {
		t.Fatalf("provider ids = %+v, want tvdb/tmdb/imdb", ids)
	}
}

func TestGetSeriesMetadataIncludesSourceNameRemoteIDs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"token": "test-token"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/series/100/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":       100,
					"name":     "Series",
					"overview": "Series overview",
					"remoteIds": []map[string]any{
						{"type": 0, "id": "201992", "sourceName": "TheMovieDB.com"},
						{"type": 0, "id": "tt18076310", "sourceName": "IMDb"},
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "100"},
		ContentType: "series",
	})
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}
	if result.ProviderIDs["tvdb"] != "100" || result.ProviderIDs["tmdb"] != "201992" || result.ProviderIDs["imdb"] != "tt18076310" {
		t.Fatalf("provider ids = %+v, want tvdb/tmdb/imdb", result.ProviderIDs)
	}
}

func TestFillRemoteIDsUsesTypeAndSourceNameWithoutOverwrite(t *testing.T) {
	t.Parallel()

	ids := map[string]string{
		"imdb": "nm-existing",
		"tmdb": "existing-tmdb",
	}
	fillRemoteIDs(ids, []RemoteID{
		{Type: 0, ID: "tt-source", SourceName: "IMDb"},
		{Type: 0, ID: "source-tmdb", SourceName: "The Movie Database"},
		{Type: 2, ID: "tt-type", SourceName: ""},
		{Type: 12, ID: "type-tmdb", SourceName: ""},
	})

	if ids["imdb"] != "nm-existing" {
		t.Fatalf("imdb overwritten: got %q", ids["imdb"])
	}
	if ids["tmdb"] != "existing-tmdb" {
		t.Fatalf("tmdb overwritten: got %q", ids["tmdb"])
	}

	ids = map[string]string{}
	fillRemoteIDs(ids, []RemoteID{
		{Type: 0, ID: "source-tmdb", SourceName: "TheMovieDB.com"},
		{Type: 0, ID: "tt-source", SourceName: "imdb.com"},
	})

	if ids["tmdb"] != "source-tmdb" || ids["imdb"] != "tt-source" {
		t.Fatalf("provider ids = %+v, want source-name tmdb/imdb", ids)
	}
}

func TestGetImagesReturnsArtworkImageURLs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"token": "test-token",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/series/99/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":   99,
					"name": "Series",
					"artworks": []map[string]any{
						{
							"id":        1,
							"type":      2,
							"image":     "https://artworks.example/poster-original.jpg",
							"thumbnail": "https://artworks.example/poster-thumb.jpg",
							"width":     2000,
							"height":    3000,
							"score":     10,
						},
						{
							"id":        2,
							"type":      3,
							"image":     "https://artworks.example/background-original.jpg",
							"thumbnail": "",
							"width":     3840,
							"height":    2160,
							"score":     8,
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	images, err := p.GetImages(context.Background(), metadata.ImageRequest{
		ProviderIDs: map[string]string{"tvdb": "99"},
		ContentType: "series",
	})
	if err != nil {
		t.Fatalf("GetImages() error = %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("len(images) = %d, want 2", len(images))
	}

	got := map[metadata.ImageType]string{}
	for _, img := range images {
		got[img.Type] = img.URL
	}

	if got[metadata.ImagePoster] != "https://artworks.example/poster-original.jpg" {
		t.Fatalf("poster URL = %q", got[metadata.ImagePoster])
	}
	if got[metadata.ImageBackdrop] != "https://artworks.example/background-original.jpg" {
		t.Fatalf("backdrop URL = %q", got[metadata.ImageBackdrop])
	}
}

func TestGetImagesPrefersTVDBPrimaryPoster(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"token": "test-token",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/series/99/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":    99,
					"name":  "Series",
					"image": "https://artworks.example/poster-primary.jpg",
					"artworks": []map[string]any{
						{
							"id":       1,
							"type":     2,
							"image":    "https://artworks.example/poster-primary.jpg",
							"language": "eng",
							"width":    2000,
							"height":   3000,
							"score":    10,
						},
						{
							"id":       2,
							"type":     2,
							"image":    "https://artworks.example/poster-textless.jpg",
							"language": "",
							"width":    2000,
							"height":   3000,
							"score":    11,
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	images, err := p.GetImages(context.Background(), metadata.ImageRequest{
		ProviderIDs: map[string]string{"tvdb": "99"},
		ContentType: "series",
	})
	if err != nil {
		t.Fatalf("GetImages() error = %v", err)
	}

	var primary, textless *metadata.RemoteImage
	for i := range images {
		switch images[i].URL {
		case "https://artworks.example/poster-primary.jpg":
			primary = &images[i]
		case "https://artworks.example/poster-textless.jpg":
			textless = &images[i]
		}
	}

	if primary == nil {
		t.Fatal("primary poster missing from GetImages() result")
	}
	if textless == nil {
		t.Fatal("alternate poster missing from GetImages() result")
	}
	if primary.Language != "en" {
		t.Fatalf("primary language = %q, want en", primary.Language)
	}
	if primary.Rating <= textless.Rating {
		t.Fatalf("primary rating = %v, textless rating = %v; want primary > textless", primary.Rating, textless.Rating)
	}
}

func TestGetImagesAddsPrimaryPosterWhenArtworkListMissesIt(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"token": "test-token",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/series/99/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":    99,
					"name":  "Series",
					"image": "https://artworks.example/poster-primary.jpg",
					"artworks": []map[string]any{
						{
							"id":       2,
							"type":     2,
							"image":    "https://artworks.example/poster-alt.jpg",
							"language": "",
							"width":    2000,
							"height":   3000,
							"score":    11,
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	images, err := p.GetImages(context.Background(), metadata.ImageRequest{
		ProviderIDs: map[string]string{"tvdb": "99"},
		ContentType: "series",
	})
	if err != nil {
		t.Fatalf("GetImages() error = %v", err)
	}

	var primary, alt *metadata.RemoteImage
	for i := range images {
		switch images[i].URL {
		case "https://artworks.example/poster-primary.jpg":
			primary = &images[i]
		case "https://artworks.example/poster-alt.jpg":
			alt = &images[i]
		}
	}

	if primary == nil {
		t.Fatal("primary poster was not appended to GetImages() result")
	}
	if alt == nil {
		t.Fatal("alternate poster missing from GetImages() result")
	}
	if primary.Rating <= alt.Rating {
		t.Fatalf("primary rating = %v, alt rating = %v; want primary > alt", primary.Rating, alt.Rating)
	}
}

// ---------------------------------------------------------------------------
// Translation tests
// ---------------------------------------------------------------------------

// newTranslationTestServer creates a test server that serves a Japanese series
// with embedded translations and per-entity translation endpoints.
func newTranslationTestServer(t *testing.T, translationCalls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"token": "test-token"},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/series/100/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":               100,
					"name":             "Original Japanese Title",
					"overview":         "Original Japanese overview",
					"originalLanguage": "jpn",
					"image":            "https://example.com/poster.jpg",
					"translations": map[string]any{
						"nameTranslations": []map[string]any{
							{"language": "jpn", "name": "Original Japanese Title"},
							{"language": "eng", "name": "English Series Title"},
						},
						"overviewTranslations": []map[string]any{
							{"language": "jpn", "overview": "Original Japanese overview"},
							{"language": "eng", "overview": "English series overview"},
						},
					},
					"seasons": []map[string]any{
						{"id": 200, "seriesId": 100, "number": 1, "type": map[string]any{"id": 1, "name": "Aired Order"}},
					},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/series/100/translations/eng":
			if translationCalls != nil {
				translationCalls.Add(1)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"name":     "English Series Title",
					"overview": "English series overview",
					"language": "eng",
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/seasons/200/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":       200,
					"seriesId": 100,
					"number":   1,
					"type":     map[string]any{"id": 1, "name": "Aired Order"},
					"episodes": []map[string]any{
						{"id": 301, "name": "Japanese Ep 1", "overview": "JP overview 1", "number": 1, "seasonNumber": 1},
						{"id": 302, "name": "Japanese Ep 2", "overview": "JP overview 2", "number": 2, "seasonNumber": 1},
						{"id": 303, "name": "Japanese Ep 3", "overview": "JP overview 3", "number": 3, "seasonNumber": 1},
					},
				},
			})

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/episodes/") && strings.HasSuffix(r.URL.Path, "/translations/eng"):
			if translationCalls != nil {
				translationCalls.Add(1)
			}
			// Extract episode ID from path.
			parts := strings.Split(r.URL.Path, "/")
			epID := parts[2]
			names := map[string]string{"301": "English Ep 1", "302": "English Ep 2", "303": "English Ep 3"}
			overviews := map[string]string{"301": "EN overview 1", "302": "EN overview 2", "303": "EN overview 3"}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"name":     names[epID],
					"overview": overviews[epID],
					"language": "eng",
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/seasons/200/translations/eng":
			if translationCalls != nil {
				translationCalls.Add(1)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"name":     "Season 1",
					"overview": "English season overview",
					"language": "eng",
				},
			})

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
}

func TestGetSeriesMetadata_TranslatesNonNativeLanguage(t *testing.T) {
	t.Parallel()

	server := newTranslationTestServer(t, nil)
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "100"},
		ContentType: "series",
		Language:    "en",
	})
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}
	if result.Title != "English Series Title" {
		t.Fatalf("Title = %q, want %q", result.Title, "English Series Title")
	}
	if result.Overview != "English series overview" {
		t.Fatalf("Overview = %q, want %q", result.Overview, "English series overview")
	}
}

func TestGetSeriesMetadata_SkipsTranslationWhenLanguageMatchesOriginal(t *testing.T) {
	t.Parallel()

	var translationCalls atomic.Int32
	server := newTranslationTestServer(t, &translationCalls)
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "100"},
		ContentType: "series",
		Language:    "ja", // matches originalLanguage "jpn"
	})
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}
	// Should use original data without fetching translations.
	if result.Title != "Original Japanese Title" {
		t.Fatalf("Title = %q, want %q", result.Title, "Original Japanese Title")
	}
	if translationCalls.Load() != 0 {
		t.Fatalf("translation endpoint called %d times, want 0", translationCalls.Load())
	}
}

func TestGetSeriesMetadata_UsesEmbeddedTranslationsWithoutDedicatedEndpoint(t *testing.T) {
	t.Parallel()

	var translationCalls atomic.Int32
	server := newTranslationTestServer(t, &translationCalls)
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "100"},
		ContentType: "series",
		Language:    "en",
	})
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}
	// Should get English data from embedded translations.
	if result.Title != "English Series Title" {
		t.Fatalf("Title = %q, want %q", result.Title, "English Series Title")
	}
	if result.Overview != "English series overview" {
		t.Fatalf("Overview = %q, want %q", result.Overview, "English series overview")
	}
	// Should NOT call the dedicated translation endpoint since embedded data was sufficient.
	if translationCalls.Load() != 0 {
		t.Fatalf("dedicated translation endpoint called %d times, want 0 (embedded was sufficient)", translationCalls.Load())
	}
}

func TestGetEpisodes_TranslatesConcurrently(t *testing.T) {
	t.Parallel()

	var translationCalls atomic.Int32
	server := newTranslationTestServer(t, &translationCalls)
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	episodes, err := p.GetEpisodes(context.Background(), metadata.EpisodesRequest{
		ProviderIDs:  map[string]string{"tvdb": "100"},
		SeasonNumber: 1,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("GetEpisodes() error = %v", err)
	}
	if len(episodes) != 3 {
		t.Fatalf("len(episodes) = %d, want 3", len(episodes))
	}

	// Verify all episodes were translated.
	for i, ep := range episodes {
		wantTitle := []string{"English Ep 1", "English Ep 2", "English Ep 3"}[i]
		wantOverview := []string{"EN overview 1", "EN overview 2", "EN overview 3"}[i]
		if ep.Title != wantTitle {
			t.Errorf("episodes[%d].Title = %q, want %q", i, ep.Title, wantTitle)
		}
		if ep.Overview != wantOverview {
			t.Errorf("episodes[%d].Overview = %q, want %q", i, ep.Overview, wantOverview)
		}
	}

	// Verify translation endpoint was called for each episode.
	if got := translationCalls.Load(); got != 3 {
		t.Fatalf("episode translation calls = %d, want 3", got)
	}
}

func TestGetEpisodes_SkipsTranslationWhenLanguageMatchesOriginal(t *testing.T) {
	t.Parallel()

	var translationCalls atomic.Int32
	server := newTranslationTestServer(t, &translationCalls)
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	episodes, err := p.GetEpisodes(context.Background(), metadata.EpisodesRequest{
		ProviderIDs:  map[string]string{"tvdb": "100"},
		SeasonNumber: 1,
		Language:     "ja", // matches originalLanguage "jpn"
	})
	if err != nil {
		t.Fatalf("GetEpisodes() error = %v", err)
	}
	if len(episodes) != 3 {
		t.Fatalf("len(episodes) = %d, want 3", len(episodes))
	}
	// Should use original data.
	if episodes[0].Title != "Japanese Ep 1" {
		t.Fatalf("episodes[0].Title = %q, want %q", episodes[0].Title, "Japanese Ep 1")
	}
	if translationCalls.Load() != 0 {
		t.Fatalf("translation endpoint called %d times, want 0", translationCalls.Load())
	}
}

func TestGetEpisodes_PartialTranslationFailureKeepsOriginalData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"token": "test-token"},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/series/100/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":               100,
					"name":             "Original Title",
					"originalLanguage": "jpn",
					"seasons": []map[string]any{
						{"id": 200, "seriesId": 100, "number": 1, "type": map[string]any{"id": 1}},
					},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/seasons/200/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id": 200, "number": 1,
					"type": map[string]any{"id": 1},
					"episodes": []map[string]any{
						{"id": 301, "name": "JP Ep 1", "overview": "JP ov 1", "number": 1, "seasonNumber": 1},
						{"id": 302, "name": "JP Ep 2", "overview": "JP ov 2", "number": 2, "seasonNumber": 1},
					},
				},
			})

		case r.URL.Path == "/episodes/301/translations/eng":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"name": "English Ep 1", "overview": "EN ov 1"},
			})

		case r.URL.Path == "/episodes/302/translations/eng":
			// Simulate a 404 — translation not available.
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "failure",
				"message": "translation not found",
			})

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	episodes, err := p.GetEpisodes(context.Background(), metadata.EpisodesRequest{
		ProviderIDs:  map[string]string{"tvdb": "100"},
		SeasonNumber: 1,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("GetEpisodes() error = %v", err)
	}
	if len(episodes) != 2 {
		t.Fatalf("len(episodes) = %d, want 2", len(episodes))
	}

	// Episode 1 should be translated.
	if episodes[0].Title != "English Ep 1" {
		t.Errorf("episodes[0].Title = %q, want %q", episodes[0].Title, "English Ep 1")
	}
	// Episode 2 should keep original data after translation failure.
	if episodes[1].Title != "JP Ep 2" {
		t.Errorf("episodes[1].Title = %q, want %q (original kept after failure)", episodes[1].Title, "JP Ep 2")
	}
	if episodes[1].Overview != "JP ov 2" {
		t.Errorf("episodes[1].Overview = %q, want %q (original kept after failure)", episodes[1].Overview, "JP ov 2")
	}
}
