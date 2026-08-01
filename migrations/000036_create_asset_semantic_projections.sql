-- +goose Up
CREATE TABLE IF NOT EXISTS asset_semantic_projection_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL REFERENCES model_providers(id) ON DELETE CASCADE,
    model TEXT NOT NULL,
    dimension INT NOT NULL,
    algorithm TEXT NOT NULL,
    source_count INT NOT NULL DEFAULT 0,
    projected_count INT NOT NULL DEFAULT 0,
    source_updated_at TIMESTAMPTZ,
    status TEXT NOT NULL CHECK (status IN ('building', 'ready', 'failed')),
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_asset_semantic_projection_runs_ready
    ON asset_semantic_projection_runs (provider_id, model, dimension, created_at DESC)
    WHERE status = 'ready';

CREATE TABLE IF NOT EXISTS asset_semantic_projection_points (
    projection_id UUID NOT NULL REFERENCES asset_semantic_projection_runs(id) ON DELETE CASCADE,
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    x2 DOUBLE PRECISION NOT NULL,
    y2 DOUBLE PRECISION NOT NULL,
    x3 DOUBLE PRECISION NOT NULL,
    y3 DOUBLE PRECISION NOT NULL,
    z3 DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (projection_id, asset_id)
);

CREATE INDEX IF NOT EXISTS idx_asset_semantic_projection_points_asset
    ON asset_semantic_projection_points (asset_id, projection_id);

-- +goose Down
DROP TABLE IF EXISTS asset_semantic_projection_points;
DROP TABLE IF EXISTS asset_semantic_projection_runs;
