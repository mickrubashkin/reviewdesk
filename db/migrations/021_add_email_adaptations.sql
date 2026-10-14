-- +goose Up
ALTER TABLE emails
ADD COLUMN adaptation_key TEXT NOT NULL DEFAULT 'default',
ADD COLUMN adaptation_label TEXT NOT NULL DEFAULT 'Default';

CREATE UNIQUE INDEX emails_active_adaptation_unique
ON emails (
  coalesce(sequence, ''),
  coalesce(stage, ''),
  sort_order,
  language,
  variant,
  adaptation_key
)
WHERE archived_at IS NULL;

-- +goose Down
DROP INDEX emails_active_adaptation_unique;

ALTER TABLE emails
DROP COLUMN adaptation_label,
DROP COLUMN adaptation_key;
