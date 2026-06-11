# Entity Image API Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Switch all image fetching from the Sportarr agents endpoint inline URL fields to the dedicated entity image API (`/api/v1/images/entity/{type}/{id}`), fixing wrong-host URLs and adding support for logos, season posters, and multiple images per type.

**Architecture:** New client method calls the entity image API. Provider helper functions convert API responses to domain types and pick primary images by type. GetImages, GetMetadata, and GetSeasons all use the entity image API. Search keeps using the agents endpoint (matching TMDB/TVDB plugin patterns).

**Tech Stack:** Go, httptest for mocking, no new dependencies

---

### Task 1: Add EntityImage types and update AgentSeason

**Files:**
- Modify: `provider/types.go`

- [ ] **Step 1: Add EntityImageResponse and EntityImage types**

Add to `provider/types.go` after the `AgentEpisodeResponse` struct:

```go
// EntityImageResponse is returned by GET /api/v1/images/entity/{type}/{id}.
type EntityImageResponse struct {
	Images []EntityImage `json:"images"`
}

type EntityImage struct {
	ID        string `json:"id"`
	ImageType string `json:"image_type"`
	URL       string `json:"url"`
	Width     *int   `json:"width"`
	Height    *int   `json:"height"`
	IsPrimary bool   `json:"is_primary"`
	Priority  int    `json:"priority"`
}
```

- [ ] **Step 2: Add CompetitionSeasonID to AgentSeason**

Add `CompetitionSeasonID` field to the `AgentSeason` struct:

```go
type AgentSeason struct {
	CompetitionSeasonID string `json:"competition_season_id"`
	SeasonNumber        int    `json:"season_number"`
	Name                string `json:"name"`
	EpisodeCount        int    `json:"episode_count"`
	PosterURL           string `json:"poster_url"`
}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add provider/types.go
git commit -m "feat: add EntityImage types and CompetitionSeasonID to AgentSeason"
```

---

### Task 2: Add Client.GetEntityImages method (TDD)

**Files:**
- Test: `provider/client_test.go`
- Modify: `provider/client.go`

- [ ] **Step 1: Write the failing test**

Add to `provider/client_test.go`:

```go
func TestGetEntityImagesParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/images/entity/league/abc-123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("completed_only") != "true" {
			t.Errorf("expected completed_only=true, got %s", r.URL.Query().Get("completed_only"))
		}
		json.NewEncoder(w).Encode(EntityImageResponse{
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
		})
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./provider/ -run TestGetEntityImagesParsesResponse -v`
Expected: FAIL — `c.GetEntityImages` undefined

- [ ] **Step 3: Implement GetEntityImages in client.go**

Add to `provider/client.go`:

```go
func (c *Client) GetEntityImages(ctx context.Context, entityType, entityID string) (*EntityImageResponse, error) {
	path := "/api/v1/images/entity/" + entityType + "/" + url.PathEscape(entityID) + "?completed_only=true"
	var resp EntityImageResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./provider/ -run TestGetEntityImagesParsesResponse -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add provider/client.go provider/client_test.go
git commit -m "feat: add Client.GetEntityImages method"
```

---

### Task 3: Add image conversion helpers (TDD)

**Files:**
- Test: `provider/provider_test.go`
- Modify: `provider/provider.go`

- [ ] **Step 1: Write the failing tests**

Add to `provider/provider_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./provider/ -run "TestPickPrimaryURL|TestEntityImagesToRemote" -v`
Expected: FAIL — `pickPrimaryURL` and `entityImagesToRemote` undefined

- [ ] **Step 3: Implement the helpers**

Add to `provider/provider.go`, after the existing imports add `"sort"` to the import block. Then add these functions after `ForTypes()`:

