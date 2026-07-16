-- +goose Up
-- Keep historical resources after an account is removed. Upload tokens are
-- short-lived credentials and can be discarded with their owner.
ALTER TABLE upload_tokens
    DROP CONSTRAINT IF EXISTS upload_tokens_user_id_fkey,
    ADD CONSTRAINT upload_tokens_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE system_configs
    DROP CONSTRAINT IF EXISTS system_configs_updated_by_user_id_fkey,
    ADD CONSTRAINT system_configs_updated_by_user_id_fkey
        FOREIGN KEY (updated_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE products
    DROP CONSTRAINT IF EXISTS products_created_by_user_id_fkey,
    DROP CONSTRAINT IF EXISTS products_updated_by_user_id_fkey,
    ADD CONSTRAINT products_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT products_updated_by_user_id_fkey
        FOREIGN KEY (updated_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE product_selling_points
    DROP CONSTRAINT IF EXISTS product_selling_points_created_by_user_id_fkey,
    DROP CONSTRAINT IF EXISTS product_selling_points_updated_by_user_id_fkey,
    ADD CONSTRAINT product_selling_points_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT product_selling_points_updated_by_user_id_fkey
        FOREIGN KEY (updated_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE assets
    DROP CONSTRAINT IF EXISTS assets_created_by_user_id_fkey,
    DROP CONSTRAINT IF EXISTS assets_updated_by_user_id_fkey,
    ADD CONSTRAINT assets_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT assets_updated_by_user_id_fkey
        FOREIGN KEY (updated_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE generation_tasks
    DROP CONSTRAINT IF EXISTS generation_tasks_created_by_user_id_fkey,
    ADD CONSTRAINT generation_tasks_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE speech_segments
    DROP CONSTRAINT IF EXISTS speech_segments_created_by_user_id_fkey,
    DROP CONSTRAINT IF EXISTS speech_segments_updated_by_user_id_fkey,
    ADD CONSTRAINT speech_segments_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT speech_segments_updated_by_user_id_fkey
        FOREIGN KEY (updated_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE voice_profiles
    DROP CONSTRAINT IF EXISTS voice_profiles_created_by_user_id_fkey,
    DROP CONSTRAINT IF EXISTS voice_profiles_updated_by_user_id_fkey,
    ADD CONSTRAINT voice_profiles_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT voice_profiles_updated_by_user_id_fkey
        FOREIGN KEY (updated_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE voice_auditions
    DROP CONSTRAINT IF EXISTS voice_auditions_created_by_user_id_fkey,
    ADD CONSTRAINT voice_auditions_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE generation_runs
    DROP CONSTRAINT IF EXISTS generation_runs_created_by_user_id_fkey,
    ADD CONSTRAINT generation_runs_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

INSERT INTO users (username, display_name, password_hash, role, status)
VALUES (
    'admin',
    '管理员',
    crypt('admin123', gen_salt('bf', 12)),
    'admin',
    'active'
)
ON CONFLICT (username) DO NOTHING;

-- +goose Down
-- The bootstrap account may have existed before this migration or been edited
-- afterwards, so a rollback must not delete it.

ALTER TABLE generation_runs
    DROP CONSTRAINT IF EXISTS generation_runs_created_by_user_id_fkey,
    ADD CONSTRAINT generation_runs_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id);

ALTER TABLE voice_auditions
    DROP CONSTRAINT IF EXISTS voice_auditions_created_by_user_id_fkey,
    ADD CONSTRAINT voice_auditions_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id);

ALTER TABLE voice_profiles
    DROP CONSTRAINT IF EXISTS voice_profiles_created_by_user_id_fkey,
    DROP CONSTRAINT IF EXISTS voice_profiles_updated_by_user_id_fkey,
    ADD CONSTRAINT voice_profiles_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id),
    ADD CONSTRAINT voice_profiles_updated_by_user_id_fkey
        FOREIGN KEY (updated_by_user_id) REFERENCES users(id);

ALTER TABLE speech_segments
    DROP CONSTRAINT IF EXISTS speech_segments_created_by_user_id_fkey,
    DROP CONSTRAINT IF EXISTS speech_segments_updated_by_user_id_fkey,
    ADD CONSTRAINT speech_segments_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id),
    ADD CONSTRAINT speech_segments_updated_by_user_id_fkey
        FOREIGN KEY (updated_by_user_id) REFERENCES users(id);

ALTER TABLE generation_tasks
    DROP CONSTRAINT IF EXISTS generation_tasks_created_by_user_id_fkey,
    ADD CONSTRAINT generation_tasks_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id);

ALTER TABLE assets
    DROP CONSTRAINT IF EXISTS assets_created_by_user_id_fkey,
    DROP CONSTRAINT IF EXISTS assets_updated_by_user_id_fkey,
    ADD CONSTRAINT assets_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id),
    ADD CONSTRAINT assets_updated_by_user_id_fkey
        FOREIGN KEY (updated_by_user_id) REFERENCES users(id);

ALTER TABLE product_selling_points
    DROP CONSTRAINT IF EXISTS product_selling_points_created_by_user_id_fkey,
    DROP CONSTRAINT IF EXISTS product_selling_points_updated_by_user_id_fkey,
    ADD CONSTRAINT product_selling_points_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id),
    ADD CONSTRAINT product_selling_points_updated_by_user_id_fkey
        FOREIGN KEY (updated_by_user_id) REFERENCES users(id);

ALTER TABLE products
    DROP CONSTRAINT IF EXISTS products_created_by_user_id_fkey,
    DROP CONSTRAINT IF EXISTS products_updated_by_user_id_fkey,
    ADD CONSTRAINT products_created_by_user_id_fkey
        FOREIGN KEY (created_by_user_id) REFERENCES users(id),
    ADD CONSTRAINT products_updated_by_user_id_fkey
        FOREIGN KEY (updated_by_user_id) REFERENCES users(id);

ALTER TABLE system_configs
    DROP CONSTRAINT IF EXISTS system_configs_updated_by_user_id_fkey,
    ADD CONSTRAINT system_configs_updated_by_user_id_fkey
        FOREIGN KEY (updated_by_user_id) REFERENCES users(id);

ALTER TABLE upload_tokens
    DROP CONSTRAINT IF EXISTS upload_tokens_user_id_fkey,
    ADD CONSTRAINT upload_tokens_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id);
