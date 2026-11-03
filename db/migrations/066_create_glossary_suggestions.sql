-- +goose Up
-- A term someone proposes for the translation glossary, often from the text
-- of an email they work on. A super admin accepts it into
-- translation_glossary or rejects it. language and translation are empty
-- when the suggestion is only the English term.
CREATE TABLE glossary_suggestions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  term TEXT NOT NULL,
  language TEXT NOT NULL DEFAULT '',
  translation TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  email_id UUID REFERENCES emails(id) ON DELETE SET NULL,
  suggested_by_user_id UUID REFERENCES users(id) ON DELETE CASCADE,
  suggested_by_email TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected')),
  decided_by_email TEXT,
  decided_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX glossary_suggestions_pending_idx ON glossary_suggestions (created_at) WHERE status = 'pending';
CREATE INDEX glossary_suggestions_suggested_by_idx ON glossary_suggestions (suggested_by_user_id);

-- +goose Down
DROP TABLE glossary_suggestions;
