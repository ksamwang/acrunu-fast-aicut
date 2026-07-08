-- name: UpsertAssetFrameSnapshot :one
INSERT INTO asset_frame_snapshots (
    asset_id,
    frame_index,
    timestamp_ms,
    storage_key,
    width,
    height
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (asset_id, frame_index)
DO UPDATE SET
    timestamp_ms = EXCLUDED.timestamp_ms,
    storage_key = EXCLUDED.storage_key,
    width = EXCLUDED.width,
    height = EXCLUDED.height
RETURNING *;

-- name: ListAssetFrameSnapshotsByAsset :many
SELECT *
FROM asset_frame_snapshots
WHERE asset_id = $1
ORDER BY frame_index ASC, created_at ASC;

-- name: DeleteAssetFrameSnapshotsByAsset :exec
DELETE FROM asset_frame_snapshots
WHERE asset_id = $1;
