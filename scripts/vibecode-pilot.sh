#!/usr/bin/env bash
# Builds the VibeCode pilot (#171) in deploy/vibecode-pilot: the probe, the real
# API and goose (static linux/amd64) into bin/, the migrations into db/ and the
# AI prompts into ai/.
# Upload the whole folder from a VibeCode session.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
pilot="$root/deploy/vibecode-pilot"
out="$pilot/bin"
mkdir -p "$out"

cd "$root/apps/api"
for cmd in vibeprobe server; do
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$out/$cmd" "./cmd/$cmd"
done

# go install can't put a cross-compiled binary into GOBIN, so build goose in a
# throwaway module instead (the same version as the Dockerfile).
goose_dir="$(mktemp -d)"
trap 'rm -rf "$goose_dir"' EXIT
(
  cd "$goose_dir"
  go mod init goosebuild >/dev/null 2>&1
  go get github.com/pressly/goose/v3/cmd/goose@v3.24.1 >/dev/null 2>&1
  # Postgres only: the other drivers triple its size.
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
    -tags "no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb" -o "$out/goose" github.com/pressly/goose/v3/cmd/goose
)
chmod 755 "$out"/*

rm -rf "$pilot/db" "$pilot/ai"
mkdir -p "$pilot/db"
cp -R "$root/db/migrations" "$pilot/db/migrations"
# The API reads its AI prompts from ai/ next to where it runs (as in the Dockerfile).
cp -R "$root/apps/api/ai" "$pilot/ai"

ls -l "$out"
echo "Ready: $pilot"
