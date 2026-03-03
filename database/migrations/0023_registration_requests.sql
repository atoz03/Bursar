CREATE TABLE IF NOT EXISTS registration_requests (
    request_id SERIAL PRIMARY KEY,
    username VARCHAR(50) NOT NULL,
    email VARCHAR(120) NOT NULL,
    password_hash TEXT NOT NULL,
    real_name VARCHAR(80) NOT NULL,
    student_id VARCHAR(40) NOT NULL,
    advisor VARCHAR(80) NOT NULL,
    expected_graduation_year INT NOT NULL,
    expected_graduation_month INT NOT NULL DEFAULT 12 CHECK (expected_graduation_month BETWEEN 1 AND 12),
    phone VARCHAR(40) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, approved, rejected
    reviewed_by VARCHAR(50) NULL,
    reviewed_at TIMESTAMP NULL,
    reject_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_registration_requests_status ON registration_requests(status);
CREATE INDEX IF NOT EXISTS idx_registration_requests_created_at ON registration_requests(created_at DESC);

-- 仅限制待审核申请，退回/通过后可重新提交。
CREATE UNIQUE INDEX IF NOT EXISTS uq_registration_requests_pending_username
ON registration_requests(username) WHERE status='pending';
CREATE UNIQUE INDEX IF NOT EXISTS uq_registration_requests_pending_email
ON registration_requests(email) WHERE status='pending';
CREATE UNIQUE INDEX IF NOT EXISTS uq_registration_requests_pending_student_id
ON registration_requests(student_id) WHERE status='pending';
