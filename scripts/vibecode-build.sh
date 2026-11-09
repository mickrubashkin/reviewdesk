#!/usr/bin/env bash
# Builds the VibeCode deploy package in deploy/vibecode (#172): the API and
# goose (static linux/amd64) into bin/, the migrations into db/, the AI prompts
# into ai/ and the web app into web/. Deploy it with ./scripts/vibecode-deploy.sh.
#
# The web build takes its VITE_* settings from personal-notes/vibecode/web.env
# when it exists, never from the root .env (local dev settings); dev and demo
# sign-in are always off.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
package="$root/deploy/vibecode"
out="$package/bin"
rm -rf "$out" "$package/db" "$package/ai" "$package/web"
mkdir -p "$out" "$package/db"

cd "$root/apps/api"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$out/server" ./cmd/server

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

cp -R "$root/db/migrations" "$package/db/migrations"
# The API reads its AI prompts from ai/ where it runs (as in the Dockerfile).
cp -R "$root/apps/api/ai" "$package/ai"

# Variables already in the environment win over Vite's .env files.
(
  if [ -f "$root/personal-notes/vibecode/web.env" ]; then
    set -a
    . "$root/personal-notes/vibecode/web.env"
    set +a
  fi
  export VITE_AUTH_DEV_LOGIN_ENABLED=false VITE_AUTH_DEMO_LOGIN_ENABLED=false
  cd "$root/apps/web"
  npm run build -- --outDir "$package/web" --emptyOutDir
)

du -sh "$out"/* "$package/web"
echo "Ready: $package"
