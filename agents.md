# AGENTS.md

## Context
ReviewDesk is an internal tool for reviewing HTML email sequences. Go backend (Chi + pgx, direct SQL) + React frontend (Mantine v9, TanStack Query, React Router).

## Repo layout
- `apps/api/cmd/server` — HTTP API entrypoint; `main.go` loads `../../.env`.
- `apps/api/cmd/seed` — imports your own templates from `db/seeds/emails/{stage}/{email}/{lang[-old]}.html` with a companion `meta.json` (not included in this repo).
- `apps/api/cmd/demoseed` — synthetic demo board (`DEMO_SEED_ENABLED=true`).
- `apps/web/src/main.tsx` — frontend entrypoint.
- `apps/web/src/App.tsx` — router. `/emails/{id}/edit` redirects to `/emails/{id}/review`.
- `db/migrations/` — goose migrations applied sequentially.

## Commands
- `make setup-dev` — DB up → migrations → seed.
- `make dev` — DB up, API, and web in parallel.
- `make db-migrate` / `make db-seed` / `make db-seed-demo`.
- `cd apps/web && npm run dev|lint|build`. **build** runs `tsc -b && vite build`.
- `cd apps/api && go test ./...`. For a single test: `go test -run TestName ./cmd/server`.
- `cd apps/api && go run ./cmd/server`.
- Smoke test: `./scripts/smoke-check.sh`.

## Environment / tooling
- `.env` lives in the repository root; API loads it via `godotenv.Load("../../.env")`, web via `envDir: "../.."` in `vite.config.ts`.
- `DATABASE_URL` is required for all Go commands and migrations.
- `goose` must be on `PATH` (used in Makefile and entrypoint.sh).
- Web dev server proxies `/api` and `/health` to `localhost:8080`.
- **Admin roles**: `admin` (manages boards, reviews, editing) and `super_admin` (manages users, permissions, HTML). `admin` does not see all administrative pages.
- AI streaming: SSE/EventSource on `/api/emails/{id}/ai-analysis-stream`. Not WebSocket.
- Comments: frontend polling every 5 seconds (`refetchInterval: 5000`), not WebSocket/SSE.
- OTP login: `AUTH_DEV_LOGIN_ENABLED=true` locally to sign in without a code. Must be `false` in production.
- `VITE_AUTH_*` variables mirror `AUTH_*` for the frontend (exposed by Vite).
- OpenAI: `OPENAI_API_KEY` enables AI review; `AI_DEBUG_ENABLED=true` enables debug endpoint.

## Product rules
- Backend is the source of truth — write to DB, then serve via API.
- Never modify the original HTML of an email; editing is template-backed: admin changes `data-edit-*` fields, backend renders final HTML.
- Comments attach to `data-review-block` + text range.
- Third-party HTML preview must be isolated (sandboxed iframe), scripts must not execute.

## Code conventions
- Handlers are thin; business logic stays in helpers/services.
- Direct SQL queries, no ORM.
- Keep changes focused.
- Dependencies: prefer a good, well-maintained existing library over a hand-written solution. Priority is maximum functionality with minimum pain, not a minimal dependency list. Still prefer what the stack already uses (Mantine, TanStack, Chi, pgx) when it covers the need.
- Approval gates: an email cannot be approved while there are open blocking comments or pending required area approvals.
