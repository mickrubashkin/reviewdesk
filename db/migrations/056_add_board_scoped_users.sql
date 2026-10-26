-- +goose Up
-- A board-scoped user sees only the boards they manage: each visitor of the
-- public demo works on their own copy of the demo board, out of sight of the
-- other visitors.
ALTER TABLE users ADD COLUMN board_scoped BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE users DROP COLUMN board_scoped;
