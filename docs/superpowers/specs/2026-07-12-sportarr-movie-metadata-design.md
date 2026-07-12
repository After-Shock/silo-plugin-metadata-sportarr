# Sportarr Movie Metadata Design

## Goal

Allow a Silo Movie library to match UFC, WWE, and other individual sporting
events against Sportarr. One Sportarr event supplies metadata for one Silo
movie, so the existing UFC-WWE library remains a Movie library. The current
league/season/episode behavior remains unchanged.

## Scope and Boundaries

This is a coordinated, ordered change in two repositories:

1. `Sportarr` first exposes a read-only, instance-local movie-agent API over
   its event records and typed event artwork.
2. `silo-plugin-metadata-sportarr` then consumes that API and declares Movie
   support.

Silo core needs no change: its plugin protocol supports movie metadata and a
movie `release_date`. Applying the released plugin update causes Silo's normal
plugin synchronization to append one disabled Sportarr Movie row to existing
Movie chains, including UFC-WWE. This work never enables or reorders that row;
an administrator explicitly places it above TMDB for the intended library.

## Sportarr Data and API Contract

### Persist typed event artwork

`Event.PosterUrl`, `ThumbUrl`, `BannerUrl`, and `FanartUrl` are currently
deserialization-only fields; sync flattens them into the untyped `Images`
list. The change persists these four nullable fields, adds the corresponding
SQLite/PostgreSQL migrations and snapshots, and updates event sync to retain
their normalized values. `Images` remains unchanged for existing callers.

The movie API exposes only typed, local URLs:

- `poster_url`: persisted `PosterUrl`; absent when unavailable.
- `backdrop_url`: persisted `FanartUrl`; absent when unavailable. A banner is
  not promoted to a backdrop.
- `still_url`: persisted `ThumbUrl`; for pre-migration rows only, the first
  legacy `Images` value is exposed as a still, never as a poster or backdrop.
- No event logo is advertised.

Each non-empty URL is a local redirect endpoint,
`/api/metadata/agents/movies/{providerId}/images/{kind}`, where `kind` is
`poster`, `backdrop`, or `still`. It resolves the same provider ID as the
detail endpoint and redirects only to the already persisted URL for that
specific kind. It never accepts a remote URL from the request. Existing
historical events need one normal Sportarr event/league refresh after the
migration to populate their typed art; until then they can expose only the
legacy still fallback.

### Stable provider IDs

`Events.ExternalId` is indexed but not unique, so it must not be used alone.
Add a persisted `MetadataAgentKey` to `Event`: a random, opaque UUID generated
once for every new event, with a unique database index. The migration adds the
nullable column, the application migration/backfill assigns a fresh UUID to
every existing null value before normal operation, and new-event creation
always generates one. It is never recalculated during a title, league, or
broadcast-date correction.

Movie candidates use `v1.<MetadataAgentKey>` as their provider ID. It exposes
no database primary key, filesystem path, artwork URL, title, or external ID;
the server resolves one exact row by its unique key. An unknown key returns
`404`, never an arbitrary `FirstOrDefault` event. Searches omit rows with a
blank external ID, missing key, or a legacy duplicate identity, but retain the
durable provider key once a candidate is published.

### Movie endpoints

Add these unauthenticated, read-only endpoints under the local media-server
agent namespace:

1. `GET /api/metadata/agents/movies/search?title=&year=` returns HTTP 200 and
   `{ "results": [] }` for an empty, invalid, or unmatched request. It never
   returns database IDs or local paths. Each result has `id`, `title`, `year`,
   `release_date`, `summary`, `studio`, `poster_url`, and `sport`.
2. `GET /api/metadata/agents/movies/{providerId}` returns the same identity
   fields plus `sort_title`, `genres`, `backdrop_url`, and `still_url`; an
   unknown, malformed, or ambiguous ID returns HTTP 404.

`release_date` is the effective broadcast date in `YYYY-MM-DD` form, using
`BroadcastDate` before the UTC event date. `year` is derived from that same
date. The promotion/league becomes `studio`; venue is intentionally not
mapped because Silo's movie plugin contract has no venue field.

Movie support is intentionally instance-local. A configured, non-empty
`sportarr.base_url` is required for Movie requests; the legacy default
`https://sportarr.net` continues to serve only the existing series endpoints
and must not be assumed to expose this local API. The manifest setup text and
configuration validation will state this requirement. When the local URL is
absent, Movie `Search`, `GetMetadata`, and `GetImages` return no Sportarr
result rather than calling the hub route.

### Conservative matching policy

Silo can automatically accept a lone high-confidence candidate. To prevent a
wrong PPV from being silently attached, Sportarr returns movie candidates only
when all of these hold:

