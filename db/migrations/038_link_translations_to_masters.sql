-- +goose Up
-- A translation points at the EN email it was translated from and at the
-- master version it matched when it was last brought up to date. The
-- translation is outdated when the master's text differs from that version.
ALTER TABLE emails
  ADD COLUMN translation_of UUID REFERENCES emails (id) ON DELETE SET NULL,
  ADD COLUMN translated_from_version INTEGER,
  ADD COLUMN translation_checked_at TIMESTAMPTZ,
  ADD COLUMN translation_checked_by TEXT;

CREATE INDEX emails_translation_of_idx ON emails (translation_of);

-- Text a translator works from: subject, preheader and the text fields.
-- URLs, images and widths are left out on purpose: changing them in EN does
-- not make a translation outdated.
-- +goose StatementBegin
CREATE FUNCTION email_text_hash(subject TEXT, preheader TEXT, fields JSONB) RETURNS TEXT
LANGUAGE sql IMMUTABLE AS $$
  SELECT md5(
    coalesce(subject, '') || E'\x1f' || coalesce(preheader, '') || E'\x1f' ||
    coalesce((
      SELECT string_agg(f.key || '=' || coalesce(f.value ->> 'value', ''), E'\x1f' ORDER BY f.key)
      FROM jsonb_each(coalesce(fields, '{}'::jsonb)) AS f
      WHERE f.value ->> 'type' = 'text'
    ), '')
  );
$$;
-- +goose StatementEnd

-- Every active email gets at least one version, so translations always have
-- a master version to point at (emails duplicated before versions existed
-- had none).
INSERT INTO email_versions (email_id, version_number, created_by_email, source, title, subject, preheader,
  original_html, template_html, editable_fields)
SELECT e.id, 1, 'system', 'initial', e.title, e.subject, e.preheader, e.original_html, e.template_html, e.editable_fields
FROM emails e
WHERE e.archived_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM email_versions v WHERE v.email_id = e.id);

-- Link existing translations: same slot, version and adaptation as an EN
-- email. They are assumed up to date with the master's current version.
UPDATE emails t
SET translation_of = m.id,
  translated_from_version = (SELECT max(v.version_number) FROM email_versions v WHERE v.email_id = m.id)
FROM emails m
WHERE t.language <> 'en'
  AND t.archived_at IS NULL
  AND m.language = 'en'
  AND m.archived_at IS NULL
  AND m.sequence = t.sequence
  AND m.stage = t.stage
  AND m.sort_order = t.sort_order
  AND m.variant = t.variant
  AND m.adaptation_key = t.adaptation_key;

-- +goose Down
DROP INDEX emails_translation_of_idx;
ALTER TABLE emails
  DROP COLUMN translation_of,
  DROP COLUMN translated_from_version,
  DROP COLUMN translation_checked_at,
  DROP COLUMN translation_checked_by;
DROP FUNCTION email_text_hash(TEXT, TEXT, JSONB);
