-- +goose Up
ALTER TABLE comments
  ADD COLUMN severity TEXT NOT NULL DEFAULT 'issue';

ALTER TABLE comments
  ADD CONSTRAINT comments_severity_check
  CHECK (severity IN ('suggestion', 'issue', 'blocking'));

-- +goose Down
ALTER TABLE comments
  DROP CONSTRAINT comments_severity_check;

ALTER TABLE comments
  DROP COLUMN severity;
