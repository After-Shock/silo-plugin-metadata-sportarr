#!/usr/bin/env bash
# Disposable, loopback-only Movie metadata integration gate.
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/.." && pwd)
plugin_repo=$(cd "$repo_root/../.." && pwd)
sportarr_root="$plugin_repo/../Sportarr/.worktrees/sportarr-movie-api"
silo_root="$plugin_repo/../Silo-Server"
silo_image="${SILO_IMAGE:-ghcr.io/silo-server/silo-server:latest}"
sportarr_base_url="${SPORTARR_SMOKE_BASE_URL:-http://sportarr:1867}"
v102_binary_url="${SPORTARR_V102_BINARY_URL:-}"
v102_manifest_url="${SPORTARR_V102_MANIFEST_URL:-}"
dotnet_image="${SPORTARR_DOTNET_IMAGE:-mcr.microsoft.com/dotnet/sdk:8.0}"
go_image="${SPORTARR_GO_IMAGE:-golang:1.26}"
dry_run=false
keep=false
docker_toolchains=false

usage() {
  cat <<'EOF'
Usage: scripts/smoke-local-movie-metadata.sh [options]

Creates a temporary Silo + Sportarr stack on an external Docker network, then
tests the real binary catalog update and Movie matching flow. No production
URL is accepted and Silo is the only loopback-published service.

Options:
  --dry-run                 Print the isolated plan without Docker changes.
  --keep                    Keep temporary resources after the run.
  --sportarr-root PATH      Movie-enabled Sportarr checkout (default: sibling sportarr-movie-api worktree).
  --silo-root PATH          Source checkout for provenance (default: sibling Silo-Server).
  --silo-image IMAGE        Silo runtime image.
  --v102-binary-url URL     Released v1.0.2 linux/amd64 binary URL.
  --v102-manifest-url URL   Released v1.0.2 manifest JSON URL.
  --docker-toolchains        Force Docker for SQLite, .NET 8, Go 1.26, and ffmpeg.
  --help                    Show this help.

Required: Docker Compose v2, curl, jq, sha256sum, openssl, shuf, and an image
compatible with the checked-out Silo API. sqlite3, .NET 8, Go, make, and ffmpeg
are used from the host when available; Docker supplies them when absent.
EOF
}

while (($#)); do
  case "$1" in
    --dry-run) dry_run=true; shift ;;
    --keep) keep=true; shift ;;
    --sportarr-root) sportarr_root=${2:?missing value}; shift 2 ;;
    --silo-root) silo_root=${2:?missing value}; shift 2 ;;
    --silo-image) silo_image=${2:?missing value}; shift 2 ;;
    --v102-binary-url) v102_binary_url=${2:?missing value}; shift 2 ;;
    --v102-manifest-url) v102_manifest_url=${2:?missing value}; shift 2 ;;
    --docker-toolchains) docker_toolchains=true; shift ;;
    --help|-h) usage; exit 0 ;;
    *) printf 'unknown option: %s\n' "$1" >&2; exit 2 ;;
  esac
done

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
log() { printf '\n==> %s\n' "$*"; }
need() { command -v "$1" >/dev/null || die "missing command: $1"; }
use_host_tool() { [[ "$docker_toolchains" == false ]] && command -v "$1" >/dev/null 2>&1; }

run_dotnet_publish() {
  local workdir=$1 runtime=$2 output=$3
  if use_host_tool dotnet; then
    (
      cd "$workdir"
      dotnet publish src/Sportarr.csproj -c Release -r "$runtime" --self-contained false -o "$output"
    )
  else
    docker run --rm --user "$(id -u):$(id -g)" \
      --mount "type=bind,src=$workdir,dst=/work" --workdir /work \
      -e DOTNET_CLI_HOME=/tmp/dotnet-cli -e NUGET_PACKAGES=/tmp/nuget \
      "$dotnet_image" dotnet publish src/Sportarr.csproj -c Release -r "$runtime" --self-contained false -o "$output"
  fi
}

