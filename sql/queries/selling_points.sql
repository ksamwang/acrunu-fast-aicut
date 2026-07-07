-- name: CreateSellingPoint :one
INSERT INTO product_selling_points (
    product_id,
    title,
    description,
    priority,
    status,
    created_by_user_id,
    updated_by_user_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $6
)
RETURNING *;

-- name: GetSellingPointByID :one
SELECT * FROM product_selling_points
WHERE id = $1;

-- name: ListSellingPointsByProduct :many
SELECT * FROM product_selling_points
WHERE product_id = $1
  AND status = COALESCE(sqlc.narg('status'), status)
ORDER BY priority DESC, created_at DESC;

-- name: UpdateSellingPoint :one
UPDATE product_selling_points
SET title = $2,
    description = $3,
    priority = $4,
    updated_by_user_id = $5,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ArchiveSellingPoint :exec
UPDATE product_selling_points
SET status = 'archived',
    updated_by_user_id = $2,
    updated_at = now()
WHERE id = $1;
