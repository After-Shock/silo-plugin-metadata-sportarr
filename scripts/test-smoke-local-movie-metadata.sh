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
dry_run_output=$("$smoke" --dry-run)
grep -Fq 'Dry run:' <<<"$dry_run_output" || fail '--dry-run did not report its plan'
grep -Fq 'external network: sportarr-movie-e2e-<unique>-net' <<<"$dry_run_output" || fail '--dry-run does not describe a per-run network'

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
grep -Fq 'Movie still is intentionally not asserted as persisted Silo artwork' "$root_dir/README.md" || fail 'Movie still persistence boundary is not documented'
grep -Fq 'TestMovieImageRPCUsesCanonicalConfiguredLocalURLs' "$root_dir/README.md" || fail 'Movie still protocol-test evidence is not documented'
if grep -Fq 'assert_fixture_image still_url' "$smoke"; then
  fail 'Movie still is incorrectly asserted as persisted Silo artwork'
fi

printf 'smoke harness static contract: PASS\n'
