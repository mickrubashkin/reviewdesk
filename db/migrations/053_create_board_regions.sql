-- +goose Up
-- A region of a board is a version of its content: the reference chain and
-- the markets whose managers adapt it, each manager with a robot in
-- Bitrix24. An email of the board belongs to one region.
CREATE TABLE board_regions (
    board_key TEXT NOT NULL REFERENCES boards (key) ON UPDATE CASCADE ON DELETE CASCADE,
    key TEXT NOT NULL,
    name TEXT NOT NULL,
    language TEXT NOT NULL,
    -- One robot each in Bitrix24.
    managers TEXT[] NOT NULL DEFAULT '{}',
    -- The reference chain the other regions adapt; one per board.
    reference BOOLEAN NOT NULL DEFAULT false,
    sort_order INTEGER NOT NULL DEFAULT 0,
    updated_by_email TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (board_key, key),
    CHECK (key ~ '^[a-z0-9-]+$'),
    CHECK (language = lower(language))
);

CREATE UNIQUE INDEX board_regions_one_reference ON board_regions (board_key) WHERE reference;

ALTER TABLE emails ADD COLUMN region_key TEXT;
ALTER TABLE emails ADD CONSTRAINT emails_region_fkey
    FOREIGN KEY (sequence, region_key) REFERENCES board_regions (board_key, key)
    ON DELETE SET NULL (region_key);

-- +goose Down
ALTER TABLE emails DROP CONSTRAINT emails_region_fkey;
ALTER TABLE emails DROP COLUMN region_key;
DROP TABLE board_regions;
