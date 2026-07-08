-- +goose Up
CREATE TABLE IF NOT EXISTS speech_segments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    start_ms INTEGER NOT NULL CHECK (start_ms >= 0),
    end_ms INTEGER NOT NULL CHECK (end_ms >= start_ms),
    transcript TEXT NOT NULL,
    confidence NUMERIC(5, 4),
    source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'imported', 'asr')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'ignored', 'archived')),
    created_by_user_id UUID REFERENCES users(id),
    updated_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_speech_segments_asset_id ON speech_segments(asset_id);
CREATE INDEX IF NOT EXISTS idx_speech_segments_status ON speech_segments(status);
CREATE INDEX IF NOT EXISTS idx_speech_segments_asset_start_ms ON speech_segments(asset_id, start_ms);

-- +goose Down
DROP TABLE IF EXISTS speech_segments;
