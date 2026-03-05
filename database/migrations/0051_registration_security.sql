-- 注册安全增强：
-- 1) 注册验证码挑战（一次性、短时有效）
-- 2) 临时邮箱域名黑名单（管理员可维护）
-- 3) 注册安全事件日志（用于管理员查询限流/拦截命中）

CREATE TABLE IF NOT EXISTS registration_captcha_challenges (
    captcha_id TEXT PRIMARY KEY,
    client_ip VARCHAR(64) NOT NULL DEFAULT '',
    question TEXT NOT NULL,
    options_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    answer_index INT NOT NULL CHECK (answer_index >= 0),
    expire_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_registration_captcha_challenges_expire_at
    ON registration_captcha_challenges(expire_at ASC);
CREATE INDEX IF NOT EXISTS idx_registration_captcha_challenges_created_at
    ON registration_captcha_challenges(created_at DESC);

CREATE TABLE IF NOT EXISTS registration_disposable_email_domains (
    domain VARCHAR(120) PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    note TEXT NOT NULL DEFAULT '',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_registration_disposable_email_domains_enabled
    ON registration_disposable_email_domains(enabled, updated_at DESC);

CREATE TABLE IF NOT EXISTS registration_security_events (
    event_id BIGSERIAL PRIMARY KEY,
    action VARCHAR(40) NOT NULL,
    decision VARCHAR(20) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    client_ip VARCHAR(64) NOT NULL DEFAULT '',
    username VARCHAR(50) NOT NULL DEFAULT '',
    email VARCHAR(120) NOT NULL DEFAULT '',
    student_id VARCHAR(40) NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_registration_security_events_created_at
    ON registration_security_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_registration_security_events_action_created
    ON registration_security_events(action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_registration_security_events_ip_created
    ON registration_security_events(client_ip, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_registration_security_events_email_created
    ON registration_security_events(email, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_registration_security_events_decision_created
    ON registration_security_events(decision, created_at DESC);
