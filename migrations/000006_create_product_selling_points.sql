-- +goose Up
CREATE TABLE IF NOT EXISTS product_selling_points (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    priority INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_by_user_id UUID REFERENCES users(id),
    updated_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_product_selling_points_product_id ON product_selling_points(product_id);
CREATE INDEX IF NOT EXISTS idx_product_selling_points_status ON product_selling_points(status);

-- +goose Down
DROP TABLE IF EXISTS product_selling_points;
