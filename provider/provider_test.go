package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Silo-Server/silo-plugin-sportarr/metadata"
)

func encodeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func newTestProvider(t *testing.T, handler http.Handler) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient(100)
	c.SetBaseURL(srv.URL)
	return NewProviderWithClient(c)
}

func TestSearchByTitle(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeJSON(t, w, AgentSearchResponse{
			Results: []AgentSearchResult{
				{ID: "league-1", Title: "Premier League", Year: 1992},
				{ID: "league-2", Title: "Premier League 2", Year: 2023},
			},
		})
	}))

	results, err := p.Search(context.Background(), metadata.SearchQuery{Title: "Premier League"})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ProviderIDs["sportarr"] != "league-1" {
		t.Errorf("expected sportarr ID league-1, got %s", results[0].ProviderIDs["sportarr"])
	}
}

func TestSearchByProviderID(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/agents/series/league-1" {
			t.Errorf("expected series lookup, got %s", r.URL.Path)
		}
		encodeJSON(t, w, AgentSeriesResponse{
			Title: "UFC", Year: 1993, Summary: "MMA league",
		})
	}))

	results, err := p.Search(context.Background(), metadata.SearchQuery{
		ProviderIDs: map[string]string{"sportarr": "league-1"},
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "UFC" {
		t.Errorf("expected name UFC, got %s", results[0].Name)
	}
}

func TestGetMetadata(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/metadata/agents/series/league-1":
			encodeJSON(t, w, AgentSeriesResponse{
				Title:   "Formula 1",
				Summary: "Open-wheel racing",
				Year:    1950,
				Genres:  []string{"Motorsport"},
				Studio:  "FIA",
			})
		case "/api/metadata/agents/series/league-1/seasons":
			encodeJSON(t, w, AgentSeasonsResponse{
				Seasons: []AgentSeason{
					{SeasonNumber: 2023, Name: "2023"},
					{SeasonNumber: 2024, Name: "2024"},
				},
			})
		case "/api/v1/images/entity/league/league-1":
			encodeJSON(t, w, EntityImageResponse{
				Images: []EntityImage{
					{ID: "p1", ImageType: "poster", URL: "https://sportarr.net/api/v1/images/p1", IsPrimary: true},
					{ID: "b1", ImageType: "backdrop", URL: "https://sportarr.net/api/v1/images/b1", IsPrimary: true},
					{ID: "l1", ImageType: "logo", URL: "https://sportarr.net/api/v1/images/l1"},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"sportarr": "league-1"},
		ContentType: "series",
	})
	if err != nil {
		t.Fatalf("get metadata failed: %v", err)
	}
	if !result.HasMetadata {
		t.Fatal("expected HasMetadata=true")
	}
	if result.Title != "Formula 1" {
		t.Errorf("expected title Formula 1, got %s", result.Title)
	}
	if result.SeasonCount != 2 {
		t.Errorf("expected 2 seasons, got %d", result.SeasonCount)
	}
	if len(result.Genres) != 1 || result.Genres[0] != "Motorsport" {
		t.Errorf("unexpected genres: %v", result.Genres)
	}
	if result.PosterPath != "https://sportarr.net/api/v1/images/p1" {
		t.Errorf("expected poster path from entity images, got %s", result.PosterPath)
	}
	if result.BackdropPath != "https://sportarr.net/api/v1/images/b1" {
		t.Errorf("expected backdrop path from entity images, got %s", result.BackdropPath)
	}
	if result.LogoPath != "https://sportarr.net/api/v1/images/l1" {
		t.Errorf("expected logo path from entity images, got %s", result.LogoPath)
	}
}

func TestGetSeasons(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/metadata/agents/series/league-1/seasons":
			encodeJSON(t, w, AgentSeasonsResponse{
				Seasons: []AgentSeason{
					{CompetitionSeasonID: "cs-2023", SeasonNumber: 2023, Name: "2023 Season", EpisodeCount: 23},
					{CompetitionSeasonID: "cs-2024", SeasonNumber: 2024, Name: "2024 Season", EpisodeCount: 24},
				},
			})
		case "/api/v1/images/entity/season/cs-2023":
			encodeJSON(t, w, EntityImageResponse{
				Images: []EntityImage{
					{ID: "sp1", ImageType: "poster", URL: "https://sportarr.net/api/v1/images/sp1", IsPrimary: true},
				},
			})
		case "/api/v1/images/entity/season/cs-2024":
			encodeJSON(t, w, EntityImageResponse{
				Images: []EntityImage{
					{ID: "sp2", ImageType: "poster", URL: "https://sportarr.net/api/v1/images/sp2"},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))

	seasons, err := p.GetSeasons(context.Background(), metadata.SeasonsRequest{
		ProviderIDs: map[string]string{"sportarr": "league-1"},
	})
	if err != nil {
		t.Fatalf("get seasons failed: %v", err)
	}
	if len(seasons) != 2 {
		t.Fatalf("expected 2 seasons, got %d", len(seasons))
	}
	if seasons[0].SeasonNumber != 2023 {
		t.Errorf("expected season 2023, got %d", seasons[0].SeasonNumber)
	}
	if seasons[1].Title != "2024 Season" {
		t.Errorf("expected title 2024 Season, got %s", seasons[1].Title)
	}
	if seasons[0].PosterPath != "https://sportarr.net/api/v1/images/sp1" {
		t.Errorf("expected poster from entity images, got %s", seasons[0].PosterPath)
	}
	if seasons[1].PosterPath != "https://sportarr.net/api/v1/images/sp2" {
		t.Errorf("expected poster from entity images, got %s", seasons[1].PosterPath)
	}
}

func TestGetSeasonsWithoutCompetitionSeasonID(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/metadata/agents/series/league-1/seasons":
			encodeJSON(t, w, AgentSeasonsResponse{
				Seasons: []AgentSeason{
					{SeasonNumber: 2023, Name: "2023 Season"},
				},
			})
		default:
			t.Errorf("unexpected request to %s (should not call entity image API)", r.URL.Path)
			w.WriteHeader(404)
		}
	}))

	seasons, err := p.GetSeasons(context.Background(), metadata.SeasonsRequest{
		ProviderIDs: map[string]string{"sportarr": "league-1"},
	})
	if err != nil {
		t.Fatalf("get seasons failed: %v", err)
	}
	if len(seasons) != 1 {
		t.Fatalf("expected 1 season, got %d", len(seasons))
	}
	if seasons[0].PosterPath != "" {
		t.Errorf("expected empty poster path when no CompetitionSeasonID, got %s", seasons[0].PosterPath)
	}
}

