# Find an email and read its status

## Find it

- Pick the board in the header, then use **Search** in **Filters**: it matches the title, subject, preheader, stage, language and status.
- **Filters** narrow the board by language, region, version (`current` / `old`: `old` is an earlier text of the email that some robots may still send), review status, whether it is live, whether it needs an upload, and open comments. **Clear filters** brings everything back.
- **My tasks** (header) lists only what waits for you.
- A link to an email or to a stage opens the board right there: use **Copy link** in the email's **More** menu or on a timeline stage.

Click an email to open it. The review page shows the email in the middle, the language pills and the version / region selectors above it, and the side panel with **Comments**, **AI**, **Details** and **History**.

## Read the status line

Every email has one status line, read left to right: what goes out now, what is being edited, what someone has to do. Hover a part to see a sentence about it.

| You see | It means |
|---|---|
| **Live v3 · 120/30d** | Robots send it, 120 times in the last 30 days; Bitrix24 has version 3. Hover it for the last send. |
| **Live v3 · quiet 9d** (yellow) | It used to go out, but the portal saw no send for 9 days, far longer than its usual gap. Check that the robot is still on. In the Matrix view, the chart icon next to it opens its sends. |
| **Live (manual)** | A CRM operator marked it live; the portal does not see it sent. |
| **v4 draft** | Version 4 is being edited. |
| **v4 ready** | Version 4 is final and can go to Bitrix24. |
| **to upload** | Ready, but no robot has it yet. |
| **re-upload** | Ready, and a robot has an older version. A name in brackets says whose robot lags behind. |
| **Expected from 12 Nov** | Just pasted into a robot. A robot sends only to deals that enter the stage after it was set up, so the first send cannot come earlier. |
| **No send since 12 Nov** | It should have been sent by now. Maybe no deal reached the stage, the robot's condition did not hold or the robot is not set up. Check it in Bitrix24. |
| **Other version live** | This one is not sent; another version of the same slot is. |
| **Not sent** | The portal sync has not seen the robots send it. |
| **EN changed** | The English email changed after this translation. See [Translations](translations.md). |
| **reference changed** | The reference region's email changed after this one matched it. See [Regions](regions.md). |
| **changed** (blue) | Someone else edited its text since you last opened it. See [What changed since your last visit](#what-changed-since-your-last-visit). |

The review status itself (**Draft** / **Ready**) is the badge in the header of the review page.

## What changed since your last visit

ReviewDesk remembers when you open an email and when you open **My tasks**. Coming back within 30 minutes is the same visit.

- On the board, an email someone else edited since you last opened it shows **changed** in its status line.
- On the review page, a note above the email lists the fields others changed since your last visit, and who; the changed blocks are outlined in blue. **See in History** opens the edits.
- In **My tasks**, items that came up since your last visit there are marked **New** and listed first in their section: someone else edited the email, made you its owner, or, for a translation, edited its EN. The **My tasks** button shows how many are new.

Only edits of the text and fields count; comments, status and owner changes by others do not. Your own edits never do. Marks start the first time you open ReviewDesk with this feature, so on that visit there are none.
