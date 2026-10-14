-- +goose Up
-- Bitrix24 expressions offered when editing emails, e.g. the responsible
-- manager's booking link, with an example value for the preview. They belong
-- to the Bitrix24 portal, so they are shared by all boards.
CREATE TABLE bitrix_variables (
  position INT PRIMARY KEY,
  label TEXT NOT NULL,
  expression TEXT NOT NULL UNIQUE,
  example TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  updated_by_email TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE bitrix_variables;
