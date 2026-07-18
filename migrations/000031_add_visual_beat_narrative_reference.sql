-- +goose Up
ALTER TABLE visual_beats
    ADD COLUMN IF NOT EXISTS narrative_beat_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE visual_beats
    DROP COLUMN IF EXISTS narrative_beat_id;
