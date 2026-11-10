# VibeCode deploy

ReviewDesk for colleagues runs on a dedicated VibeCode VM (Bitrix24's app platform), next to the Railway deploy, which stays for the demo. It's the same code: the deploys differ only in their variables. Background and decisions: #171 (pilot), #172.

## How it runs

- **The VM:** a dedicated server created in the VibeCode console (`kind: STANDALONE`, Ubuntu 24.04), set to run 24/7. Galaxy apps can't run PostgreSQL, so a Galaxy app won't do.
- **Deploy steps** (`scripts/vibecode-deploy.sh` sends them):

  | Step | Runs as | What |
  | --- | --- | --- |
  | `install` | root | `install.sh` installs `postgresql-16` once |
  | `preStart` | root | `prestart.mjs` creates the role and database in `DATABASE_URL`, then runs `goose up` |
  | `start` | `vibeapp` | `./bin/server` on port 3000; it serves the web app from `WEB_DIST_DIR` too |
- **Files:** each deploy replaces `/opt/app`. The database lives in `/var/lib/postgresql` and survives deploys. The app's variables are in `/opt/app/.env`, which the service loads. `preStart` doesn't get them, so `prestart.mjs` reads the file itself.

## Build and deploy

```sh
./scripts/vibecode-build.sh
./scripts/vibecode-deploy.sh ["changelog line"]
```

The build puts the API and goose (static linux/amd64), the migrations, the AI prompts and the web app into this folder; all of that is gitignored. The deploy needs:

- `VIBECODE_API_KEY` in `.env`: a `vibe_api_` key with only `vibe:infra`. The VM is bound to the key that manages it: another key gets an empty server list and 404, and only the owner can rebind the VM to a new key, in the console («Серверы Black Hole»). A VibeCode session can't deploy here: its key may not call `/infra`.
- In `personal-notes/vibecode/` (gitignored):
  - `server-id`: the VM's ID;
  - `server.env`: the app's variables as `KEY=value` lines, such as `BITRIX_WEBHOOK_URL` and `AUTH_ALLOWED_ORIGINS` (the app URL);
  - `web.env`: optional `VITE_*` settings for the web build. Dev and demo sign-in are always off.
  - `db-password`: generated on the first deploy.

The app calls the platform with its own key, `BITRIX_API_KEY` in `server.env`: `vibe:ai` for AI, `user` for Bitrix24 IDs, `imbot` for the bot, `vibe:storage` for backups. A bot, too, works only with the key it's bound to; `POST /v1/bots/:id/transfer`, called with the new key, moves it.

The script adds `DATABASE_URL`, `WEB_DIST_DIR` and `PORT`. The changelog line, if given, goes to the subscribers of the app's channel in Bitrix24; leave it out for fixes nobody will notice.

## Backups

The platform backs up nothing on a VM. With `DB_BACKUP_ENABLED=true` the server dumps the database once a day (UTC) with `pg_dump`, gzips it and uploads it to Vibe Storage as `reviewdesk/backups/YYYY-MM-DD.sql.gz` (`internal/backup`). It keeps the last 30 (`DB_BACKUP_KEEP`); a failure goes to the owner's chat once a day and is tried again every hour. One upload takes up to 10 MB; the dump was about 4 MB gzipped in November 2026.

To restore one, run on the VM (through `exec`, as root), with the day you need:

```sh
cd /opt/app && set -a && . ./.env && set +a
curl -sfL -H "X-Api-Key: $BITRIX_API_KEY" -o /tmp/restore.sql.gz \
  "https://vibecode.bitrix24.tech/v1/storage/objects/reviewdesk%2Fbackups%2F2026-11-10.sql.gz"
systemctl stop app
sudo -u postgres psql -X -v ON_ERROR_STOP=1 -c "DROP DATABASE reviewdesk WITH (FORCE)" -c "CREATE DATABASE reviewdesk OWNER reviewdesk"
gunzip -c /tmp/restore.sql.gz | psql -X -q -v ON_ERROR_STOP=1 "$DATABASE_URL"
GOOSE_DRIVER=postgres GOOSE_DBSTRING="$DATABASE_URL" ./bin/goose -dir db/migrations up
systemctl start app && rm /tmp/restore.sql.gz
```

## Looking inside

`POST /v1/infra/servers/:id/exec` runs a command as root on the VM, for example:

- `journalctl -u app -n 50` for the logs;
- `systemctl status app`;
- `sudo -u postgres psql -d reviewdesk` for the database.
