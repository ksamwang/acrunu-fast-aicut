-- +goose Up
CREATE TABLE IF NOT EXISTS bgm_tracks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    file_name TEXT NOT NULL,
    storage_key TEXT NOT NULL UNIQUE,
    mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    file_size_bytes BIGINT NOT NULL CHECK (file_size_bytes > 0),
    duration_ms INTEGER NOT NULL CHECK (duration_ms > 0),
    sample_rate INTEGER NOT NULL DEFAULT 0 CHECK (sample_rate >= 0),
    channels INTEGER NOT NULL DEFAULT 0 CHECK (channels >= 0),
    bpm INTEGER CHECK (bpm BETWEEN 20 AND 300),
    mood TEXT NOT NULL DEFAULT '',
    tags TEXT[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'enabled' CHECK (status IN ('enabled', 'disabled', 'archived')),
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bgm_tracks_status_created_at
    ON bgm_tracks (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_bgm_tracks_mood
    ON bgm_tracks (mood);

-- +goose Down
DROP TABLE IF EXISTS bgm_tracks;
