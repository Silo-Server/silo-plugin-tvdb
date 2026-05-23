package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Silo-Server/silo-plugin-tvdb/metadata"
)

func TestClientGetPersonExtended(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.Path == "/people/321/extended":
			if got := r.URL.Query().Get("meta"); got != "translations" {
				t.Fatalf("meta query = %q, want translations", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":         321,
					"name":       "Sigourney Weaver",
					"birth":      "1949-10-08",
					"birthPlace": "New York City, New York, USA",
					"death":      "",
					"image":      "https://artworks.thetvdb.com/banners/persons/321.jpg",
					"biographies": []map[string]any{
						{
							"language":  "eng",
							"biography": "English biography",
						},
					},
					"remoteIds": []map[string]any{
						{
							"id":         "nm0000244",
							"type":       2,
							"sourceName": "IMDb",
						},
						{
							"id":         "10205",
							"type":       12,
							"sourceName": "TMDB",
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

	person, err := client.GetPersonExtended(context.Background(), 321)
	if err != nil {
		t.Fatalf("GetPersonExtended() error = %v", err)
	}
	if person == nil {
		t.Fatal("GetPersonExtended() returned nil person")
	}
	if person.ID != 321 {
		t.Fatalf("person.ID = %d, want 321", person.ID)
	}
	if person.Name != "Sigourney Weaver" {
		t.Fatalf("person.Name = %q, want Sigourney Weaver", person.Name)
	}
	if person.Image != "https://artworks.thetvdb.com/banners/persons/321.jpg" {
		t.Fatalf("person.Image = %q", person.Image)
	}
	if len(person.Biographies) != 1 || person.Biographies[0].Biography != "English biography" {
		t.Fatalf("person.Biographies = %#v", person.Biographies)
	}
}

func TestProviderGetPersonDetail_UsesRemoteIDsAndPreferredBiography(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.Path == "/search/remoteid/nm0000244":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": []map[string]any{
					{
						"people": map[string]any{
							"id":   321,
							"name": "Sigourney Weaver",
						},
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/people/321/extended":
			if got := r.URL.Query().Get("meta"); got != "translations" {
				t.Fatalf("meta query = %q, want translations", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":         321,
					"name":       "Sigourney Weaver",
					"birth":      "1949-10-08",
					"birthPlace": "New York City, New York, USA",
					"death":      "",
					"image":      "https://artworks.thetvdb.com/banners/persons/321.jpg",
					"biographies": []map[string]any{
						{
							"language":  "eng",
							"biography": "English biography",
						},
						{
							"language":  "fra",
							"biography": "Biographie francaise",
						},
					},
					"remoteIds": []map[string]any{
						{
							"id":         "nm0000244",
							"type":       2,
							"sourceName": "IMDb",
						},
						{
							"id":         "10205",
							"type":       12,
							"sourceName": "TMDB",
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

	person, err := p.GetPersonDetail(context.Background(), metadata.PersonDetailRequest{
		ProviderIDs: map[string]string{"imdb": "nm0000244"},
		Language:    "fr-CA",
	})
	if err != nil {
		t.Fatalf("GetPersonDetail() error = %v", err)
	}
	if person == nil {
		t.Fatal("GetPersonDetail() returned nil person")
	}
	if person.Name != "Sigourney Weaver" {
		t.Fatalf("person.Name = %q, want Sigourney Weaver", person.Name)
	}
	if person.Bio != "Biographie francaise" {
		t.Fatalf("person.Bio = %q, want Biographie francaise", person.Bio)
	}
	if person.BirthDate != "1949-10-08" {
		t.Fatalf("person.BirthDate = %q, want 1949-10-08", person.BirthDate)
	}
	if person.Birthplace != "New York City, New York, USA" {
		t.Fatalf("person.Birthplace = %q", person.Birthplace)
	}
	if person.PhotoPath != "https://artworks.thetvdb.com/banners/persons/321.jpg" {
		t.Fatalf("person.PhotoPath = %q", person.PhotoPath)
	}
	if person.ProviderIDs["tvdb"] != "321" {
		t.Fatalf("person.ProviderIDs[tvdb] = %q, want 321", person.ProviderIDs["tvdb"])
	}
	if person.ProviderIDs["imdb"] != "nm0000244" {
		t.Fatalf("person.ProviderIDs[imdb] = %q, want nm0000244", person.ProviderIDs["imdb"])
	}
	if person.ProviderIDs["tmdb"] != "10205" {
		t.Fatalf("person.ProviderIDs[tmdb] = %q, want 10205", person.ProviderIDs["tmdb"])
	}
}
