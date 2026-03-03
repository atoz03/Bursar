CREATE TABLE IF NOT EXISTS node_policies (
    node_id VARCHAR(50) PRIMARY KEY,
    ssh_guard_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

