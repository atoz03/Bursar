ALTER TABLE power_users
    ADD COLUMN IF NOT EXISTS can_points_users BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS can_points_batch_filtered BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS can_points_batch_all BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS can_points_records BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS can_points_monthly BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS can_points_special_rules BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE power_users
SET
    can_points_users = CASE WHEN can_manage_points THEN TRUE ELSE can_points_users END,
    can_points_batch_filtered = CASE WHEN can_manage_points THEN TRUE ELSE can_points_batch_filtered END,
    can_points_batch_all = CASE WHEN can_manage_points THEN TRUE ELSE can_points_batch_all END,
    can_points_records = CASE WHEN can_manage_points THEN TRUE ELSE can_points_records END,
    can_points_monthly = CASE WHEN can_manage_points THEN TRUE ELSE can_points_monthly END,
    can_points_special_rules = CASE WHEN can_manage_points THEN TRUE ELSE can_points_special_rules END;
