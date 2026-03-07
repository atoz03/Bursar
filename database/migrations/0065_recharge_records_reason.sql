-- 0065_recharge_records_reason.sql
-- 积分操作记录补充备注字段，供用户侧“积分新增提醒”展示管理员备注

ALTER TABLE recharge_records
    ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';
