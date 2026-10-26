-- +goose Up
-- A person's display name; the admin users page groups their addresses
-- under it. Empty means "show the email".
ALTER TABLE users ADD COLUMN name TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN name;
