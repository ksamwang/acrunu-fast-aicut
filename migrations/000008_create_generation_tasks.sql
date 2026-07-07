-- +goose Up
CREATE TABLE IF NOT EXISTS generation_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id),
    created_by_user_id UUID REFERENCES users(id),
    task_type TEXT NOT NULL DEFAULT 'batch_video' CHECK (task_type IN ('batch_video')),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    variant_count INTEGER NOT NULL DEFAULT 1,
    target_duration_ms INTEGER,
    style_prompt TEXT,
    config_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_generation_tasks_product_id ON generation_tasks(product_id);
CREATE INDEX IF NOT EXISTS idx_generation_tasks_created_by_user_id ON generation_tasks(created_by_user_id);
CREATE INDEX IF NOT EXISTS idx_generation_tasks_status ON generation_tasks(status);
CREATE INDEX IF NOT EXISTS idx_generation_tasks_created_at ON generation_tasks(created_at);

-- +goose Down
DROP TABLE IF EXISTS generation_tasks;
