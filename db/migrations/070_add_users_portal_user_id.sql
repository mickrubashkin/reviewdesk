-- +goose Up
-- On VibeCode the gateway signs people in and passes their Bitrix24 user ID
-- (X-Vibe-User-Id). A user gets it the first time they come in that way,
-- linked by their email in Bitrix24.
ALTER TABLE users ADD COLUMN portal_user_id TEXT UNIQUE;

-- +goose Down
ALTER TABLE users DROP COLUMN portal_user_id;
