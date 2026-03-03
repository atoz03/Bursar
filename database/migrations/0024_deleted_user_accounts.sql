CREATE TABLE IF NOT EXISTS deleted_user_accounts (
    deleted_id BIGSERIAL PRIMARY KEY,
    username VARCHAR(50) NOT NULL,
    email VARCHAR(120) NOT NULL,
    student_id VARCHAR(40) NOT NULL,
    password_hash TEXT NOT NULL,
    real_name VARCHAR(80) NOT NULL,
    advisor VARCHAR(80) NOT NULL,
    expected_graduation_year INT NOT NULL,
    expected_graduation_month INT NOT NULL DEFAULT 12 CHECK (expected_graduation_month BETWEEN 1 AND 12),
    phone VARCHAR(40) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'user',
    last_login_at TIMESTAMP NULL,
    account_created_at TIMESTAMP NULL,
    account_updated_at TIMESTAMP NULL,
    balance DOUBLE PRECISION NOT NULL DEFAULT 0,
    user_status VARCHAR(20) NOT NULL DEFAULT 'normal',
    blocked_at TIMESTAMP NULL,
    deleted_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    delete_reason TEXT NOT NULL DEFAULT '',
    restored_at TIMESTAMP NULL,
    restored_by VARCHAR(50) NULL
);

CREATE INDEX IF NOT EXISTS idx_deleted_user_accounts_deleted_at ON deleted_user_accounts(deleted_at DESC);
CREATE INDEX IF NOT EXISTS idx_deleted_user_accounts_restored_at ON deleted_user_accounts(restored_at);
CREATE INDEX IF NOT EXISTS idx_deleted_user_accounts_username ON deleted_user_accounts(username);
CREATE INDEX IF NOT EXISTS idx_deleted_user_accounts_email ON deleted_user_accounts(email);
CREATE INDEX IF NOT EXISTS idx_deleted_user_accounts_student_id ON deleted_user_accounts(student_id);
