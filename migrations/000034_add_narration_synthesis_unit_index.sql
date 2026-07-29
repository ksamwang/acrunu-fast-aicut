-- +goose Up
ALTER TABLE narration_segments
    ADD COLUMN IF NOT EXISTS synthesis_unit_index INTEGER;

ALTER TABLE narration_segments
    DROP CONSTRAINT IF EXISTS narration_segments_synthesis_unit_index_check,
    ADD CONSTRAINT narration_segments_synthesis_unit_index_check
    CHECK (synthesis_unit_index IS NULL OR synthesis_unit_index >= 0);

-- +goose Down
ALTER TABLE narration_segments
    DROP CONSTRAINT IF EXISTS narration_segments_synthesis_unit_index_check;

ALTER TABLE narration_segments
    DROP COLUMN IF EXISTS synthesis_unit_index;
