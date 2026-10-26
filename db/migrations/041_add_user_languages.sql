-- +goose Up
-- Languages a person translates; outdated translations in these languages
-- show up in their tasks.
ALTER TABLE users ADD COLUMN languages TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE users DROP COLUMN languages;
