-- +goose Up
-- A super admin can confirm by hand that an email is live, e.g. one sent
-- from the partner admin panel rather than a CRM robot, which the portal
-- sync cannot see.
ALTER TABLE emails
  ADD COLUMN live_marked_at TIMESTAMPTZ,
  ADD COLUMN live_marked_by TEXT,
  ADD COLUMN live_note TEXT;

-- +goose Down
ALTER TABLE emails
  DROP COLUMN live_marked_at,
  DROP COLUMN live_marked_by,
  DROP COLUMN live_note;
