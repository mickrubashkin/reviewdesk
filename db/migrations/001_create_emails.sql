-- +goose Up
CREATE EXTENSION if NOT EXISTS pgcrypto;

CREATE TABLE emails (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sequence TEXT,
  title TEXT NOT NULL,
  subject TEXT,
  preheader TEXT,
  stage TEXT,
  sort_order INTEGER NOT NULL DEFAULT 0,
  language TEXT NOT NULL DEFAULT 'en',
  original_html TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE emails;
