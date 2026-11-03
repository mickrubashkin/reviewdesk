-- +goose Up
-- Sends are kept for months now (see SendRetentionDays) and counted per email
-- over the last 30 days; the date in the index lets that count read only the
-- index.
CREATE INDEX portal_sent_emails_board_fingerprint_sent_idx ON portal_sent_emails (board_key, fingerprint, sent_at);
DROP INDEX portal_sent_emails_board_fingerprint_idx;

-- +goose Down
CREATE INDEX portal_sent_emails_board_fingerprint_idx ON portal_sent_emails (board_key, fingerprint);
DROP INDEX portal_sent_emails_board_fingerprint_sent_idx;
