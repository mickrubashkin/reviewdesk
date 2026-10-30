-- +goose Up
-- The date of the newest "What's new" entry a person has opened (YYYY-MM-DD,
-- '' for none); the app marks the account menu while a newer one exists.
ALTER TABLE users ADD COLUMN whats_new_seen TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN whats_new_seen;
