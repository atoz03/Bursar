CREATE TABLE IF NOT EXISTS node_user_cpu_limits (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    cpu_quota_percent DOUBLE PRECISION NOT NULL CHECK (cpu_quota_percent > 0 AND cpu_quota_percent <= 100),
    reason TEXT NOT NULL DEFAULT '',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_node_user_cpu_limits_node_updated
    ON node_user_cpu_limits(node_id, updated_at DESC);
