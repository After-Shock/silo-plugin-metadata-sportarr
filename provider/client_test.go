package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchMoviesEncodesTitleAndYear(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/agents/movies/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("title"); got != "UFC & WWE: 300" {
			t.Errorf("unexpected title query: %q", got)
		}
		if got := r.URL.Query().Get("year"); got != "2024" {
			t.Errorf("unexpected year query: %q", got)
		}
		if err := json.NewEncoder(w).Encode(AgentMovieSearchResponse{Results: []AgentMovieSearchResult{{
			ID:          "v1.event-key",
			Title:       "UFC 300",
			Year:        2024,
			ReleaseDate: "2024-04-13",
			Summary:     "A title fight card.",
			Studio:      "UFC",
			PosterURL:   "https://sportarr.local/api/metadata/agents/movies/v1.event-key/images/poster",
		}}}); err != nil {
			t.Errorf("encode Movie search response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewClient(100)
	c.SetBaseURL(srv.URL)

	resp, err := c.SearchMovies(context.Background(), "UFC & WWE: 300", 2024)
	if err != nil {
		t.Fatalf("search movies failed: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if got := resp.Results[0]; got.ID != "v1.event-key" || got.ReleaseDate != "2024-04-13" || got.Studio != "UFC" {
		t.Errorf("unexpected movie result: %+v", got)
	}
}

func TestGetMovieParsesTypedArtwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/agents/movies/v1.event-key" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(AgentMovieResponse{
			ID:          "v1.event-key",
			Title:       "UFC 300",
			SortTitle:   "UFC 300",
			Year:        2024,
			ReleaseDate: "2024-04-13",
			Summary:     "A title fight card.",
			Studio:      "UFC",
			Genres:      []string{"MMA", "Sports"},
			PosterURL:   "https://sportarr.local/api/metadata/agents/movies/v1.event-key/images/poster",
			BackdropURL: "https://sportarr.local/api/metadata/agents/movies/v1.event-key/images/backdrop",
			StillURL:    "https://sportarr.local/api/metadata/agents/movies/v1.event-key/images/still",
		}); err != nil {
			t.Errorf("encode Movie detail response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewClient(100)
	c.SetBaseURL(srv.URL)

	resp, err := c.GetMovie(context.Background(), "v1.event-key")
	if err != nil {
		t.Fatalf("get movie failed: %v", err)
	}
	if got := resp; got.ReleaseDate != "2024-04-13" || got.PosterURL == "" || got.BackdropURL == "" || got.StillURL == "" {
		t.Errorf("expected typed movie artwork and release date, got %+v", got)
	}
}

func TestGetMovieReturnsTypedNotFoundOnlyFor404(t *testing.T) {
	for _, tt := range []struct {
		name         string
		status       int
		wantNotFound bool
	}{
		{name: "not found", status: http.StatusNotFound, wantNotFound: true},
		{name: "bad request", status: http.StatusBadRequest, wantNotFound: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			c := NewClient(100)
			c.SetBaseURL(srv.URL)
			_, err := c.GetMovie(context.Background(), "v1.missing")
			if err == nil {
				t.Fatal("expected an error")
			}
			var notFound *ErrNotFound
			if got := errors.As(err, &notFound); got != tt.wantNotFound {
				t.Errorf("errors.As(ErrNotFound) = %v, want %v; error: %v", got, tt.wantNotFound, err)
			}
		})
	}
}

func TestSearchParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/agents/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("title") != "NFL" {
			t.Errorf("unexpected title param: %s", r.URL.Query().Get("title"))
		}
		if err := json.NewEncoder(w).Encode(AgentSearchResponse{
			Results: []AgentSearchResult{
				{ID: "abc-123", Title: "NFL Football", Year: 2024, PosterURL: "https://sportarr.net/img/nfl.jpg"},
			},
		}); err != nil {
			t.Errorf("encode series search response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewClient(10)
	c.SetBaseURL(srv.URL)

	resp, err := c.Search(context.Background(), "NFL")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].ID != "abc-123" {
		t.Errorf("expected ID abc-123, got %s", resp.Results[0].ID)
	}
	if resp.Results[0].Title != "NFL Football" {
		t.Errorf("expected title NFL Football, got %s", resp.Results[0].Title)
	}
}

func TestGetSeriesParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/agents/series/abc-123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(AgentSeriesResponse{
			Title:     "NFL Football",
			Summary:   "American football league",
			Year:      1920,
			Genres:    []string{"American Football", "Sports"},
			Studio:    "NFL",
			PosterURL: "https://sportarr.net/img/nfl-poster.jpg",
		}); err != nil {
			t.Errorf("encode series response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewClient(10)
	c.SetBaseURL(srv.URL)

	resp, err := c.GetSeries(context.Background(), "abc-123")
	if err != nil {
		t.Fatalf("get series failed: %v", err)
	}
	if resp.Title != "NFL Football" {
		t.Errorf("expected title NFL Football, got %s", resp.Title)
	}
	if len(resp.Genres) != 2 {
		t.Errorf("expected 2 genres, got %d", len(resp.Genres))
	}
}

func TestGetSeasonEpisodesParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/agents/series/abc-123/season/2024/episodes" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(AgentEpisodesResponse{
			Episodes: []AgentEpisode{
				{
					ID:              "evt-001",
					Title:           "Super Bowl LVIII",
					SeasonNumber:    2024,
					EpisodeNumber:   1,
					AirDate:         "2024-02-11",
					DurationMinutes: 240,
				},
			},
		}); err != nil {
			t.Errorf("encode episodes response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewClient(10)
	c.SetBaseURL(srv.URL)

	resp, err := c.GetSeasonEpisodes(context.Background(), "abc-123", 2024)
	if err != nil {
		t.Fatalf("get episodes failed: %v", err)
	}
	if len(resp.Episodes) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(resp.Episodes))
	}
	if resp.Episodes[0].Title != "Super Bowl LVIII" {
		t.Errorf("expected title Super Bowl LVIII, got %s", resp.Episodes[0].Title)
	}
	if resp.Episodes[0].DurationMinutes != 240 {
		t.Errorf("expected 240 min duration, got %d", resp.Episodes[0].DurationMinutes)
	}
}

