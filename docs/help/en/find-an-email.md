# Find an email and read its status

## Find it

- Pick the board in the header, then use **Search** in **Filters**: it matches the title, subject, preheader, stage, language and status.
- **Filters** narrow the board by language, region, version (`current` / `old`), review status, whether it is live, whether it needs an upload, and open comments. **Clear filters** brings everything back.
- **My tasks** (header) lists only what waits for you.
- A link to an email or to a stage opens the board right there: use **Copy link** in the email's **More** menu or on a timeline stage.

Click an email to open it. The review page shows the email in the middle, the language pills and the version / region selectors above it, and the side panel with **Comments**, **AI**, **Details** and **History**.

## Read the status line

Every email has one status line, read left to right: what goes out now, what is being edited, what someone has to do. Hover a part to see a sentence about it.

| You see | It means |
|---|---|
| **Live v3** | Robots send it; Bitrix24 has version 3. |
| **Live (manual)** | A CRM operator marked it live; the portal does not see it sent. |
| **v4 draft** | Version 4 is being edited. |
| **v4 ready** | Version 4 is final and can go to Bitrix24. |
| **to upload** | Ready, but no robot has it yet. |
| **re-upload** | Ready, and a robot has an older version. A name in brackets says whose robot lags behind. |
| **Expected from 12 Nov** | Just pasted into a robot. A robot sends only to deals that enter the stage after it was set up, so the first send cannot come earlier. |
| **No send since 12 Nov** | It should have been sent by now. Maybe no deal reached the stage, the robot's condition did not hold or the robot is not set up. Check it in Bitrix24. |
| **Other version live** | This one is not sent; another version of the same slot is. |
| **Not sent** | The robots did not send it in the synced period. |
| **EN changed** | The English email changed after this translation. See [Translations](translations.md). |
| **reference changed** | The reference region's email changed after this one matched it. See [Regions](regions.md). |

The review status itself (**Draft** / **Ready**) is the badge in the header of the review page.
