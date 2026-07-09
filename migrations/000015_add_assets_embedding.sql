-- +goose Up
ALTER TABLE assets
    ADD COLUMN IF NOT EXISTS embedding JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE assets
    DROP COLUMN IF EXISTS embedding;
