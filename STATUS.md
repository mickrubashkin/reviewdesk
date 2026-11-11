# ReviewDesk status

Last updated: 2026-11-11

A living snapshot of where the project stands. For the pitch and setup see [README.md](README.md), for the story [docs/case-study.md](docs/case-study.md), for terms [GLOSSARY.md](GLOSSARY.md), for decisions [docs/adr/](docs/adr/), for what users were told [docs/help/en/whats-new.md](docs/help/en/whats-new.md).

## 1. What it is

ReviewDesk is a review and approval workspace for staged HTML email sequences that are sent by CRM automation robots. A partner onboarding team (region managers, translators, CRM operators, an owner) uses it to comment on exact blocks of an email, edit template-backed copy without touching the source HTML, keep translations in step with their EN master, and see which version each robot actually sends. Its core value is one place for review, edits and the hand-off to the CRM, instead of chats, screenshots and manual checks.

## 2. Current state

In production for the team; what a user sees today:

- **Boards** in four views: Timeline (default; stages as a trunk, emails as branches, one branch per region or language, present mode, a link per stage), Matrix (slots by language or region, bulk actions, drag to reorder, create a translation from EN), Cards and Archive. See [board views](docs/help/en/board-views.md).
- **Review page**: sandboxed preview, comments anchored to a block and text range, @mentions, side panel with comments, history, plan and sends.
- **Editing**: emails built from a block library (text, list, closing text, login, signature), Markdown in text fields, shared texts (footer, privacy link, logo, signature) with per-board overrides, Bitrix24 variables in fields. The original HTML is never modified.
- **Status**: Draft / Ready. Ready runs the checks of form; an edit moves a ready email back to Draft. Comments do not gate Ready.
- **Translations and regions**: each translation follows its EN master and is flagged when behind; each region's adaptation follows the reference (ADR 0001). AI pre-translation with a translation glossary anyone can suggest terms to.
- **CRM hand-off**: a "copy for Bitrix24" dialog marks the copied version as uploaded per robot; **My tasks** lists what each person has to translate, review or upload.
- **Portal sync** (read-only, nightly or manual): pulls what robots actually sent, sets Live automatically, suggests the uploaded version, and shows a **Sends** tab per email (daily sends by text, by deal stage, deals per email, versions compared, last 30/90/180 days). See [Bitrix24 upload](docs/help/en/bitrix24-upload.md).
- **AI review** of an email in the context of its journey, streamed over SSE.
- **People**: sign-in with the company Bitrix24 account through the hosting platform's gateway; roles Viewer, Translator, Region manager, Board owner, CRM operator (on top of `admin` / `super_admin`); merge duplicate users and address aliases.
- **Notifications**: mentions arrive as Bitrix24 bot messages; replying to the bot answers in the comment's thread and **Resolve** resolves it. The owner gets a daily activity digest and admin alerts through the same bot.
- **Help**: `/help` with search, inline help links, What's new and a first-login tour.

## 3. Architecture and stack

- **Backend** (`apps/api`): Go 1.25, Chi, pgx, direct SQL (no ORM). Feature packages in `internal/features/` (`ai`, `auth`, `boards`, `comments`, `emails`, `portal`); email rendering and editing in `internal/emailblocks`, `emailedit`, `emailtext`, `emailreview`; background jobs in `internal/digest`, `internal/backup`; demo sandboxes and limits in `internal/demo`; Bitrix24 bot and Telegram clients in `internal/core/`. One-off data tools (dry run first, then apply) live in `cmd/` next to the server.
- **Frontend** (`apps/web`): React 19, Vite 8, Mantine 9, TanStack Query 5, React Router 7; one folder per feature in `src/features/`. Pure logic is tested with Vitest.
- **Database**: PostgreSQL, goose migrations in `db/migrations/` (72 so far).
- **How it connects**: the web app calls `/api` on the same origin (Vite proxy locally, Pages Functions on the demo, the API process itself in production). AI analysis streams over SSE; comments poll every 5 seconds. Portal sync reads the CRM through a read-only inbound webhook. Previews are sandboxed iframes with no scripts.
- **One codebase, two deploys**: production and demo differ only by environment switches (`AUTH_MODE`, `AI_PROVIDER`, `WEB_DIST_DIR`, demo flags), never by forked code.
- **Architecture docs**: [backend](docs/Backend%20Architecture.md), [frontend](docs/Frontend%20Architecture.md).

## 4. Environments

