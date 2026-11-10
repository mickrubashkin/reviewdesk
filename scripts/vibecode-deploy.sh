#!/usr/bin/env bash
# Deploys deploy/vibecode to the dedicated VibeCode VM through the platform's
# Deploy API (#172). Build it first with ./scripts/vibecode-build.sh.
#
# Reads, all gitignored:
# - .env: VIBECODE_API_KEY, a personal vibe_api_ key with vibe:infra;
# - personal-notes/vibecode/server-id: the VM's ID (created in the console);
# - personal-notes/vibecode/app-id: the application card's ID (#187);
# - personal-notes/vibecode/server.env: the app's variables (KEY=value lines),
#   tokens aside: those are application secrets, set in the card.
# - personal-notes/vibecode/db-password: generated on the first deploy.
# Optional: a changelog line as the first argument, sent to the app channel.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
package="$root/deploy/vibecode"
state="$root/personal-notes/vibecode"
api="https://vibecode.bitrix24.tech/v1"
changelog="${1:-}"

VIBECODE_API_KEY="$(set -a; . "$root/.env"; printf '%s' "${VIBECODE_API_KEY:-}")"
[ -n "$VIBECODE_API_KEY" ] || { echo "VIBECODE_API_KEY is not set in .env"; exit 1; }
[ -s "$state/server-id" ] || { echo "Put the VM's ID into $state/server-id"; exit 1; }
[ -s "$state/app-id" ] || { echo "Put the application card's ID into $state/app-id"; exit 1; }
[ -f "$state/server.env" ] || { echo "Put the app's variables into $state/server.env"; exit 1; }
[ -x "$package/bin/server" ] && [ -f "$package/web/index.html" ] || { echo "Run ./scripts/vibecode-build.sh first"; exit 1; }
server="$(cat "$state/server-id")"
app="$(cat "$state/app-id")"

call() {
  local method="$1" path="$2"
  shift 2
  curl -sS -X "$method" "$api$path" -H "X-Api-Key: $VIBECODE_API_KEY" -H "Content-Type: application/json" "$@"
}

info="$(call GET "/infra/servers/$server")"
kind="$(jq -r '.data.kind // empty' <<<"$info")"
status="$(jq -r '.data.status // empty' <<<"$info")"
tunnel="$(jq -r '.data.blackholeStatus // empty' <<<"$info")"
if [ "$kind" != STANDALONE ] || [ "$status" != running ] || [ "$tunnel" != CONNECTED ]; then
  echo "Server $server is not a running dedicated VM (kind=$kind status=$status blackhole=$tunnel)"
  exit 1
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# A secret with a value wins over env anyway; leaving it out of env keeps the
# token out of the deploy call. A name without a value still needs env.
secrets="$(call GET "/applications/$app/secrets")"
[ "$(jq -r '.success' <<<"$secrets" 2>/dev/null)" = true ] || { echo "Can't list the app's secrets: $(head -c 500 <<<"$secrets")"; exit 1; }
jq '[.data.secrets[] | select(.kind == "user" and .hasValue) | .name]' <<<"$secrets" >"$work/secret-names.json"
jq -r '.data.secrets[] | select(.kind == "user" and (.hasValue | not)) | "Secret \(.name) has no value yet: set it at https://vibecode.bitrix24.tech/applications?app='"$app"'"' <<<"$secrets"

# Letters and digits only, so DATABASE_URL needs no escaping. tr ends on
# SIGPIPE when head has enough, which pipefail counts as a failure.
[ -s "$state/db-password" ] || { LC_ALL=C tr -dc 'a-zA-Z0-9' </dev/urandom | head -c 32 >"$state/db-password" || true; }
database_url="postgres://reviewdesk:$(cat "$state/db-password")@127.0.0.1:5432/reviewdesk"

COPYFILE_DISABLE=1 tar -czf "$work/package.tar.gz" -C "$package" --exclude README.md .
base64 <"$work/package.tar.gz" | tr -d '\n' >"$work/package.b64"
grep -E '^[A-Za-z_][A-Za-z0-9_]*=' "$state/server.env" |
  jq -Rn --slurpfile secret "$work/secret-names.json" \
    '[inputs | capture("^(?<key>[^=]+)=(?<value>.*)$") | select(.key | IN($secret[0][]) | not)] | from_entries' >"$work/env.json"

jq -n --rawfile content "$work/package.b64" \
  --slurpfile env "$work/env.json" \
  --arg database_url "$database_url" \
  --arg changelog "$changelog" \
  '{
    source: {content: $content},
    runtime: "node24",
    install: "cd /opt/app && bash install.sh",
    preStart: "cd /opt/app && node prestart.mjs",
    start: "cd /opt/app && ./bin/server",
    port: 3000,
    # The app card in the portal catalog.
    displayName: "ReviewDesk",
    description: "Review of HTML email sequences: comments, edits and checks before sending.",
    env: ($env[0] + {DATABASE_URL: $database_url, WEB_DIST_DIR: "/opt/app/web", PORT: "3000"})
  } + (if $changelog == "" then {} else {changelog: $changelog} end)' >"$work/body.json"

echo "Deploying a $(du -h "$work/package.tar.gz" | cut -f1) package to $server…"
result="$(call POST "/infra/servers/$server/deploy" -H "X-Skip-Source-Snapshot: deploy from a local build" --data-binary "@$work/body.json" --max-time 900)"
# Only the outcome: the full answer may echo the environment.
jq '{success, appUrl: .data.appUrl, error, failed: [(.data.steps // [])[] | select(.status != "ok")]}' <<<"$result" ||
  head -c 2000 <<<"$result"
[ "$(jq -r '.success' <<<"$result" 2>/dev/null)" = true ]
