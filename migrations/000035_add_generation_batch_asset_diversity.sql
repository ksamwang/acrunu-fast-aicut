-- +goose Up
ALTER TABLE generation_runs
    ADD COLUMN IF NOT EXISTS generation_batch_id UUID;

UPDATE generation_runs
SET generation_batch_id = id
WHERE generation_batch_id IS NULL;

ALTER TABLE generation_runs
    ALTER COLUMN generation_batch_id SET NOT NULL,
    ALTER COLUMN generation_batch_id SET DEFAULT gen_random_uuid();

CREATE INDEX IF NOT EXISTS idx_generation_runs_batch_created_at
    ON generation_runs (generation_batch_id, created_at);

CREATE TABLE IF NOT EXISTS generation_asset_selections (
    generation_run_id UUID NOT NULL REFERENCES generation_runs(id) ON DELETE CASCADE,
    generation_batch_id UUID NOT NULL,
    asset_id UUID NOT NULL REFERENCES assets(id),
    reuse_key TEXT NOT NULL CHECK (BTRIM(reuse_key) <> ''),
    state TEXT NOT NULL CHECK (state IN ('reserved', 'committed')),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (generation_run_id, reuse_key),
    CHECK (
        (state = 'reserved' AND expires_at IS NOT NULL)
        OR (state = 'committed' AND expires_at IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_generation_asset_selections_batch_reuse
    ON generation_asset_selections (generation_batch_id, reuse_key);

CREATE INDEX IF NOT EXISTS idx_generation_asset_selections_asset_created
    ON generation_asset_selections (asset_id, created_at DESC);

INSERT INTO generation_asset_selections (
    generation_run_id,
    generation_batch_id,
    asset_id,
    reuse_key,
    state,
    expires_at,
    created_at,
    updated_at
)
SELECT DISTINCT ON (legacy.generation_run_id, legacy.reuse_key)
    legacy.generation_run_id,
    legacy.generation_batch_id,
    legacy.asset_id,
    legacy.reuse_key,
    'committed',
    NULL,
    legacy.created_at,
    legacy.created_at
FROM (
    SELECT
        runs.id AS generation_run_id,
        runs.generation_batch_id,
        clips.asset_id,
        COALESCE(NULLIF(LOWER(BTRIM(assets.checksum)), ''), clips.asset_id::text) AS reuse_key,
        clips.created_at
    FROM clip_segments clips
    JOIN edit_plans plans ON plans.id = clips.edit_plan_id
    JOIN generation_runs runs ON runs.id = plans.generation_run_id
    JOIN assets ON assets.id = clips.asset_id
    WHERE plans.status = 'ready'
) AS legacy
ORDER BY legacy.generation_run_id, legacy.reuse_key, legacy.created_at
ON CONFLICT (generation_run_id, reuse_key) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS generation_asset_selections;

DROP INDEX IF EXISTS idx_generation_runs_batch_created_at;

ALTER TABLE generation_runs
    DROP COLUMN IF EXISTS generation_batch_id;
