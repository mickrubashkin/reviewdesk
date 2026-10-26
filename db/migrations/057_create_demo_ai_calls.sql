-- +goose Up
-- OpenAI calls made by visitors of the public demo, for the per-visitor and
-- demo-wide limits. Not tied to emails or users, so the hourly sandbox cleanup
-- does not erase calls that still count toward today's demo-wide limit.
CREATE TABLE demo_ai_calls (
    id BIGSERIAL PRIMARY KEY,
    visitor_email TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX demo_ai_calls_created_at_idx ON demo_ai_calls (created_at);

-- +goose Down
DROP TABLE demo_ai_calls;
