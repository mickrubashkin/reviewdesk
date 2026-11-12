# What's new

The newest changes first. A dot on your account menu means there is something here you have not read yet.

## 2026-11-12

- **Translate and create** and **Suggest translation** no longer fail on long emails: AI now translates in the background. The matrix cell shows **translating…** until the draft appears, and a suggestion waits for you even if you leave the page. See [Translations](translations.md).
- A reference email now counts the robots of the regions in its language that have no email of their own in that slot: they send the reference email there. **re-upload (name)** asks for their robots too, and **Details → Region** lists them. See [Robots of a region](regions.md#robots-of-a-region).

## 2026-11-11

- In **Settings → Portal sync**, CRM operators can pair each robot with the manager it sends for. The portal's sends then show which robot has which version: **Already sent by the portal** names the robot and marks just it. See [Robots and senders](bitrix24-upload.md#robots-and-senders).
- ReviewDesk shows what changed since your last visit: **changed** in the status line of an email someone else edited, a note on the review page listing the changed fields with the blocks outlined in blue, and **New** on items in **My tasks**, with their count on its button. See [What changed since your last visit](find-an-email.md#what-changed-since-your-last-visit).
- A translation whose EN email is still a draft with open comments says **EN is still a draft** above its preview, and waits greyed at the end of **My tasks → Translations to update** without being counted: EN may change again, so translate it once EN is settled. See [Update a translation after EN changed](translations.md#update-a-translation-after-en-changed).
- The review page of a translation has a **Translation** tab: what changed in EN, with **Suggest translation** and **Mark up to date**, as in the board's preview panel. When EN has changed, the page opens on it, so there is no need to go back to the board to update a translation. See [Update a translation after EN changed](translations.md#update-a-translation-after-en-changed).
- CRM operators can see ReviewDesk as a colleague sees it, read-only: **View as this user** in the user's **⋯** menu in **Settings → Users**. See [See ReviewDesk as someone else](README.md#see-reviewdesk-as-someone-else).

## 2026-11-10

- **Settings → Users** has a search: type a name, an address, a role (**Translator**, **Region manager**…), a language or a board. See [Setting up people](README.md#setting-up-people).
- A board's managers can make a translator the owner of an email in their language: **Details → Owner** now lists the translators of that language too, and the email lands in their **My tasks**. See [Hand a translation to a translator](translations.md#hand-a-translation-to-a-translator).
- An email whose robots send different texts (one manager's robot still has an older text) gets a red **2 texts** mark in the Matrix and Timeline views and on its review page; click it for the sends. CRM operators find these emails in **My tasks → Robots disagree**. See [Robots that send different texts](bitrix24-upload.md#robots-that-send-different-texts).
- On the timeline, the **+** under a stage's name adds a slot or an email to that stage. See [New slot, new email](board-views.md#new-slot-new-email-region-managers-board-owners).
- An email made from pasted HTML, or from one the portal has seen sent, keeps its bold, italics and links when you save another field or translate it. Its links keep their color.
- Where mentions come from the ReviewDesk bot in Bitrix24, reply to the bot's message to answer in the comment's thread, and press **Resolve** under it to resolve the thread. See [Answer from the Bitrix24 chat](comments.md#answer-from-the-bitrix24-chat).

## 2026-11-09

- A board can be renamed in **Board settings**; links to it keep working.

## 2026-11-08

- **Settings → Users** asks **Same person as …?** under a new user whose address has the same part before @ as someone already there, often a colleague on another regional domain. A CRM operator merges them in one click or marks them as two people. See [One person, two addresses](README.md#one-person-two-addresses).

## 2026-11-03

- **Settings → Users** shows each person's role as this help names it: Viewer, Translator, Region manager, Board owner or CRM operator. See [Setting up people](README.md#setting-up-people).
- In the Matrix view, hover **+1** in a cell to see the sends of each version in it; it reads **+1 live** in green when an old version still goes out next to the new one. See [Matrix](board-views.md#matrix).
- The **Sends** tab of an email, on the review page and in the board's side panel, charts its sends per day over the last 30, 90 or 180 days, one color per text the portal sent: after a re-upload you see when the new text started going out and whether the old one stopped. See [Sends of an email](bitrix24-upload.md#sends-of-an-email).
- Under the chart on the **Sends** tab: how many deals got the email, a warning when a deal got the same text twice in a day (two robots may be sending it), and the sends by deal stage, with stages that mostly send another stage's emails marked **other stage**. See [Sends of an email](bitrix24-upload.md#sends-of-an-email).
- **Versions of this slot** on the **Sends** tab compares the email with the other versions of its slot over the same period (old and new, or each region against the reference), to see whether an old text is still going out. See [Sends of an email](bitrix24-upload.md#sends-of-an-email).
- The **Sends** tab says when the portal sync last ran, in yellow when it is old, so a quiet chart is not mistaken for a quiet robot. The **Live** badge on the review page, the sends count on the timeline and a chart icon next to a quiet email in the matrix open the tab. See [Sends of an email](bitrix24-upload.md#sends-of-an-email).
- A live email shows how often it went out: **Live v3 · 120/30d** is 120 sends in the last 30 days, and its tooltip says when it was last sent. One that stopped going out turns yellow (**quiet 9d**). See [Find an email](find-an-email.md).
- On the timeline, a live email shows its sends of the last 30 days in the corner of its preview and when it was last sent under its subject. See [Board views](board-views.md#timeline-default).
- **Create from EN** in the Matrix view is always shown in an empty cell, not only when you point at the row. See [Translations](translations.md).
- Everyone can read the **translation glossary** (account menu) and suggest terms: from the glossary page, or by selecting a word in an email and clicking the glossary icon. A CRM operator accepts or rejects them. See [Translation glossary](translations.md#translation-glossary).
- An email marked **Live (manual)** no longer shows **to upload** or sits in **My tasks → To upload to Bitrix24**. See [Mark live by hand](bitrix24-upload.md#mark-live-by-hand).

## 2026-11-01

- A comment is an **Issue** or a **Suggestion**; **Blocking** is gone, and blocking comments became issues. Open comments no longer stop an email from being marked ready. See [Comments](comments.md).

## 2026-10-31

- New words, the same things: the role that sets up boards is now **Board owner** (was Sequence owner); emails whose EN master or reference changed are **behind** (the timeline's **Highlight → Behind**, was Outdated); the export is **Board table** (was Chain table). The [glossary](glossary.md) has them all, plus **Uploaded version** and **Experiment**.

## 2026-10-30

- **Search help** at the top of the help, or Ctrl+K (⌘K on a Mac) there: finds a word on every help page and opens the part it is in.
- **Present** on the timeline: the sequence without statuses, comments or filters, to show it to someone. Click an email to see it large. See [Board views](board-views.md#timeline-default).

## 2026-10-29

- **?** next to the harder parts of the app opens the matching part of this help in a new tab: the status line, the reference of a region's email, the region field and the timeline.
- **What's new** — this page — in the account menu.

## 2026-10-28

- **Help** in the account menu: what each role does and how to do each task.
- A short tour of the board header on your first sign-in. **Replay tour** in the account menu brings it back.

## 2026-10-27

- **Timeline** is the default board view: stages along a trunk, each stage's emails branching off it, one branch per region on a board with regions. See [Board views](board-views.md#timeline-default).
- **Highlight** picks out the emails that need something; **Copy link** on a stage gives a link that opens the board at that stage.
