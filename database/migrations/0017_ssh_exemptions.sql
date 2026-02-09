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

-- 保留历史运维要求：gpuops 默认全局豁免
INSERT INTO ssh_exemptions(node_id, local_username, created_by)
VALUES('*', 'gpuops', 'system')
ON CONFLICT (node_id, local_username) DO NOTHING;
