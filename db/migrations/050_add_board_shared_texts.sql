-- +goose Up
-- A board can have its own version of a shared text (a regional footer):
-- board_key is '' for the text all boards use, or the key of the board whose
-- emails take this one instead.
ALTER TABLE shared_texts ADD COLUMN board_key TEXT NOT NULL DEFAULT '';
ALTER TABLE shared_texts DROP CONSTRAINT shared_texts_pkey;
ALTER TABLE shared_texts ADD PRIMARY KEY (key, language, board_key);

-- +goose Down
DELETE FROM shared_texts WHERE board_key <> '';
ALTER TABLE shared_texts DROP CONSTRAINT shared_texts_pkey;
ALTER TABLE shared_texts ADD PRIMARY KEY (key, language);
ALTER TABLE shared_texts DROP COLUMN board_key;
