-- +goose Up
-- A reply to the bot's mention message in Bitrix24 goes to the comment's
-- thread (#188). notifications.bitrix_message_id is the mention's message in
-- the person's chat, which a quoted reply points to;
-- comment_messages.bitrix_message_id is the chat message a reply came from,
-- so an event delivered twice is posted once.
ALTER TABLE notifications ADD COLUMN bitrix_message_id TEXT;
CREATE INDEX notifications_bitrix_message_idx
  ON notifications (user_id, bitrix_message_id)
  WHERE bitrix_message_id IS NOT NULL;
ALTER TABLE comment_messages ADD COLUMN bitrix_message_id TEXT UNIQUE;

-- +goose Down
ALTER TABLE comment_messages DROP COLUMN bitrix_message_id;
DROP INDEX notifications_bitrix_message_idx;
ALTER TABLE notifications DROP COLUMN bitrix_message_id;
