CREATE TABLE IF NOT EXISTS account_provision_logs (
    provision_id BIGSERIAL PRIMARY KEY,
    billing_username VARCHAR(50) NOT NULL,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(50) NOT NULL,
    email VARCHAR(120) NOT NULL,
    ssh_host VARCHAR(255) NOT NULL DEFAULT '',
    ssh_port INT NOT NULL DEFAULT 22,
    download_filename VARCHAR(255) NOT NULL DEFAULT '',
    mail_sent BOOLEAN NOT NULL DEFAULT FALSE,
    mail_error TEXT NOT NULL DEFAULT '',
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_account_provision_logs_created_at
    ON account_provision_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_account_provision_logs_billing
    ON account_provision_logs(billing_username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_account_provision_logs_node_local
    ON account_provision_logs(node_id, local_username, created_at DESC);