func TestRetryOn5xx(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := json.NewEncoder(w).Encode(AgentSearchResponse{
			Results: []AgentSearchResult{{ID: "ok", Title: "OK"}},
		}); err != nil {
			t.Errorf("encode retry response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewClient(100)
	c.SetBaseURL(srv.URL)

	resp, err := c.Search(context.Background(), "test")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].ID != "ok" {
		t.Errorf("unexpected result after retry")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestNoCacheHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cache-Control") != "no-cache, no-store" {
			t.Errorf("missing Cache-Control header")
		}
		if r.Header.Get("Pragma") != "no-cache" {
			t.Errorf("missing Pragma header")
		}
		if err := json.NewEncoder(w).Encode(AgentSearchResponse{}); err != nil {
			t.Errorf("encode no-cache response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewClient(10)
	c.SetBaseURL(srv.URL)
	if _, err := c.Search(context.Background(), "test"); err != nil {
		t.Fatalf("search failed: %v", err)
	}
}

func TestGetEntityImagesParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/images/entity/league/abc-123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("completed_only") != "true" {
			t.Errorf("expected completed_only=true, got %s", r.URL.Query().Get("completed_only"))
		}
		if err := json.NewEncoder(w).Encode(EntityImageResponse{
			Images: []EntityImage{
				{
					ID:        "img-1",
					ImageType: "poster",
					URL:       "https://sportarr.net/api/v1/images/img-1",
					IsPrimary: true,
					Priority:  10,
				},
				{
					ID:        "img-2",
					ImageType: "backdrop",
					URL:       "https://sportarr.net/api/v1/images/img-2",
					Priority:  5,
				},
			},
		}); err != nil {
			t.Errorf("encode entity images response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewClient(10)
	c.SetBaseURL(srv.URL)

	resp, err := c.GetEntityImages(context.Background(), "league", "abc-123")
	if err != nil {
		t.Fatalf("get entity images failed: %v", err)
	}
	if len(resp.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(resp.Images))
	}
	if resp.Images[0].ID != "img-1" {
		t.Errorf("expected ID img-1, got %s", resp.Images[0].ID)
	}
	if resp.Images[0].ImageType != "poster" {
		t.Errorf("expected image_type poster, got %s", resp.Images[0].ImageType)
	}
	if !resp.Images[0].IsPrimary {
		t.Errorf("expected is_primary=true")
	}
	if resp.Images[1].URL != "https://sportarr.net/api/v1/images/img-2" {
		t.Errorf("unexpected URL: %s", resp.Images[1].URL)
	}
}

func TestGetEntityImagesBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/images/entity/season/s1":
			if err := json.NewEncoder(w).Encode(EntityImageResponse{
				Images: []EntityImage{{ID: "img-s1", ImageType: "poster", URL: "https://sportarr.net/api/v1/images/img-s1"}},
			}); err != nil {
				t.Errorf("encode season s1 images response: %v", err)
			}
		case "/api/v1/images/entity/season/s2":
			if err := json.NewEncoder(w).Encode(EntityImageResponse{
				Images: []EntityImage{{ID: "img-s2", ImageType: "poster", URL: "https://sportarr.net/api/v1/images/img-s2"}},
			}); err != nil {
				t.Errorf("encode season s2 images response: %v", err)
			}
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c := NewClient(100)
	c.SetBaseURL(srv.URL)

	result := c.GetEntityImagesBatch(context.Background(), "season", []string{"s1", "s2", "s1"})
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result["s1"].Images[0].ID != "img-s1" {
		t.Errorf("expected img-s1, got %s", result["s1"].Images[0].ID)
	}
	if result["s2"].Images[0].ID != "img-s2" {
		t.Errorf("expected img-s2, got %s", result["s2"].Images[0].ID)
	}
}

func TestGetEntityImagesBatchPartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/images/entity/season/s1":
			if err := json.NewEncoder(w).Encode(EntityImageResponse{
				Images: []EntityImage{{ID: "img-s1", ImageType: "poster", URL: "https://sportarr.net/api/v1/images/img-s1"}},
			}); err != nil {
				t.Errorf("encode partial batch images response: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(100)
	c.SetBaseURL(srv.URL)

	result := c.GetEntityImagesBatch(context.Background(), "season", []string{"s1", "bad-id"})
	if len(result) != 1 {
		t.Fatalf("expected 1 entry (bad-id should be skipped), got %d", len(result))
	}
	if _, ok := result["s1"]; !ok {
		t.Error("expected s1 in results")
	}
}

func TestGetEntityImagesBatchEmpty(t *testing.T) {
	c := NewClient(100)
	result := c.GetEntityImagesBatch(context.Background(), "season", nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}
