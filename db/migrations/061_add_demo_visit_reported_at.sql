-- +goose Up
-- When the report of a demo visit went to the owner's chat. The visits
-- before this column are taken as reported, so turning the reports on does
-- not send the whole history.
ALTER TABLE demo_visits ADD COLUMN reported_at TIMESTAMPTZ;
UPDATE demo_visits SET reported_at = now();

-- +goose Down
ALTER TABLE demo_visits DROP COLUMN reported_at;
