-- +goose Up
CREATE TABLE IF NOT EXISTS approval_areas (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key TEXT NOT NULL UNIQUE,
  default_name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS board_approval_areas (
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

ALTER TABLE email_area_approvals
ADD COLUMN IF NOT EXISTS board_approval_area_id UUID REFERENCES board_approval_areas(id) ON DELETE RESTRICT;

-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_name = 'email_area_approvals'
      AND column_name = 'area'
  ) THEN
    UPDATE email_area_approvals AS approvals
    SET board_approval_area_id = board_approval_areas.id
    FROM emails, boards, approval_areas, board_approval_areas
    WHERE approvals.email_id = emails.id
      AND boards.key = coalesce(nullif(trim(emails.sequence), ''), 'onboarding')
      AND approval_areas.key = approvals.area
      AND board_approval_areas.board_id = boards.id
      AND board_approval_areas.approval_area_id = approval_areas.id
      AND approvals.board_approval_area_id IS NULL;
  END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE email_area_approvals
ALTER COLUMN board_approval_area_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS email_area_approvals_email_board_area_idx
ON email_area_approvals (email_id, board_approval_area_id);

CREATE INDEX IF NOT EXISTS approval_areas_key_idx ON approval_areas (key);
CREATE INDEX IF NOT EXISTS board_approval_areas_board_id_idx ON board_approval_areas (board_id);
CREATE INDEX IF NOT EXISTS board_approval_areas_approval_area_id_idx ON board_approval_areas (approval_area_id);
CREATE INDEX IF NOT EXISTS email_area_approvals_board_approval_area_id_idx ON email_area_approvals (board_approval_area_id);

-- +goose Down
DROP INDEX IF EXISTS email_area_approvals_email_board_area_idx;
ALTER TABLE email_area_approvals
DROP COLUMN IF EXISTS board_approval_area_id;
DROP TABLE IF EXISTS board_approval_areas;
DROP TABLE IF EXISTS approval_areas;
