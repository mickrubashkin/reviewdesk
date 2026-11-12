-- +goose Up
-- A region in the reference's language with no email of its own in a slot
-- sends the reference email there, from its managers' robots (#216). A region
-- in another language without the slot sends nothing there (the portal saw
-- no such sends). So a reference email's robots are the reference region's
-- managers and the managers of every region of its language with no active
-- email in the email's slot (board, stage, position). New robots start with
-- no mark.
CREATE OR REPLACE VIEW email_robots AS
SELECT e.id AS email_id, r.robot, r.sort
FROM emails e
LEFT JOIN board_regions br ON br.board_key = e.sequence AND br.key = e.region_key
CROSS JOIN LATERAL (
  SELECT DISTINCT ON (m.robot) m.robot, m.sort
  FROM (
    SELECT own.robot, own.sort
    FROM unnest(CASE WHEN cardinality(br.managers) > 0 THEN br.managers ELSE ARRAY[''] END) WITH ORDINALITY AS own(robot, sort)
    UNION ALL
    SELECT other.robot, 1000 + other_region.sort_order * 100 + other.ord
    FROM board_regions other_region
    CROSS JOIN LATERAL unnest(other_region.managers) WITH ORDINALITY AS other(robot, ord)
    WHERE br.reference
      AND other_region.board_key = br.board_key
      AND NOT other_region.reference
      AND other_region.language = br.language
      AND other.robot <> ''
      AND NOT EXISTS (
        SELECT 1 FROM emails own_email
        WHERE own_email.sequence = e.sequence
          AND own_email.stage IS NOT DISTINCT FROM e.stage
          AND own_email.sort_order = e.sort_order
          AND own_email.region_key = other_region.key
          AND own_email.archived_at IS NULL
      )
  ) m
  ORDER BY m.robot, m.sort
) r;

-- +goose Down
CREATE OR REPLACE VIEW email_robots AS
SELECT e.id AS email_id, r.robot, r.sort
FROM emails e
LEFT JOIN board_regions br ON br.board_key = e.sequence AND br.key = e.region_key
CROSS JOIN LATERAL (
  SELECT DISTINCT ON (m.robot) m.robot, m.sort
  FROM unnest(CASE WHEN cardinality(br.managers) > 0 THEN br.managers ELSE ARRAY[''] END) WITH ORDINALITY AS m(robot, sort)
  ORDER BY m.robot, m.sort
) r;
