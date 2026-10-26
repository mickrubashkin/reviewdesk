-- +goose Up
-- Each manager of a region has a robot in Bitrix24, so an email goes into
-- several robots and each one is pasted on its own. The upload mark moves
-- from the email to its robots: a robot is a manager of the email's region,
-- or a single unnamed robot when the region lists no managers.
CREATE TABLE email_robot_uploads (
  email_id UUID NOT NULL REFERENCES emails (id) ON DELETE CASCADE,
  robot TEXT NOT NULL,
  version INTEGER NOT NULL,
  uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  uploaded_by_email TEXT NOT NULL,
  PRIMARY KEY (email_id, robot)
);

-- The robots an email goes into, in the region's order.
CREATE VIEW email_robots AS
SELECT e.id AS email_id, r.robot, r.sort
FROM emails e
LEFT JOIN board_regions br ON br.board_key = e.sequence AND br.key = e.region_key
CROSS JOIN LATERAL (
  SELECT DISTINCT ON (m.robot) m.robot, m.sort
  FROM unnest(CASE WHEN cardinality(br.managers) > 0 THEN br.managers ELSE ARRAY[''] END) WITH ORDINALITY AS m(robot, sort)
  ORDER BY m.robot, m.sort
) r;

-- Per email: each robot with its mark, and the oldest version a robot has.
-- Marks of robots no longer in the region are kept but not counted.
CREATE VIEW email_upload_summary AS
SELECT r.email_id,
  json_agg(json_build_object(
    'robot', r.robot,
    'version', u.version,
    'uploaded_at', u.uploaded_at,
    'uploaded_by_email', u.uploaded_by_email
  ) ORDER BY r.sort) AS robots,
  min(u.version) AS uploaded_version,
  min(u.uploaded_at) AS uploaded_at
FROM email_robots r
LEFT JOIN email_robot_uploads u ON u.email_id = r.email_id AND u.robot = r.robot
GROUP BY r.email_id;

-- A mark made before robots were told apart stands for every robot of the
-- email's region: the status the board shows stays as it was.
INSERT INTO email_robot_uploads (email_id, robot, version, uploaded_at, uploaded_by_email)
SELECT e.id, r.robot, e.uploaded_version, COALESCE(e.uploaded_at, now()), COALESCE(e.uploaded_by_email, '')
FROM emails e
JOIN email_robots r ON r.email_id = e.id
WHERE e.uploaded_version IS NOT NULL;

ALTER TABLE emails
  DROP COLUMN uploaded_version,
  DROP COLUMN uploaded_at,
  DROP COLUMN uploaded_by_email;

-- +goose Down
ALTER TABLE emails
  ADD COLUMN uploaded_version INT,
  ADD COLUMN uploaded_at TIMESTAMPTZ,
  ADD COLUMN uploaded_by_email TEXT;

UPDATE emails e
SET uploaded_version = s.uploaded_version,
    uploaded_at = s.uploaded_at,
    uploaded_by_email = (
      SELECT u.uploaded_by_email FROM email_robot_uploads u
      WHERE u.email_id = e.id ORDER BY u.version, u.uploaded_at LIMIT 1
    )
FROM email_upload_summary s
WHERE s.email_id = e.id AND s.uploaded_version IS NOT NULL;

DROP VIEW email_upload_summary;
DROP VIEW email_robots;
DROP TABLE email_robot_uploads;
