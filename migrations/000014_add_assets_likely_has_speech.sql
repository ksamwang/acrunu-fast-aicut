-- +goose Up
ALTER TABLE assets
    ADD COLUMN IF NOT EXISTS likely_has_speech BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE assets
    DROP COLUMN IF EXISTS likely_has_speech;
