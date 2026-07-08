-- name: CreateSpeechSegment :one
INSERT INTO speech_segments (
    asset_id,
    start_ms,
    end_ms,
    transcript,
    confidence,
    source,
    status,
    created_by_user_id,
    updated_by_user_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $8
)
RETURNING *;

-- name: ListSpeechSegmentsByAsset :many
SELECT * FROM speech_segments
WHERE asset_id = $1
  AND status = COALESCE(sqlc.narg('status'), status)
ORDER BY start_ms ASC, created_at ASC;

-- name: UpdateSpeechSegment :one
UPDATE speech_segments
SET start_ms = $2,
    end_ms = $3,
    transcript = $4,
    confidence = $5,
    source = $6,
    status = $7,
    updated_by_user_id = $8,
    updated_at = now()
WHERE id = $1
RETURNING *;
