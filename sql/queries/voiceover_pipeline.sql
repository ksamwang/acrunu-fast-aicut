-- name: CreateScriptVariant :one
INSERT INTO script_variants (
    generation_task_id,
    product_id,
    variant_index,
    hook,
    script_text,
    editing_intent,
    beats,
    voice_profile_id,
    voice_profile_name,
    reference_audio_storage_key,
    reference_audio_file_name,
    reference_text
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING *;

-- name: GetScriptVariantByID :one
SELECT * FROM script_variants
WHERE id = $1;

-- name: GetScriptVariantByGenerationTaskID :one
SELECT * FROM script_variants
WHERE generation_task_id = $1;

-- name: ListScriptVariantsByGenerationTaskID :many
SELECT * FROM script_variants
WHERE generation_task_id = $1
ORDER BY variant_index ASC;

-- name: MarkScriptVariantVoiceoverReady :exec
UPDATE script_variants
SET status = 'voiceover_ready', error_message = NULL, updated_at = now()
WHERE id = $1;

-- name: MarkScriptVariantFailed :exec
UPDATE script_variants
SET status = 'failed', error_message = $2, updated_at = now()
WHERE id = $1;

-- name: CreateVoiceover :one
INSERT INTO voiceovers (
    script_variant_id,
    voice_provider,
    voice_model,
    voice_name
) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetVoiceoverByID :one
SELECT * FROM voiceovers
WHERE id = $1;

-- name: GetVoiceoverByScriptVariantID :one
SELECT * FROM voiceovers
WHERE script_variant_id = $1;

-- name: MarkVoiceoverSynthesizing :exec
UPDATE voiceovers
SET status = 'synthesizing', error_message = NULL, updated_at = now()
WHERE id = $1;

-- name: MarkVoiceoverTranscribing :exec
UPDATE voiceovers
SET status = 'transcribing', error_message = NULL, updated_at = now()
WHERE id = $1;

-- name: MarkVoiceoverCompleted :exec
UPDATE voiceovers
SET
    status = 'completed',
    storage_key = $2,
    sample_rate = $3,
    duration_ms = $4,
    error_message = NULL,
    updated_at = now()
WHERE id = $1;

-- name: MarkVoiceoverFailed :exec
UPDATE voiceovers
SET status = 'failed', error_message = $2, updated_at = now()
WHERE id = $1;

-- name: DeleteNarrationSegmentsByVoiceoverID :exec
DELETE FROM narration_segments
WHERE voiceover_id = $1;

-- name: CreateNarrationSegment :one
INSERT INTO narration_segments (
    script_variant_id,
    voiceover_id,
    segment_index,
    text,
    start_ms,
    end_ms,
    confidence
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListNarrationSegmentsByVoiceoverID :many
SELECT * FROM narration_segments
WHERE voiceover_id = $1
ORDER BY segment_index ASC;

-- name: CreateVoiceAudition :one
INSERT INTO voice_auditions (
    generation_task_id,
    voice_profile_id,
    voice_profile_name,
    reference_audio_storage_key,
    reference_audio_file_name,
    reference_text,
    text,
    created_by_user_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetVoiceAuditionByID :one
SELECT * FROM voice_auditions
WHERE id = $1;

-- name: MarkVoiceAuditionSynthesizing :exec
UPDATE voice_auditions
SET status = 'synthesizing', error_message = NULL, updated_at = now()
WHERE id = $1;

-- name: MarkVoiceAuditionCompleted :exec
UPDATE voice_auditions
SET
    status = 'completed',
    audio_storage_key = $2,
    sample_rate = $3,
    duration_ms = $4,
    error_message = NULL,
    updated_at = now()
WHERE id = $1;

-- name: MarkVoiceAuditionFailed :exec
UPDATE voice_auditions
SET status = 'failed', error_message = $2, updated_at = now()
WHERE id = $1;

-- name: ListVoiceoverGenerationTasks :many
SELECT * FROM generation_tasks
WHERE task_type = 'voiceover_generate'
ORDER BY created_at DESC;
