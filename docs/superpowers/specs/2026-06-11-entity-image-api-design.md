# Entity Image API Migration

Switch image fetching from the Sportarr agents endpoint inline URL fields to the dedicated entity image API (`/api/v1/images/entity/{type}/{id}`).

## Problem

The agents endpoint (`/api/metadata/agents/series/{id}`) returns inline image URLs (`poster_url`, `fanart_url`, `banner_url`) that resolve to the wrong host (`the wrong host` instead of `sportarr.net`). Additionally:

- `fanart_url` is null for many leagues (e.g. NHL) even though the entity image API has backdrop images available
- No `logo` field exists on the agents endpoint
- Season `poster_url` is null on the agents seasons endpoint

The entity image API returns images with correct URLs, richer metadata (dimensions, priority, primary flag), and supports all image types including logos.

## Approach

Follow the same pattern as the TMDB and TVDB Silo plugins: the provider layer fetches images via a dedicated image API and returns `RemoteImage` structs with full URLs. The gRPC layer converts them to canonical `sportarr://` paths for storage.

Search results continue using the agents search endpoint's `poster_url` directly (matching TMDB/TVDB behavior where search uses whatever the search API returns).

## Changes

### 1. Client: New `GetEntityImages` method

**File:** `provider/client.go`

Add method: `GetEntityImages(ctx context.Context, entityType, entityID string) ([]EntityImage, error)`

Calls: `GET /api/v1/images/entity/{entityType}/{entityID}?completed_only=true`

Returns the list of `EntityImage` objects from the response.

### 2. Types: New `EntityImage` struct

**File:** `provider/types.go`

```go
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

### 3. Provider: Replace image fetching with entity image API

**File:** `provider/provider.go`

**Remove:** `getSeriesImages`, `getEpisodeImages`

**Add:** `getEntityImages(ctx, entityType, entityID string) ([]metadata.RemoteImage, error)`

Maps Sportarr `image_type` strings to `metadata.ImageType`:
- `"poster"` -> `ImagePoster`
- `"backdrop"` -> `ImageBackdrop`
- `"logo"` -> `ImageLogo`
- `"banner"` -> `ImageBanner`
- `"thumbnail"` -> `ImageStill`
- Other types: skip

Sorts results by `is_primary` desc, then `priority` desc.

Populates `Width`/`Height` on `RemoteImage` when available from the API response.

**Add:** `pickPrimaryURL(images []EntityImage, imageType string) string`

Returns the URL of the best matching image for a given type (first `is_primary`, then highest `priority`). Used by `GetMetadata` and `GetSeasons` to select the primary poster/backdrop/logo.

**Update `GetImages`:**
```
"series"  -> getEntityImages(ctx, "league", sportarrID)
"season"  -> getEntityImages(ctx, "season", sportarrID)  // NEW
"episode" -> getEntityImages(ctx, "event", sportarrID)
```

**Update `GetMetadata`:**

After fetching series data from the agents endpoint, also call `client.GetEntityImages(ctx, "league", sportarrID)`. Use `pickPrimaryURL` to set:
- `PosterPath` from primary `"poster"` image
- `BackdropPath` from primary `"backdrop"` image
- `LogoPath` from primary `"logo"` image

**Update `GetSeasons`:**

For each season in the response, call `client.GetEntityImages(ctx, "season", seasonID)` and use `pickPrimaryURL` to set `PosterPath`.

Note: The seasons endpoint returns `competition_season_id` — this is the entity ID to use for image lookups. The agents seasons endpoint currently returns `season_number` but no `competition_season_id`. This field needs to be either: (a) added to the agents seasons response, or (b) fetched from the series detail response which embeds season objects with their IDs.

**Search:** No change. Continue using agents search endpoint's `poster_url`.

### 4. Main: Populate Width/Height on ImageRecord

**File:** `main.go`

In `GetImages`, set `Width` and `Height` on `pluginv1.ImageRecord` from the `RemoteImage` fields (matching TMDB/TVDB behavior).

### 5. Canonical scheme: No changes

`sportarrCanonicalPath` and `resolveOneSportarrPath` remain unchanged. Entity image API URLs like `https://sportarr.net/api/v1/images/{uuid}` canonicalize to `sportarr:///api/v1/images/{uuid}` using the same base-URL-stripping approach.

### 6. Tests

**Files:** `provider/provider_test.go`, `provider/client_test.go`, `main_test.go`

- Add mock handler for `GET /api/v1/images/entity/{type}/{id}` returning `EntityImage` arrays
- Update existing image tests to use entity image API responses
- Test `pickPrimaryURL` with multiple images, `is_primary` and `priority` ordering
- Test `getEntityImages` with each entity type (league, season, event)
- Test that `GetMetadata` populates poster/backdrop/logo from entity images
- Test that `GetSeasons` populates poster from entity images
- Test edge cases: no images available, only non-matching types, empty response

## Season Entity IDs

The agents seasons endpoint likely returns `competition_season_id` per season (the `MediaSeasonResponse` schema includes it). Add `CompetitionSeasonID string json:"competition_season_id"` to `AgentSeason`. Use this UUID as the entity ID when calling `GetEntityImages(ctx, "season", competitionSeasonID)`.

If the agents seasons endpoint does not return `competition_season_id`, fall back to the series detail endpoint (`/api/metadata/agents/series/{id}`) which embeds a `seasons` array with this field.
