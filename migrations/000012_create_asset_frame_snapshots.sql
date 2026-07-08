-- +goose Up
CREATE TABLE IF NOT EXISTS asset_frame_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    frame_index INTEGER NOT NULL,
    timestamp_ms INTEGER NOT NULL CHECK (timestamp_ms >= 0),
    storage_key TEXT NOT NULL,
    width INTEGER,
    height INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (asset_id, frame_index)
);

CREATE INDEX IF NOT EXISTS idx_asset_frame_snapshots_asset_id ON asset_frame_snapshots(asset_id);
CREATE INDEX IF NOT EXISTS idx_asset_frame_snapshots_asset_timestamp ON asset_frame_snapshots(asset_id, timestamp_ms);

-- +goose Down
DROP TABLE IF EXISTS asset_frame_snapshots;
