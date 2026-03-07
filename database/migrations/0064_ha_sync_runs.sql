-- 0064_ha_sync_runs.sql
-- 容灾同步日志（支持手动/定时/回切操作审计）

CREATE TABLE IF NOT EXISTS ha_sync_runs (
    run_id BIGSERIAL PRIMARY KEY,
    trigger_mode VARCHAR(32) NOT NULL DEFAULT 'manual',
    direction VARCHAR(32) NOT NULL DEFAULT 'primary_to_standby',
    status VARCHAR(24) NOT NULL DEFAULT 'running',
    started_by VARCHAR(64) NOT NULL DEFAULT 'system',
    started_at TIMESTAMP NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMP NULL,
    summary TEXT NOT NULL DEFAULT '',
    detail_json JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_ha_sync_runs_started_at_desc ON ha_sync_runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_ha_sync_runs_status_started_at ON ha_sync_runs(status, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_ha_sync_runs_trigger_started_at ON ha_sync_runs(trigger_mode, started_at DESC);
