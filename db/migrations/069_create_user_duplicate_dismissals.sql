-- +goose Up
-- Two users whose addresses share the part before @ are shown as a possible
-- duplicate until a super admin merges them or says they are two people.
-- user_a is the smaller id, so a pair is stored once.
CREATE TABLE user_duplicate_dismissals (
  user_a UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  user_b UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_by_email TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_a, user_b),
  CHECK (user_a < user_b)
);

CREATE INDEX user_duplicate_dismissals_user_b_idx ON user_duplicate_dismissals (user_b);

-- +goose Down
DROP TABLE user_duplicate_dismissals;
