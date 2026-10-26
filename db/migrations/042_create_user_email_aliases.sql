-- +goose Up
-- Extra work addresses of a person. Signing in with an alias opens the
-- account it belongs to, so one person no longer ends up with two users.
CREATE TABLE user_email_aliases (
  email TEXT PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_by_email TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX user_email_aliases_user_id_idx ON user_email_aliases (user_id);

-- +goose Down
DROP TABLE user_email_aliases;
