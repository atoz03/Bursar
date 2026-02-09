-- 0013_usage_records_local_username.sql
-- 在 usage_records 中增加机器本地账号字段，区分“本地账号”和“平台账号”

ALTER TABLE usage_records
    ADD COLUMN IF NOT EXISTS local_username VARCHAR(50) NOT NULL DEFAULT '';

-- 兼容历史数据：历史记录无法恢复真实本地账号，先回填为当前 username（平台账号）
UPDATE usage_records
SET local_username = username
WHERE COALESCE(local_username, '') = '';

CREATE INDEX IF NOT EXISTS idx_usage_local_username ON usage_records(local_username);
