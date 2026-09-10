CREATE TABLE IF NOT EXISTS node_action_jobs (
    job_id BIGSERIAL PRIMARY KEY,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(50) NOT NULL DEFAULT '',
    billing_username VARCHAR(50) NOT NULL DEFAULT '',
    action_type VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempt_count INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 10,
    next_attempt_at TIMESTAMP NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMP NULL,
    delivery_token VARCHAR(64) NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP NULL,
    CONSTRAINT node_action_jobs_status_check
        CHECK (status IN ('pending', 'leased', 'succeeded', 'failed', 'cancelled')),
    CONSTRAINT node_action_jobs_attempts_check
        CHECK (attempt_count >= 0 AND max_attempts > 0)
);

CREATE INDEX IF NOT EXISTS idx_node_action_jobs_dispatch
    ON node_action_jobs(node_id, status, next_attempt_at, lease_until, job_id);

CREATE INDEX IF NOT EXISTS idx_node_action_jobs_identity
    ON node_action_jobs(node_id, local_username, action_type, created_at DESC);

-- 同一个节点账号同时只能有一个活动的创建/身份对齐任务。重新生成密钥时
-- 重置活动任务并更换 delivery_token，旧执行回执不能误完成新一代任务。
CREATE UNIQUE INDEX IF NOT EXISTS uq_node_action_jobs_active_account
    ON node_action_jobs(node_id, local_username)
    WHERE action_type = 'create_local_account'
      AND status IN ('pending', 'leased');

-- 部署新版 Controller 时，历史映射已经由节点快照验证过。把它们记为已成功
-- 的基线任务，避免迁移后仍沿用“没有任务就猜初始化状态”的旧逻辑。
INSERT INTO node_action_jobs(
    node_id,
    local_username,
    billing_username,
    action_type,
    payload,
    status,
    attempt_count,
    max_attempts,
    next_attempt_at,
    lease_until,
    delivery_token,
    last_error,
    created_at,
    updated_at,
    completed_at
)
SELECT
    una.node_id,
    una.local_username,
    una.billing_username,
    'create_local_account',
    jsonb_build_object(
        'type', 'create_local_account',
        'username', una.local_username,
        'target_uid', pu.platform_uid,
        'target_primary_gid', pu.platform_uid,
        'reason', '0086 migration baseline from aligned node snapshot'
    ),
    'succeeded',
    0,
    10,
    NOW(),
    NULL,
    '',
    '',
    NOW(),
    NOW(),
    NOW()
FROM user_node_accounts una
JOIN node_local_users nlu
  ON nlu.node_id=una.node_id
 AND nlu.local_username=una.local_username
JOIN (
    SELECT DISTINCT ON (username) username, platform_uid
    FROM (
        SELECT username, platform_uid, 0 AS priority FROM user_accounts
        UNION ALL
        SELECT username, platform_uid, 1 AS priority FROM power_users
        UNION ALL
        SELECT username, platform_uid, 2 AS priority FROM admin_accounts
    ) platform_identities
    WHERE platform_uid IS NOT NULL AND platform_uid > 0
    ORDER BY username, priority
) pu ON pu.username=una.billing_username
WHERE nlu.uid=pu.platform_uid
  AND nlu.primary_gid=pu.platform_uid
  AND NOT EXISTS (
      SELECT 1
      FROM node_action_jobs naj
      WHERE naj.node_id=una.node_id
        AND naj.local_username=una.local_username
        AND naj.billing_username=una.billing_username
        AND naj.action_type='create_local_account'
  );
