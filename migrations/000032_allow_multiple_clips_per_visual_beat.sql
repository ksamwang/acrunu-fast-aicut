-- +goose Up
ALTER TABLE clip_segments
    DROP CONSTRAINT IF EXISTS clip_segments_edit_plan_id_visual_beat_id_key;

-- segment_index remains the unique ordering key for every clip in an edit plan.
CREATE INDEX IF NOT EXISTS idx_clip_segments_edit_plan_visual_beat
    ON clip_segments (edit_plan_id, visual_beat_id, segment_index);

-- +goose Down
DROP INDEX IF EXISTS idx_clip_segments_edit_plan_visual_beat;

-- The previous schema cannot represent these plans without corrupting their timeline.
DELETE FROM edit_plans
WHERE id IN (
    SELECT edit_plan_id
    FROM clip_segments
    GROUP BY edit_plan_id, visual_beat_id
    HAVING count(*) > 1
);

ALTER TABLE clip_segments
    ADD CONSTRAINT clip_segments_edit_plan_id_visual_beat_id_key
        UNIQUE (edit_plan_id, visual_beat_id);
