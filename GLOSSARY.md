# ReviewDesk

ReviewDesk is where a team sees which emails of a CRM sequence go out, in which language and region, and keeps every version of an email in step with the one it comes from.

## Structure

**Board**:
One sequence of emails that deals receive as they move through the stages of a CRM pipeline.
_Avoid_: Sequence, chain

**Stage**:
A stage of a deal in the CRM; when a deal enters it, the stage's emails go out one after another.
_Avoid_: Step, phase

**Slot**:
One place in a stage, such as "the second email of Activation", filled by every language and region of that email. A region may lack some slots or have extra ones, but the slots it shares with the reference keep the same order; the timing is shared by the languages, and a region may wait longer or shorter than the reference.
_Avoid_: Position, email (for the place itself)

## Sources

Every email that is not itself a source has exactly one source: a translation follows its EN master, an adaptation follows its reference email or the email of the experiment its region takes part in. Never two.

**EN master**:
The English email of a slot on a board without regions; the source of the content for every other language of that slot.
_Avoid_: Original, source email

**Translation**:
An email in another language that follows an EN master and is expected to say the same thing.
_Avoid_: Localization, language version

**Region**:
A market with its own managers and CRM robots, written in one language; several regions can share a language.
_Avoid_: Market, country, locale

**Reference**:
The one region of a board whose emails the other regions adapt; on a board with regions it is the source of the content, in place of an EN master.
_Avoid_: Master region, main region

**Adaptation**:
A region's email that follows the source email of the same slot (the reference's, or an experiment's), possibly in another language, and differs from it only where the region differs.
_Avoid_: Based-on email, override, copy, manager copy, branch

## Versions and sending

**Version**:
One saved state of an email (v1, v2, …); every saved change makes a new one.
_Avoid_: Revision, variant, old/new, version label

**Draft / Ready**:
The review status of an email, set by people: a draft is being worked on, a ready email is final and may be uploaded. It says nothing about what robots send.
_Avoid_: In review, approved, live (as a status step)

**CRM operator**:
A person who pastes emails into robots and keeps ReviewDesk in step with what the CRM sends.
_Avoid_: Super admin, uploader

**Robot**:
A CRM automation rule that sends an email when a deal reaches a stage; on a board with regions each manager of a region has their own.
_Avoid_: Trigger, automation

**Uploaded version**:
The version of an email whose HTML was last pasted into a given robot; a robot is behind when it is older than the current version.
_Avoid_: Old variant, live version

**Upload**:
Pasting a version of an email into a robot, marked by a CRM operator; from then on the email is expected from that robot, for deals that enter the stage afterwards.
_Avoid_: Publish, deploy, mark live

**Portal sync**:
A read-only import of what the robots actually sent.
_Avoid_: Bitrix sync, import

**Live**:
Robots are seen sending the email: the portal sync saw a send, or, for an email the sync cannot see, a CRM operator marked it live by hand.
_Avoid_: Uploaded, published, sent (as a status)

**Behind**:
Not caught up with a newer state: an email whose source changed after it was last checked against it, or a robot whose uploaded version is older than the email's current version.
_Avoid_: Outdated, stale, out of sync

## Experiments

**Experiment**:
A deliberate, temporary difference from the reference in some regions, made to test whether it works better; it may change content, timing or the set of slots. It has its own source emails, written once and adapted by each of its regions, so that every region tests the same thing.
_Avoid_: Branch, variation, new chain

**Adopt**:
The outcome of an experiment where its emails become the reference for the whole board.

**Roll back**:
The outcome of an experiment where its regions return to the reference.

**Keep**:
The outcome of an experiment where its regions keep the difference for good, as part of their adaptation.
