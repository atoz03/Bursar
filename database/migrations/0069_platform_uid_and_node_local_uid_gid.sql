ALTER TABLE user_accounts
    ADD COLUMN IF NOT EXISTS platform_uid INT;

ALTER TABLE deleted_user_accounts
    ADD COLUMN IF NOT EXISTS platform_uid INT;

ALTER TABLE node_local_users
    ADD COLUMN IF NOT EXISTS uid INT;

ALTER TABLE node_local_users
    ADD COLUMN IF NOT EXISTS primary_gid INT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_accounts_platform_uid
    ON user_accounts(platform_uid)
    WHERE platform_uid IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_deleted_user_accounts_platform_uid_active
    ON deleted_user_accounts(platform_uid, deleted_at DESC)
    WHERE platform_uid IS NOT NULL AND restored_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_node_local_users_uid
    ON node_local_users(uid)
    WHERE uid IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_node_local_users_primary_gid
    ON node_local_users(primary_gid)
    WHERE primary_gid IS NOT NULL;
