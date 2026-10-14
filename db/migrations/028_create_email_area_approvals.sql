-- +goose Up
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

INSERT INTO approval_areas (key, default_name)
VALUES
  ('product', 'Product'),
  ('brand', 'Brand'),
  ('legal', 'Legal'),
  ('sales', 'Sales'),
  ('partnerships', 'Partnerships'),
  ('localization', 'Localization'),
  ('crm-ops', 'CRM ops')
ON CONFLICT (key) DO NOTHING;

INSERT INTO board_approval_areas (
  board_id,
  approval_area_id,
  name,
  required,
  sort_order
)
SELECT
  boards.id,
  approval_areas.id,
  approval_areas.default_name,
  true,
  area_order.sort_order
FROM boards
CROSS JOIN (
  VALUES
    ('product', 10),
    ('brand', 20),
    ('legal', 30),
    ('sales', 40),
    ('partnerships', 50),
    ('localization', 60),
    ('crm-ops', 70)
) AS area_order(area_key, sort_order)
JOIN approval_areas ON approval_areas.key = area_order.area_key
ON CONFLICT (board_id, approval_area_id) DO NOTHING;

CREATE INDEX approval_areas_key_idx ON approval_areas (key);
CREATE INDEX board_approval_areas_board_id_idx ON board_approval_areas (board_id);
CREATE INDEX board_approval_areas_approval_area_id_idx ON board_approval_areas (approval_area_id);
CREATE INDEX email_area_approvals_email_id_idx ON email_area_approvals (email_id);
CREATE INDEX email_area_approvals_board_approval_area_id_idx ON email_area_approvals (board_approval_area_id);
CREATE INDEX email_area_approvals_status_idx ON email_area_approvals (status);

-- +goose Down
DROP TABLE email_area_approvals;
DROP TABLE board_approval_areas;
DROP TABLE approval_areas;
