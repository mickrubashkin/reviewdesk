-- +goose Up
ALTER TABLE emails
ADD COLUMN slug TEXT;

UPDATE emails
SET
  slug = id::TEXT
WHERE
  slug IS NULL;

ALTER TABLE emails
ALTER COLUMN slug
SET NOT NULL;

ALTER TABLE emails
ADD CONSTRAINT emails_slug_unique UNIQUE (slug);

-- +goose Down
ALTER TABLE emails
DROP CONSTRAINT emails_slug_unique;

ALTER TABLE emails
DROP COLUMN slug;
