# Sportarr Movie Metadata Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add exact, local Sportarr event metadata to Silo Movie libraries and prove it in an isolated local stack before any push or PR.

**Architecture:** Sportarr persists a durable public event key and typed artwork, then serves read-only local Movie agent endpoints. The Silo plugin uses those endpoints only when explicitly configured with a local Sportarr URL, preserves movie release dates, and leaves existing TV behavior intact. Local validation is layered: unit tests, an API/container smoke, then a disposable Silo+Sportarr integration run on non-production ports and temporary data.

**Tech Stack:** .NET 8 / EF Core with SQLite and PostgreSQL migrations, Go 1.26 Silo plugin SDK, Go Silo development compose stack, Docker Compose.

---

## Chunk 1: Sportarr data contract and HTTP API

### Task 1: Add durable event metadata identity and typed artwork persistence

**Files:**
- Modify: `../Sportarr/src/Sportarr.Data/Models/Event.cs`
- Modify: `../Sportarr/src/Sportarr.Data/Data/SportarrDbContext.cs`
- Modify: `../Sportarr/src/Services/LeagueEventSyncService.cs`
- Modify: `../Sportarr/src/Startup/DatabaseInitializer.cs`
- Create: SQLite and PostgreSQL EF migrations plus designer/model snapshots in their existing migration projects
- Test: `../Sportarr/tests/Sportarr.Api.Tests/` new focused event metadata-key/artwork test file

- [ ] **Step 1: Write failing tests** for new events receiving a UUID-shaped metadata key, existing missing keys being backfilled once, key stability after title/date changes, and typed poster/thumb/fanart persistence.
- [ ] **Step 2: Run the focused tests** and confirm they fail because the properties/backfill do not exist.
- [ ] **Step 3: Implement the smallest model, EF configuration, sync assignment, and startup backfill**. Generate provider-appropriate SQLite and Postgres migrations; do not expose the database `Id`.
- [ ] **Step 4: Re-run focused tests**, then `dotnet test tests/Sportarr.Api.Tests/Sportarr.Api.Tests.csproj`.
- [ ] **Step 5: Commit** Sportarr persistence changes.

### Task 2: Implement conservative Movie-agent endpoints

**Files:**
- Modify: `../Sportarr/src/Endpoints/MetadataAgentEndpoints.cs`
- Test: `../Sportarr/tests/Sportarr.Api.Tests/Endpoints/` new movie-agent endpoint test file

- [ ] **Step 1: Write failing endpoint tests** for exact normalized title/year matches, no result for empty/yearless/fuzzy input, cancellation/postponement exclusion, scheduled inclusion, stable ordering/limit, no internal IDs in JSON, broadcast-date precedence, and malformed/unknown detail keys returning 404.
- [ ] **Step 2: Add failing tests for local artwork redirects**: poster/fanart/thumb each resolves only from the selected event; a legacy untyped image is still-only; no kind or cross-event key returns 404.
- [ ] **Step 3: Run the focused endpoint test project** and verify the failures describe the absent routes.
- [ ] **Step 4: Implement pure helpers** for title normalization, display-title construction, effective date, and key lookup; map the two Movie endpoints and image redirect route using those helpers.
- [ ] **Step 5: Run the endpoint tests and full Sportarr test project**, then `dotnet build Sportarr.sln`.
- [ ] **Step 6: Commit** the API changes.

## Chunk 2: Silo Sportarr plugin

### Task 3: Extend movie domain/client behavior with release-date and 404 handling

**Files:**
- Modify: `metadata/types.go`
- Modify: `provider/types.go`
- Modify: `provider/client.go`
- Modify: `provider/provider.go`
- Test: `provider/client_test.go`
- Test: `provider/provider_test.go`

- [ ] **Step 1: Write failing client/provider tests** for Movie search/query encoding, configured-local URL gating, durable provider-ID lookup, detail-404 title/year fallback, detail-404 no-metadata handling, typed local artwork, and release-date mapping.
- [ ] **Step 2: Run `go test ./provider/...`** and confirm the tests fail for the missing movie API behavior.
- [ ] **Step 3: Add Movie response types and typed `ErrNotFound` handling**; route Movie requests to `/api/metadata/agents/movies`, keep series requests byte-for-byte on their existing paths, and make missing local configuration return no Movie result.
- [ ] **Step 4: Implement the minimal provider mapping**: `ForTypes` includes movie; `Search`, `GetMetadata`, and `GetImages` branch by content type; `ReleaseDate` is carried in the domain result.
- [ ] **Step 5: Re-run `go test ./provider/...` and `go test ./...`.**
- [ ] **Step 6: Commit** the plugin provider/client changes.

### Task 4: Extend the plugin protocol mapping and manifest

**Files:**
- Modify: `main.go`
- Modify: `metadata/types.go` (if not completed in Task 3)
- Modify: `manifest.json`
- Modify: `main_test.go`

- [ ] **Step 1: Write failing tests** proving `MetadataItem.ReleaseDate` is populated, configured local URLs canonicalize to `sportarr://`, Movie image resolution returns poster/backdrop/still only, and the manifest declares disabled `movie: 50` support with accurate local-URL setup text.
- [ ] **Step 2: Run the focused package tests** and verify the release-date/manifest assertion fails.
- [ ] **Step 3: Add the protocol mapping and manifest changes** without altering the series/season/episode RPC routes.
- [ ] **Step 4: Run `make test`, `go vet ./...`, and `make lint`.**
- [ ] **Step 5: Commit** the protocol/manifest changes.

## Chunk 3: Local, pre-push integration gate

### Task 5: Add a repeatable isolated local smoke harness

