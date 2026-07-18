-- +goose Up
CREATE TABLE IF NOT EXISTS workbench_script_generation_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    mode TEXT NOT NULL CHECK (mode IN ('replace_all', 'replace_variant')),
    target_variant_id UUID,
    base_revision TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'generating', 'completed', 'failed', 'cancelled', 'applied', 'discarded')),
    input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    result_variants JSONB NOT NULL DEFAULT '[]'::jsonb,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_workbench_script_jobs_one_active_per_user
    ON workbench_script_generation_jobs (created_by_user_id)
    WHERE status IN ('queued', 'generating');

CREATE INDEX IF NOT EXISTS idx_workbench_script_jobs_user_created
    ON workbench_script_generation_jobs (created_by_user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS workbench_script_generation_jobs;
