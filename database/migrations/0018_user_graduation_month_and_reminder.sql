-- 0018_user_graduation_month_and_reminder.sql：
-- 1) 用户资料新增“预计毕业月份”
-- 2) 历史数据默认补到 12 月，避免误判过期

ALTER TABLE user_accounts
ADD COLUMN IF NOT EXISTS expected_graduation_month INT NOT NULL DEFAULT 12;

UPDATE user_accounts
SET expected_graduation_month = 12
WHERE expected_graduation_month IS NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'ck_user_accounts_expected_graduation_month'
  ) THEN
    ALTER TABLE user_accounts
    ADD CONSTRAINT ck_user_accounts_expected_graduation_month
    CHECK (expected_graduation_month BETWEEN 1 AND 12);
  END IF;
END $$;

