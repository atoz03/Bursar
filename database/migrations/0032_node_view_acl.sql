CREATE TABLE IF NOT EXISTS node_view_acl (
    node_id VARCHAR(50) NOT NULL,
    power_username VARCHAR(50) NOT NULL,
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, power_username)
);

CREATE INDEX IF NOT EXISTS idx_node_view_acl_power_username
    ON node_view_acl(power_username);
