-- +goose Up
-- Boards a person manages: on these boards they can do everything an admin
-- can (slots, bulk actions, archive, editing); other boards stay read-only.
-- Admins and super admins manage every board regardless of this list.
ALTER TABLE users ADD COLUMN managed_boards TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE users DROP COLUMN managed_boards;