func TestGetEpisodes(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeJSON(t, w, AgentEpisodesResponse{
			Episodes: []AgentEpisode{
				{ID: "ev-1", Title: "Monaco GP", SeasonNumber: 2024, EpisodeNumber: 8, AirDate: "2024-05-26", DurationMinutes: 120},
				{ID: "ev-2", Title: "Canadian GP", SeasonNumber: 2024, EpisodeNumber: 9, AirDate: "2024-06-09", DurationMinutes: 120},
			},
		})
	}))

	episodes, err := p.GetEpisodes(context.Background(), metadata.EpisodesRequest{
		ProviderIDs:  map[string]string{"sportarr": "league-1"},
		SeasonNumber: 2024,
	})
	if err != nil {
		t.Fatalf("get episodes failed: %v", err)
	}
	if len(episodes) != 2 {
		t.Fatalf("expected 2 episodes, got %d", len(episodes))
	}
	if episodes[0].Title != "Monaco GP" {
		t.Errorf("expected Monaco GP, got %s", episodes[0].Title)
	}
	if episodes[0].ProviderIDs["sportarr"] != "ev-1" {
		t.Errorf("expected sportarr ID ev-1, got %s", episodes[0].ProviderIDs["sportarr"])
	}
}

func TestGetImagesForSeries(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/images/entity/league/league-1" {
			t.Errorf("expected entity image path, got %s", r.URL.Path)
		}
		w1, h1 := 680, 1000
		w2, h2 := 1920, 1080
		encodeJSON(t, w, EntityImageResponse{
			Images: []EntityImage{
				{ID: "img-1", ImageType: "poster", URL: "https://sportarr.net/api/v1/images/img-1", IsPrimary: true, Width: &w1, Height: &h1},
				{ID: "img-2", ImageType: "backdrop", URL: "https://sportarr.net/api/v1/images/img-2", Width: &w2, Height: &h2},
				{ID: "img-3", ImageType: "logo", URL: "https://sportarr.net/api/v1/images/img-3"},
				{ID: "img-4", ImageType: "banner", URL: "https://sportarr.net/api/v1/images/img-4"},
			},
		})
	}))

	images, err := p.GetImages(context.Background(), metadata.ImageRequest{
		ProviderIDs: map[string]string{"sportarr": "league-1"},
		ContentType: "series",
	})
	if err != nil {
		t.Fatalf("get images failed: %v", err)
	}
	if len(images) != 4 {
		t.Fatalf("expected 4 images, got %d", len(images))
	}
	// Primary poster should sort first
	if images[0].Type != metadata.ImagePoster {
		t.Errorf("expected poster first (is_primary), got type %d", images[0].Type)
	}
	if images[0].Width != 680 || images[0].Height != 1000 {
		t.Errorf("expected 680x1000, got %dx%d", images[0].Width, images[0].Height)
	}
}

