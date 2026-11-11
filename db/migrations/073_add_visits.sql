-- +goose Up
-- "Changed since your visit" (#205). A visit lasts while someone keeps coming
-- back within 30 minutes; previous_* keeps when the visit before began, so a
-- page opened twice in one visit still compares with the last visit, not
-- with a minute ago.
CREATE TABLE email_views (
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  email_id UUID NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
  viewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  previous_viewed_at TIMESTAMPTZ,
  PRIMARY KEY (user_id, email_id)
);

-- changes_tracked_since is when marks started for the person: changes before
-- it are not "new", so their first visit shows none.
ALTER TABLE users
  ADD COLUMN changes_tracked_since TIMESTAMPTZ,
  ADD COLUMN my_tasks_seen_at TIMESTAMPTZ,
  ADD COLUMN my_tasks_previous_seen_at TIMESTAMPTZ;

-- The email list looks up each email's latest edit and owner change.
CREATE INDEX email_events_email_id_created_at_idx ON email_events (email_id, created_at DESC);

-- +goose Down
DROP INDEX email_events_email_id_created_at_idx;
ALTER TABLE users
  DROP COLUMN my_tasks_previous_seen_at,
  DROP COLUMN my_tasks_seen_at,
  DROP COLUMN changes_tracked_since;
DROP TABLE email_views;
