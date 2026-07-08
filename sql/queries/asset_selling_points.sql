-- name: AddAssetSellingPoint :exec
INSERT INTO asset_selling_points (
    asset_id,
    selling_point_id
) VALUES (
    $1, $2
)
ON CONFLICT DO NOTHING;

-- name: RemoveAssetSellingPoint :exec
DELETE FROM asset_selling_points
WHERE asset_id = $1
  AND selling_point_id = $2;

-- name: ListSellingPointIDsByAsset :many
SELECT selling_point_id
FROM asset_selling_points
WHERE asset_id = $1
ORDER BY created_at ASC;
