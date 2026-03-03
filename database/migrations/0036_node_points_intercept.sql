ALTER TABLE node_policies
    ADD COLUMN IF NOT EXISTS points_intercept_enabled BOOLEAN NOT NULL DEFAULT FALSE;
