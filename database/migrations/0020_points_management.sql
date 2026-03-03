-- 0020_points_management.sql
-- 积分管理：特殊用户月度积分规则 + 月初重置执行记录

CREATE TABLE IF NOT EXISTS special_monthly_points_rules (
    username VARCHAR(50) PRIMARY KEY,
    monthly_points DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (monthly_points >= 0),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS monthly_points_reset_runs (
    month_key VARCHAR(7) PRIMARY KEY, -- YYYY-MM
    run_at TIMESTAMP NOT NULL DEFAULT NOW(),
    run_by VARCHAR(50) NOT NULL DEFAULT 'system',
    total_users INT NOT NULL DEFAULT 0,
    changed_users INT NOT NULL DEFAULT 0,
    forced BOOLEAN NOT NULL DEFAULT FALSE
);

