-- +goose Up
CREATE TABLE board_ai_configs (
  board_id UUID PRIMARY KEY REFERENCES boards (id) ON DELETE CASCADE,
  instruction TEXT NOT NULL DEFAULT '',
  sequence_context TEXT NOT NULL DEFAULT '',
  rules JSONB NOT NULL DEFAULT '[]'::jsonb,
  roles JSONB NOT NULL DEFAULT '[]'::jsonb,
  updated_by_email TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE board_ai_configs;
