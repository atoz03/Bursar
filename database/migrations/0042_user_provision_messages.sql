CREATE TABLE IF NOT EXISTS user_provision_messages (
    message_id BIGSERIAL PRIMARY KEY,
    billing_username VARCHAR(50) NOT NULL,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(50) NOT NULL,
    encrypted_payload TEXT NOT NULL,
    decrypt_url TEXT NOT NULL DEFAULT '/user/accounts?tool=key-decryptor',
    ssh_host VARCHAR(255) NOT NULL DEFAULT '',
    ssh_port INT NOT NULL DEFAULT 22,
    download_filename VARCHAR(255) NOT NULL DEFAULT '',
    ssh_command TEXT NOT NULL DEFAULT '',
    mail_to VARCHAR(120) NOT NULL DEFAULT '',
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_provision_messages_user_created
    ON user_provision_messages(billing_username, created_at DESC);
