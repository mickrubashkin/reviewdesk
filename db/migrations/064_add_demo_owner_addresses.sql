-- +goose Up
-- The addresses the owner entered the demo from with their key. Entries from
-- such an address within 30 days count as the owner's too, from any browser
-- (one may load the demo in a second, empty context, such as a private tab).
-- Only the owner's addresses are kept, never a visitor's.
CREATE TABLE demo_owner_addresses (
    address TEXT PRIMARY KEY,
    seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE demo_owner_addresses;
