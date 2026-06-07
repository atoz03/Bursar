-- 0078_ssh_temporary_users.sql：临时用户名单

CREATE TABLE IF NOT EXISTS ssh_temporary_users (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(50) NOT NULL,
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_ssh_temporary_users_user ON ssh_temporary_users(local_username);

ALTER TABLE ssh_list_sources DROP CONSTRAINT IF EXISTS ssh_list_sources_list_type_check;
ALTER TABLE ssh_list_sources
  ADD CONSTRAINT ssh_list_sources_list_type_check
  CHECK (list_type IN ('whitelist', 'blacklist', 'exemptions', 'temporary_users'));
