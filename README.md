# ReviewDesk

A collaborative review and approval workspace for HTML email sequences and lifecycle journeys.

ReviewDesk is a focused review and approval layer for HTML email sequences before they are implemented in CRM, ESP, messaging, or other publishing systems. It keeps comments attached to reviewed content, makes approvals auditable, and helps teams hand off final approved assets without turning the product into a full email builder.

## Highlights

- Review board for staged HTML email sequences, language versions, and adaptations.
- Anchored comments on `data-review-block` content with severity and resolution state.
- Template-backed editing through `data-edit-*` fields while preserving original HTML.
- Required approval areas, stale approval behavior after edits, and production handoff state.
- AI-assisted journey review for clarity, conversion friction, compliance risk, and handoff readiness.
- Go API, PostgreSQL, React, Mantine, TanStack Query, OTP/session auth, and SSE AI streaming.

## Demo Flow

1. Start the local stack with `make setup-dev` and `make dev`.
2. Sign in with the local dev login and open the `Demo Onboarding Review` board.
3. Add or resolve anchored comments in the review preview.
4. Edit template-backed copy or planning metadata as an admin.
5. Show approvals becoming stale after content changes, then re-approve required areas.
6. Inspect the production handoff package.

## Stack

- Frontend: React 19, Vite, Mantine v9, TanStack Query, React Router.
- Backend: Go, Chi, pgx, direct SQL.
- Database: PostgreSQL with goose migrations.
- AI: shared cached analysis with first-run SSE/EventSource streaming.
- Auth: OTP login, session cookies, admin and super-admin roles.

## Local Setup

Prerequisites:

- Docker
- Go
- Node.js + npm
- `goose` on `PATH`

Commands:

```sh
cp .env.example .env
make setup-dev
make dev
```

Useful checks:

```sh
make lint
make test-api
make test-web
```

## Repo Layout

- `apps/api/cmd/server` - Go HTTP API.
- `apps/api/cmd/seed` - loader for your own templates in `db/seeds/emails` (not included).
- `apps/api/cmd/demoseed` - synthetic demo dataset (`db/seeds/demo-emails` plus generated emails).
- `apps/web/src` - React application.
- `db/migrations` - PostgreSQL migrations.
- `docs/case-study.md` - concise product/engineering case study.
- `docs/demo-plan.md` - public demo environment strategy.

## Demo Data

Generic public-demo data is seeded separately from normal team data:

```sh
DEMO_SEED_ENABLED=true make db-seed-demo
DEMO_SEED_ENABLED=true DEMO_RESET_CONFIRM=demo make db-reset-demo
```

See `docs/demo-plan.md` for the public demo strategy.

Public demo login is also environment-gated:

```env
AUTH_DEMO_LOGIN_ENABLED=true
VITE_AUTH_DEMO_LOGIN_ENABLED=true
```

## Notes

- Backend is the source of truth for review state, comments, approvals, versions, and handoff data.
- Original HTML is preserved; edited output is rendered from template-backed fields.
- Previewed third-party HTML is isolated in a sandboxed iframe.
- Comments use frontend polling; AI analysis uses SSE/EventSource, not WebSocket.
