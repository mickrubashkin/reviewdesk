-- +goose Up
-- Free-text condition the CRM robot checks before sending, e.g.
-- "documents not uploaded". Complements send_timing (the delay).
ALTER TABLE emails ADD COLUMN send_condition TEXT;

-- +goose Down
ALTER TABLE emails DROP COLUMN send_condition;