- `title` is non-empty, at most 256 characters, and `year` is a four-digit
  year. Missing or invalid values return an empty list.
- The display title is `Event.Title` when it already begins with the normalized
  league name; otherwise it is `League.Name + " " + Event.Title`. Both query
  and display titles use case folding, Unicode letter/digit preservation,
  `&` to `and`, punctuation-to-space conversion, and collapsed whitespace.
- The normalized query title equals the normalized display title exactly, and
  the supplied year equals the effective broadcast year. There is no prefix,
  substring, fuzzy, or promotion-only match.
- Events with `Cancelled`, `Canceled`, or `Postponed` status are excluded.
  Scheduled and completed events are eligible so correctly named future
  material can be matched.

Sportarr de-duplicates by the complete opaque provider ID, sorts candidates by
display title then provider ID, and caps the result at ten. Because the match
is exact title plus exact broadcast year, a single candidate is safe for
Silo's normal automatic path. Same-title/same-year events with different
identity tuples remain separate candidates; Silo's existing manual match UI
is used if its scorer cannot choose one. A filename that lacks a year or does
not normalize to Sportarr's canonical event title deliberately receives no
automatic Sportarr match.

## Plugin Behavior

The manifest adds `movie: 50` to `default_priority` and retains
`default_enabled: false`. `Provider.ForTypes()` becomes
`[]string{"series", "movie"}`.

For Movie requests only:

- `Search` requires a configured local URL, title, and year, calls the
  movie-search endpoint, and retains the opaque `sportarr` provider ID and
  local `poster_url`. When it receives a persisted Movie `sportarr` ID, it
  resolves that detail first; a `404` falls back in the same request to the
  conservative title/year search, allowing an old or deleted event to be
  rematched without a blank record.
- `GetMetadata` calls the movie-detail endpoint and maps title, overview,
  effective year, `release_date`, promotion studio, and typed poster/backdrop
  paths into the Silo plugin protocol. `MetadataResult` and
  `metadataItemFromResult` gain `ReleaseDate` so Silo persists it.
- A detail `404` becomes no metadata (`nil, nil`) rather than a blank
  `HasMetadata` record. Other 4xx and transport errors remain errors.
- `GetImages` for movies reuses the movie detail's typed local paths and
  returns poster, backdrop, and still candidates. The existing
  `sportarr://` canonical path and resolver turn those paths back into the
  configured local Sportarr base URL; the plugin never asks for the absent
  `/api/v1/images/entity/event/...` route.

Series, season, and episode requests retain their current endpoints, request
paths, and output. Multiple Silo files grouped as one Movie still produce one
Sportarr event match and one metadata record; the plugin does not split or
choose event parts from file paths.

## Data Flow

```text
canonical event filename with year
  -> Silo Movie matcher
  -> Sportarr plugin movie search (exact title + year)
  -> Sportarr local movie-agent API
  -> opaque provider ID selected by Silo
  -> movie detail + typed local artwork redirects
  -> Silo stores release date and caches artwork in public S3
```

## Testing and Release

Sportarr endpoint and sync tests cover:

- exact normalization and promotion display-title rules; empty, oversized,
  yearless, fuzzy, and promotion-only searches returning no candidates;
- UTC-midnight crossing where `BroadcastDate` and `EventDate` differ;
- duplicate/blank external IDs, a unique opaque `MetadataAgentKey` backfill,
  persistence across title/date corrections, IDs that do not expose local IDs
  or paths, and `404` detail/image behavior;
- canceled/postponed exclusion, scheduled eligibility, deterministic ordering,
  result limit, and no mutation;
- persisted poster/fanart/thumb values, the legacy still-only fallback, and
  local image redirects that cannot redirect to another event's URL.

Plugin tests use `httptest` to verify Movie-only endpoint selection, query
encoding, exact opaque-ID preservation and `404` search fallback, local-base
URL requirement, release-date propagation through the protocol, typed local
artwork resolution, and the unchanged series/season/episode paths. Silo-side
integration coverage exercises the exact-title/year single-candidate
auto-selection, same-title/year multiple-candidate manual path, and update
synchronization that adds the disabled Movie chain row.

Release Sportarr first and deploy/migrate it before releasing the plugin. Test
the local configured base URL directly against the new Movie search and detail
routes. Then publish a new plugin version through its existing release
workflow; do not replace the production binary manually. Applying the Silo
plugin update syncs the newly declared Movie level into existing chains as one
disabled row because the plugin opts out of default enablement; upgrade tests
cover the existing v1.0.2 series-only installation. On `sullyflix-stream`,
refresh Sportarr events to populate typed art, apply the plugin update, verify
the disabled UFC-WWE Movie row, enable it and move it ahead of TMDB, then run a
controlled metadata refresh/match review before widening the refresh.
