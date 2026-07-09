-- +goose Up
ALTER TABLE assets
    ADD COLUMN IF NOT EXISTS model_labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS review_overrides JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE assets
    DROP COLUMN IF EXISTS review_overrides,
    DROP COLUMN IF EXISTS model_labels;