run_plugin_build_all() {
  local workdir=$1
  if use_host_tool go && use_host_tool make; then
    (
      cd "$workdir"
      make VERSION=1.0.3 build-all
    )
  else
    docker run --rm --user "$(id -u):$(id -g)" \
      --mount "type=bind,src=$workdir,dst=/work" --workdir /work \
      -e GOCACHE=/tmp/go-cache -e GOMODCACHE=/tmp/go-mod \
      "$go_image" make VERSION=1.0.3 build-all
  fi
}

run_ffmpeg_fixture() {
  local output_path=$1 media_dir filename
  media_dir=$(dirname "$output_path")
  filename=$(basename "$output_path")
  if use_host_tool ffmpeg; then
    ffmpeg -hide_banner -loglevel error -f lavfi -i color=c=black:s=64x64:d=1 -f lavfi -i anullsrc=r=48000:cl=stereo -shortest -c:v libx264 -pix_fmt yuv420p -c:a aac "$output_path"
  else
    docker run --rm --user "$(id -u):$(id -g)" --entrypoint ffmpeg \
      --mount "type=bind,src=$media_dir,dst=/media" \
      "$sportarr_image" -hide_banner -loglevel error -f lavfi -i color=c=black:s=64x64:d=1 -f lavfi -i anullsrc=r=48000:cl=stereo -shortest -c:v libx264 -pix_fmt yuv420p -c:a aac "/media/$filename"
  fi
}

validate_sportarr_movie_source() {
  local endpoint_source="$sportarr_root/src/Endpoints/MetadataAgentEndpoints.cs"
  local event_source="$sportarr_root/src/Sportarr.Data/Models/Event.cs"
  local context_source="$sportarr_root/src/Sportarr.Data/Data/SportarrDbContext.cs"
  local migration_dir="$sportarr_root/src/Sportarr.Data/Migrations"
  [[ -f "$endpoint_source" && -f "$event_source" && -f "$context_source" && -d "$migration_dir" ]] || die "--sportarr-root must contain the Sportarr Movie metadata source tree: $sportarr_root"
  grep -Fq '/api/metadata/agents/movies/search' "$endpoint_source" || die "--sportarr-root does not expose the Movie-agent search endpoint"
  grep -Fq 'MetadataAgentKey' "$event_source" || die "--sportarr-root does not contain persisted Movie metadata identity"
  grep -Fq 'FanartUrl' "$event_source" || die "--sportarr-root does not contain typed Movie artwork fields"
  grep -Fq 'MetadataAgentKey' "$context_source" || die "--sportarr-root does not configure the Movie metadata key schema"
  grep -R -Fq 'MetadataAgentKey' "$migration_dir" || die "--sportarr-root does not contain the Movie metadata schema migration"
}

