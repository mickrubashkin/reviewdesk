-- +goose Up
-- People @mentioned in a comment message. Kept even for self-mentions, so
-- the message can highlight every name it mentions.
CREATE TABLE comment_mentions (
  message_id UUID NOT NULL REFERENCES comment_messages (id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  PRIMARY KEY (message_id, user_id)
);

CREATE INDEX comment_mentions_user_id_idx ON comment_mentions (user_id);

-- Something a person should see: shown in My tasks until read, and sent by
-- email. Each channel keeps its own state (read_at for the app,
-- email_status for email), so per-person notification settings can later
-- skip a channel without changing the table. kind is only 'mention' for now.
CREATE TABLE notifications (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('mention')),
  actor_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
  email_id UUID NOT NULL REFERENCES emails (id) ON DELETE CASCADE,
  comment_id UUID REFERENCES comments (id) ON DELETE CASCADE,
  message_id UUID REFERENCES comment_messages (id) ON DELETE CASCADE,
  excerpt TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  read_at TIMESTAMPTZ,
  email_status TEXT NOT NULL DEFAULT 'pending'
    CHECK (email_status IN ('pending', 'sent', 'failed', 'skipped')),
  emailed_at TIMESTAMPTZ
);

CREATE INDEX notifications_user_id_created_at_idx
  ON notifications (user_id, created_at DESC);

-- +goose Down
DROP TABLE notifications;
DROP TABLE comment_mentions;
