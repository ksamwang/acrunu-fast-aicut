-- +goose Up
CREATE TABLE IF NOT EXISTS voiceover_replacements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    generation_run_id UUID NOT NULL REFERENCES generation_runs(id) ON DELETE CASCADE,
    generation_task_id UUID NOT NULL UNIQUE REFERENCES generation_tasks(id) ON DELETE CASCADE,
    script_variant_id UUID NOT NULL UNIQUE REFERENCES script_variants(id) ON DELETE CASCADE,
    voiceover_id UUID NOT NULL UNIQUE REFERENCES voiceovers(id) ON DELETE CASCADE,
    render_task_id UUID UNIQUE REFERENCES generation_tasks(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'generating' CHECK (status IN ('generating', 'ready', 'applying', 'applied', 'failed', 'cancelled')),
    error_message TEXT,
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_voiceover_replacements_run_created_at
    ON voiceover_replacements (generation_run_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_voiceover_replacements_one_active_per_run
    ON voiceover_replacements (generation_run_id)
    WHERE status IN ('generating', 'ready', 'applying');

-- +goose Down
DROP TABLE IF EXISTS voiceover_replacements;