```go
func mapImageType(t string) (metadata.ImageType, bool) {
	switch t {
	case "poster":
		return metadata.ImagePoster, true
	case "backdrop":
		return metadata.ImageBackdrop, true
	case "logo":
		return metadata.ImageLogo, true
	case "banner":
		return metadata.ImageBanner, true
	case "thumbnail":
		return metadata.ImageStill, true
	default:
		return 0, false
	}
}

func pickPrimaryURL(images []EntityImage, imageType string) string {
	var best *EntityImage
	for i := range images {
		img := &images[i]
		if img.ImageType != imageType {
			continue
		}
		if best == nil {
			best = img
			continue
		}
		if img.IsPrimary && !best.IsPrimary {
			best = img
		} else if img.IsPrimary == best.IsPrimary && img.Priority > best.Priority {
			best = img
		}
	}
	if best == nil {
		return ""
	}
	return best.URL
}

func entityImagesToRemote(images []EntityImage) []metadata.RemoteImage {
	sorted := make([]EntityImage, len(images))
	copy(sorted, images)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].IsPrimary != sorted[j].IsPrimary {
			return sorted[i].IsPrimary
		}
		return sorted[i].Priority > sorted[j].Priority
	})

	var out []metadata.RemoteImage
	for _, img := range sorted {
		imgType, ok := mapImageType(img.ImageType)
		if !ok {
			continue
		}
		ri := metadata.RemoteImage{
			URL:  img.URL,
			Type: imgType,
		}
		if img.Width != nil {
			ri.Width = *img.Width
		}
		if img.Height != nil {
			ri.Height = *img.Height
		}
		out = append(out, ri)
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./provider/ -run "TestPickPrimaryURL|TestEntityImagesToRemote" -v`
Expected: PASS

- [ ] **Step 5: Run all existing tests to verify no regressions**

