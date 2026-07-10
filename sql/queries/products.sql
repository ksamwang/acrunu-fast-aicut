-- name: CreateProduct :one
INSERT INTO products (
    name,
    description,
    category,
    status,
    metadata,
    created_by_user_id,
    updated_by_user_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $6
)
RETURNING *;

-- name: GetProductByID :one
SELECT * FROM products
WHERE id = $1;

-- name: ListProducts :many
SELECT * FROM products
WHERE status = COALESCE(sqlc.narg('status'), status)
ORDER BY created_at DESC;

-- name: UpdateProduct :one
UPDATE products
SET name = $2,
    description = $3,
    category = $4,
    metadata = $5,
    updated_by_user_id = $6,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ArchiveProduct :exec
UPDATE products
SET status = 'archived',
    updated_by_user_id = $2,
    updated_at = now()
WHERE id = $1;

-- name: DeleteProduct :exec
DELETE FROM products
WHERE id = $1;
