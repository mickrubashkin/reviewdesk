-- +goose Up
ALTER TABLE emails
ADD COLUMN body_text TEXT;

-- +goose Down
ALTER TABLE emails
DROP COLUMN body_text;
