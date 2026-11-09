-- +goose Up
-- On VibeCode a notification goes to the person's Bitrix24 chat through the
-- bot when ReviewDesk knows their Bitrix24 ID; the email is sent only when
-- that is not possible. bitrix_status is that channel's outcome, as
-- email_status is the email's.
ALTER TABLE notifications
  ADD COLUMN bitrix_status TEXT NOT NULL DEFAULT 'skipped'
    CHECK (bitrix_status IN ('sent', 'failed', 'skipped')),
  ADD COLUMN bitrix_sent_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE notifications DROP COLUMN bitrix_sent_at, DROP COLUMN bitrix_status;
