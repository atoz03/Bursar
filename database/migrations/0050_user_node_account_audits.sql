CREATE TABLE IF NOT EXISTS user_node_account_audits (
    audit_id BIGSERIAL PRIMARY KEY,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    old_billing_username VARCHAR(50) NOT NULL DEFAULT '',
    new_billing_username VARCHAR(50) NOT NULL DEFAULT '',
    action VARCHAR(40) NOT NULL DEFAULT '',
    operator VARCHAR(50) NOT NULL DEFAULT '',
    source VARCHAR(20) NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_node_account_audits_node_local_time
    ON user_node_account_audits(node_id, local_username, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_node_account_audits_created
    ON user_node_account_audits(created_at DESC);
