# ReviewDesk help

ReviewDesk is the one place where a team sees which emails of a board go out, in which language and region, and where a change to the English email does not have to be copied by hand into every other version.

Find your role below, then use the task pages when you need them. Words you do not know are in the [glossary](glossary.md). The short tour from your first sign-in comes back with **Replay tour** in the account menu. A **?** next to a part of the app opens the matching part of this help, and [What's new](whats-new.md) lists recent changes. **Search help** at the top of the help (Ctrl+K, ⌘K on a Mac) finds a word on every page and opens the part it is in.

## Who are you?

| You are… | You… |
|---|---|
| **[Viewer](#viewer)** | look at the boards and their status, and comment now and then |
| **[Region manager](#region-manager)** | keep your region's emails and translations up to date and mark them ready |
| **[Board owner](#board-owner)** | set up boards: stages, slots, regions, who works on what |
| **[CRM operator](#crm-operator)** | paste ready emails into the Bitrix24 robots and keep ReviewDesk in step with what is sent |

### Viewer

You can open every board and every email, and comment on any of them.

1. [Board views](board-views.md) — start with **Timeline**: the whole board at a glance.
2. [Find an email and read its status](find-an-email.md) — what goes out now and what is being changed.
3. [Comment and mention a colleague](comments.md) — select the words in the email and click **Comment**.

**My tasks** (in the header) shows where someone mentioned you.

### Region manager

Everything a viewer does, plus editing the emails of your region.

1. Start in **My tasks**: mentions, open comments on your emails, your drafts, and **Translations to update**.
2. [Edit the text of an email](editing.md), then [mark it ready](editing.md#mark-the-email-ready).
3. [Translations](translations.md) — when the EN email changes, its translations show **EN changed**; update only what changed.
4. [Regions and the reference](regions.md) — when the reference changes, your email shows **reference changed**; take what applies to your region.

When your email is ready, a CRM operator pastes it into Bitrix24. You will see **to upload**, then **Expected from …**, then **Live**.

Edit only your own region: ReviewDesk lets you edit every email of the board, so a wrong edit to another region or the reference changes their emails too.

### Board owner

Everything a region manager does, on every board, plus:

- **New → New board** — start a board of your own. A new board does not reach any CRM robot until a CRM operator pastes its emails, so you can try things freely.
- **Board settings** — stages, regions and the reference, approval areas.
- **New → New slot**, moving slots in the [Matrix](board-views.md#matrix), archiving: [Board views](board-views.md).
- **Settings → Users** — add people, and set who works on which board and translates which language. See [Setting up people](#setting-up-people).

### CRM operator

Everything a board owner does, plus:

- [Paste into Bitrix24 and mark the upload](bitrix24-upload.md), and mark an email live by hand.
- **Settings → Portal sync** — what the robots actually send.
- **Settings → Content** — shared texts, blocks, Bitrix variables, translation glossary, AI rules.
- Editing the template HTML of an email.

## Setting up people

How the roles above are set in **Settings → Users**. Board owners add people and set **Manages** and **Translates**; only a CRM operator changes someone's role.

| Role | Role in Users | Also set |
|---|---|---|
| Viewer | `reviewer` | — |
| Region manager | `reviewer` | **Manages**: the board; **Translates**: their languages, so translations that are behind show in their **My tasks** |
| Board owner | `admin` | — |
| CRM operator | `super_admin` | — |

Someone who only translates (no region of their own) is a `reviewer` with **Translates** set: they edit emails in those languages but do not mark them ready.

## All task pages

- [Find an email and read its status](find-an-email.md)
- [Comment and mention a colleague](comments.md)
- [Edit the text of an email](editing.md)
- [Translations](translations.md)
- [Regions and the reference](regions.md)
- [Paste into Bitrix24 and mark the upload](bitrix24-upload.md)
- [Export](export.md)
- [Board views](board-views.md)
- [Glossary](glossary.md)
