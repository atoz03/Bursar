ALTER TABLE deleted_user_accounts
ADD COLUMN IF NOT EXISTS node_accounts_json JSONB NOT NULL DEFAULT '[]'::jsonb;
