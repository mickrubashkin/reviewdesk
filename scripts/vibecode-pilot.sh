#!/usr/bin/env bash
# Builds the VibeCode pilot (#171) into deploy/vibecode-pilot/bin: the probe and
# the real API, static linux/amd64. Upload the whole folder from a VibeCode session.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
out="$root/deploy/vibecode-pilot/bin"
mkdir -p "$out"

cd "$root/apps/api"
for cmd in vibeprobe server; do
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$out/$cmd" "./cmd/$cmd"
  chmod 755 "$out/$cmd"
done

ls -l "$out"
echo "Ready: $root/deploy/vibecode-pilot"