**Files:**
- Create: `scripts/smoke-local-movie-metadata.sh`
- Create: `scripts/compose.silo-smoke.yml`
- Create: `scripts/seed-movie-metadata-fixture.sh`
- Create: `scripts/fixtures/ufc-300-event.json`
- Modify: `README.md` or `docs/development.md` with prerequisites and cleanup
- Test: the script's shellcheck-compatible dry-run/help behavior, if this repository has shell-test conventions

- [ ] **Step 1: Write the smoke script interface first** with `--help`, unique `COMPOSE_PROJECT_NAME=sportarr-movie-e2e`, a temporary directory, and explicit cleanup trap. It must refuse the known production base URL and use only loopback ports.
- [ ] **Step 2: Verify the initial script fails** because the Movie endpoints/plugin capability are not yet available (red state is recorded before implementation lands).
- [ ] **Step 3: Implement the container topology and source build**:
  1. create an external Docker network named `sportarr-movie-e2e-net`;
  2. build Sportarr exactly as its Dockerfile requires: publish both `linux-x64` and `linux-arm64` output into `publish/docker-linux-x64` and `publish/docker-linux-arm64`, then `docker build -t sportarr-movie-e2e .`;
  3. run Sportarr with a temporary bind-mounted config directory on `sportarr-movie-e2e-net` and network alias `sportarr`;
  4. use `scripts/compose.silo-smoke.yml` to attach the Silo `silo` service to that same external network; configure the plugin with `http://sportarr:1867`, never a host-loopback URL; and
  5. assert from inside the Silo container that `http://sportarr:1867/api/health` succeeds before the plugin test begins.
- [ ] **Step 4: Implement deterministic fixture/bootstrap setup**:
  1. wait for Sportarr migrations, then use the disposable SQLite database only through `scripts/seed-movie-metadata-fixture.sh` to insert one league and one event with a nonblank external ID, `BroadcastDate=2024-04-13`, a UTC event time that crosses the date boundary, typed poster/fanart/thumb URLs, and known local image targets; this avoids expanding the public CreateEvent API solely for testing;
  2. create a one-second valid movie fixture named with the exact canonical title and year under the temporary Silo media root using `ffmpeg`;
  3. wait for Silo `/api/v1/auth/setup`, call `POST /api/v1/auth/setup` to create the disposable admin, and retain its returned bearer token only in the temporary directory;
  4. use that token to create `POST /api/v1/libraries` with type `movies` and the container media path, wait for its initial scan (or call `POST /api/v1/scan`), then obtain the generated Movie content ID through the authenticated catalog/library response.
- [ ] **Step 5: Exercise the real update and match flow**:
  1. run a temporary `plugin-catalog` HTTP service on `sportarr-movie-e2e-net`. It serves Silo's binary `RepositoryIndex` JSON: first, the released v1.0.2 manifest plus `binaries["linux/amd64"]` containing the binary URL and SHA-256 checksum; then, the locally built higher-version manifest and binary with its checksum. Both the catalog and `sportarr` service names must resolve from the Silo container.
  2. add `http://plugin-catalog:PORT/index.json` through `POST /api/v1/admin/plugins/repositories`, install v1.0.2 into the disposable Silo instance from that catalog, immediately set that installation's update policy to `notify` with `PUT /api/v1/admin/plugins/installations/{id}`, then switch the catalog index to the locally built higher version;
  3. invoke `POST /api/v1/admin/tasks/check_plugin_updates/run`, poll `GET /api/v1/admin/tasks/check_plugin_updates`, and read the installation response until `available_version` equals the local higher version. Fail the smoke if update discovery does not record that value; only then call `POST /api/v1/admin/plugins/installations/{id}/update` so Silo runs its actual update synchronization;
  4. retrieve `GET /api/v1/libraries/{id}/providers` and assert the newly supported Movie entry exists but is disabled; then set the disposable chain with `PUT /api/v1/libraries/{id}/providers`, enabling Sportarr ahead of TMDB;
  5. configure `sportarr.base_url=http://sportarr:1867`, call `POST /api/v1/admin/items/{contentID}/match/search`, select the Sportarr candidate, and call `POST /api/v1/admin/items/{contentID}/match/apply` with its provider ID;
  6. read the persisted item through the authenticated item/catalog endpoint and assert `release_date=2024-04-13`, the Sportarr provider key, and resolved poster/backdrop/still all originate from the `sportarr` container. Inspect the Sportarr access log or an instrumented fixture route to prove the Silo plugin made the in-network request.
- [ ] **Step 6: Run the smoke script from a clean working tree**, preserve only scrubbed logs on failure, and confirm the cleanup removes its containers, networks, volumes, and temporary media.
- [ ] **Step 7: Commit** the harness and documentation.

### Task 6: Pre-push verification and release readiness

**Files:**
- Modify only if verification exposes a real defect.

- [ ] **Step 1: Run full Sportarr verification**: `dotnet test tests/Sportarr.Api.Tests/Sportarr.Api.Tests.csproj` and `dotnet build Sportarr.sln`.
- [ ] **Step 2: Run full plugin verification**: `make test`, `go vet ./...`, `make lint`, and the repository's local-path guard if present.
- [ ] **Step 3: Run `scripts/smoke-local-movie-metadata.sh`** and inspect its final API and Silo assertions; do not push if any local integration assertion is skipped.
- [ ] **Step 4: Inspect diffs and migration output**: `git diff --check` in both repositories, confirm no local SDK `replace`, credentials, temporary data, or generated binaries are staged.
- [ ] **Step 5: Commit remaining intentional changes**, then prepare separate Sportarr and plugin PRs, with Sportarr first.
