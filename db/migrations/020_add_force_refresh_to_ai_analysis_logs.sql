-- +goose Up
ALTER TABLE ai_analysis_logs
ADD COLUMN force_refresh BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX ai_analysis_logs_refresh_limit_idx
ON ai_analysis_logs (email_id, user_id, created_at)
WHERE force_refresh = TRUE;

-- +goose Down
DROP INDEX ai_analysis_logs_refresh_limit_idx;

ALTER TABLE ai_analysis_logs
DROP COLUMN force_refresh;
