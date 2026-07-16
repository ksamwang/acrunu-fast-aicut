-- +goose Up
ALTER TABLE generation_tasks
    DROP CONSTRAINT IF EXISTS generation_tasks_task_type_check;

ALTER TABLE generation_tasks
    ADD CONSTRAINT generation_tasks_task_type_check
    CHECK (task_type IN (
        'batch_video',
        'test',
        'asset_extract_frames',
        'asset_analyze',
        'asset_embedding',
        'voice_profile_preview',
        'voice_audition',
        'voiceover_generate',
        'edit_plan_generate',
        'generation_render'
    ));

ALTER TABLE generation_run_tasks
    DROP CONSTRAINT IF EXISTS generation_run_tasks_stage_check;

ALTER TABLE generation_run_tasks
    ADD CONSTRAINT generation_run_tasks_stage_check
    CHECK (stage IN ('voiceover', 'edit_plan', 'render'));

ALTER TABLE generation_runs
    ADD COLUMN IF NOT EXISTS output_storage_key TEXT,
    ADD COLUMN IF NOT EXISTS output_mime_type TEXT,
    ADD COLUMN IF NOT EXISTS output_duration_ms INTEGER CHECK (output_duration_ms > 0),
    ADD COLUMN IF NOT EXISTS output_width INTEGER CHECK (output_width > 0),
    ADD COLUMN IF NOT EXISTS output_height INTEGER CHECK (output_height > 0),
    ADD COLUMN IF NOT EXISTS output_file_size_bytes BIGINT CHECK (output_file_size_bytes > 0),
    ADD COLUMN IF NOT EXISTS renderer TEXT,
    ADD COLUMN IF NOT EXISTS render_version TEXT;

-- +goose Down
ALTER TABLE generation_runs
    DROP COLUMN IF EXISTS render_version,
    DROP COLUMN IF EXISTS renderer,
    DROP COLUMN IF EXISTS output_file_size_bytes,
    DROP COLUMN IF EXISTS output_height,
    DROP COLUMN IF EXISTS output_width,
    DROP COLUMN IF EXISTS output_duration_ms,
    DROP COLUMN IF EXISTS output_mime_type,
    DROP COLUMN IF EXISTS output_storage_key;

ALTER TABLE generation_run_tasks
    DROP CONSTRAINT IF EXISTS generation_run_tasks_stage_check;

ALTER TABLE generation_run_tasks
    ADD CONSTRAINT generation_run_tasks_stage_check
    CHECK (stage IN ('voiceover', 'edit_plan'));

ALTER TABLE generation_tasks
    DROP CONSTRAINT IF EXISTS generation_tasks_task_type_check;

ALTER TABLE generation_tasks
    ADD CONSTRAINT generation_tasks_task_type_check
    CHECK (task_type IN (
        'batch_video',
        'test',
        'asset_extract_frames',
        'asset_analyze',
        'asset_embedding',
        'voice_profile_preview',
        'voice_audition',
        'voiceover_generate',
        'edit_plan_generate'
    ));
