-- +goose Up
-- AI translation calls run in the background (#199): the request starts a
-- job and answers at once, so a slow model does not hit the gateway's time
-- limit. 'create' fills a missing language from the EN email (email_id is
-- the master); 'suggest' updates a translation after EN changed (email_id is
-- the translation). The client polls the job.
CREATE TABLE translation_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  kind TEXT NOT NULL CHECK (kind IN ('create', 'suggest')),
  email_id UUID NOT NULL REFERENCES emails (id) ON DELETE CASCADE,
  language TEXT NOT NULL,
  -- The board, to list its jobs on the matrix.
  sequence TEXT NOT NULL DEFAULT '',
  -- For 'suggest', the EN version the suggestion follows; a newer EN
  -- version makes it outdated.
  master_version INT,
  status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'done', 'failed')),
  result JSONB,
  error TEXT NOT NULL DEFAULT '',
  created_email_id UUID REFERENCES emails (id) ON DELETE SET NULL,
  started_by UUID REFERENCES users (id) ON DELETE SET NULL,
  started_by_email TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at TIMESTAMPTZ,
  -- Set when the person closes a failed job or a suggestion.
  dismissed_at TIMESTAMPTZ
);

-- One running job per email, kind and language: a second click joins it.
CREATE UNIQUE INDEX translation_jobs_one_running ON translation_jobs (kind, email_id, language) WHERE status = 'running';
CREATE INDEX translation_jobs_board ON translation_jobs (sequence, started_at DESC);
CREATE INDEX translation_jobs_email ON translation_jobs (email_id, started_at DESC);

-- +goose Down
DROP TABLE translation_jobs;
