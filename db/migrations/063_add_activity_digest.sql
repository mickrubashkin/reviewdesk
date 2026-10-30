-- +goose Up
-- When each person was seen: a row each time users.last_seen_at moves (at
-- most every ten minutes), for the daily activity digest, which deletes the
-- rows older than 90 days.
CREATE TABLE user_presence (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX user_presence_seen_at_idx ON user_presence (seen_at);

-- The days whose activity digest went to the owner's chat, so a restart or a
-- second instance does not send one twice.
CREATE TABLE activity_digests (
    day DATE PRIMARY KEY,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE activity_digests;
DROP TABLE user_presence;
