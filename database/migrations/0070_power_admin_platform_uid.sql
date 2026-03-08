ALTER TABLE admin_accounts
    ADD COLUMN IF NOT EXISTS platform_uid INT;

ALTER TABLE power_users
    ADD COLUMN IF NOT EXISTS platform_uid INT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_admin_accounts_platform_uid
    ON admin_accounts(platform_uid)
    WHERE platform_uid IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_power_users_platform_uid
    ON power_users(platform_uid)
    WHERE platform_uid IS NOT NULL;
