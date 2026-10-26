-- +goose Up
-- Terms the AI pre-translation must keep or translate strictly, e.g.
-- "Partner Portal". translations maps a language (de, es, pl, br) to the
-- required wording; a language without one keeps the English term. Shared by
-- all boards.
CREATE TABLE translation_glossary (
  position INT PRIMARY KEY,
  term TEXT NOT NULL UNIQUE,
  translations JSONB NOT NULL DEFAULT '{}'::jsonb,
  note TEXT NOT NULL DEFAULT '',
  updated_by_email TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE translation_glossary;
