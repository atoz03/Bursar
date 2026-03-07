ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS points_overdraft_memory_limit_gb DOUBLE PRECISION NULL;
