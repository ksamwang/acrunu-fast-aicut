-- name: CreateAsset :one
INSERT INTO assets (
    product_id,
    asset_name,
    storage_key,
    file_name,
    file_ext,
    mime_type,
    file_size,
    checksum,
    source_type,
    ingestion_source,
    duration_ms,
    width,
    height,
    fps,
    codec,
    status,
    analysis_status,
    usability_status,
    manual_clean_status,
    source_path,
    source_original_name,
    source_in_ms,
    source_out_ms,
    has_audio,
    default_use_original_audio,
    audio_codec,
    bitrate_kbps,
    likely_has_speech,
    scene_description,
    shot_size,
    camera_movement,
    subjects,
    scene_tags,
    quality_tags,
    model_labels,
    model_result,
    review_overrides,
    reviewer_notes,
    analysis_error,
    analyzed_at,
    metadata,
    created_by_user_id,
    updated_by_user_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
    $21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
    $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41, $42, $43
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
  AND analysis_status = COALESCE(sqlc.narg('analysis_status'), analysis_status)
  AND usability_status = COALESCE(sqlc.narg('usability_status'), usability_status)
  AND shot_size = COALESCE(sqlc.narg('shot_size'), shot_size)
  AND (
    sqlc.narg('selling_point_id')::uuid IS NULL
    OR EXISTS (
      SELECT 1
      FROM asset_selling_points asp
      WHERE asp.asset_id = assets.id
        AND asp.selling_point_id = sqlc.narg('selling_point_id')::uuid
    )
  )
  AND (
    sqlc.narg('tag')::text IS NULL
    OR scene_description ILIKE '%' || sqlc.narg('tag')::text || '%'
    OR shot_size = sqlc.narg('tag')::text
    OR camera_movement = sqlc.narg('tag')::text
    OR subjects ? sqlc.narg('tag')::text
    OR scene_tags ? sqlc.narg('tag')::text
    OR quality_tags ? sqlc.narg('tag')::text
  )
  AND (
    sqlc.narg('min_duration_ms')::int IS NULL
    OR COALESCE(duration_ms, 0) >= sqlc.narg('min_duration_ms')::int
  )
  AND (
    sqlc.narg('max_duration_ms')::int IS NULL
    OR COALESCE(duration_ms, 0) <= sqlc.narg('max_duration_ms')::int
  )
  AND has_audio = COALESCE(sqlc.narg('has_audio'), has_audio)
  AND likely_has_speech = COALESCE(sqlc.narg('likely_has_speech'), likely_has_speech)
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
    has_audio = $7,
    audio_codec = $8,
    bitrate_kbps = $9,
    likely_has_speech = $10,
    updated_by_user_id = $11,
    updated_at = now()
WHERE id = $1;

-- name: UpdateAssetDefaultUseOriginalAudio :exec
UPDATE assets
SET default_use_original_audio = $2,
    updated_by_user_id = $3,
    updated_at = now()
WHERE id = $1;

-- name: UpdateAssetAnalysis :exec
UPDATE assets
SET analysis_status = $2,
    usability_status = $3,
    scene_description = $4,
    shot_size = $5,
    camera_movement = $6,
    subjects = $7,
    scene_tags = $8,
    quality_tags = $9,
    model_labels = $10,
    model_result = $11,
    analysis_error = $12,
    analyzed_at = $13,
    updated_by_user_id = $14,
    updated_at = now()
WHERE id = $1;

-- name: UpdateAssetReview :exec
UPDATE assets
SET reviewer_notes = $2,
    review_overrides = $3,
    usability_status = $4,
    updated_by_user_id = $5,
    updated_at = now()
WHERE id = $1;

-- name: UpdateAssetEmbedding :exec
UPDATE assets
SET embedding = $2,
    updated_at = now()
WHERE id = $1;

-- name: UpdateAssetMetadata :exec
UPDATE assets
SET metadata = $2,
    updated_by_user_id = $3,
    updated_at = now()
WHERE id = $1;

-- name: ArchiveAsset :exec
UPDATE assets
SET status = 'archived',
    archived_at = now(),
    updated_by_user_id = $2,
    updated_at = now()
WHERE id = $1;

-- name: RestoreAsset :exec
UPDATE assets
SET status = 'ready',
    archived_at = NULL,
    updated_by_user_id = $2,
    updated_at = now()
WHERE id = $1;
