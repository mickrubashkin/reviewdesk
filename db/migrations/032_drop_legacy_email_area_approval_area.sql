-- +goose Up
ALTER TABLE email_area_approvals
DROP CONSTRAINT IF EXISTS email_area_approvals_area_check;

ALTER TABLE email_area_approvals
DROP CONSTRAINT IF EXISTS email_area_approvals_email_id_area_key;

ALTER TABLE email_area_approvals
DROP COLUMN IF EXISTS area;

-- +goose Down
ALTER TABLE email_area_approvals
ADD COLUMN IF NOT EXISTS area TEXT;

UPDATE email_area_approvals AS approvals
SET area = approval_areas.key
FROM board_approval_areas
JOIN approval_areas ON approval_areas.id = board_approval_areas.approval_area_id
WHERE approvals.board_approval_area_id = board_approval_areas.id
  AND approvals.area IS NULL;

ALTER TABLE email_area_approvals
ALTER COLUMN area SET NOT NULL;

ALTER TABLE email_area_approvals
ADD CONSTRAINT email_area_approvals_area_check
CHECK (area IN (
  'product',
  'brand',
  'legal',
  'sales',
  'partnerships',
  'localization',
  'crm-ops'
));

ALTER TABLE email_area_approvals
ADD CONSTRAINT email_area_approvals_email_id_area_key
UNIQUE (email_id, area);
