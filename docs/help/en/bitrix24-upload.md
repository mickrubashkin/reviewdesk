# Paste into Bitrix24 and mark the upload

CRM operators only. ReviewDesk reads Bitrix24 but never writes to it: the HTML is pasted into the CRM robot by hand, and the upload is marked in ReviewDesk.

## What to upload

**My tasks → To upload to Bitrix24** lists the ready emails some robot does not have in the current version. The board filter for uploads shows the same per board.

## Upload an email

1. Open the email and choose **More → Copy for Bitrix24…**.
2. Copy the HTML and paste it into the robot in Bitrix24.
3. In **Details → Bitrix24**, mark it uploaded for that robot. On a board with regions every manager's robot is marked on its own; **re-upload (name)** shows whose robot lags behind.

After the upload the status line shows **Expected from …**: the robot sends only to deals that enter the stage after it was set up.

## Already sent, but not marked

**My tasks → Already sent by the portal** lists emails the portal sends in the current version that are not marked uploaded. Check and mark them.

## Mark live by hand

For an email the portal sync cannot see (sent some other way, e.g. from the admin panel), a CRM operator can mark it live by hand in **Details**. It shows as **Live (manual)** and no longer asks for an upload: no **to upload** or **re-upload**, and it leaves **My tasks → To upload to Bitrix24**. When you change such an email, paste the new version wherever it is sent from yourself.

## Portal sync

**Settings → Portal sync** imports what the robots actually sent. It runs every night when it is turned on for the board, or by hand. One sync per board runs at a time. Each send (its date, deal and stage, not the text) is kept for 180 days, for the send counts on the emails.

A sync by hand loads the last **Days back** days. To load a long period, such as the first 180 days of history, run it in chunks with **Until days ago**: 180 to 120, then 120 to 60, then 60 to 0. Each chunk is shorter, and a chunk that fails can be run again alone. A chunk only adds sends; the counts on the emails always cover everything kept.

## Sends of an email

**Details → Sends** shows how many times the portal sent the email each day, over the last 30, 90 or 180 days, in your time zone. Each color is one text the portal sent: when a re-upload changes the text, the new text gets its own color, so you can see when it started and whether the old one stopped (a robot that was not updated keeps sending the old text). A text is named by the day it was first sent. With more than three texts the older ones are shown together in gray. A CRM operator can open each text in **Portal sync**.

Under the chart:

- **Sent to N deals** sets the sends against the deals they went to. A deal can get the email more than once when it comes back to the stage or a robot repeats the email. When a deal got the same text twice within a day, a yellow note warns that two robots may be sending the email.
- **By deal stage** counts the sends by the stage the deal was in when the email went out. A stage marked **other stage** mostly sends the emails of another stage of the board: check the robot that sends this email from it. The mark appears only when the email also went out from a stage of its own, since an email sent only from one stage may just sit in a stage of the board named differently.
