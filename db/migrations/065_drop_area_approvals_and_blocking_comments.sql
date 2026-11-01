-- +goose Up
-- The review status is Draft/Ready: area approvals and blocking comments no
-- longer gate anything. Their history stays in email_events.
DROP TABLE email_area_approvals;
DROP TABLE board_approval_areas;
DROP TABLE approval_areas;

UPDATE comments SET severity = 'issue' WHERE severity = 'blocking';

ALTER TABLE comments
  DROP CONSTRAINT comments_severity_check;

ALTER TABLE comments
  ADD CONSTRAINT comments_severity_check
  CHECK (severity IN ('suggestion', 'issue'));

-- +goose Down
-- The tables come back empty: the approvals dropped above are not restored.
ALTER TABLE comments
  DROP CONSTRAINT comments_severity_check;

ALTER TABLE comments
  ADD CONSTRAINT comments_severity_check
  CHECK (severity IN ('suggestion', 'issue', 'blocking'));

CREATE TABLE approval_areas (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key TEXT NOT NULL UNIQUE,
  default_name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at TIMESTAMPTZ
);

CREATE TABLE board_approval_areas (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  approval_area_id UUID NOT NULL REFERENCES approval_areas(id) ON DELETE RESTRICT,
  name TEXT NOT NULL,
  required BOOLEAN NOT NULL DEFAULT true,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at TIMESTAMPTZ,
  UNIQUE (board_id, approval_area_id)
);

CREATE TABLE email_area_approvals (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email_id UUID NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
  board_approval_area_id UUID NOT NULL REFERENCES board_approval_areas(id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
    'pending',
    'approved',
    'changes_requested',
    'stale'
  )),
  decided_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  decided_by_email TEXT,
  decision_note TEXT,
  content_snapshot_hash TEXT,
  decided_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (email_id, board_approval_area_id)
);

CREATE INDEX approval_areas_key_idx ON approval_areas (key);
CREATE INDEX board_approval_areas_board_id_idx ON board_approval_areas (board_id);
CREATE INDEX board_approval_areas_approval_area_id_idx ON board_approval_areas (approval_area_id);
CREATE INDEX email_area_approvals_email_id_idx ON email_area_approvals (email_id);
CREATE INDEX email_area_approvals_board_approval_area_id_idx ON email_area_approvals (board_approval_area_id);
CREATE INDEX email_area_approvals_status_idx ON email_area_approvals (status);
