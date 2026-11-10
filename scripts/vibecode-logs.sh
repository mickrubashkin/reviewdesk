#!/usr/bin/env bash
# Prints the last lines of the app's log on the VibeCode VM (#172), through
# the platform's exec API. Reads VIBECODE_API_KEY from .env and the VM's ID
# from personal-notes/vibecode/server-id, as vibecode-deploy.sh does.
# Usage: scripts/vibecode-logs.sh [lines]   (default: 100)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
lines="${1:-100}"
[[ "$lines" =~ ^[0-9]+$ ]] || { echo "Usage: $0 [lines]"; exit 2; }

VIBECODE_API_KEY="$(set -a; . "$root/.env"; printf '%s' "${VIBECODE_API_KEY:-}")"
[ -n "$VIBECODE_API_KEY" ] || { echo "VIBECODE_API_KEY is not set in .env"; exit 1; }
[ -s "$root/personal-notes/vibecode/server-id" ] || { echo "Put the VM's ID into personal-notes/vibecode/server-id"; exit 1; }
server="$(cat "$root/personal-notes/vibecode/server-id")"

result="$(curl -sS -X POST "https://vibecode.bitrix24.tech/v1/infra/servers/$server/exec" \
  -H "X-Api-Key: $VIBECODE_API_KEY" -H "Content-Type: application/json" \
  --data "$(jq -n --arg command "journalctl -u app -n $lines --no-pager" '{command: $command}')" \
  --max-time 120)"
jq -r '.data.stdout // empty, .data.stderr // empty' <<<"$result"
[ "$(jq -r '.success' <<<"$result" 2>/dev/null)" = true ] || { head -c 2000 <<<"$result"; exit 1; }
