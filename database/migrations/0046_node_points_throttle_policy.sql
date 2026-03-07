ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS points_throttle_threshold DOUBLE PRECISION NULL;

ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS points_limited_cpu_quota_percent DOUBLE PRECISION NULL;

ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS points_blocked_cpu_quota_percent DOUBLE PRECISION NULL;
