#!/usr/bin/env bash
# Seed one deterministic Movie-agent event into a disposable migrated SQLite DB.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: seed-movie-metadata-fixture.sh --database PATH [--fixture PATH] [--docker-toolchains]

The target must be the disposable Sportarr SQLite database created by the
local smoke harness after Sportarr has completed migrations. This command
refuses to guess a database path and never contacts a running service.
By default it uses host sqlite3 when available and falls back to a disposable
SQLite Docker container. --docker-toolchains forces that container path.
EOF
}

database=
fixture="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/fixtures/ufc-300-event.json"
sqlite_image="${SPORTARR_SQLITE_IMAGE:-keinos/sqlite3:latest}"
# The official Sportarr image creates its disposable SQLite database as this
# account. Keeping the helper on that account avoids privileged host writes.
sqlite_user="${SPORTARR_SQLITE_USER:-99:100}"
docker_toolchains=false
while (($#)); do
  case "$1" in
    --database) database=${2:?--database needs a value}; shift 2 ;;
    --fixture) fixture=${2:?--fixture needs a value}; shift 2 ;;
    --docker-toolchains) docker_toolchains=true; shift ;;
    --help|-h) usage; exit 0 ;;
    *) printf 'unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

command -v jq >/dev/null || { echo 'jq is required' >&2; exit 1; }
[[ -n "$database" && -f "$database" ]] || { echo 'a migrated --database file is required' >&2; exit 1; }
[[ -f "$fixture" ]] || { echo "fixture not found: $fixture" >&2; exit 1; }
database_dir=$(cd "$(dirname "$database")" && pwd)
database_name=$(basename "$database")

sqlite_exec() {
  if [[ "$docker_toolchains" == false ]] && command -v sqlite3 >/dev/null 2>&1; then
    sqlite3 "$database"
  else
    command -v docker >/dev/null || { echo 'docker is required when sqlite3 is unavailable or --docker-toolchains is used' >&2; return 1; }
    docker run --rm -i --user "$sqlite_user" \
      --mount "type=bind,src=$database_dir,dst=/db" --workdir /db \
      "$sqlite_image" sqlite3 "$database_name"
  fi
}

has_column() { printf "SELECT 1 FROM pragma_table_info('%s') WHERE name='%s';\n" "$1" "$2" | sqlite_exec | grep -qx 1; }
for spec in 'Leagues ExternalId' 'Leagues Name' 'Events MetadataAgentKey' 'Events BroadcastDate' 'Events PosterUrl' 'Events FanartUrl' 'Events ThumbUrl'; do
  set -- $spec
  has_column "$1" "$2" || { echo "database is missing $1.$2; run against Sportarr after the Movie metadata migration" >&2; exit 1; }
done

sql_quote() { sed "s/'/''/g" <<<"$1"; }
league_external=$(jq -r '.league.external_id' "$fixture")
league_name=$(jq -r '.league.name' "$fixture")
league_sport=$(jq -r '.league.sport' "$fixture")
event_external=$(jq -r '.event.external_id' "$fixture")
event_key=$(jq -r '.event.metadata_agent_key' "$fixture")
event_title=$(jq -r '.event.title' "$fixture")
event_sport=$(jq -r '.event.sport' "$fixture")
event_season=$(jq -r '.event.season' "$fixture")
event_date=$(jq -r '.event.event_date' "$fixture")
broadcast_date=$(jq -r '.event.broadcast_date' "$fixture")
event_status=$(jq -r '.event.status' "$fixture")
event_description=$(jq -r '.event.description' "$fixture")
poster_url=$(jq -r '.event.poster_url' "$fixture")
fanart_url=$(jq -r '.event.fanart_url' "$fixture")
thumb_url=$(jq -r '.event.thumb_url' "$fixture")

sqlite_exec <<SQL
PRAGMA foreign_keys = ON;
DELETE FROM Events WHERE ExternalId = '$(sql_quote "$event_external")';
DELETE FROM Leagues WHERE ExternalId = '$(sql_quote "$league_external")';
INSERT INTO Leagues (ExternalId, Name, Sport, Monitored, MonitorType, RetentionDays, SearchForMissingEvents, SearchForCutoffUnmetEvents, MonitorFinals, MonitorPlayoffs, MonitorPreseason, Added)
VALUES ('$(sql_quote "$league_external")', '$(sql_quote "$league_name")', '$(sql_quote "$league_sport")', 1, 0, 0, 0, 0, 0, 0, 0, '2024-01-01T00:00:00Z');
INSERT INTO Events (ExternalId, MetadataAgentKey, Title, Sport, LeagueId, Season, EventDate, BroadcastDate, Monitored, HasFile, Images, Added, Status, Description, PosterUrl, FanartUrl, ThumbUrl)
VALUES ('$(sql_quote "$event_external")', '$(sql_quote "$event_key")', '$(sql_quote "$event_title")', '$(sql_quote "$event_sport")',
        (SELECT Id FROM Leagues WHERE ExternalId = '$(sql_quote "$league_external")'), '$(sql_quote "$event_season")',
        '$(sql_quote "$event_date")', '$(sql_quote "$broadcast_date")', 1, 0, '[]', '2024-01-01T00:00:00Z',
        '$(sql_quote "$event_status")', '$(sql_quote "$event_description")', '$(sql_quote "$poster_url")',
        '$(sql_quote "$fanart_url")', '$(sql_quote "$thumb_url")');
SELECT ExternalId, MetadataAgentKey, BroadcastDate, PosterUrl, FanartUrl, ThumbUrl FROM Events WHERE ExternalId = '$(sql_quote "$event_external")';
SQL
