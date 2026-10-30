-- +goose Up
-- The version of the first-login tour a person last finished or skipped; the
-- app shows the tour again when its version is higher.
ALTER TABLE users ADD COLUMN tour_version INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE users DROP COLUMN tour_version;
