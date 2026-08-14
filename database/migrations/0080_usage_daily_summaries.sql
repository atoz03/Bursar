-- 运营看板按天聚合层：避免每次打开看板都扫描完整 usage_records。
CREATE TABLE IF NOT EXISTS usage_daily_summaries (
    usage_date DATE NOT NULL,
    node_id VARCHAR(50) NOT NULL,
    username VARCHAR(50) NOT NULL,
    usage_records BIGINT NOT NULL DEFAULT 0,
    gpu_process_records BIGINT NOT NULL DEFAULT 0,
    cpu_process_records BIGINT NOT NULL DEFAULT 0,
    cpu_active_seconds BIGINT NOT NULL DEFAULT 0,
    gpu_active_seconds BIGINT NOT NULL DEFAULT 0,
    total_cpu_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    total_memory_mb DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_cost NUMERIC(18,4) NOT NULL DEFAULT 0,
    gpu_cost NUMERIC(18,4) NOT NULL DEFAULT 0,
    total_cost NUMERIC(18,4) NOT NULL DEFAULT 0,
    last_usage_at TIMESTAMP NOT NULL,
    PRIMARY KEY (usage_date, node_id, username)
);

CREATE INDEX IF NOT EXISTS idx_usage_daily_summaries_username_date
    ON usage_daily_summaries(username, usage_date);

-- 首次升级时一次性回填历史数据。之后由控制器在写入 usage_records 时增量维护。
INSERT INTO usage_daily_summaries (
    usage_date, node_id, username,
    usage_records, gpu_process_records, cpu_process_records,
    cpu_active_seconds, gpu_active_seconds,
    total_cpu_percent, total_memory_mb, cpu_cost, gpu_cost, total_cost, last_usage_at
)
SELECT
    ur.timestamp::date,
    ur.node_id,
    ur.username,
    COUNT(1),
    SUM(CASE WHEN ur.gpu_count > 0 THEN 1 ELSE 0 END),
    SUM(CASE WHEN ur.gpu_count = 0 THEN 1 ELSE 0 END),
    SUM(CASE WHEN ur.cpu_percent > 0 THEN GREATEST(COALESCE(n.interval_seconds, 1), 1) ELSE 0 END),
    SUM(CASE WHEN ur.gpu_count > 0 THEN GREATEST(COALESCE(n.interval_seconds, 1), 1) ELSE 0 END),
    COALESCE(SUM(ur.cpu_percent), 0),
    COALESCE(SUM(ur.memory_mb), 0),
    COALESCE(SUM(CASE WHEN ur.gpu_count = 0 THEN ur.cost ELSE 0 END), 0),
    COALESCE(SUM(CASE WHEN ur.gpu_count > 0 THEN ur.cost ELSE 0 END), 0),
    COALESCE(SUM(ur.cost), 0),
    MAX(ur.timestamp)
FROM usage_records ur
LEFT JOIN nodes n ON n.node_id = ur.node_id
GROUP BY ur.timestamp::date, ur.node_id, ur.username
ON CONFLICT (usage_date, node_id, username) DO NOTHING;
