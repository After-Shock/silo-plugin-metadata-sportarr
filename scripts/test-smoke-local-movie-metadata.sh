#!/usr/bin/env bash
# Lightweight contract test for the smoke harness.  It deliberately performs
# no Docker work so it can run in CI and on contributor machines.
set -euo pipefail

root_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
smoke="$root_dir/scripts/smoke-local-movie-metadata.sh"
compose="$root_dir/scripts/compose.silo-smoke.yml"
seed="$root_dir/scripts/seed-movie-metadata-fixture.sh"
fixture="$root_dir/scripts/fixtures/ufc-300-event.json"

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

for file in "$smoke" "$compose" "$seed" "$fixture"; do
  [[ -f "$file" ]] || fail "missing required harness artifact: $file"
done

[[ -x "$smoke" ]] || fail "smoke script is not executable"
[[ -x "$seed" ]] || fail "seed script is not executable"

"$smoke" --help | grep -Fq -- '--dry-run' || fail '--help does not document --dry-run'
"$smoke" --help | grep -Fq -- '--docker-toolchains' || fail '--help does not document --docker-toolchains'
dry_run_output=$("$smoke" --dry-run)
grep -Fq 'Dry run:' <<<"$dry_run_output" || fail '--dry-run did not report its plan'
grep -Fq 'external network: sportarr-movie-e2e-<unique>-net' <<<"$dry_run_output" || fail '--dry-run does not describe a per-run network'
docker_dry_run_output=$("$smoke" --docker-toolchains --dry-run)
grep -Fq 'toolchains:       Docker (forced)' <<<"$docker_dry_run_output" || fail '--docker-toolchains is not reflected in the dry-run plan'

grep -Fq 'project="sportarr-movie-e2e-${RANDOM}${RANDOM}"' "$smoke" || fail 'compose project is not unique'
grep -Fq 'network="${project}-net"' "$smoke" || fail 'external network is not derived from the project'
grep -Fq 'https://sportarr.net' "$smoke" || fail 'production Sportarr URL refusal missing'
grep -Fq '127.0.0.1:' "$compose" || fail 'compose does not publish loopback-only ports'
grep -Fq 'external: true' "$compose" || fail 'compose network is not external'
grep -Fq 'http://sportarr:1867' "$smoke" || fail 'Silo is not configured to use its in-network Sportarr alias'
if rg -n -F 'plugin-catalog:8080' "$root_dir/scripts" -g '!test-smoke-local-movie-metadata.sh'; then
  fail 'plugin catalog still uses its incorrect internal port 8080'
fi
grep -Fq 'plugin-catalog:80' "$smoke" || fail 'plugin catalog does not use nginx internal port 80'
grep -Fq 'network="${project}-net"' "$smoke" || fail 'network is not derived from the unique compose project'
grep -Fq 'NETWORK_CREATED=0' "$smoke" || fail 'network ownership state is not initialized'
grep -Fq 'NETWORK_CREATED=1' "$smoke" || fail 'network ownership state is not recorded after create'
grep -Fq 'if ((NETWORK_CREATED)); then' "$smoke" || fail 'cleanup does not guard network removal by ownership'
grep -Fq 'Sportarr/.worktrees/sportarr-movie-api' "$smoke" || fail 'default Sportarr source is not the Movie API worktree'
grep -Fq 'validate_sportarr_movie_source' "$smoke" || fail 'Movie API source validation is missing'
grep -Fq 'tmp_dir/plugin-build' "$smoke" || fail 'local plugin artifacts are not isolated in the temporary directory'
grep -Fq 'api GET /admin/tasks/check_plugin_updates' "$smoke" || fail 'update task completion is not polled'
grep -Fq 'assert_fixture_image' "$smoke" || fail 'persisted artwork bytes are not verified'
grep -Fq "trap 'on_err \$LINENO' ERR" "$smoke" || fail 'smoke does not identify the failing step and exit status'
grep -Fq 'setup response (sensitive fields redacted)' "$smoke" || fail 'smoke does not log a redacted setup response'
grep -Fq 'test("token|password|secret"; "i")' "$smoke" || fail 'setup diagnostic does not redact sensitive fields'
grep -Fq 'ffmpeg_stderr="$tmp_dir/logs/ffmpeg.stderr"' "$smoke" || fail 'smoke does not retain ffmpeg stderr for diagnostics'
grep -Fq 'run_ffmpeg_fixture()' "$smoke" || fail 'smoke does not isolate host and Docker ffmpeg execution'
grep -Fq -- '--entrypoint ffmpeg' "$smoke" || fail 'forced Docker toolchains do not use the built Sportarr ffmpeg'
grep -Fq -- '--mount "type=bind,src=$media_dir,dst=/media"' "$smoke" || fail 'Docker ffmpeg does not use a bounded media mount'
grep -Fq 'mcr.microsoft.com/dotnet/sdk:8.0' "$smoke" || fail 'Docker .NET 8 fallback image is missing'
grep -Fq 'golang:1.26' "$smoke" || fail 'Docker Go 1.26 fallback image is missing'
grep -Fq 'make VERSION=1.0.3 build-all' "$smoke" || fail 'Docker Go fallback does not invoke the build-all Make target'
grep -Fq -- '--docker-toolchains' "$seed" || fail 'seed helper does not support forced Docker toolchains'
grep -Fq 'keinos/sqlite3:latest' "$seed" || fail 'seed helper lacks a SQLite Docker fallback image'
grep -Fq 'sqlite_exec()' "$seed" || fail 'seed helper does not route SQLite through a safe helper'
if grep -Fq 'docker run --rm -i' "$seed"; then :; else
  fail 'seed Docker SQLite fallback does not feed SQL over stdin'
fi
grep -Fq -- '--mount "type=bind,src=$database_dir,dst=/db"' "$seed" || fail 'seed Docker SQLite fallback does not use a bounded database-directory mount'
grep -Fq 'sqlite_user="${SPORTARR_SQLITE_USER:-99:100}"' "$seed" || fail 'seed Docker SQLite fallback does not use the Sportarr database owner'
if rg -n '\beval\b|sh -c' "$seed"; then
  fail 'seed helper must not evaluate database paths or fixture data through a shell'
fi
if grep -Fq 'need "$c"' "$smoke" && grep -Fq 'sqlite3 dotnet go' "$smoke"; then
  fail 'smoke still requires absent host SQLite, .NET, or Go toolchains'
fi
grep -Fq 'Movie still is intentionally not asserted as persisted Silo artwork' "$root_dir/README.md" || fail 'Movie still persistence boundary is not documented'
grep -Fq 'TestMovieImageRPCUsesCanonicalConfiguredLocalURLs' "$root_dir/README.md" || fail 'Movie still protocol-test evidence is not documented'
grep -Fq 'and ffmpeg if those host tools are absent' "$root_dir/README.md" || fail 'README does not document Docker toolchain fallback'
if grep -Fq 'assert_fixture_image still_url' "$smoke"; then
  fail 'Movie still is incorrectly asserted as persisted Silo artwork'
fi

printf 'smoke harness static contract: PASS\n'
