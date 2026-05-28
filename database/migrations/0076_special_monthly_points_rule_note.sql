ALTER TABLE special_monthly_points_rules
    ADD COLUMN IF NOT EXISTS note TEXT NOT NULL DEFAULT '';
