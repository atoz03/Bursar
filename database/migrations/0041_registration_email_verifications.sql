CREATE TABLE IF NOT EXISTS registration_email_verifications (
    verify_id BIGSERIAL PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    username VARCHAR(50) NOT NULL,
    email VARCHAR(120) NOT NULL,
    password_hash TEXT NOT NULL,
    real_name VARCHAR(80) NOT NULL,
    student_id VARCHAR(40) NOT NULL,
    advisor VARCHAR(80) NOT NULL,
    expected_graduation_year INT NOT NULL,
    expected_graduation_month INT NOT NULL DEFAULT 12 CHECK (expected_graduation_month BETWEEN 1 AND 12),
    phone VARCHAR(40) NOT NULL,
    expire_at TIMESTAMP NOT NULL,
    consumed_request_id INT NULL REFERENCES registration_requests(request_id) ON DELETE SET NULL,
    consumed_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_registration_email_verifications_expire_at
    ON registration_email_verifications(expire_at ASC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_registration_email_verifications_pending_username
    ON registration_email_verifications(username) WHERE consumed_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_registration_email_verifications_pending_email
    ON registration_email_verifications(email) WHERE consumed_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_registration_email_verifications_pending_student_id
    ON registration_email_verifications(student_id) WHERE consumed_at IS NULL;
