-- 0017_ssh_exemptions.sql：SSH 豁免名单（最高优先级）

CREATE TABLE IF NOT EXISTS ssh_exemptions (
    node_id VARCHAR(50) NOT NULL,         -- 具体节点或 "*" 表示所有节点
    local_username VARCHAR(50) NOT NULL,
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_ssh_exemptions_user ON ssh_exemptions(local_username);
