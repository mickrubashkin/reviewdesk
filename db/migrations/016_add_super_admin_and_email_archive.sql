-- +goose Up
ALTER TABLE users
DROP CONSTRAINT users_role_check;

ALTER TABLE users
ADD CONSTRAINT users_role_check CHECK (role IN ('super_admin', 'admin', 'reviewer'));

ALTER TABLE emails
ADD COLUMN archived_at TIMESTAMPTZ,
ADD COLUMN archived_by UUID REFERENCES users(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE emails
DROP COLUMN archived_by,
DROP COLUMN archived_at;

UPDATE users
SET role = 'admin'
WHERE role = 'super_admin';

ALTER TABLE users
DROP CONSTRAINT users_role_check;

ALTER TABLE users
ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'reviewer'));
