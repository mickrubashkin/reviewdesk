-- +goose Up
ALTER TABLE emails
ADD COLUMN content_parts JSONB;

-- +goose Down
ALTER TABLE emails
DROP COLUMN content_parts;
