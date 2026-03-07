-- 0048_points_max_overdraft.sql
-- 每月最大欠费积分上限（超过后触发强制清理进程）

INSERT INTO app_settings(key, value, updated_at)
VALUES('monthly_points_max_overdraft_limit', '500', NOW())
ON CONFLICT (key) DO NOTHING;
