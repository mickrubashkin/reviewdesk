-- +goose Up
ALTER TABLE emails
ADD COLUMN review_html TEXT;

-- +goose Down
ALTER TABLE emails
DROP COLUMN review_html;
