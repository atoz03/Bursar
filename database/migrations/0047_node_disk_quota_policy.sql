ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS disk_quota_installed BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS disk_quota_mounts TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[];

ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS disk_quota_enabled BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS disk_quota_mountpoint TEXT NULL;

ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS disk_quota_soft_mb DOUBLE PRECISION NULL;

ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS disk_quota_hard_mb DOUBLE PRECISION NULL;

CREATE TABLE IF NOT EXISTS node_user_disk_quotas (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    mountpoint TEXT NOT NULL,
    used_mb DOUBLE PRECISION NOT NULL DEFAULT 0,
    soft_mb DOUBLE PRECISION NOT NULL DEFAULT 0,
    hard_mb DOUBLE PRECISION NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username, mountpoint)
);

CREATE INDEX IF NOT EXISTS idx_node_user_disk_quotas_node_mount
    ON node_user_disk_quotas(node_id, mountpoint, updated_at DESC);
