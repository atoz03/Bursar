-- 磁盘满告警的持久化锁存状态。
-- 同一节点/挂载点在未稳定恢复前只产生一次安全事件，控制器重启也不会重复告警。
CREATE TABLE IF NOT EXISTS node_disk_alert_states (
    node_id VARCHAR(50) NOT NULL,
    mountpoint TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT FALSE,
    below_rearm_since TIMESTAMP NULL,
    last_alerted_at TIMESTAMP NULL,
    last_observed_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, mountpoint)
);

CREATE INDEX IF NOT EXISTS idx_node_disk_alert_states_active
    ON node_disk_alert_states(active, updated_at DESC);

-- 已存在的磁盘告警视为“已通知”，避免升级后对仍然满盘的节点再报警一次。
INSERT INTO node_disk_alert_states(
    node_id, mountpoint, active, below_rearm_since,
    last_alerted_at, last_observed_at, updated_at
)
SELECT DISTINCT ON (e.node_id, NULLIF(TRIM(m.item->>'name'), ''))
       e.node_id,
       NULLIF(TRIM(m.item->>'name'), '') AS mountpoint,
       TRUE,
       NULL,
       e.created_at,
       e.created_at,
       NOW()
FROM node_security_events e
CROSS JOIN LATERAL jsonb_array_elements(
    CASE
      WHEN jsonb_typeof(e.details->'risk_mounts') = 'array' THEN e.details->'risk_mounts'
      ELSE '[]'::jsonb
    END
) AS m(item)
WHERE e.event_type = 'disk_full_risk'
  AND NULLIF(TRIM(m.item->>'name'), '') IS NOT NULL
ORDER BY e.node_id, NULLIF(TRIM(m.item->>'name'), ''), e.created_at DESC
ON CONFLICT (node_id, mountpoint) DO NOTHING;
