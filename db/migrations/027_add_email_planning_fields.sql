-- +goose Up
ALTER TABLE emails
ADD COLUMN owner_email TEXT,
ADD COLUMN reviewer_email TEXT,
ADD COLUMN due_date DATE,
ADD COLUMN implementation_notes TEXT;

-- +goose Down
ALTER TABLE emails
DROP COLUMN implementation_notes,
DROP COLUMN due_date,
DROP COLUMN reviewer_email,
DROP COLUMN owner_email;
