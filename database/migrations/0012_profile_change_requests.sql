-- 0012_profile_change_requests.sql：用户关键信息变更申请（管理员审核）

CREATE TABLE IF NOT EXISTS profile_change_requests (
    request_id SERIAL PRIMARY KEY,
    billing_username VARCHAR(50) NOT NULL,
    old_username VARCHAR(50) NOT NULL,
    old_email VARCHAR(120) NOT NULL,
    old_student_id VARCHAR(40) NOT NULL,
    new_username VARCHAR(50) NOT NULL,
    new_email VARCHAR(120) NOT NULL,
    new_student_id VARCHAR(40) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, approved, rejected
    reviewed_by VARCHAR(50) NULL,
    reviewed_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_profile_change_requests_status ON profile_change_requests(status);
CREATE INDEX IF NOT EXISTS idx_profile_change_requests_user ON profile_change_requests(billing_username);
