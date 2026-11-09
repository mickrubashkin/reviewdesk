# VibeCode pilot

A throwaway deploy for #171. It checks on VibeCode, before any real work there, that:

- a static Go binary can run from `/opt/app` (no `noexec`, the mode survives install);
- SSE gets through the Node wrapper and the platform gateway unbuffered;
- the platform AI answers a Chat Completions request with a strict `json_schema`;
- the CRM portal's inbound webhook answers from the platform server;
- on a dedicated VM: PostgreSQL 16 installs, the migrations run, the real API starts against it, and the database outlives a redeploy;
- what the gateway and the platform API tell about the signed-in user.

The first run (on Galaxy) covered the first four. Galaxy has no PostgreSQL (an app's own database there is SQLite), so the database is checked in one of two ways:

- **Galaxy with an external PostgreSQL** (tried first): `DATABASE_URL` points at a separate `reviewdesk_pilot` database on the Railway server, with its own role.
- **A dedicated VM**: PostgreSQL installed on the machine (below).

The wrapper runs the migrations and starts the real API only against a database whose name ends in `pilot`; any other gets the read-only check alone.

## Build

```sh
./scripts/vibecode-pilot.sh
```

It fills this folder with build output (gitignored):

- `bin/vibeprobe`: a small server with `--version`, `/health` and a test SSE route;
- `bin/server`: the real API, started for a few seconds only to show that it runs;
- `bin/goose` (Postgres only) and `db/migrations/`;
- `ai/`: the prompts the API reads at start, as in the Dockerfile.

All binaries are static linux/amd64.

## Deploy on Galaxy with an external database

From a VibeCode session, deploy this folder as is with the platform's default steps (`npm install`, `npm start`); `install.sh` and `setup-db.mjs` are for a VM and stay unused. Environment:

| Variable | Value |
| --- | --- |
| `DATABASE_URL` | the pilot database's public URL: `postgres://reviewdesk_pilot:<password>@<railway proxy host>:<port>/reviewdesk_pilot` |
| `BITRIX_WEBHOOK_URL` | the CRM portal's inbound webhook, the same as on Railway |

Database then shows the connect time and the median of twenty `SELECT 1` round trips: what each query pays for the database being elsewhere.

## Deploy on a dedicated VM

A VibeCode session can't create a dedicated VM: its key may not call `/infra`, and its own create tool always makes a Galaxy app. Deploy from your terminal instead:

```sh
./scripts/vibecode-pilot-deploy.sh
```

It needs `VIBECODE_API_KEY` (a personal `vibe_api_` key with `vibe:infra` and `vibe:ai`) and `BITRIX_WEBHOOK_URL` in `.env`. It creates the server (`placement: "dedicated"`, `kind: STANDALONE`), waits for it, and deploys the folder. The server ID and a generated database password are kept in `personal-notes/vibecode-pilot/`, so running it again redeploys to the same server. The deploy uses:

| Field | Value |
| --- | --- |
| `runtime` | `node24` |
| `install` | `cd /opt/app && bash install.sh` (installs `postgresql-16` with apt) |
| `preStart` | `cd /opt/app && node setup-db.mjs` (role, database, migrations; result in `pilot-setup.json`) |
| `start` | `cd /opt/app && node server.mjs` |
| `port` | `3000` |

Environment:

| Variable | Value |
| --- | --- |
| `DATABASE_URL` | `postgres://reviewdesk:<a new password>@127.0.0.1:5432/reviewdesk_pilot`; `setup-db.mjs` creates this role and database |
| `BITRIX_WEBHOOK_URL` | the CRM portal's inbound webhook, the same as on Railway |
| `BITRIX_API_BASE_URL`, `BITRIX_API_KEY` | `https://vibecode.bitrix24.tech/v1` and the same personal key (a VM gets no app key of its own) |
| `PILOT_AI_MODEL` | optional, default `bitrix/bitrixgpt-5.5` |

The report shows which variables are set, never their values.

After the checks, **deploy the same folder a second time** and run Database again.

## Run the checks

Open the app URL:

1. **Binary and environment** loads by itself.
   - `probeVersion.status: 0` means the binary runs. `EACCES` means `noexec` or a lost mode; `ENOENT` on a file that exists means it is not static.
   - `serverStart`: with the database set up, `signal: SIGTERM` and `API server listening` mean the real API ran until the timeout stopped it. An exit with a database error means the setup failed.
   - `setup` is the `preStart` result, step by step.
   - `vibeHeaders` shows what the gateway passes; the session token is cut short.
2. **SSE**: "Go through the wrapper" is the path the AI review uses. If only "Node only" streams, the wrapper's proxy buffers; if both are buffered, the gateway does.
3. **Platform AI**, **CRM webhook**: one request each. The CRM check reports counts only.
4. **Database**: connect time, round-trip median and max, the PostgreSQL and goose versions. `migrations` and `serverStart` in the first block show the migrations and the real API against it. On a VM, `setup.steps` shows the database setup.
5. **Identity**: looks up the signed-in user with `/users/:id` and `/me`. It shows field names, the time taken and whether an email is there, never the values.

Copy the blocks into #172. Delete the pilot server afterwards.
