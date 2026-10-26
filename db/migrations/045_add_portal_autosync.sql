-- +goose Up
-- At most one running sync per board, enforced by the database so a manual
-- run, the nightly run and a second server during a deploy cannot overlap.
-- Runs left "running" by a stopped server are closed first.
UPDATE portal_sync_runs r
SET status = 'failed', error = 'interrupted', finished_at = now()
WHERE r.status = 'running'
  AND (
    r.started_at < now() - interval '35 minutes'
    OR EXISTS (
      SELECT 1 FROM portal_sync_runs newer
      WHERE newer.board_key = r.board_key AND newer.status = 'running' AND newer.started_at > r.started_at
    )
  );

CREATE UNIQUE INDEX portal_sync_runs_one_running_idx ON portal_sync_runs (board_key) WHERE status = 'running';

-- Per-board nightly sync: whether it runs, from which Bitrix24 funnel and how
-- many days back. Boards without a row are not synced automatically.
CREATE TABLE portal_sync_settings (
  board_key TEXT PRIMARY KEY REFERENCES boards (key) ON UPDATE CASCADE ON DELETE CASCADE,
  auto_sync BOOLEAN NOT NULL DEFAULT false,
  category_id INTEGER,
  days INTEGER NOT NULL DEFAULT 3 CHECK (days BETWEEN 1 AND 365),
  updated_by_email TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (NOT auto_sync OR category_id IS NOT NULL)
);

-- +goose Down
DROP TABLE portal_sync_settings;
DROP INDEX portal_sync_runs_one_running_idx;
