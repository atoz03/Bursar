CREATE TABLE IF NOT EXISTS process_kill_records (
    record_id BIGSERIAL PRIMARY KEY,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL DEFAULT '',
    billing_username VARCHAR(50) NOT NULL DEFAULT '',
    action_type VARCHAR(40) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    pids_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_process_kill_records_created_at
    ON process_kill_records(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_process_kill_records_billing_created
    ON process_kill_records(billing_username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_process_kill_records_node_local_created
    ON process_kill_records(node_id, local_username, created_at DESC);
