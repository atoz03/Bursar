-- 记录用户 GPU 可见性的策略模式，用于区分「未设置限制」与「完全不可见」。
-- 旧模型只有 node_user_gpu_visibility 允许列表：空列表既表示「无限制」，
-- 也无法表达「一张 GPU 都不可见」，两者冲突时一律按放行处理（fail-open）。
-- 这里用独立的策略表显式承载 deny_all，避免继续用「空集合」承担两种语义。
CREATE TABLE IF NOT EXISTS node_user_gpu_visibility_policy (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    deny_all BOOLEAN NOT NULL DEFAULT FALSE,
    reason TEXT NOT NULL DEFAULT '',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_node_user_gpu_visibility_policy_node_updated
    ON node_user_gpu_visibility_policy(node_id, local_username, updated_at DESC);

-- 回填存量允许列表，使「存在策略行」等价于「存在可见性限制」，
-- 读取路径即可统一以策略表为准，无需再对老数据做特殊分支。
INSERT INTO node_user_gpu_visibility_policy(node_id, local_username, deny_all, reason, updated_by, updated_at)
SELECT node_id,
       local_username,
       FALSE,
       (array_agg(reason ORDER BY updated_at DESC, gpu_index ASC))[1],
       (array_agg(updated_by ORDER BY updated_at DESC, gpu_index ASC))[1],
       MAX(updated_at)
FROM node_user_gpu_visibility
GROUP BY node_id, local_username
ON CONFLICT (node_id, local_username) DO NOTHING;