[[ -d "$sportarr_root" ]] || die "Sportarr checkout not found: $sportarr_root"
[[ -d "$silo_root" ]] || die "Silo checkout not found: $silo_root"
validate_sportarr_movie_source
[[ "$silo_image" != *sullyflix* ]] || die 'refusing production-looking Silo image'
[[ "$sportarr_base_url" != https://sportarr.net && "$sportarr_base_url" != https://www.sportarr.net ]] || die 'refusing the production Sportarr base URL'
[[ "$sportarr_base_url" == http://sportarr:1867 ]] || die 'the smoke harness only permits the in-network http://sportarr:1867 base URL'

if "$dry_run"; then
  cat <<EOF
Dry run: no Docker resources or files will be created.
  external network: sportarr-movie-e2e-<unique>-net
  compose project:  sportarr-movie-e2e-<unique>
  Silo port:        127.0.0.1:<allocated>:8080
  in-network URLs:  http://sportarr:1867 and http://plugin-catalog:80
  production URL:   https://sportarr.net (refused)
  v1.0.2 binary:    ${v102_binary_url:-<required for full run>}
  v1.0.2 manifest:  ${v102_manifest_url:-<required for full run>}
  toolchains:       $([[ "$docker_toolchains" == true ]] && printf 'Docker (forced)' || printf 'host when available, Docker fallback')
EOF
  exit 0
fi

for c in docker curl jq sha256sum openssl shuf; do need "$c"; done
docker compose version >/dev/null 2>&1 || die 'Docker Compose v2 is required'

[[ -n "$v102_binary_url" ]] || die 'pass --v102-binary-url (a released v1.0.2 binary is not committed)'
[[ -n "$v102_manifest_url" ]] || die 'pass --v102-manifest-url (an exact released manifest is required)'

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/sportarr-movie-e2e.XXXXXXXX")
project="sportarr-movie-e2e-${RANDOM}${RANDOM}"
network="${project}-net"
NETWORK_CREATED=0
silo_port=$(shuf -i 20000-29999 -n 1)
sportarr_container="${project}-sportarr"
sportarr_image="sportarr-movie-e2e:${project}"
export COMPOSE_PROJECT_NAME="$project" SMOKE_NETWORK="$network" SILO_IMAGE="$silo_image" SILO_PORT="$silo_port"
export SMOKE_SECRET_KEY="$(openssl rand -base64 48 | tr -d '\n')"
export SMOKE_CATALOG_DIR="$tmp_dir/catalog" SMOKE_MEDIA_DIR="$tmp_dir/media" SMOKE_SILO_DATA_DIR="$tmp_dir/silo-data"
compose=(docker compose -f "$script_dir/compose.silo-smoke.yml")

cleanup() {
  result=$?
  if "$keep"; then
    printf 'Kept resources: %s (project %s)\n' "$tmp_dir" "$project" >&2
    exit "$result"
  fi
  if ((result != 0)); then
    failure_log="${TMPDIR:-/tmp}/sportarr-movie-e2e-failure-${project}.log"
    {
      docker logs "$sportarr_container" 2>&1 || true
      "${compose[@]}" logs 2>&1 || true
    } | sed -E 's/(Bearer[[:space:]]+)[^[:space:]]+/\1[REDACTED]/g; s/(password|SECRET_KEY)[=:][^[:space:]]+/\1=[REDACTED]/g' > "$failure_log"
    printf 'Scrubbed failure log: %s\n' "$failure_log" >&2
  fi
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker rm -f "$sportarr_container" >/dev/null 2>&1 || true
  if ((NETWORK_CREATED)); then
    docker network rm "$network" >/dev/null 2>&1 || true
  fi
  docker image rm "$sportarr_image" >/dev/null 2>&1 || true
  rm -rf "$tmp_dir"
  exit "$result"
}
trap cleanup EXIT INT TERM

smoke_step='initializing smoke harness'
on_err() {
  local status=$? line=$1
  printf 'ERROR: smoke step failed: %s (exit %d, line %d)\n' "$smoke_step" "$status" "$line" >&2
  exit "$status"
}
trap 'on_err $LINENO' ERR

api_token=
api() {
  local method=$1 path=$2 body=${3:-}
  local args=(-fsS -X "$method" "http://127.0.0.1:$silo_port/api/v1$path" -H "Authorization: Bearer $api_token")
  [[ -n "$body" ]] && args+=(-H 'Content-Type: application/json' --data "$body")
  curl "${args[@]}"
}

wait_for() {
  local description=$1 command=$2
  for _ in $(seq 1 90); do eval "$command" && return 0; sleep 2; done
  die "timed out waiting for $description"
}

mkdir -p "$tmp_dir"/{catalog/images,media,silo-data,sportarr-config,logs}
printf 'smoke image\n' > "$tmp_dir/catalog/images/ufc-300-poster.jpg"
cp "$tmp_dir/catalog/images/ufc-300-poster.jpg" "$tmp_dir/catalog/images/ufc-300-backdrop.jpg"
cp "$tmp_dir/catalog/images/ufc-300-poster.jpg" "$tmp_dir/catalog/images/ufc-300-still.jpg"

log 'source-building Sportarr into both Dockerfile-required publish directories'
cp -a "$sportarr_root/." "$tmp_dir/sportarr-build"
(
  rm -rf publish/docker-linux-x64 publish/docker-linux-arm64
  run_dotnet_publish "$tmp_dir/sportarr-build" linux-x64 publish/docker-linux-x64
  run_dotnet_publish "$tmp_dir/sportarr-build" linux-arm64 publish/docker-linux-arm64
  cd "$tmp_dir/sportarr-build"
  docker build -t "$sportarr_image" .
)
docker network create "$network" >/dev/null
NETWORK_CREATED=1
docker run -d --name "$sportarr_container" --network "$network" --network-alias sportarr -v "$tmp_dir/sportarr-config:/config" "$sportarr_image" >/dev/null
wait_for 'Sportarr migrations and health' "docker exec '$sportarr_container' curl -fsS http://localhost:1867/api/health > '$tmp_dir/sportarr-health.json'"
jq -e '.status == "healthy"' "$tmp_dir/sportarr-health.json" >/dev/null || die 'Sportarr did not report healthy'

log 'seeding the disposable SQLite database with UFC 300 and typed local artwork targets'
seed_args=(--database "$tmp_dir/sportarr-config/sportarr.db")
"$docker_toolchains" && seed_args+=(--docker-toolchains)
"$script_dir/seed-movie-metadata-fixture.sh" "${seed_args[@]}" > "$tmp_dir/seed.txt"

log 'building the local plugin and temporary v1.0.2 -> v1.0.3 binary catalog'
cp -a "$repo_root/." "$tmp_dir/plugin-build"
rm -rf "$tmp_dir/plugin-build/dist"
run_plugin_build_all "$tmp_dir/plugin-build"
curl -fsSL "$v102_binary_url" -o "$tmp_dir/catalog/sportarr-v1.0.2-linux-amd64"
cp "$tmp_dir/plugin-build/dist/plugin-linux-amd64" "$tmp_dir/catalog/sportarr-v1.0.3-linux-amd64"
chmod +x "$tmp_dir/catalog"/sportarr-v1.0.*-linux-amd64
curl -fsSL "$v102_manifest_url" -o "$tmp_dir/v1.0.2-manifest.json"
jq -e '.plugin_id == "silo.sportarr" and .version == "1.0.2"' "$tmp_dir/v1.0.2-manifest.json" >/dev/null || die 'v1.0.2 manifest does not match the released plugin'
old_sha=$(sha256sum "$tmp_dir/catalog/sportarr-v1.0.2-linux-amd64" | awk '{print $1}')
new_sha=$(sha256sum "$tmp_dir/catalog/sportarr-v1.0.3-linux-amd64" | awk '{print $1}')
jq -n --slurpfile m "$tmp_dir/v1.0.2-manifest.json" --arg c "$old_sha" '{plugins:[{manifest:$m[0],binaries:{"linux/amd64":{url:"/sportarr-v1.0.2-linux-amd64",checksum:$c}}}]}' > "$tmp_dir/catalog/index-v102.json"
jq --arg version 1.0.3 '.version=$version | .checksum="__CHECKSUM__"' "$repo_root/manifest.json" > "$tmp_dir/local-manifest.json"
jq -n --slurpfile m "$tmp_dir/local-manifest.json" --arg c "$new_sha" '{plugins:[{manifest:$m[0],binaries:{"linux/amd64":{url:"/sportarr-v1.0.3-linux-amd64",checksum:$c}}}]}' > "$tmp_dir/catalog/index-local.json"
cp "$tmp_dir/catalog/index-v102.json" "$tmp_dir/catalog/index.json"

log 'starting Silo and asserting both aliases resolve from its container'
"${compose[@]}" up -d
wait_for 'Silo setup endpoint' "curl -fsS http://127.0.0.1:$silo_port/api/v1/auth/setup > '$tmp_dir/setup.json'"
jq -e '.needs_setup == true' "$tmp_dir/setup.json" >/dev/null || die 'Silo setup was not available'
"${compose[@]}" exec -T silo sh -ec 'curl -fsS http://sportarr:1867/api/health | grep -q healthy; curl -fsS http://plugin-catalog:80/index.json >/dev/null'

log 'bootstrapping a disposable admin and scanning an actual one-second Movie'
smoke_step='creating the disposable Silo administrator'
setup=$(curl -fsS -X POST "http://127.0.0.1:$silo_port/api/v1/auth/setup" -H 'Content-Type: application/json' --data '{"username":"smoke-admin","email":"smoke@example.invalid","password":"smoke-password-only","create_default_profile":true,"default_profile_name":"Smoke"}')
setup_redacted=$(jq -c 'with_entries(if (.key | test("token|password|secret"; "i")) then .value = "[REDACTED]" else . end)' <<<"$setup")
printf 'setup response (sensitive fields redacted): %s\n' "$setup_redacted"
smoke_step='extracting the disposable Silo access token'
api_token=$(jq -r '.access_token' <<<"$setup")
[[ -n "$api_token" && "$api_token" != null ]] || die 'setup did not return a bearer token'
smoke_step='saving the disposable Silo access token'
printf '%s' "$api_token" > "$tmp_dir/token"
ffmpeg_stderr="$tmp_dir/logs/ffmpeg.stderr"
smoke_step='generating the one-second Movie fixture'
run_ffmpeg_fixture "$tmp_dir/media/UFC 300 (2024).mkv" 2>"$ffmpeg_stderr"
smoke_step='creating the disposable Movie library'
library=$(api POST /libraries '{"name":"Smoke UFC Movies","type":"movies","paths":["/media"]}')
library_id=$(jq -r '.id' <<<"$library")
[[ "$library_id" =~ ^[0-9]+$ ]] || die 'library create did not return an id'
content_id=
for _ in $(seq 1 60); do
  items=$(api GET "/catalog?library_id=$library_id" || true)
  content_id=$(jq -r '.. | objects | .content_id? // empty' <<<"$items" | head -1)
  [[ -n "$content_id" ]] && break
  sleep 2
done
[[ -n "$content_id" ]] || die 'initial scan never returned a Movie content ID'

log 'installing v1.0.2, setting notify, discovering v1.0.3, and applying it'
repository=$(api POST /admin/plugins/repositories '{"url":"http://plugin-catalog:80/index.json","display_name":"smoke catalog","enabled":true}')
repository_id=$(jq -r '.id' <<<"$repository")
installation=$(api POST /admin/plugins/installations "{\"repository_id\":$repository_id,\"plugin_id\":\"silo.sportarr\",\"version\":\"1.0.2\"}")
installation_id=$(jq -r '.id' <<<"$installation")
api PUT "/admin/plugins/installations/$installation_id" '{"update_policy":"notify"}' >/dev/null
cp "$tmp_dir/catalog/index-local.json" "$tmp_dir/catalog/index.json"
api POST /admin/tasks/check_plugin_updates/run >/dev/null
task_diagnostics=
task_completed=false
for _ in $(seq 1 45); do
  task_diagnostics=$(api GET /admin/tasks/check_plugin_updates)
  task_state=$(jq -r '.state' <<<"$task_diagnostics")
  task_result=$(jq -r '.last_execution.status // empty' <<<"$task_diagnostics")
  if [[ "$task_state" == idle && "$task_result" == completed ]]; then
    task_completed=true
    break
  fi
  if [[ "$task_state" == idle && -n "$task_result" && "$task_result" != completed ]]; then
    printf '%s\n' "$task_diagnostics" >&2
    die "check_plugin_updates ended with $task_result: $(jq -r '.last_execution.error_message // "no task error supplied"' <<<"$task_diagnostics")"
  fi
  sleep 2
done
if ! "$task_completed"; then
  printf '%s\n' "$task_diagnostics" >&2
  die 'check_plugin_updates did not complete before its timeout'
fi
available=
for _ in $(seq 1 45); do
  installations=$(api GET /admin/plugins/installations)
  available=$(jq -r --argjson id "$installation_id" '.[] | select(.id == $id) | .available_version // empty' <<<"$installations")
  [[ "$available" == 1.0.3 ]] && break
  sleep 2
done
[[ "$available" == 1.0.3 ]] || die 'available_version was not recorded after check_plugin_updates'
api POST "/admin/plugins/installations/$installation_id/update" >/dev/null

log 'asserting disabled Movie row, then enabling and ordering Sportarr ahead of TMDB'
providers=$(api GET "/libraries/$library_id/providers")
jq -e --argjson id "$installation_id" '.levels.movie[] | select(.plugin_installation_id == $id and .capability_id == "sportarr" and .enabled == false)' <<<"$providers" >/dev/null || die 'Movie Sportarr row was not disabled after update'
levels=$(jq --argjson id "$installation_id" '.levels.movie |= map(if .plugin_installation_id == $id and .capability_id == "sportarr" then .enabled=true | .priority=0 else .priority += 10 end) | {levels:.levels}' <<<"$providers")
api PUT "/libraries/$library_id/providers" "$levels" >/dev/null
api PUT "/admin/plugins/installations/$installation_id/config" "{\"key\":\"sportarr\",\"value\":{\"base_url\":\"$sportarr_base_url\"}}" >/dev/null

log 'matching UFC 300 and proving persisted release date, provider ID, and Sportarr artwork'
candidates=$(api POST "/admin/items/$content_id/match/search" "{\"library_id\":$library_id,\"title\":\"UFC 300\",\"year\":2024}")
provider_id=$(jq -r '.candidates[] | .provider_ids.sportarr // empty' <<<"$candidates" | head -1)
[[ "$provider_id" == v1.ab1c2d3e-4f50-4678-9abc-def012345678 ]] || die "unexpected Sportarr provider id: ${provider_id:-none}"
api POST "/admin/items/$content_id/match/apply" "{\"library_id\":$library_id,\"provider_ids\":{\"sportarr\":\"$provider_id\"}}" >/dev/null
item=$(api GET "/catalog/items/$content_id")
jq -e --arg id "$provider_id" '.release_date == "2024-04-13" and ((.provider_ids.sportarr // .external_ids.sportarr) == $id) and ([.poster_url,.backdrop_url] | all(. != null and . != ""))' <<<"$item" >/dev/null || die 'persisted release date, provider ID, poster, or backdrop assertion failed'
fixture_sha=$(sha256sum "$tmp_dir/catalog/images/ufc-300-poster.jpg" | awk '{print $1}')
assert_fixture_image() {
  local field=$1
  local expected_target=$2
  local persisted_url
  persisted_url=$(jq -r ".$field" <<<"$item")
  [[ -n "$persisted_url" && "$persisted_url" != null ]] || die "persisted $field URL is empty"
  if [[ "$persisted_url" == /* ]]; then
    persisted_url="http://127.0.0.1:8080$persisted_url"
  fi
  "${compose[@]}" exec -T silo sh -ec '
    url=$1
    expected_sha=$2
    expected_target=$3
    artifact=$(mktemp)
    headers=$(mktemp)
    trap '\''rm -f "$artifact" "$headers"'\'' EXIT
    curl -fsSL --max-redirs 4 -D "$headers" -o "$artifact" "$url"
    grep -qi "^content-type: image/jpeg" "$headers"
    { test "$url" = "$expected_target" || grep -Fqi "location: $expected_target" "$headers"; }
    actual_sha=$(sha256sum "$artifact" | cut -d " " -f1)
    test "$actual_sha" = "$expected_sha"
  ' sh "$persisted_url" "$fixture_sha" "$expected_target" || die "Silo could not resolve $field to the expected fixture image"
}
assert_fixture_image poster_url http://plugin-catalog:80/images/ufc-300-poster.jpg
assert_fixture_image backdrop_url http://plugin-catalog:80/images/ufc-300-backdrop.jpg
docker logs "$sportarr_container" > "$tmp_dir/logs/sportarr.log" 2>&1
grep -Fq '/api/metadata/agents/movies' "$tmp_dir/logs/sportarr.log" || die 'Sportarr access log did not prove the in-network Movie request'
log 'local Movie metadata smoke passed'
