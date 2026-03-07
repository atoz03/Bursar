CREATE TABLE IF NOT EXISTS node_user_memory_limits (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    memory_limit_gb DOUBLE PRECISION NOT NULL CHECK (memory_limit_gb > 0),
    reason TEXT NOT NULL DEFAULT '',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_node_user_memory_limits_node_updated
    ON node_user_memory_limits(node_id, updated_at DESC);