func TestGetImagesForEpisode(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/images/entity/event/ev-1" {
			t.Errorf("expected entity image path for event, got %s", r.URL.Path)
		}
		encodeJSON(t, w, EntityImageResponse{
			Images: []EntityImage{
				{ID: "img-t1", ImageType: "thumbnail", URL: "https://sportarr.net/api/v1/images/img-t1"},
			},
		})
	}))

	images, err := p.GetImages(context.Background(), metadata.ImageRequest{
		ProviderIDs: map[string]string{"sportarr": "ev-1"},
		ContentType: "episode",
	})
	if err != nil {
		t.Fatalf("get images failed: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].Type != metadata.ImageStill {
		t.Errorf("expected still type, got %d", images[0].Type)
	}
}

func TestGetImagesForSeason(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/images/entity/season/season-uuid-1" {
			t.Errorf("expected entity image path for season, got %s", r.URL.Path)
		}
		encodeJSON(t, w, EntityImageResponse{
			Images: []EntityImage{
				{ID: "img-sp1", ImageType: "poster", URL: "https://sportarr.net/api/v1/images/img-sp1", IsPrimary: true},
			},
		})
	}))

	images, err := p.GetImages(context.Background(), metadata.ImageRequest{
		ProviderIDs: map[string]string{"sportarr": "season-uuid-1"},
		ContentType: "season",
	})
	if err != nil {
		t.Fatalf("get images failed: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].Type != metadata.ImagePoster {
		t.Errorf("expected poster type, got %d", images[0].Type)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not make any HTTP request for empty query")
	}))

	results, err := p.Search(context.Background(), metadata.SearchQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

func TestGetMetadataNoProviderID(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not make any HTTP request without provider ID")
	}))

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestPickPrimaryURL(t *testing.T) {
	images := []EntityImage{
		{ImageType: "poster", URL: "https://sportarr.net/api/v1/images/p1", Priority: 5},
		{ImageType: "poster", URL: "https://sportarr.net/api/v1/images/p2", IsPrimary: true, Priority: 1},
		{ImageType: "backdrop", URL: "https://sportarr.net/api/v1/images/b1", IsPrimary: true},
		{ImageType: "logo", URL: "https://sportarr.net/api/v1/images/l1"},
	}

	tests := []struct {
		name      string
		imageType string
		want      string
	}{
		{"primary poster wins over higher priority", "poster", "https://sportarr.net/api/v1/images/p2"},
		{"backdrop", "backdrop", "https://sportarr.net/api/v1/images/b1"},
		{"logo", "logo", "https://sportarr.net/api/v1/images/l1"},
		{"no match returns empty", "banner", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickPrimaryURL(images, tt.imageType)
			if got != tt.want {
				t.Errorf("pickPrimaryURL(%q) = %q, want %q", tt.imageType, got, tt.want)
			}
		})
	}
}

