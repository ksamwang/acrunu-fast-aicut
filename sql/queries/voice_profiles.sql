-- name: CreateVoiceProfile :one
INSERT INTO voice_profiles (
    id,
    name,
    language,
    style_tags,
    reference_text,
    reference_audio_storage_key,
    reference_audio_file_name,
    reference_audio_mime_type,
    reference_audio_size,
    preview_text,
    status,
    is_default,
    created_by_user_id,
    updated_by_user_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
RETURNING *;

-- name: GetVoiceProfileByID :one
SELECT * FROM voice_profiles
WHERE id = $1;

-- name: ListVoiceProfiles :many
SELECT * FROM voice_profiles
ORDER BY is_default DESC, created_at ASC;

-- name: UpdateVoiceProfileMetadata :one
UPDATE voice_profiles
SET
    name = $2,
    language = $3,
    style_tags = $4,
    reference_text = $5,
    preview_text = $6,
    status = $7,
    is_default = $8,
    updated_by_user_id = $9,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateVoiceProfileWithReference :one
UPDATE voice_profiles
SET
    name = $2,
    language = $3,
    style_tags = $4,
    reference_text = $5,
    reference_audio_storage_key = $6,
    reference_audio_file_name = $7,
    reference_audio_mime_type = $8,
    reference_audio_size = $9,
    preview_text = $10,
    status = $11,
    is_default = $12,
    updated_by_user_id = $13,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ClearDefaultVoiceProfiles :exec
UPDATE voice_profiles
SET is_default = FALSE, updated_at = now()
WHERE is_default = TRUE;

-- name: SetDefaultVoiceProfile :one
UPDATE voice_profiles
SET
    is_default = TRUE,
    updated_by_user_id = $2,
    updated_at = now()
WHERE id = $1
  AND status = 'enabled'
RETURNING *;

-- name: QueueVoiceProfilePreview :exec
UPDATE voice_profiles
SET
    preview_status = 'queued',
    preview_audio_storage_key = NULL,
    preview_error = NULL,
    updated_at = now()
WHERE id = $1;

-- name: MarkVoiceProfilePreviewSynthesizing :exec
UPDATE voice_profiles
SET
    preview_status = 'processing',
    preview_error = NULL,
    updated_at = now()
WHERE id = $1;

-- name: MarkVoiceProfilePreviewReady :exec
UPDATE voice_profiles
SET
    preview_status = 'ready',
    preview_audio_storage_key = $2,
    preview_error = NULL,
    updated_at = now()
WHERE id = $1;

-- name: MarkVoiceProfilePreviewFailed :exec
UPDATE voice_profiles
SET
    preview_status = 'failed',
    preview_error = $2,
    updated_at = now()
WHERE id = $1;

-- name: DeleteVoiceProfile :exec
DELETE FROM voice_profiles
WHERE id = $1;
