# VibeCode pilot

A throwaway deploy for #171. It checks on VibeCode, before any real work there, that:

- a static Go binary can run from `/opt/app` (no `noexec`, the mode survives install);
- SSE gets through the Node wrapper and the platform gateway unbuffered;
- the platform AI answers a Chat Completions request with a strict `json_schema`;
- the CRM portal's inbound webhook answers from the platform server.

## Build

```sh
./scripts/vibecode-pilot.sh
```

This puts `bin/vibeprobe` (a small server with `--version`, `/health` and a test SSE route) and `bin/server` (the real API, started once only to show that it runs) into this folder. Both are static linux/amd64. `bin/` is gitignored.

## Deploy

Upload this whole folder from a VibeCode session (`node24-pg`, `npm start`). Let the session's deploy tool write `vibecode.json` and use `icon.svg` as the app icon. There are no dependencies, so `npm install` has nothing to do.

Environment:

| Variable | Value |
| --- | --- |
| `BITRIX_WEBHOOK_URL` | the CRM portal's inbound webhook, the same as on Railway |
| `PILOT_AI_MODEL` | optional, default `bitrix/bitrixgpt-5.5` |

The platform provides `PORT`, `BITRIX_API_BASE_URL`, `BITRIX_API_KEY` and, with PostgreSQL, probably `DATABASE_URL`. The report shows which ones are set, never their values.

## Run the checks

Open the app URL:

1. **Binary and environment** loads by itself. `probeVersion.status: 0` means the binary runs. `EACCES` means `noexec` or a lost mode; `ENOENT` on a file that exists means it is not static. `serverStart` should end on a database error (missing, unreachable, or a missing table), which still shows the API starts. `vibeHeaders` shows what the gateway passes.
2. **SSE**: run both buttons. "Go through the wrapper" is the path the AI review uses. If only "Node only" streams, the wrapper's proxy buffers; if both are buffered, the gateway does.
3. **Platform AI** and **CRM webhook**: one request each. The CRM check reports counts only.

Also run `curl -N <app URL>/api/probe/sse` from a terminal: the ten `tick` events should arrive a second apart.

Copy the blocks into #171. Delete the pilot app afterwards; the real deploy comes with #172.
