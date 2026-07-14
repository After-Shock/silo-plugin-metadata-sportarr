# Sportarr Public Image Redirect Resolution Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve local Sportarr image paths to their public HTTPS redirect target so Silo can cache UFC posters in Garage without bypassing SSRF protections.

**Architecture:** The `sportarr://` source remains unchanged. A new provider/client redirect lookup performs a request to the configured local Sportarr image endpoint without following redirects and returns only a valid absolute HTTPS `Location`; the plugin image resolver uses it for bare `/api/...` paths and otherwise fails closed.

**Tech Stack:** Go 1.26, `net/http`, Silo plugin SDK, Docker Go tests, Postgres/Silo image cache.

---

## Chunk 1: Redirect-safe plugin resolver

### Task 1: Resolve local API image paths to public redirect targets

**Files:**
- Modify: `provider/client.go`
- Modify: `provider/provider.go`
- Modify: `provider/client_test.go` and/or `provider/provider_test.go`
- Modify: `main.go`
- Modify: `main_test.go`

- [ ] **Step 1: Write failing tests at the provider and RPC boundaries**

Use `httptest.NewServer` as the configured local base URL. Its image endpoint
must return `302 Location: https://cdn.example.test/poster.jpg`. Assert the
new lookup returns that exact public HTTPS URL without fetching the redirect
target. Add a `metadataServer.ResolveImageURL` test proving a bare `/api/...`
path returns that redirect URL. Add negative cases for missing, relative,
non-HTTPS, malformed, localhost, loopback, link-local, RFC1918, carrier-grade
NAT, unspecified, multicast, reserved, and hostname redirect locations that
resolve to a private IP or no addresses; each must return no substituted URL.
Keep DNS lookup injectable so those hostname tests do not depend on network
availability.

- [ ] **Step 2: Verify RED**

Run the focused provider and main-package test selection in the Docker Go image.
Expected: failure because the resolver currently returns `http://sportarr:1867/api/...`
and does not inspect the local endpoint redirect.

- [ ] **Step 3: Implement the minimal redirect lookup**

Add a client method that builds only a local `/api/...` request, uses a cloned
HTTP client with `CheckRedirect` returning `http.ErrUseLastResponse`, closes the
response body, accepts only 3xx responses whose parsed `Location` is absolute
`https`, has no user info, and resolves exclusively to globally routable IPs.
Use one validation helper for literals and DNS results that rejects private,
carrier-grade NAT, loopback, link-local, unspecified, multicast, reserved, and
all other non-global addresses; use an injectable DNS lookup function for
hostname validation. Return an empty result/error for every other case. Expose it via `Provider`. Update
`ResolveImageURL`/`ResolveImageURLs` to invoke that provider method only for
bare `/api/...` paths. Preserve `sportarr://` and external URL behavior.

- [ ] **Step 4: Verify GREEN**

Run the focused tests, then:

```bash
docker run --rm -v silo-plugin-go-modcache:/go/pkg/mod -v "$PWD:/src" -w /src --entrypoint /usr/local/go/bin/go golang:1.26.4 test ./... -count=1
docker run --rm -v silo-plugin-go-modcache:/go/pkg/mod -v "$PWD:/src" -w /src --entrypoint /usr/local/go/bin/go golang:1.26.4 vet ./...
```

Expected: all tests and vet pass.

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go provider/client.go provider/provider.go provider/client_test.go provider/provider_test.go docs/superpowers/specs/2026-07-14-sportarr-image-redirect-resolution-design.md docs/superpowers/plans/2026-07-14-sportarr-image-redirect-resolution.md
git commit -m "fix: resolve Sportarr image redirects"
```

## Chunk 2: Versioned pilot and exact UFC retry

### Task 2: Deploy and verify public artwork caching

**Files:**
- Deploy: existing Sportarr plugin binary/manifest under `/opt/silo/plugins/silo.sportarr/1.0.3/install-903977688/`

- [ ] **Step 1: Build and deploy version 1.0.6**

Repeat the prior audited deployment procedure: Docker Linux amd64 build with
`-X main.version=1.0.6`, staged manifest with matching checksum, timestamped
backups of both active files, only-Silo restart, a bounded 60-second health
check, and rollback on either health failure or runtime-manifest mismatch.

- [ ] **Step 2: Retry exactly the existing 92 IDs**

Use `/tmp/sportarr-ufc-poster-job-ids.txt` in `silo-postgres` without expanding
selection. Pause only Silo, reassert each ID's queued/failed Sportarr item-poster
and library-5/UFC membership in a transaction, require target/updated set
equality plus zero anti-join rows, then restart only Silo.

- [ ] **Step 3: Verify Garage and browser delivery**

All 92 captured jobs must become `succeeded`. A real Sportarr movie item must
retain its canonical `poster_source_path` while its `poster_path` becomes S3
backed. Request the Silo API-provided poster URL and require a non-404 image
response. Confirm no fresh private-host SSRF errors.
