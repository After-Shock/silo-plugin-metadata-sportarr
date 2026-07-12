package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
	"github.com/Silo-Server/silo-plugin-sportarr/metadata"
	"github.com/Silo-Server/silo-plugin-sportarr/provider"
)

// Sportarr is a specialist provider: sports leagues rendered as TV shows. It
// must not become the default #1 metadata provider on every new TV series
// library, and it must not out-rank the general providers (TVDB priority 2,
// TMDB priority 3) when a user does opt in. The host seeds it disabled when the
// capability metadata declares default_enabled=false, and orders it by its
// declared default_priority, so both properties live in the manifest.
func TestManifestOptsOutOfDefaultEnable(t *testing.T) {
	m, err := publicmanifest.Load(manifestJSON)
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}
	meta := m.Capabilities[0].GetMetadata().AsMap()

	enabled, ok := meta["default_enabled"].(bool)
	if !ok {
		t.Fatalf("expected default_enabled bool in capability metadata, got %T (%v)", meta["default_enabled"], meta["default_enabled"])
	}
	if enabled {
		t.Errorf("sportarr must declare default_enabled=false so it is seeded disabled")
	}

	dp, ok := meta["default_priority"].(map[string]any)
	if !ok {
		t.Fatalf("expected default_priority map, got %T", meta["default_priority"])
	}
	// Series metadata remains below TVDB(2)/TMDB(3). Movie metadata is an
	// explicitly configured local integration, so it stays well below general
	// movie providers as well.
	for _, level := range []string{"series", "season", "episode", "movie"} {
		p, ok := dp[level].(float64)
		if !ok {
			t.Fatalf("expected numeric priority for %q, got %T", level, dp[level])
		}
		if p != 50 {
			t.Errorf("sportarr %s priority = %v, want 50", level, p)
		}
	}

	presentation := m.GetPresentation()
	if presentation == nil {
		t.Fatal("expected presentation metadata")
	}
	if got := presentation.GetDescriptionMarkdown(); got == "" || !containsAll(got, "Movie", "local Sportarr base_url") {
		t.Errorf("description must explain that Movie metadata requires a configured local Sportarr base_url, got %q", got)
	}
	if got := presentation.GetSetupMarkdown(); got == "" || !containsAll(got, "Movie", "local Sportarr base_url", "default hub") {
		t.Errorf("setup must explain that Movie metadata requires a configured local Sportarr base_url rather than the default hub, got %q", got)
	}
}

func TestManifestLoads(t *testing.T) {
	m, err := publicmanifest.Load(manifestJSON)
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}
	if m.PluginId != "silo.sportarr" {
		t.Errorf("expected plugin_id silo.sportarr, got %s", m.PluginId)
	}
	if len(m.Capabilities) != 1 {
		t.Fatalf("expected 1 capability, got %d", len(m.Capabilities))
	}
	if m.Capabilities[0].Id != "sportarr" {
		t.Errorf("expected capability id sportarr, got %s", m.Capabilities[0].Id)
	}
	if m.Capabilities[0].Type != "metadata_provider.v1" {
		t.Errorf("expected capability type metadata_provider.v1, got %s", m.Capabilities[0].Type)
	}
}

func TestSportarrCanonicalPath(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		imageURL string
		want     string
	}{
		{"empty", "https://sportarr.net", "", ""},
		{"full url", "https://sportarr.net", "https://sportarr.net/api/images/abc123", "sportarr:///api/images/abc123"},
		{"same effective default port", "http://sportarr.local", "http://sportarr.local:80/api/images/abc123", "sportarr:///api/images/abc123"},
		{"configured local URL", "http://sportarr.local:1867", "http://sportarr.local:1867/api/images/abc123", "sportarr:///api/images/abc123"},
		{"near-prefix port stays external", "http://sportarr.local:1867", "http://sportarr.local:18670/api/images/abc123", "http://sportarr.local:18670/api/images/abc123"},
		{"base path boundary", "http://sportarr.local:1867/sportarr", "http://sportarr.local:1867/sportarr/images/abc123", "sportarr:///images/abc123"},
		{"near-prefix base path stays external", "http://sportarr.local:1867/sportarr", "http://sportarr.local:1867/sportarrx/images/abc123", "http://sportarr.local:1867/sportarrx/images/abc123"},
		{"external url", "https://sportarr.net", "https://example.com/image.jpg", "https://example.com/image.jpg"},
		{"different scheme stays external", "https://sportarr.local", "http://sportarr.local/api/images/abc123", "http://sportarr.local/api/images/abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sportarrCanonicalPath(tt.baseURL, tt.imageURL)
			if got != tt.want {
				t.Errorf("sportarrCanonicalPath(%q, %q) = %q, want %q", tt.baseURL, tt.imageURL, got, tt.want)
			}
		})
	}
}

