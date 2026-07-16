-- +goose Up
CREATE TABLE IF NOT EXISTS visual_beats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    edit_plan_id UUID NOT NULL REFERENCES edit_plans(id) ON DELETE CASCADE,
    beat_index INTEGER NOT NULL CHECK (beat_index >= 0),
    narration_segment_id UUID NOT NULL REFERENCES narration_segments(id),
    start_ms INTEGER NOT NULL CHECK (start_ms >= 0),
    end_ms INTEGER NOT NULL CHECK (end_ms > start_ms),
    label TEXT NOT NULL DEFAULT '',
    selling_point TEXT NOT NULL DEFAULT '',
    visual_goal TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL CHECK (source_type IN ('visual_only', 'talking_head', 'mixed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (edit_plan_id, beat_index)
);

CREATE INDEX IF NOT EXISTS idx_visual_beats_edit_plan_timeline
    ON visual_beats (edit_plan_id, start_ms);

ALTER TABLE clip_segments
    ADD COLUMN IF NOT EXISTS visual_beat_id UUID;

-- Every historical one-clip narration segment becomes a single visual beat.
-- Reusing the clip UUID keeps existing plans deterministic during the transition.
INSERT INTO visual_beats (
    id, edit_plan_id, beat_index, narration_segment_id,
    start_ms, end_ms, label, selling_point, visual_goal, source_type
)
SELECT
    c.id,
    c.edit_plan_id,
    c.segment_index,
    c.narration_segment_id,
    c.timeline_in_ms,
    c.timeline_in_ms + c.timeline_duration_ms,
    c.label,
    '',
    c.visual_goal,
    c.source_type
FROM clip_segments c
ON CONFLICT (id) DO NOTHING;

UPDATE clip_segments
SET visual_beat_id = id
WHERE visual_beat_id IS NULL;

ALTER TABLE clip_segments
    ALTER COLUMN visual_beat_id SET NOT NULL,
    DROP CONSTRAINT IF EXISTS clip_segments_edit_plan_id_narration_segment_id_key,
    ADD CONSTRAINT clip_segments_visual_beat_id_fkey
        FOREIGN KEY (visual_beat_id) REFERENCES visual_beats(id),
    ADD CONSTRAINT clip_segments_edit_plan_id_visual_beat_id_key
        UNIQUE (edit_plan_id, visual_beat_id);

CREATE INDEX IF NOT EXISTS idx_clip_segments_visual_beat_id
    ON clip_segments (visual_beat_id);

-- +goose Down
DROP INDEX IF EXISTS idx_clip_segments_visual_beat_id;

ALTER TABLE clip_segments
    DROP CONSTRAINT IF EXISTS clip_segments_edit_plan_id_visual_beat_id_key,
    DROP CONSTRAINT IF EXISTS clip_segments_visual_beat_id_fkey,
    DROP COLUMN IF EXISTS visual_beat_id,
    ADD CONSTRAINT clip_segments_edit_plan_id_narration_segment_id_key
        UNIQUE (edit_plan_id, narration_segment_id);

DROP INDEX IF EXISTS idx_visual_beats_edit_plan_timeline;
DROP TABLE IF EXISTS visual_beats;
