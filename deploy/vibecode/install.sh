#!/usr/bin/env bash
# Deploy step `install` (root, on the dedicated VM's Ubuntu 24.04): installs
# PostgreSQL 16 from Ubuntu's repository; a no-op once it is there. Its data
# lives in /var/lib/postgresql, which deploys don't touch.
set -euo pipefail

. /etc/os-release
if [ "${ID:-}" != "ubuntu" ] || [ ! -d /run/systemd/system ]; then
  echo "install.sh: needs a dedicated VM with Ubuntu and systemd, got ${PRETTY_NAME:-an unknown OS}" >&2
  exit 1
fi

if ! command -v pg_lsclusters >/dev/null || ! pg_lsclusters -h | grep -q '^16 '; then
  apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get install -y postgresql-16
fi
pg_lsclusters
