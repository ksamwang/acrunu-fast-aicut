-- +goose Up
ALTER TABLE visual_beats
    ADD COLUMN IF NOT EXISTS duration_class TEXT NOT NULL DEFAULT 'legacy';

ALTER TABLE visual_beats
    DROP CONSTRAINT IF EXISTS visual_beats_duration_class_check,
    ADD CONSTRAINT visual_beats_duration_class_check
        CHECK (duration_class IN ('legacy', 'brief', 'standard', 'action'));

-- +goose Down
ALTER TABLE visual_beats
    DROP CONSTRAINT IF EXISTS visual_beats_duration_class_check,
    DROP COLUMN IF EXISTS duration_class;
