#!/usr/bin/env bash
# Deploy step `install` on a dedicated VM (runs as root on a clean Ubuntu 24.04).
# Installs PostgreSQL 16 from Ubuntu's own repository; a no-op when it is there.
# Never fails the deploy: a failed build left a Galaxy host busy, and the
# report shows the missing database anyway.
set -uo pipefail

. /etc/os-release 2>/dev/null || true
if [ "${ID:-}" != "ubuntu" ] || [ ! -d /run/systemd/system ]; then
  echo "install.sh: not a dedicated VM (${PRETTY_NAME:-unknown OS}, no systemd); skipping PostgreSQL."
  echo "install.sh: Galaxy apps can't run PostgreSQL; create the server with placement: dedicated."
  exit 0
fi

if ! command -v pg_lsclusters >/dev/null || ! pg_lsclusters -h | grep -q '^16 '; then
  if ! { apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y postgresql-16; }; then
    echo "install.sh: installing postgresql-16 failed; see above."
    exit 0
  fi
fi
pg_lsclusters
