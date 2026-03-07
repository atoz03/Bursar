ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS node_cpu_price_per_core_minute DOUBLE PRECISION NULL;
