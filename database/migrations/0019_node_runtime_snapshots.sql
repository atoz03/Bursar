-- 0019_node_runtime_snapshots.sql：
-- 节点运行时快照（用于节点详情页实时趋势与 SSH 在线用户展示）

CREATE TABLE IF NOT EXISTS node_runtime_snapshots (
    report_id VARCHAR(64) PRIMARY KEY,
    node_id VARCHAR(50) NOT NULL,
    report_ts TIMESTAMP NOT NULL,
    cpu_percent_sum DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_mb_sum DOUBLE PRECISION NOT NULL DEFAULT 0,
    gpu_process_count INT NOT NULL DEFAULT 0,
    cpu_process_count INT NOT NULL DEFAULT 0,
    ssh_user_count INT NOT NULL DEFAULT 0,
    ssh_users_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    cost_total DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_node_runtime_snapshots_node_ts
ON node_runtime_snapshots(node_id, report_ts DESC);

