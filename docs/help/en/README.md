# ReviewDesk help

ReviewDesk is the one place where a team sees which emails of a board go out, in which language and region, and where a change to the English email does not have to be copied by hand into every other version.

Find your role below, then use the task pages when you need them. Words you do not know are in the [glossary](glossary.md). The short tour from your first sign-in comes back with **Replay tour** in the account menu. A **?** next to a part of the app opens the matching part of this help, and [What's new](whats-new.md) lists recent changes. **Search help** at the top of the help (Ctrl+K, ⌘K on a Mac) finds a word on every page and opens the part it is in.

## Who are you?

| You are… | You… |
|---|---|
| **[Viewer](#viewer)** | look at the boards and their status, and comment now and then |
| **[Translator](translations.md)** | keep the translations in your languages in step with the EN email |
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

Everything a viewer does, plus working on the boards you manage (**Manages** in Settings → Users). There you can do what a board owner does, except start new boards and set up people: edit and mark emails ready, add slots, change **Board settings**, archive, export.

1. Start in **My tasks**: mentions, open comments on your emails, your drafts, and **Translations to update**.
2. [Edit the text of an email](editing.md), then [mark it ready](editing.md#mark-the-email-ready).
3. [Translations](translations.md) — when the EN email changes, its translations show **EN changed**; update only what changed.
4. [Regions and the reference](regions.md) — when the reference changes, your email shows **reference changed**; take what applies to your region.

When your email is ready, a CRM operator pastes it into Bitrix24. You will see **to upload**, then **Expected from …**, then **Live**.

Edit only your own region: ReviewDesk lets you edit every email of the board, so a wrong edit to another region or the reference changes their emails too.

### Board owner

Everything a region manager does, on every board, plus:

- **New → New board** — start a board of your own. A new board does not reach any CRM robot until a CRM operator pastes its emails, so you can try things freely.
- **Board settings** — the board's name, stages, regions and the reference. Renaming a board keeps its links working.
- **New → New slot**, moving slots in the [Matrix](board-views.md#matrix), archiving: [Board views](board-views.md).
- **Settings → Users** — add people, and set who works on which board and translates which language. See [Setting up people](#setting-up-people).

### CRM operator

Everything a board owner does, plus:

- [Paste into Bitrix24 and mark the upload](bitrix24-upload.md), and mark an email live by hand.
- **Settings → Portal sync** — what the robots actually send.
- **Settings → Content** — shared texts, blocks, Bitrix variables, translation glossary (with the suggested terms), AI rules.
- Editing the template HTML of an email.

## Setting up people

How the roles above are set in **Settings → Users**. Board owners add people and set **Manages** and **Translates**; only a CRM operator changes someone's role. The **Role** column shows the role of this help; **Change role** shows the role it comes from. The search above the table finds people by name, address, role, language or board.

| Role | Change role | Also set |
|---|---|---|
| Viewer | `reviewer` | — |
| Translator | `reviewer` | **Translates**: their languages |
| Region manager | `reviewer` | **Manages**: the board; **Translates**: their languages, so translations that are behind show in their **My tasks** |
| Board owner | `admin` | — |
| CRM operator | `super_admin` | — |

A translator has no region of their own: they edit emails in their languages on every board but do not mark them ready. Board owners and CRM operators manage every board; **Translates** only fills their **My tasks**.

### One person, two addresses

Someone may sign in with a second work address, for example on another regional domain. A CRM operator sees **Same person as …?** under a new user whose address has the same part before @ as an older user's, and the owner's chat gets a message about it. **Merge** folds the new user into the older one: their work moves over, and the new address opens the older account from then on. **Two people** removes the hint. Nothing is merged on its own, because the same name on another domain can be someone else.

To expect a second address before it signs in, open the user's **⋯** menu and choose **Add another address…**.

### See ReviewDesk as someone else

A CRM operator can check what a colleague gets: open their **⋯** menu in **Settings → Users** and choose **View as this user**. ReviewDesk then shows their boards, their **My tasks**, their mentions and the buttons their role has. Nothing can be changed while viewing: saving, commenting and marking are refused, and the colleague does not show as having been in. A bar at the bottom says whom you are viewing as; **Exit** brings you back to **Settings → Users**. Viewing as another CRM operator is not offered, and each start is kept in the **Auth events** log.

### Signing in from Bitrix24

Where ReviewDesk opens from the Bitrix24 menu, your Bitrix24 account signs you in: there is no code by email and no **Log out**. On your first visit ReviewDesk finds the email of your Bitrix24 account, which takes a few seconds, and opens the user with that address or one of its other addresses. Someone new from a work domain becomes a Viewer, as with a code. Anyone else sees **No access yet** until a CRM operator adds them in **Settings → Users** with the address of their Bitrix24 account, and the owner's chat gets a message about them.

There, mentions come as a message from the ReviewDesk bot in Bitrix24, not by email. For that ReviewDesk needs the person's Bitrix24 ID: it gets it on their first visit, and also looks it up every day by their address and other addresses. **Settings → Users** shows the ID under the person; when ReviewDesk could not find it, or found two people, a CRM operator sets it with **Set Bitrix24 ID…** in the user's **⋯** menu. The ID is the number in the address of their Bitrix24 profile. Without an ID, the mention is only in **My tasks**, or comes by email where ReviewDesk sends email. You can answer a mention, or resolve its thread, right in the bot's chat; see [Answer from the Bitrix24 chat](comments.md#answer-from-the-bitrix24-chat).

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
