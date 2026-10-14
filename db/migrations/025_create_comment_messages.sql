-- +goose Up
CREATE TABLE comment_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  comment_id UUID NOT NULL REFERENCES comments (id) ON DELETE CASCADE,
  user_id UUID REFERENCES users (id) ON DELETE SET NULL,
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX comment_messages_comment_id_created_at_idx
  ON comment_messages (comment_id, created_at);

INSERT INTO comment_messages (
  comment_id,
  user_id,
  body,
  created_at,
  updated_at
)
SELECT
  id,
  user_id,
  body,
  created_at,
  created_at
FROM comments;

-- +goose Down
DROP TABLE comment_messages;
