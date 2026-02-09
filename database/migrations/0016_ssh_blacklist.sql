-- 0016_ssh_blacklist.sql：SSH 黑名单

CREATE TABLE IF NOT EXISTS ssh_blacklist (
    node_id VARCHAR(50) NOT NULL,         -- 具体节点或 "*" 表示所有节点
    local_username VARCHAR(50) NOT NULL,
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_ssh_blacklist_user ON ssh_blacklist(local_username);
