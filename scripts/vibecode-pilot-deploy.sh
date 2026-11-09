#!/usr/bin/env bash
# Deploys deploy/vibecode-pilot through the platform's Deploy API (#172): a
# VibeCode session can't, its key may not call /infra.
#
# The server is the one in personal-notes/vibecode-pilot/server-id; without it
# the script creates a dedicated VM and keeps its ID there.
# - A dedicated VM (kind STANDALONE) gets PostgreSQL on the machine (install.sh,
#   setup-db.mjs) and a generated password kept in personal-notes/.
# - A Galaxy app (kind GALAXY_APP) has no PostgreSQL: DATABASE_URL comes from
#   personal-notes/vibecode-pilot/database-url (the external pilot database), if
#   that file exists.
#
# Needs in .env: VIBECODE_API_KEY (a personal vibe_api_ key with vibe:infra and
# vibe:ai) and BITRIX_WEBHOOK_URL. Run ./scripts/vibecode-pilot.sh first.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
pilot="$root/deploy/vibecode-pilot"
state="$root/personal-notes/vibecode-pilot"
api="https://vibecode.bitrix24.tech/v1"

set -a
. "$root/.env"
set +a
: "${VIBECODE_API_KEY:?VIBECODE_API_KEY is not set in .env}"
: "${BITRIX_WEBHOOK_URL:?BITRIX_WEBHOOK_URL is not set in .env}"
[ -x "$pilot/bin/server" ] || { echo "Run ./scripts/vibecode-pilot.sh first"; exit 1; }
mkdir -p "$state"

call() {
  local method="$1" path="$2"
  shift 2
  curl -sS -X "$method" "$api$path" -H "X-Api-Key: $VIBECODE_API_KEY" -H "Content-Type: application/json" "$@"
}

if [ ! -s "$state/server-id" ]; then
  image="$(call GET /infra/providers/bitrix-cloud/images |
    jq -r '[.data[] | select((.name // "" | test("24\\.04")) or (.description // "" | test("24\\.04")))][0].id // empty')"
  if [ -z "$image" ]; then
    echo "No Ubuntu 24.04 image found. Images:"
    call GET /infra/providers/bitrix-cloud/images | jq '.data'
    exit 1
  fi

  created="$(call POST /infra/servers -d "$(jq -n --arg image "$image" \
    '{provider: "bitrix-cloud", placement: "dedicated", name: "reviewdesk-pilot", plan: "bc-small", region: "ru-central1-b", image: $image}')")"
  kind="$(jq -r '.data.kind // empty' <<<"$created")"
  if [ "$kind" != "STANDALONE" ]; then
    echo "Expected a STANDALONE server, got:"
    jq . <<<"$created"
    exit 1
  fi
  jq -r '.data.id' <<<"$created" >"$state/server-id"
  echo "Created server $(cat "$state/server-id")"
fi
server="$(cat "$state/server-id")"
kind="$(call GET "/infra/servers/$server" | jq -r '.data.kind // empty')"
echo "Server $server: ${kind:-not found}"

if [ "$kind" = GALAXY_APP ]; then
  database_url="$(cat "$state/database-url" 2>/dev/null || true)"
  install="cd /opt/app && npm install --omit=dev"
  pre_start=""
elif [ "$kind" = STANDALONE ]; then
  install="cd /opt/app && bash install.sh"
  pre_start="cd /opt/app && node setup-db.mjs"
else
  exit 1
fi

[ "$kind" = STANDALONE ] && echo "Waiting for running + CONNECTED…"
for _ in $([ "$kind" = STANDALONE ] && seq 1 60); do
  info="$(call GET "/infra/servers/$server")"
  status="$(jq -r '.data.status' <<<"$info")"
  tunnel="$(jq -r '.data.blackholeStatus' <<<"$info")"
  echo "  status=$status blackhole=$tunnel"
  [ "$status" = running ] && [ "$tunnel" = CONNECTED ] && break
  [ "$status" = error ] && { jq . <<<"$info"; exit 1; }
  sleep 10
done
if [ "$kind" = STANDALONE ]; then
  [ "$status" = running ] && [ "$tunnel" = CONNECTED ] || { echo "Server not ready after 10 minutes"; exit 1; }
  # Letters and digits only, so DATABASE_URL needs no escaping.
  # tr ends on SIGPIPE when head has enough, which pipefail counts as a failure.
  [ -s "$state/db-password" ] || { LC_ALL=C tr -dc 'a-zA-Z0-9' </dev/urandom | head -c 32 >"$state/db-password" || true; }
  database_url="postgres://reviewdesk:$(cat "$state/db-password")@127.0.0.1:5432/reviewdesk_pilot"
fi

archive="$(mktemp -d)/pilot.tar.gz"
body="$(dirname "$archive")/body.json"
trap 'rm -rf "$(dirname "$archive")"' EXIT
COPYFILE_DISABLE=1 tar -czf "$archive" -C "$pilot" --exclude README.md --exclude pilot-setup.json .
base64 <"$archive" | tr -d '\n' >"$archive.b64"
jq -n --rawfile content "$archive.b64" \
  --arg database_url "$database_url" \
  --arg webhook "$BITRIX_WEBHOOK_URL" \
  --arg key "$VIBECODE_API_KEY" \
  --arg install "$install" \
  --arg pre_start "$pre_start" \
  '{
    source: {content: ($content | rtrimstr("\n"))},
    runtime: "node24",
    install: $install,
    start: "cd /opt/app && node server.mjs",
    port: 3000,
    # The app card in the portal catalog, which every employee can see.
    displayName: "ReviewDesk pilot (test)",
    description: "A test deploy that checks the platform for ReviewDesk. Not for use.",
    env: ({
      BITRIX_WEBHOOK_URL: $webhook,
      BITRIX_API_BASE_URL: "https://vibecode.bitrix24.tech/v1",
      BITRIX_API_KEY: $key
    } + (if $database_url == "" then {} else {DATABASE_URL: $database_url} end))
  } + (if $pre_start == "" then {} else {preStart: $pre_start} end)' >"$body"
echo "Deploying $(du -h "$archive" | cut -f1) archive…"

result="$(call POST "/infra/servers/$server/deploy" -H "X-Skip-Source-Snapshot: pilot deploy from a local folder" --data-binary "@$body" --max-time 900)"
# Only the outcome: the full answer may echo the environment.
jq '{success, appUrl: .data.appUrl, error, steps: (.data.steps // null | if . then map({name: (.name // .step), status}) else null end)}' <<<"$result" ||
  echo "$result" | head -c 2000
