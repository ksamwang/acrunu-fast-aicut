-- +goose Up
CREATE TABLE IF NOT EXISTS asset_embedding_objects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    object_type TEXT NOT NULL,
    object_id UUID NOT NULL,
    text TEXT NOT NULL,
    text_hash TEXT NOT NULL,
    provider_id UUID NOT NULL REFERENCES model_providers(id),
    model TEXT NOT NULL,
    dimension INT NOT NULL,
    embedding VECTOR NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'ready',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (asset_id, object_type, object_id, provider_id, model, dimension)
);

CREATE INDEX IF NOT EXISTS idx_asset_embedding_objects_asset
    ON asset_embedding_objects (asset_id);

CREATE INDEX IF NOT EXISTS idx_asset_embedding_objects_model_dimension
    ON asset_embedding_objects (model, dimension);

-- +goose Down
DROP TABLE IF EXISTS asset_embedding_objects;
