-- +goose Up
CREATE TABLE email_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  actor_email TEXT NOT NULL,
  action TEXT NOT NULL,
  email_id UUID REFERENCES emails(id) ON DELETE SET NULL,
  email_slug TEXT,
  email_title TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  changes JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX email_events_created_at_idx ON email_events (created_at DESC);
CREATE INDEX email_events_actor_email_idx ON email_events (actor_email);
CREATE INDEX email_events_action_idx ON email_events (action);
CREATE INDEX email_events_email_title_idx ON email_events (email_title);
CREATE INDEX email_events_email_slug_idx ON email_events (email_slug);

-- +goose Down
DROP TABLE email_events;
