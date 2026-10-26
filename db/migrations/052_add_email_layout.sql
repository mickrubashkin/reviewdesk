-- +goose Up
-- An email can be built from the block library: layout lists its blocks in
-- order ([{"block": "intro_body"}, {"block": "intro_body", "suffix": "2"}]).
-- The API assembles template_html (and original_html) from the frame and the
-- blocks whenever the email or a block changes, so rendering, comments and
-- the portal comparison work as for any other email. NULL is the old format:
-- the email's own template.
--
-- A version keeps the layout it had. 'block' is a version written because a
-- block of the library changed.
ALTER TABLE emails ADD COLUMN layout JSONB;
ALTER TABLE email_versions ADD COLUMN layout JSONB;

CREATE INDEX emails_layout_idx ON emails USING gin (layout jsonb_path_ops) WHERE layout IS NOT NULL;

ALTER TABLE email_versions DROP CONSTRAINT email_versions_source_check;
ALTER TABLE email_versions ADD CONSTRAINT email_versions_source_check
  CHECK (source IN ('initial', 'manual', 'restore', 'shared_text', 'block'));

-- +goose Down
ALTER TABLE email_versions DROP CONSTRAINT email_versions_source_check;
UPDATE email_versions SET source = 'manual' WHERE source = 'block';
ALTER TABLE email_versions ADD CONSTRAINT email_versions_source_check
  CHECK (source IN ('initial', 'manual', 'restore', 'shared_text'));
DROP INDEX emails_layout_idx;
ALTER TABLE email_versions DROP COLUMN layout;
ALTER TABLE emails DROP COLUMN layout;
