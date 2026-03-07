ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS ssh_exclusive_block_others BOOLEAN NOT NULL DEFAULT TRUE;

CREATE TABLE IF NOT EXISTS node_exclusive_gpu_users (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    gpu_index INTEGER NOT NULL CHECK (gpu_index >= 0),
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, gpu_index)
);

CREATE INDEX IF NOT EXISTS idx_node_exclusive_gpu_users_node_user
    ON node_exclusive_gpu_users(node_id, local_username);
