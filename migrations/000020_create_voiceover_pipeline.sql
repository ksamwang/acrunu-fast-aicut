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
        'voiceover_generate'
    ));

CREATE TABLE IF NOT EXISTS voice_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CHECK (length(btrim(name)) > 0),
    language TEXT NOT NULL DEFAULT '中文',
    style_tags JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(style_tags) = 'array'),
    reference_text TEXT NOT NULL CHECK (length(btrim(reference_text)) > 0),
    reference_audio_storage_key TEXT NOT NULL,
    reference_audio_file_name TEXT NOT NULL,
    reference_audio_mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    reference_audio_size BIGINT NOT NULL CHECK (reference_audio_size > 0),
    preview_text TEXT NOT NULL CHECK (length(btrim(preview_text)) > 0),
    preview_audio_storage_key TEXT,
    preview_status TEXT NOT NULL DEFAULT 'queued' CHECK (preview_status IN ('queued', 'processing', 'ready', 'failed')),
    preview_error TEXT,
    status TEXT NOT NULL DEFAULT 'enabled' CHECK (status IN ('enabled', 'disabled')),
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_by_user_id UUID REFERENCES users(id),
    updated_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_voice_profiles_single_default
    ON voice_profiles (is_default)
    WHERE is_default;
CREATE INDEX IF NOT EXISTS idx_voice_profiles_status ON voice_profiles (status);
CREATE INDEX IF NOT EXISTS idx_voice_profiles_created_at ON voice_profiles (created_at DESC);

CREATE TABLE IF NOT EXISTS script_variants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    generation_task_id UUID NOT NULL REFERENCES generation_tasks(id),
    product_id UUID NOT NULL REFERENCES products(id),
    variant_index INTEGER NOT NULL CHECK (variant_index > 0),
    hook TEXT NOT NULL DEFAULT '',
    script_text TEXT NOT NULL CHECK (length(btrim(script_text)) > 0),
    editing_intent TEXT NOT NULL DEFAULT '',
    beats JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(beats) = 'array'),
    voice_profile_id UUID NOT NULL REFERENCES voice_profiles(id),
    voice_profile_name TEXT NOT NULL,
    reference_audio_storage_key TEXT NOT NULL,
    reference_audio_file_name TEXT NOT NULL,
    reference_text TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'voiceover_pending' CHECK (status IN ('voiceover_pending', 'voiceover_ready', 'failed')),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (generation_task_id, variant_index)
);

CREATE INDEX IF NOT EXISTS idx_script_variants_generation_task_id ON script_variants (generation_task_id);
CREATE INDEX IF NOT EXISTS idx_script_variants_product_id ON script_variants (product_id);
CREATE INDEX IF NOT EXISTS idx_script_variants_voice_profile_id ON script_variants (voice_profile_id);

CREATE TABLE IF NOT EXISTS voiceovers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    script_variant_id UUID NOT NULL UNIQUE REFERENCES script_variants(id),
    storage_key TEXT,
    voice_provider TEXT NOT NULL DEFAULT 'cosyvoice',
    voice_model TEXT NOT NULL DEFAULT '',
    voice_name TEXT NOT NULL,
    sample_rate INTEGER,
    duration_ms INTEGER,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'synthesizing', 'transcribing', 'completed', 'failed')),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_voiceovers_status ON voiceovers (status);

CREATE TABLE IF NOT EXISTS narration_segments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    script_variant_id UUID NOT NULL REFERENCES script_variants(id),
    voiceover_id UUID NOT NULL REFERENCES voiceovers(id),
    segment_index INTEGER NOT NULL CHECK (segment_index >= 0),
    text TEXT NOT NULL CHECK (length(btrim(text)) > 0),
    start_ms INTEGER NOT NULL CHECK (start_ms >= 0),
    end_ms INTEGER NOT NULL CHECK (end_ms > start_ms),
    confidence NUMERIC,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (voiceover_id, segment_index)
);

CREATE INDEX IF NOT EXISTS idx_narration_segments_voiceover_id ON narration_segments (voiceover_id, segment_index);
CREATE INDEX IF NOT EXISTS idx_narration_segments_script_variant_id ON narration_segments (script_variant_id, segment_index);

CREATE TABLE IF NOT EXISTS voice_auditions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    generation_task_id UUID NOT NULL UNIQUE REFERENCES generation_tasks(id),
    voice_profile_id UUID NOT NULL REFERENCES voice_profiles(id),
    voice_profile_name TEXT NOT NULL,
    reference_audio_storage_key TEXT NOT NULL,
    reference_audio_file_name TEXT NOT NULL,
    reference_text TEXT NOT NULL,
    text TEXT NOT NULL CHECK (length(btrim(text)) > 0),
    audio_storage_key TEXT,
    sample_rate INTEGER,
    duration_ms INTEGER,
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'synthesizing', 'completed', 'failed')),
    error_message TEXT,
    created_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_voice_auditions_created_by_user_id ON voice_auditions (created_by_user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS voice_auditions;
DROP TABLE IF EXISTS narration_segments;
DROP TABLE IF EXISTS voiceovers;
DROP TABLE IF EXISTS script_variants;
DROP TABLE IF EXISTS voice_profiles;

ALTER TABLE generation_tasks
    DROP CONSTRAINT IF EXISTS generation_tasks_task_type_check;

ALTER TABLE generation_tasks
    ADD CONSTRAINT generation_tasks_task_type_check
    CHECK (task_type IN ('batch_video', 'test', 'asset_extract_frames', 'asset_analyze', 'asset_embedding'));