- **Production**: a dedicated VM on VibeCode, the Bitrix24 app platform, with PostgreSQL 16 on the same machine (since 2026-11-09; before that Railway). The API serves the web app. Access only through the company portal's gateway; AI goes through the platform's model router; daily gzipped `pg_dump` to Vibe Storage, last 30 kept. Deploy with `make deploy`, logs with `make logs`. Details in [deploy/vibecode/README.md](deploy/vibecode/README.md).
- **Public demo**: [reviewdesk.rubashkin.xyz](https://reviewdesk.rubashkin.xyz/) (the old `demo.` subdomain redirects there). API and PostgreSQL on Railway, frontend on Cloudflare Pages, built from the public repo. No sign-up: each visitor gets an isolated copy of a synthetic board under a board-scoped user; sandboxes expire after 24 hours, are cleaned hourly and capped at 500. Entries, writes and AI calls are rate-limited per visitor. Visits are counted without storing visitors' IP addresses.
- **Local dev**: Docker PostgreSQL, `make setup-dev` (migrate and seed own templates, or the demo board), `make dev` (API on :8080, web on :5173), `AUTH_DEV_LOGIN_ENABLED=true` to sign in without a code. `make test` also runs the database tests when the local DB is up.
- **Private to public sync**: on every push to `main`, a GitHub workflow runs `scripts/public/sync.sh`. It exports the tree, drops private paths (`scripts/public/exclude.txt`), rewrites company names and assets to a neutral brand, swaps in public versions of a few files, stops if anything from the deny list is left, then opens and merges a PR in the public repo once its checks pass. The public history is squashed; each sync commit names the private PRs it carries.

## 5. Recent changes (since 2026-10-01)

About 200 commits. Grouped:

**Features**
- **Board model**: slot matrix and side panel, timeline view (now the default) with per-region branches and present mode, archive view, bulk actions, slot drag, rename a board, view and stage in the URL.
- **Regions and blocks**: regions with a reference and adaptations; a block library, block-based emails and conversion of existing boards to blocks; Markdown in texts; shared texts with board overrides.
- **Review workflow**: statuses simplified to Draft / Ready with checks of form; area approvals and the comment gate removed (2026-11-01); My tasks; translators take outdated translations; AI updates of outdated translations; translation glossary; apply reviewers' suggested wording from comments in bulk (with dry run).
- **CRM integration**: read-only portal sync (2026-10-09), nightly autosync, automatic Live, import of missing emails, uploads tracked per robot, first-send estimate, a Sends tab with charts and live counts on the board.
- **People**: display names, user merge and aliases, user deletion, shared mailboxes refused, board owners get admin rights on their boards, hint for likely duplicate users, read-only "view as" a user for CRM operators.
- **Help and onboarding**: in-app help by role, search, inline links, What's new, first-login tour.
- **Demo**: login-free entry, per-visitor sandbox, guided review loop, write and AI caps, visit tracking with returning-visitor and owner detection, link previews.
- **Notifications**: daily activity digest; Bitrix24 bot for the owner's messages and for mentions, with replies and Resolve from the chat.

**Infrastructure**
- Production moved to a dedicated VibeCode VM: gateway sign-in, platform AI router, daily database backups, tokens kept as application secrets, outbound email dropped there (2026-11-09 to 11-10).
- Public repo sync automated on every push to `main`; demo moved to `reviewdesk.rubashkin.xyz`.
- CI runs the database integration tests; Vitest added for frontend logic; `make start|build|test|deploy|logs` targets.
- Domain glossary, first ADR, agent skills and a `/wrap-up` routine.

**Fixes**
- Comments and editing on phones and in Safari; portal syncs cut off by a restart are closed; the digest is not sent twice when the bot answers late; partner names redacted in stored portal subjects; Bitrix expressions kept in rendered HTML; findings from a security review closed.

## 6. In progress / next

No open branches or PRs besides this one. Evidence is GitHub Issues.

- **Next**: #163 flag an email whose robots send different texts (`ready-for-agent`), then #155 tell robots apart by the deal's responsible person (`ready-for-agent`).
- **Needs triage**: #148 opens and clicks for live emails; #169 limit a region manager's edits to their region.
- **Demo**: #120 screenshots and a short video.
- **Frozen until the colleagues' trial** (milestone "After the colleagues' trial", 13 issues): experiments (#117), board-wide content search (#126), checks before Ready (#127), detecting edits made directly in the CRM (#129), send history per partner and date (#128), runbook for a colleague (#131), public API and webhooks (#123), analytics (#121), timeline polish (#119) and others.
- **Inferred**: a live check of bot replies and Resolve with a second person; #125 and #130 were designed around email and need rethinking now that production sends none.

## 7. Known limitations / tech debt

- **Bus factor of one**: one maintainer; hosting and service accounts are not yet handed over (#131).
- **No outbound email in production**: mentions reach only people with a linked Bitrix24 ID; others see them in My tasks only.
- **Portal sync** cannot yet tell apart several robots sending the same email (#155, #163); a deploy restarts the server and interrupts a running sync, which must be started again.
- **One-off `cmd/` tools** are not packaged for the production VM yet (inferred from the deploy package contents).
- **Legacy model fields**: `adaptation_key` and `variant = 'old'` remain for old copies; no new uses.
- **Roadmap items not started**: `sqlc` for high-risk queries, centralized session-expiry handling, structured AI logs and prompt versioning, confirmations before destructive board actions.
- **Realtime**: comments poll every 5 seconds; no live collaboration.
- **Demo limits** are in-memory counters that reset on restart.
- **Database tests** share state and must run one package at a time (`go test -p 1`).

## 8. Metrics and impact

Same figures as the [README](README.md#why-it-exists) and the [case study](docs/case-study.md#result):

- Team: 10 people (partner onboarding), rolled out from one pilot colleague to two, then the whole team.
- Emails: 100+ localized emails in a 5-language chain.
- Before: changing one email took about a week end to end.
- After: the team rebuilt the entire chain in one working week.
- Time saved per month: _TBD_
- Other: _TBD_

## Open questions for the maintainer

Could not be determined from the code and git history:

- The remaining metrics in section 8 (time saved per month, anything else worth showing).
- Whether restore from the VibeCode backups has been tested end to end.
- When the colleagues' trial ends, which unfreezes the milestone.
