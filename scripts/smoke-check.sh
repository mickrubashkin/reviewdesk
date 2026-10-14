#!/usr/bin/env sh
set -eu

origin="${1:-${SMOKE_ORIGIN:-}}"

if [ -z "$origin" ]; then
  echo "Usage: $0 https://reviewdesk.example.com" >&2
  echo "Or set SMOKE_ORIGIN=https://reviewdesk.example.com" >&2
  exit 2
fi

origin="${origin%/}"

check_json() {
  url="$1"
  python_expr="$2"

  body="$(mktemp)"
  status="$(curl -sS -o "$body" -w "%{http_code}" "$url")"
  if [ "$status" != "200" ]; then
    echo "Smoke check failed: $url returned HTTP $status" >&2
    cat "$body" >&2
    rm -f "$body"
    exit 1
  fi

  python3 - "$body" "$python_expr" <<'PY'
import json
import sys

path = sys.argv[1]
expr = sys.argv[2]
with open(path, "r", encoding="utf-8") as handle:
    payload = json.load(handle)

if expr == "health":
    ok = payload.get("status") == "ok" and payload.get("database") == "ok"
elif expr == "boards":
    ok = isinstance(payload, list)
else:
    ok = False

if not ok:
    raise SystemExit(f"unexpected payload for {expr}: {payload!r}")
PY
  rm -f "$body"
}

check_boards_proxy() {
  url="$origin/api/boards"
  body="$(mktemp)"

  if [ -n "${SMOKE_SESSION_COOKIE:-}" ]; then
    status="$(curl -sS -H "Cookie: $SMOKE_SESSION_COOKIE" -o "$body" -w "%{http_code}" "$url")"
    if [ "$status" != "200" ]; then
      echo "Smoke check failed: authenticated $url returned HTTP $status" >&2
      cat "$body" >&2
      rm -f "$body"
      exit 1
    fi
    python3 - "$body" boards <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    payload = json.load(handle)

if not isinstance(payload, list):
    raise SystemExit(f"unexpected boards payload: {payload!r}")
PY
    rm -f "$body"
    return
  fi

  status="$(curl -sS -o "$body" -w "%{http_code}" "$url")"
  if [ "$status" != "401" ]; then
    echo "Smoke check failed: unauthenticated $url returned HTTP $status; expected 401" >&2
    cat "$body" >&2
    rm -f "$body"
    exit 1
  fi

  rm -f "$body"
}

check_json "$origin/health" health
check_boards_proxy

echo "Smoke check passed for $origin"
