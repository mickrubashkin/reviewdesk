-- +goose Up
-- What the visitor's browser tells about a demo entry: visitor_id is a random
-- id it keeps across visits (so returning visitors can be told apart), owner
-- marks the owner's own entries, time_zone and language give a rough place.
-- No IP address is kept.
ALTER TABLE demo_visits
    ADD COLUMN visitor_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN owner BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN time_zone TEXT NOT NULL DEFAULT '',
    ADD COLUMN language TEXT NOT NULL DEFAULT '';

CREATE INDEX demo_visits_visitor_id_idx ON demo_visits (visitor_id) WHERE visitor_id <> '';

-- +goose Down
DROP INDEX demo_visits_visitor_id_idx;
ALTER TABLE demo_visits
    DROP COLUMN visitor_id,
    DROP COLUMN owner,
    DROP COLUMN time_zone,
    DROP COLUMN language;
