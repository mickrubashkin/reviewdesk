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
- Public visitors use `Enter demo`; no email OTP is required.
- The demo session uses `demo-reviewer@example.com` as a fixed low-privilege reviewer account.
- Optional: add a separate `Try admin mode` demo session only if edit/approval interaction is part of the recorded story.

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
- Demo login is implemented behind `AUTH_DEMO_LOGIN_ENABLED`.
- Demo backend can reset synthetic data automatically on deploy.
- Public demo uses a separate frontend, backend, and PostgreSQL database.
- Remaining demo work: edge rate limits, screenshots, and short demo video.

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

The demo board key is `demo-onboarding`. Reset deletes only demo-owned board/email/session data and then recreates the generic demo scenario. It also removes the migration-created `onboarding` fallback board only when that board has no emails.

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

## Demo Story

- Open `Demo Onboarding Review`.
- Review the `Complete Setup` hero email in the `activation` stage.
- Show one open blocking comment and one resolved comment.
- Show required approval areas with one approved area and one stale area.
- Open the `Upgrade Nudge` email to show a handoff-ready `production_approved` state.
