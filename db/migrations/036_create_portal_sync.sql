-- +goose Up
-- Read-only mirror of emails actually sent by Bitrix24 CRM robots, used to
-- check what really goes out against what the service holds.
CREATE TABLE portal_sync_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  board_key TEXT NOT NULL,
  category_id INTEGER NOT NULL,
  days INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'done', 'failed')),
  progress TEXT NOT NULL DEFAULT '',
  activities_seen INTEGER NOT NULL DEFAULT 0,
  emails_stored INTEGER NOT NULL DEFAULT 0,
  groups_count INTEGER NOT NULL DEFAULT 0,
  error TEXT,
  started_by_email TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at TIMESTAMPTZ
);

CREATE TABLE portal_stages (
  board_key TEXT NOT NULL,
  stage_id TEXT NOT NULL,
  name TEXT NOT NULL,
  sort INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (board_key, stage_id)
);

CREATE TABLE portal_sent_emails (
  activity_id BIGINT PRIMARY KEY,
  board_key TEXT NOT NULL,
  deal_id BIGINT NOT NULL,
  sent_at TIMESTAMPTZ NOT NULL,
  subject TEXT NOT NULL DEFAULT '',
  body_html TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  stage_id TEXT,
  synced_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX portal_sent_emails_board_fingerprint_idx ON portal_sent_emails (board_key, fingerprint);

-- One row per distinct body. Decisions survive re-syncs because rows are
-- upserted by (board_key, fingerprint).
CREATE TABLE portal_email_groups (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  board_key TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  language TEXT NOT NULL DEFAULT '',
  subject TEXT NOT NULL DEFAULT '',
  normalized_text TEXT NOT NULL DEFAULT '',
  sample_activity_id BIGINT NOT NULL,
  send_count INTEGER NOT NULL DEFAULT 0,
  first_sent_at TIMESTAMPTZ NOT NULL,
  last_sent_at TIMESTAMPTZ NOT NULL,
  stages JSONB NOT NULL DEFAULT '{}'::jsonb,
  match_email_id UUID REFERENCES emails (id) ON DELETE SET NULL,
  match_score REAL NOT NULL DEFAULT 0,
  match_status TEXT NOT NULL DEFAULT 'unmatched' CHECK (match_status IN ('matched', 'differs', 'unmatched')),
  decision TEXT CHECK (decision IN ('confirmed', 'ignored', 'created')),
  decision_email_id UUID REFERENCES emails (id) ON DELETE SET NULL,
  decided_by_email TEXT,
  decided_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (board_key, fingerprint)
);

-- +goose Down
DROP TABLE portal_email_groups;
DROP TABLE portal_sent_emails;
DROP TABLE portal_stages;
DROP TABLE portal_sync_runs;
