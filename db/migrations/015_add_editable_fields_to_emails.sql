-- +goose Up
ALTER TABLE emails
ADD COLUMN template_html TEXT,
ADD COLUMN editable_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
ADD COLUMN template_hash TEXT,
ADD COLUMN template_version TEXT;

UPDATE emails
SET
  template_html = original_html
WHERE
  template_html IS NULL;

ALTER TABLE emails
ALTER COLUMN template_html
SET NOT NULL;

-- +goose Down
ALTER TABLE emails
DROP COLUMN template_version,
template_hash,
editable_fields,
template_html;
