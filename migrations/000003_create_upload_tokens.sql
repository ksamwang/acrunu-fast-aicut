-- +goose Up
CREATE TABLE IF NOT EXISTS upload_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash TEXT NOT NULL UNIQUE,
    user_id UUID NOT NULL REFERENCES users(id),
    product_id UUID,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'used', 'expired', 'revoked')),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_upload_tokens_user_id ON upload_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_upload_tokens_product_id ON upload_tokens(product_id);
CREATE INDEX IF NOT EXISTS idx_upload_tokens_status ON upload_tokens(status);
CREATE INDEX IF NOT EXISTS idx_upload_tokens_expires_at ON upload_tokens(expires_at);

-- +goose Down
DROP TABLE IF EXISTS upload_tokens;
