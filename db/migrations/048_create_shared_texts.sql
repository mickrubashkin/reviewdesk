-- +goose Up
-- Texts many emails share, one per key and language: footer, privacy link,
-- logo, signature. An email field with the same key points at it with
-- "shared": true in editable_fields and keeps a copy of the value, so
-- rendering does not change. Editing a shared text writes a new version of
-- every email that points at it.
--
-- en_version is the EN text's version this translation was last checked
-- against; a translation is outdated when the EN version is newer.
CREATE TABLE shared_texts (
  key TEXT NOT NULL,
  language TEXT NOT NULL,
  type TEXT NOT NULL,
  value TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1,
  en_version INTEGER,
  updated_by_email TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (key, language),
  CONSTRAINT shared_texts_type_check CHECK (type IN ('text', 'url', 'image')),
  CONSTRAINT shared_texts_language_check CHECK (language = lower(language))
);

ALTER TABLE email_versions DROP CONSTRAINT email_versions_source_check;
ALTER TABLE email_versions ADD CONSTRAINT email_versions_source_check
  CHECK (source IN ('initial', 'manual', 'restore', 'shared_text'));

-- Like email_text_hash, but leaves out the fields that are shared in skip
-- (the translation's own fields): a shared text has its own translation, so
-- a change to it in EN does not make the email translation outdated.
-- +goose StatementBegin
CREATE FUNCTION email_text_hash(subject TEXT, preheader TEXT, fields JSONB, skip JSONB) RETURNS TEXT
LANGUAGE sql IMMUTABLE AS $$
  SELECT md5(
    coalesce(subject, '') || E'\x1f' || coalesce(preheader, '') || E'\x1f' ||
    coalesce((
      SELECT string_agg(f.key || '=' || coalesce(f.value ->> 'value', ''), E'\x1f' ORDER BY f.key)
      FROM jsonb_each(coalesce(fields, '{}'::jsonb)) AS f
      WHERE f.value ->> 'type' = 'text'
        AND coalesce(skip -> f.key ->> 'shared', 'false') <> 'true'
    ), '')
  );
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION email_text_hash(TEXT, TEXT, JSONB, JSONB);
ALTER TABLE email_versions DROP CONSTRAINT email_versions_source_check;
ALTER TABLE email_versions ADD CONSTRAINT email_versions_source_check
  CHECK (source IN ('initial', 'manual', 'restore'));
DROP TABLE shared_texts;
