-- name: CreateGenerationTask :one
INSERT INTO generation_tasks (
    product_id,
    created_by_user_id,
    task_type,
    status,
    variant_count,
    target_duration_ms,
    style_prompt,
    config_snapshot
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: GetGenerationTaskByID :one
SELECT * FROM generation_tasks
WHERE id = $1;

-- name: ListGenerationTasks :many
SELECT * FROM generation_tasks
WHERE status = COALESCE(sqlc.narg('status'), status)
ORDER BY created_at DESC;

-- name: UpdateGenerationTaskStatus :exec
UPDATE generation_tasks
SET status = $2,
    error_message = $3,
    updated_at = now(),
    started_at = CASE WHEN $2 = 'running' AND started_at IS NULL THEN now() ELSE started_at END,
    finished_at = CASE WHEN $2 IN ('completed', 'failed', 'cancelled') THEN now() ELSE finished_at END
WHERE id = $1;
