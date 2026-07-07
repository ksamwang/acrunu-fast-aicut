-- +goose Up
CREATE TABLE IF NOT EXISTS assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id),
    storage_key TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_ext TEXT,
    mime_type TEXT,
    file_size BIGINT NOT NULL DEFAULT 0,
    checksum TEXT,
    source_type TEXT NOT NULL CHECK (source_type IN ('visual_only', 'talking_head')),
    duration_ms INTEGER,
    width INTEGER,
    height INTEGER,
    fps NUMERIC(10, 4),
    codec TEXT,
    status TEXT NOT NULL DEFAULT 'uploaded' CHECK (status IN ('uploaded', 'analyzing', 'ready', 'failed', 'archived')),
    manual_clean_status TEXT NOT NULL DEFAULT 'cleaned' CHECK (manual_clean_status IN ('cleaned', 'needs_review')),
    source_original_name TEXT,
    source_in_ms INTEGER,
    source_out_ms INTEGER,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by_user_id UUID REFERENCES users(id),
    updated_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_assets_product_id ON assets(product_id);
CREATE INDEX IF NOT EXISTS idx_assets_source_type ON assets(source_type);
CREATE INDEX IF NOT EXISTS idx_assets_status ON assets(status);
CREATE INDEX IF NOT EXISTS idx_assets_created_by_user_id ON assets(created_by_user_id);
CREATE INDEX IF NOT EXISTS idx_assets_checksum ON assets(checksum);

-- +goose Down
DROP TABLE IF EXISTS assets;
