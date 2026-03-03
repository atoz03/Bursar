CREATE TABLE IF NOT EXISTS admin_profiles (
    username VARCHAR(50) PRIMARY KEY,
    real_name VARCHAR(80) NOT NULL DEFAULT '',
    email VARCHAR(120) NOT NULL DEFAULT '',
    phone VARCHAR(40) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_profiles_email ON admin_profiles(email);
