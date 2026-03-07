-- 清理 nodes.disk_quota_mounts 历史脏数据：
-- 1) 移除 NULL 元素
-- 2) 移除空字符串元素
-- 3) 去重并排序，避免后续扫描报错与前端展示抖动
UPDATE nodes
SET disk_quota_mounts = COALESCE(
  (
    SELECT ARRAY(
      SELECT DISTINCT TRIM(m)
      FROM unnest(COALESCE(nodes.disk_quota_mounts, ARRAY[]::TEXT[])) AS m
      WHERE m IS NOT NULL AND TRIM(m) <> ''
      ORDER BY TRIM(m)
    )::TEXT[]
  ),
  ARRAY[]::TEXT[]
);
