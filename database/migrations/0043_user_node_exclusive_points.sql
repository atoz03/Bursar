-- 0043_user_node_exclusive_points.sql
-- 用户节点专属积分：仅能在指定节点消耗

CREATE TABLE IF NOT EXISTS user_node_exclusive_points (
    username VARCHAR(50) NOT NULL,
    node_id VARCHAR(50) NOT NULL,
    balance DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (balance >= 0),
    updated_by VARCHAR(50) NOT NULL DEFAULT 'system',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (username, node_id)
);

CREATE INDEX IF NOT EXISTS idx_user_node_exclusive_points_username
    ON user_node_exclusive_points(username);

CREATE INDEX IF NOT EXISTS idx_user_node_exclusive_points_node
    ON user_node_exclusive_points(node_id);

ALTER TABLE recharge_records
    ADD COLUMN IF NOT EXISTS points_scope VARCHAR(20) NOT NULL DEFAULT 'general';

ALTER TABLE recharge_records
    ADD COLUMN IF NOT EXISTS node_id VARCHAR(50);

