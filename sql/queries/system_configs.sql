-- name: UpsertSystemConfig :one
INSERT INTO system_configs (
    config_key,
    config_value,
    config_type,
    is_secret,
    description,
    updated_by_user_id
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (config_key) DO UPDATE SET
    config_value = EXCLUDED.config_value,
    config_type = EXCLUDED.config_type,
    is_secret = EXCLUDED.is_secret,
    description = EXCLUDED.description,
    updated_by_user_id = EXCLUDED.updated_by_user_id,
    updated_at = now()
RETURNING *;

-- name: GetSystemConfigByKey :one
SELECT * FROM system_configs
WHERE config_key = $1;

-- name: ListSystemConfigs :many
SELECT * FROM system_configs
ORDER BY config_key ASC;
