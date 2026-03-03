ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS ssh_exclusive_enabled BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS node_exclusive_users (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_node_exclusive_users_node ON node_exclusive_users(node_id);
