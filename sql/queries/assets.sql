-- name: CreateAsset :one
INSERT INTO assets (
    product_id,
    storage_key,
    file_name,
    file_ext,
    mime_type,
    file_size,
    checksum,
    source_type,
    duration_ms,
    width,
    height,
    fps,
    codec,
    status,
    manual_clean_status,
    source_original_name,
    source_in_ms,
    source_out_ms,
    metadata,
    created_by_user_id,
    updated_by_user_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $20
)
RETURNING *;

-- name: GetAssetByID :one
SELECT * FROM assets
WHERE id = $1;

-- name: ListAssets :many
SELECT * FROM assets
WHERE product_id = COALESCE(sqlc.narg('product_id'), product_id)
  AND source_type = COALESCE(sqlc.narg('source_type'), source_type)
  AND status = COALESCE(sqlc.narg('status'), status)
ORDER BY created_at DESC;

-- name: UpdateAssetStatus :exec
UPDATE assets
SET status = $2,
    updated_by_user_id = $3,
    updated_at = now()
WHERE id = $1;

-- name: UpdateAssetMediaInfo :exec
UPDATE assets
SET duration_ms = $2,
    width = $3,
    height = $4,
    fps = $5,
    codec = $6,
    updated_by_user_id = $7,
    updated_at = now()
WHERE id = $1;
