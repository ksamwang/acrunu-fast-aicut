-- +goose Up
ALTER TABLE assets
    ADD COLUMN IF NOT EXISTS asset_name TEXT,
    ADD COLUMN IF NOT EXISTS source_path TEXT,
    ADD COLUMN IF NOT EXISTS ingestion_source TEXT NOT NULL DEFAULT 'local-agent' CHECK (ingestion_source IN ('local-agent', 'server-upload', 'manual-import')),
    ADD COLUMN IF NOT EXISTS analysis_status TEXT NOT NULL DEFAULT 'pending_analysis' CHECK (analysis_status IN ('pending_analysis', 'analyzing', 'ready', 'failed')),
    ADD COLUMN IF NOT EXISTS usability_status TEXT NOT NULL DEFAULT 'usable' CHECK (usability_status IN ('usable', 'needs_review', 'rejected')),
    ADD COLUMN IF NOT EXISTS has_audio BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS audio_codec TEXT,
    ADD COLUMN IF NOT EXISTS bitrate_kbps INTEGER,
    ADD COLUMN IF NOT EXISTS scene_description TEXT,
    ADD COLUMN IF NOT EXISTS shot_size TEXT,
    ADD COLUMN IF NOT EXISTS camera_movement TEXT,
    ADD COLUMN IF NOT EXISTS subjects JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS scene_tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS quality_tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS model_result JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS reviewer_notes TEXT,
    ADD COLUMN IF NOT EXISTS analysis_error TEXT,
    ADD COLUMN IF NOT EXISTS analyzed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

UPDATE assets
SET asset_name = file_name
WHERE asset_name IS NULL;

CREATE INDEX IF NOT EXISTS idx_assets_analysis_status ON assets(analysis_status);
CREATE INDEX IF NOT EXISTS idx_assets_usability_status ON assets(usability_status);
CREATE INDEX IF NOT EXISTS idx_assets_ingestion_source ON assets(ingestion_source);
CREATE INDEX IF NOT EXISTS idx_assets_analyzed_at ON assets(analyzed_at);

-- +goose Down
DROP INDEX IF EXISTS idx_assets_analyzed_at;
DROP INDEX IF EXISTS idx_assets_ingestion_source;
DROP INDEX IF EXISTS idx_assets_usability_status;
DROP INDEX IF EXISTS idx_assets_analysis_status;

ALTER TABLE assets
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS analyzed_at,
    DROP COLUMN IF EXISTS analysis_error,
    DROP COLUMN IF EXISTS reviewer_notes,
    DROP COLUMN IF EXISTS model_result,
    DROP COLUMN IF EXISTS quality_tags,
    DROP COLUMN IF EXISTS scene_tags,
    DROP COLUMN IF EXISTS subjects,
    DROP COLUMN IF EXISTS camera_movement,
    DROP COLUMN IF EXISTS shot_size,
    DROP COLUMN IF EXISTS scene_description,
    DROP COLUMN IF EXISTS bitrate_kbps,
    DROP COLUMN IF EXISTS audio_codec,
    DROP COLUMN IF EXISTS has_audio,
    DROP COLUMN IF EXISTS usability_status,
    DROP COLUMN IF EXISTS analysis_status,
    DROP COLUMN IF EXISTS ingestion_source,
    DROP COLUMN IF EXISTS source_path,
    DROP COLUMN IF EXISTS asset_name;
