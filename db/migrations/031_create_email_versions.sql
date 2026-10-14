-- +goose Up
CREATE TABLE email_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email_id UUID NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
  version_number INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  created_by_email TEXT NOT NULL,
  source TEXT NOT NULL,
  restored_from_version_id UUID REFERENCES email_versions(id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  subject TEXT,
  preheader TEXT,
  original_html TEXT NOT NULL,
  template_html TEXT NOT NULL,
  editable_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
  changed_field_count INTEGER NOT NULL DEFAULT 0,
  changed_metadata_count INTEGER NOT NULL DEFAULT 0,
  html_changed BOOLEAN NOT NULL DEFAULT false,
  CONSTRAINT email_versions_email_version_number_key UNIQUE (email_id, version_number),
  CONSTRAINT email_versions_source_check CHECK (source IN ('initial', 'manual', 'restore')),
  CONSTRAINT email_versions_changed_field_count_check CHECK (changed_field_count >= 0),
  CONSTRAINT email_versions_changed_metadata_count_check CHECK (changed_metadata_count >= 0)
);

CREATE INDEX email_versions_email_id_version_number_idx ON email_versions (email_id, version_number DESC);
CREATE INDEX email_versions_created_at_idx ON email_versions (created_at DESC);
CREATE INDEX email_versions_created_by_email_idx ON email_versions (created_by_email);

INSERT INTO email_versions (
  email_id,
  version_number,
  created_at,
  created_by_email,
  source,
  title,
  subject,
  preheader,
  original_html,
  template_html,
  editable_fields
)
SELECT
  id,
  1,
  coalesce(updated_at, created_at, now()),
  'system',
  'initial',
  title,
  subject,
  preheader,
  original_html,
  template_html,
  editable_fields
FROM emails
WHERE archived_at IS NULL;

-- +goose Down
DROP TABLE email_versions;
