ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS node_price_per_minute DOUBLE PRECISION NULL;