Run: `go test ./...`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add provider/provider.go provider/provider_test.go
git commit -m "feat: add image conversion helpers (pickPrimaryURL, entityImagesToRemote)"
```

---

### Task 4: Switch GetImages to entity image API (TDD)

**Files:**
- Test: `provider/provider_test.go`
- Modify: `provider/provider.go`

- [ ] **Step 1: Rewrite TestGetImagesForSeries to use entity image API**

Replace `TestGetImagesForSeries` in `provider/provider_test.go`:

```go
func TestGetImagesForSeries(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/images/entity/league/league-1" {
			t.Errorf("expected entity image path, got %s", r.URL.Path)
		}
		w1, h1 := 680, 1000
		w2, h2 := 1920, 1080
		json.NewEncoder(w).Encode(EntityImageResponse{
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
```

- [ ] **Step 2: Rewrite TestGetImagesForEpisode to use entity image API**

Replace `TestGetImagesForEpisode` in `provider/provider_test.go`:

```go
func TestGetImagesForEpisode(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/images/entity/event/ev-1" {
			t.Errorf("expected entity image path for event, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(EntityImageResponse{
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
```

- [ ] **Step 3: Add TestGetImagesForSeason**

Add to `provider/provider_test.go`:

```go
func TestGetImagesForSeason(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/images/entity/season/season-uuid-1" {
			t.Errorf("expected entity image path for season, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(EntityImageResponse{
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
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./provider/ -run "TestGetImagesFor" -v`
Expected: FAIL — tests hit wrong endpoints / get 404s

- [ ] **Step 5: Update GetImages in provider.go**

Replace the `GetImages`, `getSeriesImages`, and `getEpisodeImages` methods in `provider/provider.go` with:

```go
func (p *Provider) GetImages(ctx context.Context, req metadata.ImageRequest) ([]metadata.RemoteImage, error) {
	sportarrID := req.ProviderIDs["sportarr"]
	if sportarrID == "" {
		return nil, nil
	}

	var entityType string
	switch req.ContentType {
	case "series":
		entityType = "league"
	case "season":
		entityType = "season"
	case "episode":
		entityType = "event"
	default:
		return nil, nil
	}

	resp, err := p.client.GetEntityImages(ctx, entityType, sportarrID)
	if err != nil {
		return nil, err
	}
	return entityImagesToRemote(resp.Images), nil
}
```

Delete the `getSeriesImages` and `getEpisodeImages` methods entirely.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./provider/ -run "TestGetImagesFor" -v`
Expected: PASS

- [ ] **Step 7: Run all tests**

Run: `go test ./...`
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add provider/provider.go provider/provider_test.go
git commit -m "feat: switch GetImages to entity image API"
```

---

### Task 5: Update GetMetadata to fetch entity images (TDD)

**Files:**
- Test: `provider/provider_test.go`
- Modify: `provider/provider.go`

- [ ] **Step 1: Update TestGetMetadata to verify entity image fields**

Replace `TestGetMetadata` in `provider/provider_test.go`:

```go
func TestGetMetadata(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/metadata/agents/series/league-1":
			json.NewEncoder(w).Encode(AgentSeriesResponse{
				Title:   "Formula 1",
				Summary: "Open-wheel racing",
				Year:    1950,
				Genres:  []string{"Motorsport"},
				Studio:  "FIA",
			})
		case "/api/metadata/agents/series/league-1/seasons":
			json.NewEncoder(w).Encode(AgentSeasonsResponse{
				Seasons: []AgentSeason{
					{SeasonNumber: 2023, Name: "2023"},
					{SeasonNumber: 2024, Name: "2024"},
				},
			})
		case "/api/v1/images/entity/league/league-1":
			json.NewEncoder(w).Encode(EntityImageResponse{
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./provider/ -run TestGetMetadata -v`
Expected: FAIL — PosterPath/BackdropPath/LogoPath assertions fail (old code uses agents endpoint fields)

- [ ] **Step 3: Update GetMetadata in provider.go**

Replace the `GetMetadata` method in `provider/provider.go`:

```go
func (p *Provider) GetMetadata(ctx context.Context, req metadata.MetadataRequest) (*metadata.MetadataResult, error) {
	sportarrID := req.ProviderIDs["sportarr"]
	if sportarrID == "" {
		return nil, nil
	}

	series, err := p.client.GetSeries(ctx, sportarrID)
	if err != nil {
		return nil, err
	}

	result := &metadata.MetadataResult{
		HasMetadata:   true,
		Title:         series.Title,
		Overview:      series.Summary,
		Year:          series.Year,
		ContentRating: series.ContentRating,
		ProviderIDs:   map[string]string{"sportarr": sportarrID},
	}

	result.Genres = append(result.Genres, series.Genres...)
	if series.Studio != "" {
		result.Studios = []string{series.Studio}
	}

	seasons, err := p.client.GetSeasons(ctx, sportarrID)
	if err == nil && seasons != nil {
		result.SeasonCount = len(seasons.Seasons)
	}

	imgs, err := p.client.GetEntityImages(ctx, "league", sportarrID)
	if err == nil {
		result.PosterPath = pickPrimaryURL(imgs.Images, "poster")
		result.BackdropPath = pickPrimaryURL(imgs.Images, "backdrop")
		result.LogoPath = pickPrimaryURL(imgs.Images, "logo")
	}

	return result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./provider/ -run TestGetMetadata -v`
Expected: PASS

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add provider/provider.go provider/provider_test.go
git commit -m "feat: fetch entity images in GetMetadata for poster/backdrop/logo"
```

---

### Task 6: Update GetSeasons to fetch entity images (TDD)

**Files:**
- Test: `provider/provider_test.go`
- Modify: `provider/provider.go`

- [ ] **Step 1: Update TestGetSeasons to verify entity image poster**

Replace `TestGetSeasons` in `provider/provider_test.go`:

```go
func TestGetSeasons(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/metadata/agents/series/league-1/seasons":
			json.NewEncoder(w).Encode(AgentSeasonsResponse{
				Seasons: []AgentSeason{
					{CompetitionSeasonID: "cs-2023", SeasonNumber: 2023, Name: "2023 Season", EpisodeCount: 23},
					{CompetitionSeasonID: "cs-2024", SeasonNumber: 2024, Name: "2024 Season", EpisodeCount: 24},
				},
			})
		case "/api/v1/images/entity/season/cs-2023":
			json.NewEncoder(w).Encode(EntityImageResponse{
				Images: []EntityImage{
					{ID: "sp1", ImageType: "poster", URL: "https://sportarr.net/api/v1/images/sp1", IsPrimary: true},
				},
			})
		case "/api/v1/images/entity/season/cs-2024":
			json.NewEncoder(w).Encode(EntityImageResponse{
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./provider/ -run TestGetSeasons -v`
Expected: FAIL — PosterPath assertions fail (old code uses agents PosterURL field)

- [ ] **Step 3: Update GetSeasons in provider.go**

Replace the `GetSeasons` method in `provider/provider.go`:

```go
func (p *Provider) GetSeasons(ctx context.Context, req metadata.SeasonsRequest) ([]metadata.SeasonResult, error) {
	sportarrID := req.ProviderIDs["sportarr"]
	if sportarrID == "" {
		return nil, nil
	}

	resp, err := p.client.GetSeasons(ctx, sportarrID)
	if err != nil {
		return nil, err
	}

	seasons := make([]metadata.SeasonResult, 0, len(resp.Seasons))
	for _, s := range resp.Seasons {
		posterPath := ""
		if s.CompetitionSeasonID != "" {
			imgs, err := p.client.GetEntityImages(ctx, "season", s.CompetitionSeasonID)
			if err == nil {
				posterPath = pickPrimaryURL(imgs.Images, "poster")
			}
		}
		seasons = append(seasons, metadata.SeasonResult{
			ContentID:    fmt.Sprintf("%s:%d", sportarrID, s.SeasonNumber),
			SeasonNumber: s.SeasonNumber,
			Title:        s.Name,
			PosterPath:   posterPath,
		})
	}
	return seasons, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./provider/ -run TestGetSeasons -v`
Expected: PASS

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add provider/provider.go provider/provider_test.go
git commit -m "feat: fetch season poster images from entity image API"
```

---

### Task 7: Populate Width/Height on ImageRecord in main.go

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Update GetImages handler to set Width and Height**

In `main.go`, in the `GetImages` method, update the `ImageRecord` construction to include Width and Height. Change:

```go
		response.Images = append(response.Images, &pluginv1.ImageRecord{
			Kind: kind,
			Url:  sportarrCanonicalPath(s.runtime.baseURL, img.URL),
		})
```

To:

```go
		response.Images = append(response.Images, &pluginv1.ImageRecord{
			Kind:   kind,
			Url:    sportarrCanonicalPath(s.runtime.baseURL, img.URL),
			Width:  int32(img.Width),
			Height: int32(img.Height),
		})
```

- [ ] **Step 2: Run all tests**

Run: `go test ./...`
Expected: all PASS

- [ ] **Step 3: Build to verify**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: populate Width/Height on ImageRecord from entity image data"
```

---

### Task 8: Clean up unused agent image fields (optional)

**Files:**
- Modify: `provider/types.go`
- Modify: `provider/provider.go`

- [ ] **Step 1: Remove PosterURL from AgentSeason**

In `provider/types.go`, remove the `PosterURL` field from `AgentSeason` since it is no longer used:

```go
type AgentSeason struct {
	CompetitionSeasonID string `json:"competition_season_id"`
	SeasonNumber        int    `json:"season_number"`
	Name                string `json:"name"`
	EpisodeCount        int    `json:"episode_count"`
}
```

- [ ] **Step 2: Remove PosterURL/FanartURL/BannerURL usage check**

Verify that `AgentSeriesResponse` fields `PosterURL`, `FanartURL`, `BannerURL` are still used in `searchByID` (for `ImageURL`). If `searchByID` still references `series.PosterURL`, keep the field. Otherwise remove it.

The `searchByID` method in `provider.go` uses `series.PosterURL` for the search result `ImageURL`. So keep `PosterURL` on `AgentSeriesResponse` but `FanartURL` and `BannerURL` are no longer referenced — remove them.

In `provider/types.go`, update `AgentSeriesResponse`:

```go
type AgentSeriesResponse struct {
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	ContentRating string   `json:"content_rating"`
	Year          int      `json:"year"`
	Genres        []string `json:"genres"`
	Studio        string   `json:"studio"`
	PosterURL     string   `json:"poster_url"`
}
```

- [ ] **Step 3: Run all tests**

Run: `go test ./...`
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add provider/types.go
git commit -m "refactor: remove unused agent image URL fields"
```
