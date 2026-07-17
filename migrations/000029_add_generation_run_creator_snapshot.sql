-- +goose Up
ALTER TABLE generation_runs
    ADD COLUMN IF NOT EXISTS created_by_name_snapshot TEXT NOT NULL DEFAULT '';

UPDATE generation_runs AS run
SET created_by_name_snapshot = COALESCE(NULLIF(BTRIM(app_user.display_name), ''), app_user.username, '')
FROM users AS app_user
WHERE run.created_by_user_id = app_user.id
  AND run.created_by_name_snapshot = '';

-- +goose Down
ALTER TABLE generation_runs
    DROP COLUMN IF EXISTS created_by_name_snapshot;
