-- +goose Up
-- A manual sync can load a range, from days to until_days ago (0 = up to now),
-- so a long backfill runs in short chunks.
ALTER TABLE portal_sync_runs ADD COLUMN until_days INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE portal_sync_runs DROP COLUMN until_days;
