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

- `VIBECODE_API_KEY` in `.env`: a personal `vibe_api_` key with `vibe:infra`. A VibeCode session can't deploy here: its key may not call `/infra`.
- In `personal-notes/vibecode/` (gitignored):
  - `server-id`: the VM's ID;
  - `server.env`: the app's variables as `KEY=value` lines, such as `BITRIX_WEBHOOK_URL` and `AUTH_ALLOWED_ORIGINS` (the app URL);
  - `web.env`: optional `VITE_*` settings for the web build. Dev and demo sign-in are always off.
  - `db-password`: generated on the first deploy.

The script adds `DATABASE_URL`, `WEB_DIST_DIR` and `PORT`. The changelog line, if given, goes to the subscribers of the app's channel in Bitrix24; leave it out for fixes nobody will notice.

## Looking inside

`POST /v1/infra/servers/:id/exec` runs a command as root on the VM, for example:

- `journalctl -u app -n 50` for the logs;
- `systemctl status app`;
- `sudo -u postgres psql -d reviewdesk` for the database.
