-- +goose Up
CREATE TABLE operational_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  level TEXT NOT NULL CHECK (level IN ('info', 'warn', 'error')),
  event_type TEXT NOT NULL,
  message TEXT NOT NULL,
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  user_email TEXT,
  request_id TEXT,
  method TEXT,
  path TEXT,
  status_code INTEGER,
  duration_ms INTEGER,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX operational_events_created_at_idx ON operational_events (created_at DESC);
CREATE INDEX operational_events_level_idx ON operational_events (level);
CREATE INDEX operational_events_event_type_idx ON operational_events (event_type);
CREATE INDEX operational_events_request_id_idx ON operational_events (request_id);
CREATE INDEX operational_events_user_email_idx ON operational_events (user_email);

-- +goose Down
DROP TABLE operational_events;