func TestPickPrimaryURLPriorityTiebreak(t *testing.T) {
	images := []EntityImage{
		{ImageType: "poster", URL: "https://sportarr.net/api/v1/images/low", Priority: 1},
		{ImageType: "poster", URL: "https://sportarr.net/api/v1/images/high", Priority: 10},
	}
	got := pickPrimaryURL(images, "poster")
	if got != "https://sportarr.net/api/v1/images/high" {
		t.Errorf("expected highest priority poster, got %s", got)
	}
}

func TestPickPrimaryURLEmpty(t *testing.T) {
	got := pickPrimaryURL(nil, "poster")
	if got != "" {
		t.Errorf("expected empty for nil images, got %s", got)
	}
}

func TestEntityImagesToRemote(t *testing.T) {
	w1, h1 := 680, 1000
	images := []EntityImage{
		{ImageType: "poster", URL: "https://sportarr.net/api/v1/images/p1", Width: &w1, Height: &h1, Priority: 1},
		{ImageType: "backdrop", URL: "https://sportarr.net/api/v1/images/b1", IsPrimary: true},
		{ImageType: "logo", URL: "https://sportarr.net/api/v1/images/l1"},
		{ImageType: "banner", URL: "https://sportarr.net/api/v1/images/bn1"},
		{ImageType: "thumbnail", URL: "https://sportarr.net/api/v1/images/t1"},
		{ImageType: "headshot", URL: "https://sportarr.net/api/v1/images/skip"},
	}

	result := entityImagesToRemote(images)

	if len(result) != 5 {
		t.Fatalf("expected 5 images (headshot skipped), got %d", len(result))
	}

	// Primary backdrop should sort first
	if result[0].Type != metadata.ImageBackdrop {
		t.Errorf("expected backdrop first (is_primary), got type %d", result[0].Type)
	}
	if result[0].URL != "https://sportarr.net/api/v1/images/b1" {
		t.Errorf("unexpected URL: %s", result[0].URL)
	}

	// Check width/height populated
	found := false
	for _, img := range result {
		if img.Type == metadata.ImagePoster {
			found = true
			if img.Width != 680 || img.Height != 1000 {
				t.Errorf("expected 680x1000, got %dx%d", img.Width, img.Height)
			}
		}
	}
	if !found {
		t.Error("poster not found in results")
	}
}

func TestEntityImagesToRemoteEmpty(t *testing.T) {
	result := entityImagesToRemote(nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestMovieSearchRequiresLocalURLTitleAndYear(t *testing.T) {
	c := NewClient(100)
	c.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("Movie request must not use the default hub URL: %s", req.URL)
		return nil, nil
	})
	p := NewProviderWithClient(c)

	if got := p.ForTypes(); len(got) != 2 || got[0] != "series" || got[1] != "movie" {
		t.Fatalf("ForTypes() = %v, want series and movie", got)
	}
	results, err := p.Search(context.Background(), metadata.SearchQuery{
		ContentType: "movie",
		Title:       "UFC 300",
		Year:        2024,
	})
	if err != nil || results != nil {
		t.Fatalf("default hub Movie search = (%v, %v), want (nil, nil)", results, err)
	}

	p = newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("Movie search must require both title and year")
	}))
	for _, query := range []metadata.SearchQuery{
		{ContentType: "movie", Title: "UFC 300"},
		{ContentType: "movie", Year: 2024},
	} {
		results, err := p.Search(context.Background(), query)
		if err != nil || results != nil {
			t.Fatalf("incomplete Movie search = (%v, %v), want (nil, nil)", results, err)
		}
	}
}

