ALTER TABLE nodes
  ADD COLUMN IF NOT EXISTS os_version TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS kernel_version TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS node_local_users (
  node_id VARCHAR(50) NOT NULL,
  local_username VARCHAR(64) NOT NULL,
  home_created_at TIMESTAMP NULL,
  last_login_at TIMESTAMP NULL,
  home_used_gb DOUBLE PRECISION NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_node_local_users_node_updated
ON node_local_users(node_id, updated_at DESC);