func TestResolveOneSportarrPath(t *testing.T) {
	base := "https://sportarr.net"

	tests := []struct {
		name string
		path string
		want string
	}{
		{"empty", "", ""},
		{"canonical", "sportarr:///api/images/abc123", "https://sportarr.net/api/images/abc123"},
		{"full url passthrough", "https://example.com/image.jpg", "https://example.com/image.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveOneSportarrPath(base, tt.path, "")
			if got != tt.want {
				t.Errorf("resolveOneSportarrPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestMovieMetadataItemMapsReleaseDateAndCanonicalLocalImages(t *testing.T) {
	const baseURL = "http://sportarr.local:1867"
	item, err := metadataItemFromResult(&metadata.MetadataResult{
		ProviderIDs:  map[string]string{"sportarr": "v1.ufc-300"},
		Title:        "UFC 300",
		ReleaseDate:  "2024-04-13",
		PosterPath:   baseURL + "/api/metadata/agents/movies/v1.ufc-300/images/poster",
		BackdropPath: baseURL + "/api/metadata/agents/movies/v1.ufc-300/images/fanart",
	}, "movie", baseURL)
	if err != nil {
		t.Fatalf("map Movie metadata item: %v", err)
	}

	if got := item.GetReleaseDate(); got != "2024-04-13" {
		t.Errorf("release date = %q, want 2024-04-13", got)
	}
	if got, want := item.GetPosterPath(), "sportarr:///api/metadata/agents/movies/v1.ufc-300/images/poster"; got != want {
		t.Errorf("poster path = %q, want %q", got, want)
	}
	if got, want := item.GetBackdropPath(), "sportarr:///api/metadata/agents/movies/v1.ufc-300/images/fanart"; got != want {
		t.Errorf("backdrop path = %q, want %q", got, want)
	}
}

func TestMovieImageRPCUsesCanonicalConfiguredLocalURLs(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/metadata/agents/movies/v1.ufc-300"; got != want {
			t.Errorf("Movie detail path = %q, want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode(provider.AgentMovieResponse{
			PosterURL:   srv.URL + "/api/metadata/agents/movies/v1.ufc-300/images/poster",
			BackdropURL: srv.URL + "/api/metadata/agents/movies/v1.ufc-300/images/fanart",
			StillURL:    srv.URL + "/api/metadata/agents/movies/v1.ufc-300/images/still",
		})
	}))
	defer srv.Close()

	server := &metadataServer{runtime: &runtimeServer{
		provider: provider.NewProvider(srv.URL),
		baseURL:  srv.URL,
	}}
	providerIDs, err := stringStruct(map[string]string{"sportarr": "v1.ufc-300"})
	if err != nil {
		t.Fatalf("make provider IDs: %v", err)
	}
	response, err := server.GetImages(context.Background(), &pluginv1.GetImagesRequest{
		ItemType:    "movie",
		ProviderId:  "v1.ufc-300",
		ProviderIds: providerIDs,
	})
	if err != nil {
		t.Fatalf("get Movie images: %v", err)
	}

	want := map[string]string{
		"poster":   "sportarr:///api/metadata/agents/movies/v1.ufc-300/images/poster",
		"backdrop": "sportarr:///api/metadata/agents/movies/v1.ufc-300/images/fanart",
		"still":    "sportarr:///api/metadata/agents/movies/v1.ufc-300/images/still",
	}
	if len(response.GetImages()) != len(want) {
		t.Fatalf("Movie image count = %d, want %d", len(response.GetImages()), len(want))
	}
	for _, image := range response.GetImages() {
		wantURL, ok := want[image.GetKind()]
		if !ok {
			t.Errorf("unexpected or duplicate Movie image kind %q", image.GetKind())
			continue
		}
		if image.GetUrl() != wantURL {
			t.Errorf("Movie %s URL = %q, want %q", image.GetKind(), image.GetUrl(), wantURL)
		}
		resolved, err := server.ResolveImageURL(context.Background(), &pluginv1.ResolveImageURLRequest{Path: image.GetUrl()})
		if err != nil {
			t.Fatalf("resolve Movie %s URL: %v", image.GetKind(), err)
		}
		if got, want := resolved.GetUrl(), srv.URL+strings.TrimPrefix(wantURL, "sportarr://"); got != want {
			t.Errorf("resolved Movie %s URL = %q, want %q", image.GetKind(), got, want)
		}
		delete(want, image.GetKind())
	}
	if len(want) != 0 {
		t.Errorf("missing Movie image kinds: %v", want)
	}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