func TestMovieSearchRejectsPublicHubURLVariants(t *testing.T) {
	for _, baseURL := range []string{
		"https://sportarr.net:443",
		"https://SPORTARR.NET",
		"http://sportarr.net",
		"http://sportarr.net:80",
	} {
		t.Run(baseURL, func(t *testing.T) {
			c := NewClient(100)
			c.SetBaseURL(baseURL)
			c.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				t.Fatalf("Movie request must not use public Sportarr URL %q", baseURL)
				return nil, nil
			})
			p := NewProviderWithClient(c)

			results, err := p.Search(context.Background(), metadata.SearchQuery{
				ContentType: "movie",
				Title:       "UFC 300",
				Year:        2024,
			})
			if err != nil || results != nil {
				t.Fatalf("Movie search for public URL %q = (%v, %v), want (nil, nil)", baseURL, results, err)
			}
		})
	}
}

func TestMovieSearchUsesMovieAgent(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/agents/movies/search" {
			t.Errorf("expected Movie search path, got %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("title"); got != "UFC 300" {
			t.Errorf("title = %q, want UFC 300", got)
		}
		if got := r.URL.Query().Get("year"); got != "2024" {
			t.Errorf("year = %q, want 2024", got)
		}
		encodeJSON(t, w, AgentMovieSearchResponse{Results: []AgentMovieSearchResult{{
			ID: "v1.event-key", Title: "UFC 300", Year: 2024, Summary: "A title fight card.", PosterURL: "https://sportarr.local/poster",
		}}})
	}))

	results, err := p.Search(context.Background(), metadata.SearchQuery{
		ContentType: "movie", Title: "UFC 300", Year: 2024,
	})
	if err != nil {
		t.Fatalf("Movie search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one Movie result, got %d", len(results))
	}
	if got := results[0]; got.ProviderIDs["sportarr"] != "v1.event-key" || got.Overview != "A title fight card." || got.ImageURL != "https://sportarr.local/poster" {
		t.Errorf("unexpected Movie search result: %+v", got)
	}
}

func TestMovieSearchByProviderIDFetchesDetail(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/agents/movies/v1.event-key" {
			t.Errorf("expected Movie detail path, got %s", r.URL.Path)
		}
		encodeJSON(t, w, AgentMovieResponse{
			ID: "v1.event-key", Title: "UFC 300", Year: 2024, Summary: "A title fight card.", PosterURL: "https://sportarr.local/poster",
		})
	}))

	results, err := p.Search(context.Background(), metadata.SearchQuery{
		ContentType: "movie", ProviderIDs: map[string]string{"sportarr": "v1.event-key"},
	})
	if err != nil {
		t.Fatalf("Movie provider-ID search failed: %v", err)
	}
	if len(results) != 1 || results[0].Name != "UFC 300" {
		t.Fatalf("unexpected Movie provider-ID search result: %+v", results)
	}
}

