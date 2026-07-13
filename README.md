# Silo Sportarr Plugin

First-party Silo metadata plugin backed by [Sportarr](https://sportarr.net). Provides sports league metadata as TV shows, with series, seasons, and episodes mapped from leagues, seasons, and events.

## Dependency Model

This repository consumes `github.com/Silo-Server/silo-plugin-sdk` as a normal Go module dependency. CI and release builds run with `GOWORK=off` and expect the SDK version in `go.mod` to resolve from a published semver tag.

For local multi-repo development, use a temporary `replace` or a local `go.work` that points at `dev/github/silo-plugin-sdk`. Do not commit machine-local filesystem replaces as the supported release path.

## Development

```sh
go test ./...
go build .
```

## Local Movie metadata smoke

`scripts/smoke-local-movie-metadata.sh` is the pre-push integration gate for
the local Sportarr Movie-agent flow. It creates a fresh temporary directory,
a unique `sportarr-movie-e2e-*` Compose project, and its matching external
`sportarr-movie-e2e-*-net` Docker network. Silo is the only published service,
and its port is bound to `127.0.0.1`; Sportarr and the temporary plugin catalog
are reachable only by their Docker aliases (`sportarr` and `plugin-catalog`).

Start with the non-destructive contract check:

```sh
scripts/test-smoke-local-movie-metadata.sh
scripts/smoke-local-movie-metadata.sh --dry-run
```

By default the harness resolves the sibling Sportarr Movie implementation at
`Sportarr/.worktrees/sportarr-movie-api`. Pass `--sportarr-root` when it lives
elsewhere. Before any Docker build (including dry-run), the harness verifies
that this checkout contains the Movie-agent search route plus the persisted
metadata-key and typed-artwork model fields; it refuses an ordinary Sportarr
main checkout that lacks those changes.

For a full run, supply the exact released v1.0.2 linux/amd64 binary and its
matching manifest. They are deliberately inputs rather than committed local
replacements or binary artifacts:

```sh
scripts/smoke-local-movie-metadata.sh \
  --v102-binary-url 'https://.../silo.sportarr-v1.0.2-linux-amd64' \
  --v102-manifest-url 'https://.../manifest-v1.0.2.json'
```

The required host tools are Docker Compose v2, curl, jq, sha256sum, ffmpeg,
openssl, and shuf. Docker automatically supplies SQLite, .NET 8, and Go 1.26
(including `make build-all`) if those host tools are absent; use
`--docker-toolchains` to force those disposable containers even when they are
installed locally. The script copies the supplied Sportarr checkout into
its temporary directory before publishing both Dockerfile-required
`publish/docker-linux-x64` and `publish/docker-linux-arm64` directories, so
it does not alter that checkout. It likewise copies the plugin checkout before
creating the local `dist/` binaries, so no caller-worktree artifact remains.
It seeds only the migrated disposable SQLite
database with the UFC 300 fixture, bootstraps a disposable Silo administrator,
installs v1.0.2 from a temporary binary RepositoryIndex, records a notify-only
update, applies the local build, and verifies the disabled Movie row before it
enables and reorders Sportarr for the test library. The update check must reach
a completed task state before `available_version` is inspected. After matching,
the harness fetches persisted poster and backdrop URLs from inside the Silo
container and verifies an image response plus the known fixture bytes. Movie still is intentionally not asserted as persisted Silo artwork: Silo Movie item persistence has poster/backdrop/logo fields only. Plugin-level protocol coverage
in `TestMovieImageRPCUsesCanonicalConfiguredLocalURLs` proves that Sportarr
still is emitted and resolves correctly.

Cleanup is automatic on success, failure, or interruption: Compose containers,
volumes, the external network, image, temporary media, database, token, and
catalog are removed. Use `--keep` only to inspect a failed local run; it prints
the temporary path and project name for manual cleanup.

## Attribution

Metadata provided by [Sportarr](https://sportarr.net).

## License

`silo-plugin-metadata-sportarr` is licensed under `AGPL-3.0-or-later`. See [LICENSE](LICENSE).
