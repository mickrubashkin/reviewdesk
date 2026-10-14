-- +goose Up
ALTER TABLE users
ADD COLUMN last_seen_at TIMESTAMPTZ;

CREATE INDEX users_last_seen_at_idx ON users (last_seen_at DESC);

-- +goose Down
DROP INDEX users_last_seen_at_idx;

ALTER TABLE users
DROP COLUMN last_seen_at;
