ALTER TABLE node_policies
    ALTER COLUMN points_intercept_enabled SET DEFAULT TRUE;

-- 历史兼容：早期仅配置其它节点策略时会落 points_intercept_enabled=false。
-- 对“未显式配置阈值/配额”的记录回填为开启，避免欠费限速失效。
UPDATE node_policies
SET points_intercept_enabled = TRUE
WHERE points_intercept_enabled = FALSE
  AND points_throttle_threshold IS NULL
  AND points_limited_cpu_quota_percent IS NULL
  AND points_blocked_cpu_quota_percent IS NULL;
