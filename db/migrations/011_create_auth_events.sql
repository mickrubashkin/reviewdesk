-- +goose Up
CREATE TABLE auth_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  email TEXT NOT NULL,
  event_type TEXT NOT NULL,
  success BOOLEAN NOT NULL,
  ip_address TEXT,
  user_agent TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX auth_events_created_at_idx ON auth_events (created_at DESC);
CREATE INDEX auth_events_email_idx ON auth_events (email);
CREATE INDEX auth_events_event_type_idx ON auth_events (event_type);

-- +goose Down
DROP TABLE auth_events;
