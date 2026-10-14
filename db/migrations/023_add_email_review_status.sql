-- +goose Up
ALTER TABLE emails
ADD COLUMN review_status TEXT NOT NULL DEFAULT 'in_review',
ADD CONSTRAINT emails_review_status_check
CHECK (review_status IN ('draft', 'in_review', 'changes_requested', 'approved'));

-- +goose Down
ALTER TABLE emails
DROP CONSTRAINT emails_review_status_check,
DROP COLUMN review_status;
