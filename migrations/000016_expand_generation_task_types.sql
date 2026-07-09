-- +goose Up
ALTER TABLE generation_tasks
    DROP CONSTRAINT IF EXISTS generation_tasks_task_type_check;

ALTER TABLE generation_tasks
    ADD CONSTRAINT generation_tasks_task_type_check
    CHECK (task_type IN ('batch_video', 'test', 'asset_extract_frames', 'asset_analyze'));

-- +goose Down
ALTER TABLE generation_tasks
    DROP CONSTRAINT IF EXISTS generation_tasks_task_type_check;

ALTER TABLE generation_tasks
    ADD CONSTRAINT generation_tasks_task_type_check
    CHECK (task_type IN ('batch_video', 'test'));
