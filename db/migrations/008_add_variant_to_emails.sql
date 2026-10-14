-- +goose Up
ALTER TABLE emails ADD COLUMN variant TEXT NOT NULL DEFAULT 'new';

-- +goose Down
ALTER TABLE emails DROP COLUMN variant;
