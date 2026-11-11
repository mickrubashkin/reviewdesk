-- +goose Up
-- Who sent each email on the portal (#155): the CRM activity's responsible
-- user, fixed at send time. A robot sends for the deals of one manager, so
-- the sender tells robots apart once a person pairs the robot with them.
ALTER TABLE portal_sent_emails ADD COLUMN responsible_id BIGINT;

-- CRM portal users seen as senders. Their ids are the CRM portal's, not the
-- sign-in portal's (users.portal_user_id).
CREATE TABLE portal_users (
  user_id BIGINT PRIMARY KEY,
  name TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL DEFAULT '',
  synced_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A robot of a board (a region manager's name) paired with the CRM portal
-- user whose deals it sends to. Set by a person; only pairs here count.
CREATE TABLE portal_robot_users (
  board_key TEXT NOT NULL REFERENCES boards (key) ON UPDATE CASCADE ON DELETE CASCADE,
  robot TEXT NOT NULL CHECK (robot <> ''),
  portal_user_id BIGINT NOT NULL,
  set_by_email TEXT NOT NULL DEFAULT '',
  set_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (board_key, robot),
  UNIQUE (board_key, portal_user_id)
);

-- +goose Down
DROP TABLE portal_robot_users;
DROP TABLE portal_users;
ALTER TABLE portal_sent_emails DROP COLUMN responsible_id;
