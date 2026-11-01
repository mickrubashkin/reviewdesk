# Glossary

**Board** — the emails that deals receive as they move through the stages of a CRM pipeline, e.g. the onboarding of new partners. Has stages, slots and, optionally, regions.

**Stage** — a stage of a deal in the CRM (e.g. *Registered*, *Activation*). A deal enters a stage, and the stage's emails go out to it one after another.

**Slot** — one place in a stage: "the second email of *Activation*, 3 days after the deal enters it". A slot holds the same email in every language. Its name, position, timing and send condition are shared by all languages.

**EN master** — the English email of a slot on a board without regions: the source of the content; other languages translate it. On a board with regions the reference is the source instead.

**Translation** — an email in another language linked to the EN email it came from. When the EN email changes after the translation, the translation is **behind** and shows **EN changed**.

**Region** — a group of partners with their own managers and their own Bitrix24 robots (e.g. a country). A region differs from the others in a few things: currency, links to contracts and rules, sometimes timing.

**Reference** — the region whose emails the other regions adapt. A region's email can be *based on* the reference email; when the reference changes, it is **behind** and shows **reference changed**.

**Version** — every saved change to an email makes a new version (v1, v2, …). The history is in **History**; an old version can be restored.

**Uploaded version** — the version whose HTML was last pasted into a given robot. A robot whose uploaded version is older than the current one is **behind** and needs a re-upload.

**Behind** — not caught up with a newer state: an email whose source (EN master or reference) changed since it last matched it, or a robot with an older version than the current one.

**Draft / Ready** — the review status, set by people. *Draft* is being edited; *Ready* is final and can go to Bitrix24. It says nothing about what robots send.

**Live** — robots send this email: the nightly portal sync saw it sent, or a CRM operator marked it live by hand. Live is seen, not set: it is not a step after *Ready*.

**Robot** — the CRM automation rule in Bitrix24 that sends an email when a deal reaches a stage. The email's HTML is pasted into the robot by hand. On a board with regions every manager of a region can have their own robot.

**Upload** — pasting the HTML of a ready email into a robot. ReviewDesk does not write to Bitrix24; a CRM operator marks the upload in ReviewDesk.

**Portal sync** — a read-only import of what the robots actually sent, once a night or by hand. It is how ReviewDesk knows what is live.

**Block** — a reusable part of an email (header, button, footer). An email built from blocks changes when the block changes.

**Shared text** — a text used by many emails (e.g. the legal footer). A region can have its own version of it.

**Bitrix variable** — a placeholder that Bitrix24 fills in when it sends the email, e.g. the partner's name.

**Issue / Suggestion** — the two kinds of comment: an issue is something to fix, a suggestion is nice to have. Neither stops the email from being marked ready.

**Experiment** — a deliberate, temporary difference from the reference in some regions, made to test whether it works better (content, timing or which emails go out). It has its own emails, adapted by each of its regions. It ends one of three ways: *adopt* (its emails become the reference), *roll back* (its regions return to the reference) or *keep* (its regions keep the difference for good).

> ReviewDesk talks about "CRM robots" in general, but the portal sync, robots and variables are built for Bitrix24 and read it only. Other CRMs are not supported.
