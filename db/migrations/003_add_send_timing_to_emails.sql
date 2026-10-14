-- +goose Up
ALTER TABLE emails
ADD COLUMN send_timing TEXT;

-- +goose Down
ALTER TABLE emails
DROP COLUMN send_timing;
