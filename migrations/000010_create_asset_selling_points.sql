-- +goose Up
CREATE TABLE IF NOT EXISTS asset_selling_points (
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    selling_point_id UUID NOT NULL REFERENCES product_selling_points(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (asset_id, selling_point_id)
);

CREATE INDEX IF NOT EXISTS idx_asset_selling_points_selling_point_id ON asset_selling_points(selling_point_id);

-- +goose Down
DROP TABLE IF EXISTS asset_selling_points;
