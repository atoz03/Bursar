ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS node_model_price_overrides JSONB NOT NULL DEFAULT '{}'::jsonb;
