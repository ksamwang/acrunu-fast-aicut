-- +goose Up
ALTER TABLE assets
    ADD COLUMN IF NOT EXISTS default_use_original_audio BOOLEAN NOT NULL DEFAULT FALSE;

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

CREATE TABLE IF NOT EXISTS generation_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id),
    created_by_user_id UUID REFERENCES users(id),
    voiceover_task_id UUID UNIQUE REFERENCES generation_tasks(id) ON DELETE SET NULL,
    script_variant_id UUID UNIQUE REFERENCES script_variants(id) ON DELETE SET NULL,
    voiceover_id UUID UNIQUE REFERENCES voiceovers(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'generating' CHECK (status IN ('generating', 'completed', 'failed')),
    stage TEXT NOT NULL DEFAULT 'queued' CHECK (stage IN (
        'queued',
        'voicing',
        'aligning',
        'retrieving',
        'planning',
        'plan_ready',
        'rendering',
        'completed',
        'failed'
    )),
    progress INTEGER NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),
    error_message TEXT,
    config_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_generation_runs_product_created_at
    ON generation_runs (product_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_generation_runs_status_created_at
    ON generation_runs (status, created_at DESC);

CREATE TABLE IF NOT EXISTS generation_run_tasks (
    generation_run_id UUID NOT NULL REFERENCES generation_runs(id) ON DELETE CASCADE,
    generation_task_id UUID NOT NULL UNIQUE REFERENCES generation_tasks(id) ON DELETE CASCADE,
    stage TEXT NOT NULL CHECK (stage IN ('voiceover', 'edit_plan')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (generation_run_id, generation_task_id)
);

CREATE INDEX IF NOT EXISTS idx_generation_run_tasks_run_stage
    ON generation_run_tasks (generation_run_id, stage);

CREATE TABLE IF NOT EXISTS edit_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    generation_run_id UUID NOT NULL UNIQUE REFERENCES generation_runs(id) ON DELETE CASCADE,
    script_variant_id UUID NOT NULL REFERENCES script_variants(id),
    voiceover_id UUID NOT NULL REFERENCES voiceovers(id),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'planning', 'ready', 'failed')),
    candidate_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
    plan_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    llm_provider TEXT NOT NULL DEFAULT '',
    llm_model TEXT NOT NULL DEFAULT '',
    prompt_version TEXT NOT NULL DEFAULT '',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_edit_plans_status_created_at
    ON edit_plans (status, created_at DESC);

CREATE TABLE IF NOT EXISTS clip_segments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    edit_plan_id UUID NOT NULL REFERENCES edit_plans(id) ON DELETE CASCADE,
    segment_index INTEGER NOT NULL CHECK (segment_index >= 0),
    narration_segment_id UUID NOT NULL REFERENCES narration_segments(id),
    asset_id UUID NOT NULL REFERENCES assets(id),
    speech_segment_id UUID REFERENCES speech_segments(id),
    source_in_ms INTEGER NOT NULL CHECK (source_in_ms >= 0),
    source_out_ms INTEGER NOT NULL CHECK (source_out_ms > source_in_ms),
    timeline_in_ms INTEGER NOT NULL CHECK (timeline_in_ms >= 0),
    timeline_duration_ms INTEGER NOT NULL CHECK (timeline_duration_ms > 0),
    source_type TEXT NOT NULL CHECK (source_type IN ('visual_only', 'talking_head')),
    label TEXT NOT NULL DEFAULT '',
    visual_goal TEXT NOT NULL DEFAULT '',
    use_original_audio BOOLEAN NOT NULL DEFAULT FALSE,
    audio_gain_db NUMERIC(8, 3) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (edit_plan_id, segment_index),
    UNIQUE (edit_plan_id, narration_segment_id)
);

CREATE INDEX IF NOT EXISTS idx_clip_segments_edit_plan_timeline
    ON clip_segments (edit_plan_id, timeline_in_ms);
CREATE INDEX IF NOT EXISTS idx_clip_segments_asset_id
    ON clip_segments (asset_id);

-- +goose Down
DROP TABLE IF EXISTS clip_segments;
DROP TABLE IF EXISTS edit_plans;
DROP TABLE IF EXISTS generation_run_tasks;
DROP TABLE IF EXISTS generation_runs;

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
        'voiceover_generate'
    ));

ALTER TABLE assets
    DROP COLUMN IF EXISTS default_use_original_audio;
