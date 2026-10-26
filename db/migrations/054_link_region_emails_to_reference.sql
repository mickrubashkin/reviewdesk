-- +goose Up
-- An email of a region adapts an email of the board's reference region: it
-- remembers which one and the version it last matched, like a translation
-- remembers its EN master. Unlike a translation it keeps its own blocks,
-- timing and links, so the link only tells what changed and what differs.
ALTER TABLE emails
  ADD COLUMN based_on UUID REFERENCES emails (id) ON DELETE SET NULL,
  ADD COLUMN based_on_version INTEGER,
  ADD COLUMN based_on_checked_at TIMESTAMPTZ,
  ADD COLUMN based_on_checked_by TEXT,
  ADD CONSTRAINT emails_translation_or_based_on CHECK (translation_of IS NULL OR based_on IS NULL);

CREATE INDEX emails_based_on_idx ON emails (based_on);

-- +goose Down
DROP INDEX emails_based_on_idx;
ALTER TABLE emails
  DROP CONSTRAINT emails_translation_or_based_on,
  DROP COLUMN based_on,
  DROP COLUMN based_on_version,
  DROP COLUMN based_on_checked_at,
  DROP COLUMN based_on_checked_by;
