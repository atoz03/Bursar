-- 0084_points_balance_email_alerts.sql：积分余额预警邮件的布防状态与待发送队列

CREATE TABLE IF NOT EXISTS points_balance_email_alert_states (
    username VARCHAR(50) PRIMARY KEY,
    armed BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS points_balance_email_alerts (
    alert_id BIGSERIAL PRIMARY KEY,
    username VARCHAR(50) NOT NULL,
    threshold DOUBLE PRECISION NOT NULL CHECK (threshold > 0),
    balance DOUBLE PRECISION NOT NULL,
    triggered_at TIMESTAMP NOT NULL DEFAULT NOW(),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMP NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMP NULL,
    sent_at TIMESTAMP NULL,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT points_balance_email_alerts_status_check
        CHECK (status IN ('pending', 'sending', 'sent'))
);

CREATE INDEX IF NOT EXISTS idx_points_balance_email_alerts_delivery
ON points_balance_email_alerts(status, next_attempt_at, triggered_at);

CREATE INDEX IF NOT EXISTS idx_points_balance_email_alerts_username
ON points_balance_email_alerts(username, triggered_at DESC);
