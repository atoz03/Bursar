CREATE TABLE IF NOT EXISTS node_user_gpu_visibility (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    gpu_index INTEGER NOT NULL CHECK (gpu_index >= 0),
    reason TEXT NOT NULL DEFAULT '',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username, gpu_index)
);

CREATE INDEX IF NOT EXISTS idx_node_user_gpu_visibility_node_updated
    ON node_user_gpu_visibility(node_id, local_username, updated_at DESC);
