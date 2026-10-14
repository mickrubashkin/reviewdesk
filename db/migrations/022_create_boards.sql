-- +goose Up
CREATE TABLE boards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  stages JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO boards (key, name, stages)
SELECT
  sequence_key,
  initcap(replace(replace(sequence_key, '-', ' '), '_', ' ')) AS name,
  coalesce(jsonb_agg(stage ORDER BY min_sort_order) FILTER (WHERE stage <> ''), '[]'::jsonb) AS stages
FROM (
  SELECT
    coalesce(nullif(trim(sequence), ''), 'default') AS sequence_key,
    coalesce(nullif(trim(stage), ''), '') AS stage,
    min(sort_order) AS min_sort_order
  FROM emails
  WHERE archived_at IS NULL
  GROUP BY coalesce(nullif(trim(sequence), ''), 'default'), coalesce(nullif(trim(stage), ''), '')
) AS board_stages
GROUP BY sequence_key
ON CONFLICT (key) DO NOTHING;

INSERT INTO boards (key, name, stages)
SELECT 'onboarding', 'Onboarding', '[]'::jsonb
WHERE NOT EXISTS (SELECT 1 FROM boards WHERE key = 'onboarding');

-- +goose Down
DROP TABLE boards;
