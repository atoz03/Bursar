CREATE TABLE IF NOT EXISTS ssh_list_sources (
  list_type VARCHAR(20) NOT NULL CHECK (list_type IN ('whitelist', 'blacklist', 'exemptions')),
  node_id VARCHAR(50) NOT NULL,
  local_username VARCHAR(50) NOT NULL,
  source_type VARCHAR(20) NOT NULL DEFAULT 'local' CHECK (source_type IN ('local', 'platform')),
  source_platform_username VARCHAR(50) NOT NULL DEFAULT '',
  updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  PRIMARY KEY (list_type, node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_ssh_list_sources_node ON ssh_list_sources(list_type, node_id);
CREATE INDEX IF NOT EXISTS idx_ssh_list_sources_local ON ssh_list_sources(list_type, local_username);
