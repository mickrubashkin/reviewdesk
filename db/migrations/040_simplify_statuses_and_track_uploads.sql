-- +goose Up
-- Statuses become draft → ready; "live" comes from the portal or a manual
-- mark. The old review states collapse: anything not approved is a draft.
ALTER TABLE emails DROP CONSTRAINT IF EXISTS emails_review_status_check;
UPDATE emails SET review_status = 'ready' WHERE review_status IN ('approved', 'production_approved');
UPDATE emails SET review_status = 'draft' WHERE review_status <> 'ready';
ALTER TABLE emails
  ALTER COLUMN review_status SET DEFAULT 'draft',
  ADD CONSTRAINT emails_review_status_check CHECK (review_status IN ('draft', 'ready'));

-- The version a super admin pasted into the Bitrix24 robot. When the email
-- changes after that, Bitrix still has the older version.
ALTER TABLE emails
  ADD COLUMN uploaded_version INT,
  ADD COLUMN uploaded_at TIMESTAMPTZ,
  ADD COLUMN uploaded_by_email TEXT;

-- Emails already live are taken to be in Bitrix as they are now.
UPDATE emails e
SET uploaded_version = v.version_number,
    uploaded_at = now(),
    uploaded_by_email = 'migration'
FROM (
  SELECT email_id, max(version_number) AS version_number FROM email_versions GROUP BY email_id
) v
WHERE v.email_id = e.id
  AND e.archived_at IS NULL
  AND (
    e.live_marked_at IS NOT NULL
    OR EXISTS (
      SELECT 1 FROM portal_email_groups g
      WHERE (g.decision IN ('confirmed', 'created') AND g.decision_email_id = e.id)
         OR (g.decision IS NULL AND g.match_status = 'matched' AND g.match_email_id = e.id)
    )
  );

-- +goose Down
ALTER TABLE emails
  DROP COLUMN uploaded_by_email,
  DROP COLUMN uploaded_at,
  DROP COLUMN uploaded_version;
ALTER TABLE emails DROP CONSTRAINT IF EXISTS emails_review_status_check;
UPDATE emails SET review_status = CASE review_status WHEN 'ready' THEN 'approved' ELSE 'in_review' END;
ALTER TABLE emails
  ALTER COLUMN review_status SET DEFAULT 'in_review',
  ADD CONSTRAINT emails_review_status_check CHECK (review_status IN ('draft', 'in_review', 'changes_requested', 'approved', 'production_approved'));
