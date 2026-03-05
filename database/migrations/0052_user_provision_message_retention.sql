ALTER TABLE user_provision_messages
    ADD COLUMN IF NOT EXISTS first_decrypted_at TIMESTAMP NULL,
    ADD COLUMN IF NOT EXISTS destroy_after_at TIMESTAMP NULL,
    ADD COLUMN IF NOT EXISTS destroyed_at TIMESTAMP NULL;

CREATE INDEX IF NOT EXISTS idx_user_provision_messages_destroy_after
    ON user_provision_messages(destroy_after_at)
    WHERE destroy_after_at IS NOT NULL AND destroyed_at IS NULL;
