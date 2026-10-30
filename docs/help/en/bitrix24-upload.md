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

For an email the portal sync cannot see (sent some other way), a CRM operator can mark it live by hand in **Details**. It shows as **Live (manual)**.

## Portal sync

**Settings → Portal sync** imports what the robots actually sent. It runs every night when it is turned on for the board, or by hand. One sync per board runs at a time.
