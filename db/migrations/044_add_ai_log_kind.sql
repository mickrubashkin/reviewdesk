-- +goose Up
-- What an AI call was for: an email review ("analysis") or a translation
-- suggestion ("translation"), so the logs and their costs can be told apart.
ALTER TABLE ai_analysis_logs ADD COLUMN kind TEXT NOT NULL DEFAULT 'analysis';

-- +goose Down
ALTER TABLE ai_analysis_logs DROP COLUMN kind;
