-- +goose Up
-- One row per visitor of the public demo, kept after their sandbox is deleted
-- so the demo has a history of visits. actions holds what the visitor did
-- (email event counts by action, marked_ready, ai_calls); it is filled when
-- the sandbox goes, and is NULL while the sandbox is live.
CREATE TABLE demo_visits (
    visitor_email TEXT PRIMARY KEY,
    source TEXT NOT NULL DEFAULT '',
    mobile BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ,
    actions JSONB
);

CREATE INDEX demo_visits_created_at_idx ON demo_visits (created_at);

-- +goose Down
DROP TABLE demo_visits;
