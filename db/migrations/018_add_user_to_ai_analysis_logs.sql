-- +goose Up
ALTER TABLE ai_analysis_logs
ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE SET NULL,
ADD COLUMN user_email TEXT;

CREATE INDEX ai_analysis_logs_user_email_idx ON ai_analysis_logs (user_email);

-- +goose Down
DROP INDEX ai_analysis_logs_user_email_idx;

ALTER TABLE ai_analysis_logs
DROP COLUMN user_email,
DROP COLUMN user_id;
