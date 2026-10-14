-- +goose Up
UPDATE approval_areas
SET key = 'crm-ops', updated_at = now()
WHERE key = 'crm_ops';

-- +goose Down
UPDATE approval_areas
SET key = 'crm_ops', updated_at = now()
WHERE key = 'crm-ops';
