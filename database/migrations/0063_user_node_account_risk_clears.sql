CREATE TABLE IF NOT EXISTS user_node_account_risk_clears (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    cleared_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    cleared_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_user_node_account_risk_clears_cleared_at
    ON user_node_account_risk_clears(cleared_at DESC);
