# AGENTS.md

## Context
ReviewDesk is a review and approval tool for HTML email sequences. Go backend (Chi + pgx, direct SQL) + React frontend (Mantine v9, TanStack Query, React Router).

## Repo layout
- `apps/api/cmd/server` — HTTP API entrypoint; `main.go` loads `../../.env`.
- `apps/api/cmd/seed` — imports your own templates from `db/seeds/emails/{stage}/{email}/{lang[-old]}.html` with a companion `meta.json` (not in the public repo).
- `apps/api/cmd/demoseed` — synthetic demo board (`DEMO_SEED_ENABLED=true`).
- `apps/api/cmd/syncmeta` — regenerates `meta.json`.
- `apps/web/src/main.tsx` — frontend entrypoint.
- `apps/web/src/App.tsx` — router. `/emails/{id}/edit` redirects to `/emails/{id}/review`.
- `db/migrations/` — goose migrations applied sequentially.
- `docs/help/en/` — user help in Markdown, built into the web app at `/help` (`apps/web/src/features/help/`). Links between pages are plain `page.md#anchor`.

## Commands
- `make setup-dev` — DB up → migrations → seed (`db/seeds/emails` if present, else the demo board).
- `make dev` — DB up, API, and web in parallel.
- `make db-migrate` / `make db-seed` / `make db-seed-demo` / `make db-sync-meta`.
- `cd apps/web && npm run dev|lint|test|build`. **build** runs `tsc -b && vite build`; **test** runs Vitest (`*.test.ts` next to the code, for pure logic).
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
- The API accepts only the Origin in `AUTH_ALLOWED_ORIGINS` (else `CORS_ORIGIN`, `http://localhost:5173`): a second dev server on another port (e.g. to check a branch in a worktree) cannot sign in unless its Vite proxy sets that `Origin` header.
- OTP login: `AUTH_DEV_LOGIN_ENABLED=true` locally to sign in without a code. Must be `false` in production.
- `VITE_AUTH_*` variables mirror `AUTH_*` for the frontend (exposed by Vite).
- Portal sync: manual from Settings → Portal sync (or `cmd/portalsync`), nightly at 03:00 UTC+2 when `PORTAL_AUTOSYNC_ENABLED=true` and the board turns it on. One running sync per board (unique index).
- OpenAI: `OPENAI_API_KEY` enables AI review; `AI_DEBUG_ENABLED=true` enables debug endpoint.
- Demo visit messages: with `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` set, the demo server posts each entry and, once a visitor has been quiet for 30 minutes, a report of the visit (`internal/demo/notify.go`). `demo_visits.reported_at` marks the reported ones. The browser sends a random `visitor_id` kept in `localStorage` (returning visitors), its time zone and language (rough place; no IP is stored), and the owner's key: `?owner=<DEMO_OWNER_KEY>` once marks that browser's visits as the owner's.

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

## Board views
- Views: `cards`, `matrix`, `timeline`, `archive`; `timeline` is the default. The view is in the URL (`?view=…`) and wins over the last one kept in `sessionStorage`. `?stage=<stage>` is a one-shot reveal: BoardHome scrolls to `[data-stage]` and drops the param; the timeline also unfolds that stage. Each timeline stage has a "Copy link" for it.
- Timeline (`features/boards/BoardHome/Timeline/`): the stages are the nodes of a trunk (a deal's way through the pipeline) and each stage's emails a branch off it. Days count per stage from a deal entering it (`timingDays`, as in the first-send estimate); stages carry no timing between them, so the trunk has no time scale. On a board with regions a stage has one branch per region (reference first), lined up slot by slot with gaps, and waits that differ from the reference are marked; otherwise there is one branch per language.
- Timeline previews are sandboxed iframes (`sandbox=""`, no scripts), kept only within 600px of the timeline's own scroll box; the HTML comes from the React Query cache (`["emails", id, "review"]`).
- The logic is pure (`timeline.ts`) and tested (`timeline.test.ts`, Vitest); `BoardTimeline.tsx` and `TimelineNode.tsx` only draw it.

## Git workflow
- Never commit to `main` directly. One branch per change: `feat/…`, `fix/…`, `perf/…`, `chore/…`.
- Conventional commit subjects (`feat: …`, `fix: …`, `perf: …`, `chore: …`), lowercase, imperative.
- Branches are merged into `main` with a merge commit (`Merge branch '…'`), no squash.
- Assume anything committed can become public, because a public copy of this code is synced from it. Never commit secrets, `.env`, production data, or `personal-notes/` (gitignored), and keep company names, real people, domains and data out of code, docs, fixtures and screenshots. The sync checks text only: screenshots must come from the demo data, never from a real board.
- Ending a session: `/handoff` (`.claude/skills/handoff`) brings open work to PRs, deletes merged branches (`scripts/handoff.sh`), leaves git clean and updates the plan. `/handoff merge` also merges once checks pass.

## User help
When a change alters what a user sees or does, update the help in the same PR:
- `docs/help/en/` — the pages that describe it. Renaming a heading breaks the anchors in `helpTopics` (`features/help/help.ts`); the help test catches it.
- The tour (`features/tour/tour.ts`) if a step it shows changed; bump `TOUR_VERSION` only when everyone should see the tour again.
- An entry in `docs/help/en/whats-new.md` under `## YYYY-MM-DD` (ISO date, newest first). Every new entry brings the dot back on everyone's account menu, so add only what users will notice.
- Help is published with the public copy: write it neutrally, check it against `scripts/public/denylist.txt`, and take screenshots from the demo board only.

## Verifying changes
- Backend: `cd apps/api && go test ./...`. Frontend: `cd apps/web && npm run lint && npm test && npm run build`.
- For UI changes run `make dev`, open the affected page in the browser and check it as the right role: `super_admin` for users/permissions/HTML, `admin` for boards and reviews. Use `AUTH_DEV_LOGIN_ENABLED=true` to sign in locally.
- Migrations are goose files in `db/migrations/`, applied in order. Add a new file, never edit an applied one.
