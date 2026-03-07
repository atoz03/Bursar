CREATE TABLE IF NOT EXISTS user_node_unbind_records (
    record_id BIGSERIAL PRIMARY KEY,
    source_type VARCHAR(20) NOT NULL DEFAULT 'user_request', -- user_request, admin_forced
    request_id INT NULL,
    billing_username VARCHAR(50) NOT NULL,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, approved, rejected, forced
    reason TEXT NOT NULL DEFAULT '',
    initiated_by VARCHAR(50) NOT NULL DEFAULT '',
    reviewed_by VARCHAR(50) NULL,
    reviewed_at TIMESTAMP NULL,
    executed_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_node_unbind_records_request
    ON user_node_unbind_records(request_id);
CREATE INDEX IF NOT EXISTS idx_user_node_unbind_records_created
    ON user_node_unbind_records(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_node_unbind_records_node_local
    ON user_node_unbind_records(node_id, local_username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_node_unbind_records_billing
    ON user_node_unbind_records(billing_username, created_at DESC);
