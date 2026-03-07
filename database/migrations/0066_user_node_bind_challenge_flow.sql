CREATE TABLE IF NOT EXISTS user_node_bind_challenges (
    challenge_id BIGSERIAL PRIMARY KEY,
    request_id INT NOT NULL DEFAULT 0,
    billing_username VARCHAR(50) NOT NULL,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    challenge_token VARCHAR(128) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active', -- active, verified, approved, failed, expired, cancelled
    fail_reason TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMP NOT NULL,
    claimed_at TIMESTAMP NULL,
    verified_at TIMESTAMP NULL,
    cooldown_until TIMESTAMP NULL,
    failure_processed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_node_bind_challenges_billing_created
    ON user_node_bind_challenges(billing_username, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_node_bind_challenges_target_created
    ON user_node_bind_challenges(node_id, local_username, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_node_bind_challenges_active_expires
    ON user_node_bind_challenges(expires_at ASC)
    WHERE status='active';

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_node_bind_challenges_unique_token
    ON user_node_bind_challenges(challenge_token);

CREATE TABLE IF NOT EXISTS user_node_bind_cooldowns (
    billing_username VARCHAR(50) PRIMARY KEY,
    failure_streak INT NOT NULL DEFAULT 0,
    cooldown_until TIMESTAMP NULL,
    last_failed_at TIMESTAMP NULL,
    last_succeeded_at TIMESTAMP NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_node_bind_freezes (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    freeze_until TIMESTAMP NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    triggered_by VARCHAR(50) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_user_node_bind_freezes_until
    ON user_node_bind_freezes(freeze_until ASC);

CREATE TABLE IF NOT EXISTS user_node_bind_temp_access (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    billing_username VARCHAR(50) NOT NULL,
    challenge_id BIGINT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_user_node_bind_temp_access_billing
    ON user_node_bind_temp_access(billing_username, expires_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_node_bind_temp_access_expires
    ON user_node_bind_temp_access(expires_at ASC);
