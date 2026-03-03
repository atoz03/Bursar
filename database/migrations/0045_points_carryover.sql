-- 0045_points_carryover.sql
-- 通用积分结转：新增结转积分池与月度结转上限配置

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS carryover_balance DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (carryover_balance >= 0);

ALTER TABLE deleted_user_accounts
    ADD COLUMN IF NOT EXISTS carryover_balance DOUBLE PRECISION NOT NULL DEFAULT 0;

INSERT INTO app_settings(key, value, updated_at)
VALUES('monthly_points_carryover_limit', '500', NOW())
ON CONFLICT (key) DO NOTHING;
