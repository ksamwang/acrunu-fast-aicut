-- +goose Up
-- text-embedding-v4 is the default configured embedding model. Keep this
-- partial expression index separate so other providers and dimensions remain valid.
CREATE INDEX IF NOT EXISTS idx_asset_embedding_objects_hnsw_v4_1024
    ON asset_embedding_objects
    USING hnsw ((embedding::vector(1024)) vector_cosine_ops)
    WITH (m = 16, ef_construction = 64)
    WHERE status = 'ready'
      AND model = 'text-embedding-v4'
      AND dimension = 1024;

CREATE INDEX IF NOT EXISTS idx_asset_embedding_objects_ready_lookup
    ON asset_embedding_objects (provider_id, model, dimension, object_type, asset_id)
    WHERE status = 'ready';

-- +goose Down
DROP INDEX IF EXISTS idx_asset_embedding_objects_ready_lookup;
DROP INDEX IF EXISTS idx_asset_embedding_objects_hnsw_v4_1024;
