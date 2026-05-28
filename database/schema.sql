-- GPU 集群管理系统数据库结构（用于手工初始化/审计）

CREATE TABLE IF NOT EXISTS users (
    user_id SERIAL PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    balance DECIMAL(10,2) NOT NULL DEFAULT 100.0,
    carryover_balance DOUBLE PRECISION NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'normal', -- normal, warning, limited, blocked
    blocked_at TIMESTAMP NULL,
    last_charge_time TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS usage_records (
    record_id SERIAL PRIMARY KEY,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(50) NOT NULL DEFAULT '',
    username VARCHAR(50) NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    pid INT NOT NULL DEFAULT 0,
    cpu_percent FLOAT NOT NULL,
    memory_mb FLOAT NOT NULL,
    gpu_count INT NOT NULL DEFAULT 0,
    command TEXT NOT NULL DEFAULT '',
    gpu_usage JSONB NOT NULL,
    cost DECIMAL(10,4) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS process_kill_records (
    record_id BIGSERIAL PRIMARY KEY,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL DEFAULT '',
    billing_username VARCHAR(50) NOT NULL DEFAULT '',
    action_type VARCHAR(40) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    pids_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS recharge_records (
    recharge_id SERIAL PRIMARY KEY,
    username VARCHAR(50) NOT NULL,
    amount DECIMAL(10,2) NOT NULL,
    method VARCHAR(50) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS resource_prices (
    price_id SERIAL PRIMARY KEY,
    gpu_model VARCHAR(50) UNIQUE NOT NULL,
    price_per_minute DECIMAL(10,4) NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    filename TEXT PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- 上报幂等表：用于防止 Agent 重试导致重复扣费
CREATE TABLE IF NOT EXISTS metric_reports (
    report_id TEXT PRIMARY KEY,
    node_id VARCHAR(50) NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    interval_seconds INT NOT NULL,
    received_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- 节点状态表：用于运维查看在线/上报情况
CREATE TABLE IF NOT EXISTS nodes (
    node_id VARCHAR(50) PRIMARY KEY,
    last_seen_at TIMESTAMP NOT NULL,
    last_report_id TEXT NOT NULL,
    last_report_ts TIMESTAMP NOT NULL,
    interval_seconds INT NOT NULL,
    cpu_model TEXT NOT NULL DEFAULT '',
    cpu_count INT NOT NULL DEFAULT 0,
    gpu_model TEXT NOT NULL DEFAULT '',
    gpu_count INT NOT NULL DEFAULT 0,
    os_version TEXT NOT NULL DEFAULT '',
    kernel_version TEXT NOT NULL DEFAULT '',
    agent_version TEXT NOT NULL DEFAULT '',
    node_ip TEXT NOT NULL DEFAULT '',
    node_mac TEXT NOT NULL DEFAULT '',
    disk_total_gb DOUBLE PRECISION NOT NULL DEFAULT 0,
    disk_used_gb DOUBLE PRECISION NOT NULL DEFAULT 0,
    home_total_gb DOUBLE PRECISION NOT NULL DEFAULT 0,
    home_used_gb DOUBLE PRECISION NOT NULL DEFAULT 0,
    mnt_total_gb DOUBLE PRECISION NOT NULL DEFAULT 0,
    mnt_used_gb DOUBLE PRECISION NOT NULL DEFAULT 0,
    net_rx_bytes BIGINT NOT NULL DEFAULT 0,
    net_tx_bytes BIGINT NOT NULL DEFAULT 0,
    net_rx_mb_month DOUBLE PRECISION NOT NULL DEFAULT 0,
    net_tx_mb_month DOUBLE PRECISION NOT NULL DEFAULT 0,
    traffic_month VARCHAR(7) NOT NULL DEFAULT '',
    gpu_process_count INT NOT NULL DEFAULT 0,
    cpu_process_count INT NOT NULL DEFAULT 0,
    usage_records_count INT NOT NULL DEFAULT 0,
    ssh_active_count INT NOT NULL DEFAULT 0,
    disk_quota_installed BOOLEAN NOT NULL DEFAULT FALSE,
    disk_quota_mounts TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    system_services_checked_at TIMESTAMP NULL,
    system_services JSONB NOT NULL DEFAULT '[]'::JSONB,
    cost_total DECIMAL(12,4) NOT NULL DEFAULT 0.0,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS node_local_users (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    uid INT NULL,
    primary_gid INT NULL,
    home_created_at TIMESTAMP NULL,
    last_login_at TIMESTAMP NULL,
    home_used_gb DOUBLE PRECISION NOT NULL DEFAULT 0,
    has_sudo BOOLEAN NOT NULL DEFAULT FALSE,
    has_docker BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);
CREATE INDEX IF NOT EXISTS idx_node_local_users_node_updated ON node_local_users(node_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_node_local_users_uid ON node_local_users(uid) WHERE uid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_node_local_users_primary_gid ON node_local_users(primary_gid) WHERE primary_gid IS NOT NULL;

CREATE TABLE IF NOT EXISTS node_user_disk_quotas (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    mountpoint TEXT NOT NULL,
    used_mb DOUBLE PRECISION NOT NULL DEFAULT 0,
    soft_mb DOUBLE PRECISION NOT NULL DEFAULT 0,
    hard_mb DOUBLE PRECISION NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username, mountpoint)
);
CREATE INDEX IF NOT EXISTS idx_node_user_disk_quotas_node_mount
    ON node_user_disk_quotas(node_id, mountpoint, updated_at DESC);

CREATE TABLE IF NOT EXISTS node_user_cpu_limits (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    cpu_quota_percent DOUBLE PRECISION NOT NULL CHECK (cpu_quota_percent > 0 AND cpu_quota_percent <= 100),
    reason TEXT NOT NULL DEFAULT '',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);
CREATE INDEX IF NOT EXISTS idx_node_user_cpu_limits_node_updated
    ON node_user_cpu_limits(node_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS node_user_memory_limits (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    memory_limit_gb DOUBLE PRECISION NOT NULL CHECK (memory_limit_gb > 0),
    reason TEXT NOT NULL DEFAULT '',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);
CREATE INDEX IF NOT EXISTS idx_node_user_memory_limits_node_updated
    ON node_user_memory_limits(node_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS node_user_gpu_visibility (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    gpu_index INTEGER NOT NULL CHECK (gpu_index >= 0),
    reason TEXT NOT NULL DEFAULT '',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username, gpu_index)
);
CREATE INDEX IF NOT EXISTS idx_node_user_gpu_visibility_node_updated
    ON node_user_gpu_visibility(node_id, local_username, updated_at DESC);

CREATE TABLE IF NOT EXISTS node_security_events (
    event_id BIGSERIAL PRIMARY KEY,
    report_id TEXT NOT NULL DEFAULT '',
    node_id VARCHAR(50) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    severity VARCHAR(20) NOT NULL DEFAULT 'info',
    reason TEXT NOT NULL DEFAULT '',
    related_usernames TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_node_security_events_node_time
    ON node_security_events(node_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_node_security_events_type_time
    ON node_security_events(event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_node_security_events_users
    ON node_security_events USING GIN (related_usernames);

CREATE TABLE IF NOT EXISTS node_policies (
    node_id VARCHAR(50) PRIMARY KEY,
    ssh_guard_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ssh_exclusive_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ssh_exclusive_block_others BOOLEAN NOT NULL DEFAULT TRUE,
    points_intercept_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    points_throttle_threshold DOUBLE PRECISION NULL,
    points_limited_cpu_quota_percent DOUBLE PRECISION NULL,
    points_blocked_cpu_quota_percent DOUBLE PRECISION NULL,
    points_overdraft_memory_limit_gb DOUBLE PRECISION NULL,
    disk_quota_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    disk_quota_mountpoint TEXT NULL,
    disk_quota_soft_mb DOUBLE PRECISION NULL,
    disk_quota_hard_mb DOUBLE PRECISION NULL,
    node_price_per_minute DOUBLE PRECISION NULL,
    node_cpu_price_per_core_minute DOUBLE PRECISION NULL,
    node_model_price_overrides JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS node_exclusive_users (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);
CREATE INDEX IF NOT EXISTS idx_node_exclusive_users_node ON node_exclusive_users(node_id);

CREATE TABLE IF NOT EXISTS node_exclusive_gpu_users (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    gpu_index INTEGER NOT NULL CHECK (gpu_index >= 0),
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username, gpu_index)
);
CREATE INDEX IF NOT EXISTS idx_node_exclusive_gpu_users_node_user
    ON node_exclusive_gpu_users(node_id, local_username);

CREATE TABLE IF NOT EXISTS node_view_acl (
    node_id VARCHAR(50) NOT NULL,
    power_username VARCHAR(50) NOT NULL,
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, power_username)
);
CREATE INDEX IF NOT EXISTS idx_node_view_acl_power_username
    ON node_view_acl(power_username);

-- 管理员账号（Web 登录）
CREATE TABLE IF NOT EXISTS admin_accounts (
    username VARCHAR(50) PRIMARY KEY,
    password_hash TEXT NOT NULL,
    platform_uid INT NULL,
    last_login_at TIMESTAMP NULL,
    two_factor_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    two_factor_secret TEXT NOT NULL DEFAULT '',
    two_factor_pending_secret TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_admin_accounts_platform_uid
    ON admin_accounts(platform_uid)
    WHERE platform_uid IS NOT NULL;

CREATE TABLE IF NOT EXISTS power_users (
    username VARCHAR(50) PRIMARY KEY,
    password_hash TEXT NOT NULL,
    platform_uid INT NULL,
    can_view_board BOOLEAN NOT NULL DEFAULT TRUE,
    can_view_nodes BOOLEAN NOT NULL DEFAULT TRUE,
    can_manage_nodes BOOLEAN NOT NULL DEFAULT FALSE,
    can_manage_points BOOLEAN NOT NULL DEFAULT FALSE,
    can_points_users BOOLEAN NOT NULL DEFAULT FALSE,
    can_points_batch_filtered BOOLEAN NOT NULL DEFAULT FALSE,
    can_points_batch_all BOOLEAN NOT NULL DEFAULT FALSE,
    can_points_records BOOLEAN NOT NULL DEFAULT FALSE,
    can_points_monthly BOOLEAN NOT NULL DEFAULT FALSE,
    can_points_special_rules BOOLEAN NOT NULL DEFAULT FALSE,
    can_review_requests BOOLEAN NOT NULL DEFAULT FALSE,
    can_manage_platform_users BOOLEAN NOT NULL DEFAULT FALSE,
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    last_login_at TIMESTAMP NULL,
    two_factor_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    two_factor_secret TEXT NOT NULL DEFAULT '',
    two_factor_pending_secret TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_power_users_platform_uid
    ON power_users(platform_uid)
    WHERE platform_uid IS NOT NULL;

CREATE TABLE IF NOT EXISTS admin_profiles (
    username VARCHAR(50) PRIMARY KEY,
    real_name VARCHAR(80) NOT NULL DEFAULT '',
    email VARCHAR(120) NOT NULL DEFAULT '',
    phone VARCHAR(40) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_admin_profiles_email ON admin_profiles(email);

-- 用户“节点账号”绑定：按节点(node_id=机器编号/端口) + 本地账号(local_username) 映射到计费账号(billing_username)
CREATE TABLE IF NOT EXISTS user_node_accounts (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(50) NOT NULL,
    billing_username VARCHAR(50) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE TABLE IF NOT EXISTS user_node_account_risk_clears (
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    cleared_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    cleared_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);
CREATE INDEX IF NOT EXISTS idx_user_node_account_risk_clears_cleared_at
    ON user_node_account_risk_clears(cleared_at DESC);

CREATE TABLE IF NOT EXISTS user_node_account_audits (
    audit_id BIGSERIAL PRIMARY KEY,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    old_billing_username VARCHAR(50) NOT NULL DEFAULT '',
    new_billing_username VARCHAR(50) NOT NULL DEFAULT '',
    action VARCHAR(40) NOT NULL DEFAULT '',
    operator VARCHAR(50) NOT NULL DEFAULT '',
    source VARCHAR(20) NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_node_account_audits_node_local_time
    ON user_node_account_audits(node_id, local_username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_node_account_audits_created
    ON user_node_account_audits(created_at DESC);

CREATE TABLE IF NOT EXISTS account_provision_logs (
    provision_id BIGSERIAL PRIMARY KEY,
    billing_username VARCHAR(50) NOT NULL,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(50) NOT NULL,
    email VARCHAR(120) NOT NULL,
    ssh_host VARCHAR(255) NOT NULL DEFAULT '',
    ssh_port INT NOT NULL DEFAULT 22,
    download_filename VARCHAR(255) NOT NULL DEFAULT '',
    mail_sent BOOLEAN NOT NULL DEFAULT FALSE,
    mail_error TEXT NOT NULL DEFAULT '',
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS user_provision_messages (
    message_id BIGSERIAL PRIMARY KEY,
    billing_username VARCHAR(50) NOT NULL,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(50) NOT NULL,
    encrypted_payload TEXT NOT NULL,
    decrypt_url TEXT NOT NULL DEFAULT '/user/accounts?tool=key-decryptor',
    ssh_host VARCHAR(255) NOT NULL DEFAULT '',
    ssh_port INT NOT NULL DEFAULT 22,
    download_filename VARCHAR(255) NOT NULL DEFAULT '',
    ssh_command TEXT NOT NULL DEFAULT '',
    mail_to VARCHAR(120) NOT NULL DEFAULT '',
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    first_decrypted_at TIMESTAMP NULL,
    destroy_after_at TIMESTAMP NULL,
    destroyed_at TIMESTAMP NULL
);

-- 用户自助登记/开号申请（管理员审核）
CREATE TABLE IF NOT EXISTS user_requests (
    request_id SERIAL PRIMARY KEY,
    request_type VARCHAR(20) NOT NULL, -- bind, open, unbind
    billing_username VARCHAR(50) NOT NULL,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(50) NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, verified, approved, rejected, challenge_active
    reviewed_by VARCHAR(50) NULL,
    reviewed_at TIMESTAMP NULL,
    reject_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_node_unbind_records (
    record_id BIGSERIAL PRIMARY KEY,
    source_type VARCHAR(20) NOT NULL DEFAULT 'user_request', -- user_request, admin_forced
    request_id INT NULL,
    billing_username VARCHAR(50) NOT NULL,
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, approved, rejected, forced
    reason TEXT NOT NULL DEFAULT '',
    initiated_by VARCHAR(50) NOT NULL DEFAULT '',
    reviewed_by VARCHAR(50) NULL,
    reviewed_at TIMESTAMP NULL,
    executed_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_node_unbind_records_request
    ON user_node_unbind_records(request_id);
CREATE INDEX IF NOT EXISTS idx_user_node_unbind_records_created
    ON user_node_unbind_records(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_node_unbind_records_node_local
    ON user_node_unbind_records(node_id, local_username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_node_unbind_records_billing
    ON user_node_unbind_records(billing_username, created_at DESC);

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

CREATE INDEX IF NOT EXISTS idx_usage_username ON usage_records(username);
CREATE INDEX IF NOT EXISTS idx_usage_local_username ON usage_records(local_username);
CREATE INDEX IF NOT EXISTS idx_usage_timestamp ON usage_records(timestamp);
CREATE INDEX IF NOT EXISTS idx_usage_node ON usage_records(node_id);
CREATE INDEX IF NOT EXISTS idx_usage_timestamp_username ON usage_records(timestamp, username);
CREATE INDEX IF NOT EXISTS idx_process_kill_records_created_at ON process_kill_records(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_process_kill_records_billing_created ON process_kill_records(billing_username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_process_kill_records_node_local_created ON process_kill_records(node_id, local_username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_node_accounts_billing ON user_node_accounts(billing_username);
CREATE INDEX IF NOT EXISTS idx_account_provision_logs_created_at ON account_provision_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_account_provision_logs_billing ON account_provision_logs(billing_username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_account_provision_logs_node_local ON account_provision_logs(node_id, local_username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_provision_messages_user_created ON user_provision_messages(billing_username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_provision_messages_destroy_after
    ON user_provision_messages(destroy_after_at)
    WHERE destroy_after_at IS NOT NULL AND destroyed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_user_requests_status ON user_requests(status);
CREATE INDEX IF NOT EXISTS idx_user_requests_billing ON user_requests(billing_username);
CREATE INDEX IF NOT EXISTS idx_user_node_bind_challenges_billing_created
    ON user_node_bind_challenges(billing_username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_node_bind_challenges_target_created
    ON user_node_bind_challenges(node_id, local_username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_node_bind_challenges_active_expires
    ON user_node_bind_challenges(expires_at ASC)
    WHERE status='active';
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_node_bind_challenges_unique_token
    ON user_node_bind_challenges(challenge_token);
CREATE INDEX IF NOT EXISTS idx_user_node_bind_freezes_until
    ON user_node_bind_freezes(freeze_until ASC);
CREATE INDEX IF NOT EXISTS idx_user_node_bind_temp_access_billing
    ON user_node_bind_temp_access(billing_username, expires_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_node_bind_temp_access_expires
    ON user_node_bind_temp_access(expires_at ASC);

-- 普通用户账号（Web 登录）
CREATE TABLE IF NOT EXISTS user_accounts (
    username VARCHAR(50) PRIMARY KEY,
    email VARCHAR(120) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    real_name VARCHAR(80) NOT NULL,
    student_id VARCHAR(40) NOT NULL,
    advisor VARCHAR(80) NOT NULL,
    expected_graduation_year INT NOT NULL,
    expected_graduation_month INT NOT NULL DEFAULT 12 CHECK (expected_graduation_month BETWEEN 1 AND 12),
    phone VARCHAR(40) NOT NULL,
    platform_uid INT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'user',
    last_login_at TIMESTAMP NULL,
    two_factor_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    two_factor_secret TEXT NOT NULL DEFAULT '',
    two_factor_pending_secret TEXT NOT NULL DEFAULT '',
    reset_token_hash TEXT NULL,
    reset_token_expire_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_accounts_email ON user_accounts(email);
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_accounts_student_id ON user_accounts(student_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_accounts_platform_uid ON user_accounts(platform_uid) WHERE platform_uid IS NOT NULL;

-- 应用配置（如 SMTP）
CREATE TABLE IF NOT EXISTS app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- 容灾同步任务日志（手动/定时/回切）
CREATE TABLE IF NOT EXISTS ha_sync_runs (
    run_id BIGSERIAL PRIMARY KEY,
    trigger_mode VARCHAR(32) NOT NULL DEFAULT 'manual',
    direction VARCHAR(32) NOT NULL DEFAULT 'primary_to_standby',
    status VARCHAR(24) NOT NULL DEFAULT 'running',
    started_by VARCHAR(64) NOT NULL DEFAULT 'system',
    started_at TIMESTAMP NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMP NULL,
    summary TEXT NOT NULL DEFAULT '',
    detail_json JSONB NOT NULL DEFAULT '[]'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_ha_sync_runs_started_at_desc ON ha_sync_runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_ha_sync_runs_status_started_at ON ha_sync_runs(status, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_ha_sync_runs_trigger_started_at ON ha_sync_runs(trigger_mode, started_at DESC);

CREATE TABLE IF NOT EXISTS announcements (
    announcement_id SERIAL PRIMARY KEY,
    title VARCHAR(200) NOT NULL,
    content TEXT NOT NULL,
    pinned BOOLEAN NOT NULL DEFAULT FALSE,
    attachment_filename TEXT NOT NULL DEFAULT '',
    attachment_content_type TEXT NOT NULL DEFAULT '',
    attachment_size_bytes BIGINT NOT NULL DEFAULT 0,
    attachment_sha256 TEXT NOT NULL DEFAULT '',
    attachment_data BYTEA,
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_announcements_created_at ON announcements(created_at DESC);

-- 管理员记事本
CREATE TABLE IF NOT EXISTS admin_notes (
    note_id SERIAL PRIMARY KEY,
    note_date DATE NOT NULL,
    title VARCHAR(200) NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_admin_notes_note_date ON admin_notes(note_date DESC, updated_at DESC);

-- 用户关键信息变更申请（用户名/邮箱/学号）
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

-- 平台账号注册申请（管理员审核后才创建账号）
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
    reject_notify_mail_checked BOOLEAN NOT NULL DEFAULT FALSE,
    reject_notify_mail_sent BOOLEAN NOT NULL DEFAULT FALSE,
    reject_notify_mail_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_registration_requests_status ON registration_requests(status);
CREATE INDEX IF NOT EXISTS idx_registration_requests_created_at ON registration_requests(created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_registration_requests_pending_username
ON registration_requests(username) WHERE status='pending';
CREATE UNIQUE INDEX IF NOT EXISTS uq_registration_requests_pending_email
ON registration_requests(email) WHERE status='pending';
CREATE UNIQUE INDEX IF NOT EXISTS uq_registration_requests_pending_student_id
ON registration_requests(student_id) WHERE status='pending';

-- 平台账号注册邮箱验证（点击邮件链接后才会进入 registration_requests）
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
CREATE INDEX IF NOT EXISTS idx_registration_email_verifications_expire_at ON registration_email_verifications(expire_at ASC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_registration_email_verifications_pending_username
ON registration_email_verifications(username) WHERE consumed_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_registration_email_verifications_pending_email
ON registration_email_verifications(email) WHERE consumed_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_registration_email_verifications_pending_student_id
ON registration_email_verifications(student_id) WHERE consumed_at IS NULL;

-- 注册验证码挑战（一次性、短时有效）
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

-- 临时邮箱域名黑名单（管理员可维护）
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

-- 注册安全事件日志（限流/拦截命中、通过等）
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

-- 已删除平台账号归档（可恢复）
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
    platform_uid INT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'user',
    last_login_at TIMESTAMP NULL,
    account_created_at TIMESTAMP NULL,
    account_updated_at TIMESTAMP NULL,
    balance DOUBLE PRECISION NOT NULL DEFAULT 0,
    carryover_balance DOUBLE PRECISION NOT NULL DEFAULT 0,
    user_status VARCHAR(20) NOT NULL DEFAULT 'normal',
    blocked_at TIMESTAMP NULL,
    deleted_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    delete_reason TEXT NOT NULL DEFAULT '',
    node_accounts_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    restored_at TIMESTAMP NULL,
    restored_by VARCHAR(50) NULL
);
CREATE INDEX IF NOT EXISTS idx_deleted_user_accounts_deleted_at ON deleted_user_accounts(deleted_at DESC);
CREATE INDEX IF NOT EXISTS idx_deleted_user_accounts_restored_at ON deleted_user_accounts(restored_at);
CREATE INDEX IF NOT EXISTS idx_deleted_user_accounts_username ON deleted_user_accounts(username);
CREATE INDEX IF NOT EXISTS idx_deleted_user_accounts_email ON deleted_user_accounts(email);
CREATE INDEX IF NOT EXISTS idx_deleted_user_accounts_student_id ON deleted_user_accounts(student_id);
CREATE INDEX IF NOT EXISTS idx_deleted_user_accounts_platform_uid_active
ON deleted_user_accounts(platform_uid, deleted_at DESC)
WHERE platform_uid IS NOT NULL AND restored_at IS NULL;

-- SSH 白名单（允许不注册直接登录）
CREATE TABLE IF NOT EXISTS ssh_whitelist (
    node_id VARCHAR(50) NOT NULL,  -- 具体节点或 "*" 表示所有节点
    local_username VARCHAR(50) NOT NULL,
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_ssh_whitelist_user ON ssh_whitelist(local_username);

-- SSH 黑名单（强制禁止登录，优先级高于白名单/映射）
CREATE TABLE IF NOT EXISTS ssh_blacklist (
    node_id VARCHAR(50) NOT NULL,  -- 具体节点或 "*" 表示所有节点
    local_username VARCHAR(50) NOT NULL,
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_ssh_blacklist_user ON ssh_blacklist(local_username);

-- SSH 豁免名单（最高优先级，绕过白/黑名单与注册限制）
CREATE TABLE IF NOT EXISTS ssh_exemptions (
    node_id VARCHAR(50) NOT NULL,  -- 具体节点或 "*" 表示所有节点
    local_username VARCHAR(50) NOT NULL,
    created_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, local_username)
);

CREATE INDEX IF NOT EXISTS idx_ssh_exemptions_user ON ssh_exemptions(local_username);
INSERT INTO ssh_exemptions(node_id, local_username, created_by)
VALUES('*', 'gpuops', 'system')
ON CONFLICT (node_id, local_username) DO NOTHING;

-- SSH 名单来源元信息（记录是按节点账号添加还是按平台账号添加）
CREATE TABLE IF NOT EXISTS ssh_list_sources (
    list_type VARCHAR(20) NOT NULL CHECK (list_type IN ('whitelist', 'blacklist', 'exemptions')),
    node_id VARCHAR(50) NOT NULL,
    local_username VARCHAR(50) NOT NULL,
    source_type VARCHAR(20) NOT NULL DEFAULT 'local' CHECK (source_type IN ('local', 'platform')),
    source_platform_username VARCHAR(50) NOT NULL DEFAULT '',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (list_type, node_id, local_username)
);
CREATE INDEX IF NOT EXISTS idx_ssh_list_sources_node ON ssh_list_sources(list_type, node_id);
CREATE INDEX IF NOT EXISTS idx_ssh_list_sources_local ON ssh_list_sources(list_type, local_username);

-- 节点运行时快照（用于节点详情页趋势图与 SSH 用户展示）
CREATE TABLE IF NOT EXISTS node_runtime_snapshots (
    report_id VARCHAR(64) PRIMARY KEY,
    node_id VARCHAR(50) NOT NULL,
    report_ts TIMESTAMP NOT NULL,
    cpu_percent_sum DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_mb_sum DOUBLE PRECISION NOT NULL DEFAULT 0,
    gpu_process_count INT NOT NULL DEFAULT 0,
    cpu_process_count INT NOT NULL DEFAULT 0,
    ssh_user_count INT NOT NULL DEFAULT 0,
    ssh_users_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    cost_total DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_node_runtime_snapshots_node_ts
ON node_runtime_snapshots(node_id, report_ts DESC);

-- 特殊用户月度积分规则（覆盖默认发放规则）
CREATE TABLE IF NOT EXISTS special_monthly_points_rules (
    username VARCHAR(50) PRIMARY KEY,
    monthly_points DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (monthly_points >= 0),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    note TEXT NOT NULL DEFAULT '',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- 月初积分重置执行记录（避免重复执行）
CREATE TABLE IF NOT EXISTS monthly_points_reset_runs (
    month_key VARCHAR(7) PRIMARY KEY, -- YYYY-MM
    run_at TIMESTAMP NOT NULL DEFAULT NOW(),
    run_by VARCHAR(50) NOT NULL DEFAULT 'system',
    total_users INT NOT NULL DEFAULT 0,
    changed_users INT NOT NULL DEFAULT 0,
    forced BOOLEAN NOT NULL DEFAULT FALSE
);
