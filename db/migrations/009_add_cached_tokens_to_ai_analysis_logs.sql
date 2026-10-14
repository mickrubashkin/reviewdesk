-- +goose Up
ALTER TABLE ai_analysis_logs
ADD COLUMN cached_tokens INTEGER;

-- +goose Down
ALTER TABLE ai_analysis_logs
DROP COLUMN cached_tokens;
