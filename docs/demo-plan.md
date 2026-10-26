# ReviewDesk Demo Plan

Goal: run a public demo without risking the production data used by the current team.

## Recommended Shape

- Production stays on the current production database and private access model.
- Public demo runs on `demo.reviewdesk.example.com` or equivalent.
- Demo uses a separate frontend, backend, and PostgreSQL database.
- Demo data is disposable and can be reset from demo seed at any time.
- Do not add workspaces/teams only to support the first public demo.

Subdomain is preferred over `/demo` because it keeps routes, cookies, API proxying, and frontend asset paths simple.

## Demo Access

- Demo-login mode is enabled only in the demo environment.
- Public visitors open the board directly; no login page and no email OTP.
- Each visitor gets a sandbox: their own board-scoped user (`visitor-…@visitors.demo.invalid`, a reviewer who manages one board) and their own copy of the demo board (`demo-visit-…`), seeded on entry. A board-scoped user sees only the boards they manage, so visitors never see each other's boards, comments, or edits.
- The session ends with the sandbox, 24 hours after the visit. The API deletes expired sandboxes every hour, and the oldest ones when there are more than 500. A visitor who comes back later gets a fresh copy.
- Entries are rate-limited to 20 per hour per address.
- A visitor's writes are capped per visitor: 240 a minute and 2,000 an hour in all, 60 an hour that add something (comments, replies, emails, slots, stages; previews and edits do not count), and request bodies up to 2 MB. Over a limit the API answers 429 with `Retry-After`. Counters are in memory and reset on restart. See `internal/demo/write_limit.go`.
- The shared `demo-onboarding` board is still seeded by `cmd/demoseed`. Visitors do not see it.

## Allowed Demo Actions

Safe baseline:

- browse boards and emails;
- add comments;
- resolve/reopen own comments;
- inspect activity, approvals, AI analysis, and handoff state.

Interactive demo option:

- allow edit, approve, and re-approve actions in the demo database;
- reset demo data on a schedule or through an admin-only reset command.

## Protection

- Put the demo behind Cloudflare or equivalent edge protection.
- Rate-limit auth, comments, HTML upload/create, and AI endpoints.
- Disable public HTML upload/create unless it is required for the demo.
- Keep AI debug endpoints disabled.
- Limit AI force refresh or make demo AI cached-only.
- Keep production and demo secrets, databases, and cookies separate.

## Current Implementation

- Demo seed/reset strategy is implemented and isolated from normal seed data.
- Demo login is implemented behind `AUTH_DEMO_LOGIN_ENABLED`, with a sandbox per visitor (`internal/demo`).
- Demo backend can reset synthetic data automatically on deploy.
- Public demo uses a separate frontend, backend, and PostgreSQL database.
- Demo AI is live but limited: each visitor gets 5 OpenAI calls (analysis and translations; cached results are free) and the whole demo 200 per 24 hours, counted in `demo_ai_calls`.
- Demo notification emails (@mentions) are only logged, even if `RESEND_API_KEY` is set.
- Every demo entry is recorded in `demo_visits` (source, device, and what the visitor did), which outlives the sandbox and demo resets. See Visit Stats.
- Remaining demo work: screenshots and short demo video.

## Seed Commands

Demo seed is intentionally separate from the normal production/current-team seed. It uses synthetic generic content, not copied or anonymized production emails.

```sh
DEMO_SEED_ENABLED=true make db-seed-demo
DEMO_SEED_ENABLED=true DEMO_RESET_CONFIRM=demo make db-reset-demo
```

In the Railway backend service console, run the compiled demo seed binary from `/app`:

```sh
DEMO_SEED_ENABLED=true DEMO_RESET_CONFIRM=demo /app/demoseed -reset
```

To reset demo data automatically on each demo backend deploy, set these variables on the Railway backend service:

```env
RUN_DEMO_SEED=true
DEMO_SEED_ENABLED=true
DEMO_RESET_ON_DEPLOY=true
DEMO_RESET_CONFIRM=demo
```

The demo board key is `demo-onboarding`. Reset deletes only demo-owned data: that board, every visitor sandbox, and the demo sessions, and then recreates the generic demo scenario. It also removes the migration-created `onboarding` fallback board only when that board has no emails.

The seeded dataset is production-like but disposable:

- 8 stages: `signup`, `profile`, `activation`, `education`, `integration`, `first-value`, `expansion`, `retention`;
- 20 generic email concepts across 3 languages: `en`, `de`, `fr`;
- 60 emails total, with mixed review statuses, comments, approval states, stale approvals, and handoff-ready examples;
- optional hand-crafted fixtures under `db/seeds/demo-emails` can override generated emails with the same slug.

Demo login is controlled by environment variables:

```env
AUTH_DEMO_LOGIN_ENABLED=true
VITE_AUTH_DEMO_LOGIN_ENABLED=true
```

Keep both disabled in production.

## Visit Stats

`demo_visits` has one row per visitor: the source (`utm_source` of the link, else the referring site; every LinkedIn address counts as `linkedin`; empty is direct), whether the device is mobile, and the actions. The actions are email event counts by action, `marked_ready` for a status change to ready (the last step of the demo guide), and `ai_calls`. They are read from the live events while the sandbox lives and copied into the visit when the sandbox is deleted.

Nobody can sign in to the demo as an admin, so the stats are a command. In the Railway backend service console:

```sh
/app/demostats              # visits per day and per source, last 14 days
/app/demostats -days 30 -visits   # and each visit with its actions
```

Days are in `Europe/Madrid` (`-tz` to change). Locally: `cd apps/api && go run ./cmd/demostats`.

## Demo Story

- Open `Demo Onboarding Review`.
- Review the `Complete Setup` hero email in the `activation` stage.
- Show one open blocking comment and one resolved comment.
- Show required approval areas with one approved area and one stale area.
- Open the `Upgrade Nudge` email to show a handoff-ready `production_approved` state.
