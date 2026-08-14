-- 节点实时监控状态：整机 CPU/内存与逐 GPU 指标。
CREATE TABLE IF NOT EXISTS node_monitor_status (
    node_id VARCHAR(50) PRIMARY KEY REFERENCES nodes(node_id) ON DELETE CASCADE,
    report_ts TIMESTAMP NOT NULL,
    metrics_available BOOLEAN NOT NULL DEFAULT FALSE,
    host_cpu_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    host_memory_total_mb DOUBLE PRECISION NOT NULL DEFAULT 0,
    host_memory_used_mb DOUBLE PRECISION NOT NULL DEFAULT 0,
    host_load_1 DOUBLE PRECISION NOT NULL DEFAULT 0,
    host_load_5 DOUBLE PRECISION NOT NULL DEFAULT 0,
    host_load_15 DOUBLE PRECISION NOT NULL DEFAULT 0,
    host_uptime_seconds BIGINT NOT NULL DEFAULT 0,
    agent_memory_mb DOUBLE PRECISION NOT NULL DEFAULT 0,
    gpu_devices JSONB NOT NULL DEFAULT '[]'::JSONB,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_node_monitor_status_report_ts
ON node_monitor_status(report_ts DESC);