func TestMovieSearchByProviderIDFallsBackToConservativeSearch(t *testing.T) {
	requests := 0
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/api/metadata/agents/movies/v1.stale-key":
			w.WriteHeader(http.StatusNotFound)
		case "/api/metadata/agents/movies/search":
			if got := r.URL.Query().Get("title"); got != "UFC 300" {
				t.Errorf("fallback title = %q, want UFC 300", got)
			}
			if got := r.URL.Query().Get("year"); got != "2024" {
				t.Errorf("fallback year = %q, want 2024", got)
			}
			encodeJSON(t, w, AgentMovieSearchResponse{Results: []AgentMovieSearchResult{{ID: "v1.current-key", Title: "UFC 300", Year: 2024}}})
		default:
			t.Errorf("unexpected Movie request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	results, err := p.Search(context.Background(), metadata.SearchQuery{
		ContentType: "movie", Title: "UFC 300", Year: 2024, ProviderIDs: map[string]string{"sportarr": "v1.stale-key"},
	})
	if err != nil {
		t.Fatalf("Movie fallback search failed: %v", err)
	}
	if requests != 2 || len(results) != 1 || results[0].ProviderIDs["sportarr"] != "v1.current-key" {
		t.Fatalf("unexpected Movie fallback result after %d requests: %+v", requests, results)
	}
}

func TestGetMovieMetadataMapsReleaseDateAndTypedArtwork(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/agents/movies/v1.event-key" {
			t.Errorf("expected Movie detail path, got %s", r.URL.Path)
		}
		encodeJSON(t, w, AgentMovieResponse{
			ID:          "v1.event-key",
			Title:       "UFC 300",
			SortTitle:   "UFC 300",
			Year:        2024,
			ReleaseDate: "2024-04-13",
			Summary:     "A title fight card.",
			Studio:      "UFC",
			Genres:      []string{"MMA", "Sports"},
			PosterURL:   "https://sportarr.local/poster",
			BackdropURL: "https://sportarr.local/backdrop",
			StillURL:    "https://sportarr.local/still",
		})
	}))

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ContentType: "movie", ProviderIDs: map[string]string{"sportarr": "v1.event-key"},
	})
	if err != nil {
		t.Fatalf("get Movie metadata failed: %v", err)
	}
	if result == nil || !result.HasMetadata {
		t.Fatal("expected Movie metadata")
	}
	if got := result; got.ReleaseDate != "2024-04-13" || got.Title != "UFC 300" || got.Overview != "A title fight card." || got.SortTitle != "UFC 300" || len(got.Studios) != 1 || got.Studios[0] != "UFC" || len(got.Genres) != 2 || got.PosterPath == "" || got.BackdropPath == "" || got.StillPath == "" {
		t.Errorf("unexpected Movie metadata: %+v", got)
	}
}

func TestGetMovieMetadataReturnsNilOnNotFound(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ContentType: "movie", ProviderIDs: map[string]string{"sportarr": "v1.missing"},
	})
	if err != nil || result != nil {
		t.Fatalf("missing Movie metadata = (%v, %v), want (nil, nil)", result, err)
	}
}

func TestGetMovieImagesUsesDetailArtwork(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/agents/movies/v1.event-key" {
			t.Errorf("expected Movie detail path, got %s", r.URL.Path)
		}
		encodeJSON(t, w, AgentMovieResponse{
			PosterURL: "https://sportarr.local/poster", BackdropURL: "https://sportarr.local/backdrop", StillURL: "https://sportarr.local/still",
		})
	}))

	images, err := p.GetImages(context.Background(), metadata.ImageRequest{
		ContentType: "movie", ProviderIDs: map[string]string{"sportarr": "v1.event-key"},
	})
	if err != nil {
		t.Fatalf("get Movie images failed: %v", err)
	}
	if len(images) != 3 || images[0].Type != metadata.ImagePoster || images[0].URL != "https://sportarr.local/poster" || images[1].Type != metadata.ImageBackdrop || images[2].Type != metadata.ImageStill {
		t.Fatalf("unexpected Movie images: %+v", images)
	}
}

func TestGetMovieImagesReturnsNilOnNotFound(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	images, err := p.GetImages(context.Background(), metadata.ImageRequest{
		ContentType: "movie", ProviderIDs: map[string]string{"sportarr": "v1.missing"},
	})
	if err != nil || images != nil {
		t.Fatalf("missing Movie images = (%v, %v), want (nil, nil)", images, err)
	}
}
