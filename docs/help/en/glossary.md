# Glossary

**Board** — one email sequence, e.g. the onboarding of new partners. Has stages, slots and, optionally, regions.

**Stage** — a stage of a deal in the CRM (e.g. *Registered*, *Activation*). A deal enters a stage, and the stage's emails go out to it one after another.

**Slot** — one place in a stage: "the second email of *Activation*, 3 days after the deal enters it". A slot holds the same email in every language. Its name, position, timing and send condition are shared by all languages.

**EN master** — the English email of a slot. EN is always the source of the content; other languages translate it.

**Translation** — an email in another language linked to the EN email it came from. When the EN email changes after the translation, the translation shows **EN changed**.

**Region** — a group of partners with their own managers and their own Bitrix24 robots (e.g. a country). A region differs from the others in a few things: currency, links to contracts and rules, sometimes timing.

**Reference** — the region whose chain the other regions adapt. A region's email can be *based on* the reference email; when the reference changes, it shows **reference changed**.

**Version** — every saved change to an email makes a new version (v1, v2, …). The history is in **History**; an old version can be restored.

**Version label (`current` / `old`)** — `old` is the previous variant of an email that may still be sent by some robots. Do not assume `old` is dead: the status line tells whether it is still sent.

**Draft / Ready** — the review status. *Draft* is being edited; *Ready* is final and can go to Bitrix24.

**Live** — robots send this email: the nightly portal sync saw it sent, or a CRM operator marked it live by hand.

**Robot** — the CRM automation rule in Bitrix24 that sends an email when a deal reaches a stage. The email's HTML is pasted into the robot by hand. On a board with regions every manager of a region can have their own robot.

**Upload** — pasting the HTML of a ready email into a robot. ReviewDesk does not write to Bitrix24; a CRM operator marks the upload in ReviewDesk.

**Portal sync** — a read-only import of what the robots actually sent, once a night or by hand. It is how ReviewDesk knows what is live.

**Block** — a reusable part of an email (header, button, footer). An email built from blocks changes when the block changes.

**Shared text** — a text used by many emails (e.g. the legal footer). A region can have its own version of it.

**Bitrix variable** — a placeholder that Bitrix24 fills in when it sends the email, e.g. the partner's name.

**Blocking comment** — a comment that must be resolved before the email can be marked ready. A **suggestion** does not block.

**Approval area** — a part of the review someone has to approve (e.g. *Compliance*), set per board. An edit makes the approval stale.

> ReviewDesk talks about "CRM robots" in general, but the portal sync, robots and variables are built for Bitrix24 and read it only. Other CRMs are not supported.
