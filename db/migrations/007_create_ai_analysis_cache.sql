-- +goose Up
CREATE TABLE ai_analysis_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email_id UUID NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
  model TEXT NOT NULL,
  response_language TEXT NOT NULL,
  prompt_hash TEXT NOT NULL,
  analysis JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (email_id, model, response_language, prompt_hash)
);

-- +goose Down
DROP TABLE ai_analysis_cache;
