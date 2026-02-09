-- 0014_power_users.sql：高级用户（受限管理角色）

CREATE TABLE IF NOT EXISTS power_users (
    username VARCHAR(50) PRIMARY KEY,
    password_hash TEXT NOT NULL,
    can_view_board BOOLEAN NOT NULL DEFAULT TRUE,
    can_view_nodes BOOLEAN NOT NULL DEFAULT TRUE,
    can_review_requests BOOLEAN NOT NULL DEFAULT FALSE,
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    last_login_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
