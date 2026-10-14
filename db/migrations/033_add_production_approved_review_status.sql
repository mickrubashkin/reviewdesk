-- +goose Up
ALTER TABLE emails
DROP CONSTRAINT IF EXISTS emails_review_status_check;

ALTER TABLE emails
ADD CONSTRAINT emails_review_status_check
CHECK (review_status IN (
  'draft',
  'in_review',
  'changes_requested',
  'approved',
  'production_approved'
));

-- +goose Down
UPDATE emails
SET review_status = 'approved'
WHERE review_status = 'production_approved';

ALTER TABLE emails
DROP CONSTRAINT IF EXISTS emails_review_status_check;

ALTER TABLE emails
ADD CONSTRAINT emails_review_status_check
CHECK (review_status IN (
  'draft',
  'in_review',
  'changes_requested',
  'approved'
));
