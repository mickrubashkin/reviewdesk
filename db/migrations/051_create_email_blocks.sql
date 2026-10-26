-- +goose Up
-- The block library emails will be built from: the frame (the document
-- around the content) and blocks, table rows with data-edit-* fields. The
-- API fills in the starting set (emailblocks.Defaults) on start.
--
-- fields describes the block's fields in order: key and type come from the
-- HTML; label, required, max_length and shared are kept across HTML edits.
CREATE TABLE email_blocks (
  key TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  html TEXT NOT NULL,
  fields JSONB NOT NULL DEFAULT '[]'::jsonb,
  sort_order INTEGER NOT NULL DEFAULT 0,
  version INTEGER NOT NULL DEFAULT 1,
  updated_by_email TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE email_blocks;
