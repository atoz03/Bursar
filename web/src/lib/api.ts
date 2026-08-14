export type ApiError = {
  message: string;
  status?: number;
  body?: string;
};

function normalizeServerError(status: number, bodyText: string): ApiError {
  const text = (bodyText ?? "").trim();
  let serverMsg = "";
  if (text) {
    try {
      const j = JSON.parse(text);
      serverMsg = String(j?.error ?? j?.message ?? "").trim();
    } catch {
      serverMsg = text;
    }
  }
  const m: Record<string, string> = {
    unauthorized: "未授权，请重新登录后重试",
    csrf_required: "登录状态已过期，请刷新页面后重试",
    invalid_credentials: "用户名或密码错误",
    totp_required: "该账号已启用双重验证，请输入 6 位动态验证码",
    totp_invalid: "双重验证码错误，请重试",
    pending_review: "管理员正在审核该账号，请耐心等待审核结果",
    pending_email_verification: "该账号尚未完成邮箱验证，请先前往邮箱点击验证链接",
    blacklisted_account: "该账号已进入黑名单，无法登录",
    captcha_missing: "请先完成登录验证码",
    captcha_invalid: "验证码错误，请换一题后重试",
    captcha_expired: "验证码已过期，请换一题后重试",
    captcha_used: "验证码已失效，请换一题后重试",
    session_disabled: "当前未启用登录会话",
    not_found: "请求的资源不存在",
    forbidden: "当前账号没有权限执行该操作",
  };
  const msg = m[serverMsg] || serverMsg || `请求失败（状态码 ${status}）`;
  return { message: msg, status, body: text };
}

export type AuthMeResp = {
  authenticated: boolean;
  username?: string;
  role?: string;
  two_factor_enabled?: boolean;
  can_view_board?: boolean;
  can_view_nodes?: boolean;
  can_manage_nodes?: boolean;
  can_manage_points?: boolean;
  can_points_users?: boolean;
  can_points_batch_filtered?: boolean;
  can_points_batch_all?: boolean;
  can_points_records?: boolean;
  can_points_monthly?: boolean;
  can_points_special_rules?: boolean;
  can_review_requests?: boolean;
  can_manage_platform_users?: boolean;
  expires_at?: string;
  csrf_token?: string;
  server_now?: string;
  server_tz_name?: string;
  server_tz_offset_minutes?: number;
};

export type UserGuidelineResp = {
  content: string;
  updated_by?: string;
  updated_at?: string | null;
};

export type UserProfile = {
  username: string;
  platform_uid?: number;
  email?: string;
  real_name?: string;
  student_id?: string;
  advisor?: string;
  expected_graduation_year?: number;
  expected_graduation_month?: number;
  phone?: string;
  role: string;
};

export type AdminProfile = {
  username: string;
  real_name: string;
  email: string;
  phone: string;
  created_at: string;
  updated_at: string;
};

export type TwoFactorState = {
  username: string;
  role: string;
  enabled: boolean;
  pending_setup: boolean;
  issuer: string;
  account_name: string;
};

export type TwoFactorSetup = {
  username: string;
  role: string;
  enabled: boolean;
  issuer: string;
  account_name: string;
  secret: string;
  otpauth_url: string;
};

export type BalanceResp = {
  username: string;
  balance: number;
  general_balance?: number;
  carryover_balance?: number;
  exclusive_balance?: number;
  total_balance?: number;
  exclusive_balances?: NodeExclusivePointsBalance[];
  status: "normal" | "warning" | "limited" | "blocked" | string;
  account_created_at?: string | null;
  month_remaining_points?: number;
  month_used_points?: number;
  total_used_points?: number;
  warning_threshold_points?: number;
  limited_threshold_points?: number;
  monthly_max_overdraft_limit?: number;
  current_overdraft_points?: number;
  overdraft_exceeded?: boolean;
  manual_blocked?: boolean;
};

export type NodeExclusivePointsBalance = {
  username: string;
  node_id: string;
  balance: number;
  updated_by: string;
  created_at: string;
  updated_at: string;
};

export type UsageRecord = {
  node_id: string;
  username: string;
  local_username?: string;
  billing_username?: string;
  registered?: boolean;
  timestamp: string;
  pid?: number;
  cpu_percent: number;
  memory_mb: number;
  gpu_count?: number;
  command?: string;
  gpu_usage: string;
  cost: number;
};

export type ProcessKillRecord = {
  record_id: number;
  node_id: string;
  local_username: string;
  billing_username: string;
  registered?: boolean;
  action_type: string;
  reason: string;
  pids: number[];
  timestamp: string;
};

export type UsageUserSummary = {
  username: string;
  usage_records: number;
  gpu_process_records: number;
  cpu_process_records: number;
  total_cpu_percent: number;
  total_memory_mb: number;
  total_cost: number;
};

export type UsageMonthlySummary = {
  month: string;
  username: string;
  usage_records: number;
  gpu_process_records: number;
  cpu_process_records: number;
  total_cpu_percent: number;
  total_memory_mb: number;
  total_cost: number;
};

export type RechargeSummary = {
  username: string;
  recharge_count: number;
  recharge_total: number;
  last_recharge: string;
};

export type NodeStatus = {
  node_id: string;
  last_seen_at: string;
  last_report_id: string;
  last_report_ts: string;
  interval_seconds: number;
  cpu_model?: string;
  cpu_count?: number;
  gpu_model?: string;
  gpu_count?: number;
  os_version?: string;
  kernel_version?: string;
  agent_version?: string;
  node_ip?: string;
  node_mac?: string;
  disk_total_gb?: number;
  disk_used_gb?: number;
  home_total_gb?: number;
  home_used_gb?: number;
  mnt_total_gb?: number;
  mnt_used_gb?: number;
  net_rx_mb_month?: number;
  net_tx_mb_month?: number;
  gpu_process_count: number;
  cpu_process_count: number;
  usage_records_count: number;
  ssh_active_count?: number;
  disk_quota_installed?: boolean;
  disk_quota_mounts?: string[];
  system_services_checked_at?: string | null;
  system_services?: NodeSystemServiceStatus[];
  ssh_guard_enabled?: boolean;
  ssh_exclusive_enabled?: boolean;
  points_intercept_enabled?: boolean;
  points_throttle_threshold?: number;
  points_limited_cpu_quota_percent?: number;
  points_blocked_cpu_quota_percent?: number;
  points_overdraft_memory_limit_gb?: number;
  disk_quota_enabled?: boolean;
  disk_quota_mountpoint?: string;
  disk_quota_soft_mb?: number;
  disk_quota_hard_mb?: number;
  node_price_per_minute?: number | null;
  node_model_price_overrides?: Record<string, number>;
  security_event_count_7d?: number;
  suspicious_user_count_7d?: number;
  cost_total: number;
  updated_at: string;
};

export type GPUDeviceStatus = {
  index: number;
  uuid?: string;
  name?: string;
  bus_id?: string;
  utilization_percent: number;
  memory_used_mb: number;
  memory_total_mb: number;
  temperature_c?: number;
  power_draw_w?: number;
  power_limit_w?: number;
  compute_processes: number;
};

export type NodeMonitorStatus = NodeStatus & {
  monitor_report_ts?: string;
  monitor_metrics_available: boolean;
  host_cpu_percent: number;
  host_memory_total_mb: number;
  host_memory_used_mb: number;
  host_load_1: number;
  host_load_5: number;
  host_load_15: number;
  host_uptime_seconds: number;
  agent_memory_mb: number;
  gpu_devices: GPUDeviceStatus[];
};

export type NodeSystemServiceStatus = {
  name: string;
  deployed: boolean;
  load_state?: string;
  unit_file_state?: string;
  active_state?: string;
  sub_state?: string;
  healthy: boolean;
};

export type NodeModelPriceOverride = {
  gpu_model: string;
  price_per_minute: number;
};

export type NodeViewAccessResp = {
  node_id: string;
  restricted: boolean;
  allowed_power_users: string[];
  candidates: string[];
};

export type NodeSSHExclusiveResp = {
  node_id: string;
  enabled: boolean;
  block_other_ssh: boolean;
  exclusive_users: string[];
  candidate_local_users: string[];
  gpu_count: number;
  gpu_assignments: Array<{
    local_username: string;
    gpu_indices: number[];
  }>;
  exempt_users?: string[];
};

export type NodeLocalUser = {
  node_id: string;
  local_username: string;
  platform_username?: string;
  mapping_exists: boolean;
  platform_exists: boolean;
  admin_mapping?: boolean;
  admin_username?: string;
  cpu_quota_percent?: number;
  cpu_quota_reason?: string;
  cpu_quota_updated_at?: string;
  memory_limit_gb?: number;
  memory_limit_reason?: string;
  memory_limit_updated_at?: string;
  gpu_visible_indices?: number[];
  gpu_visibility_reason?: string;
  gpu_visibility_updated_at?: string;
  home_created_at?: string;
  last_login_at?: string;
  home_used_gb: number;
  quota_mountpoint?: string;
  quota_used_mb?: number;
  quota_soft_mb?: number;
  quota_hard_mb?: number;
  has_sudo?: boolean;
  has_docker?: boolean;
  updated_at?: string;
};

export type NodeUserCPULimit = {
  node_id: string;
  local_username: string;
  billing_username?: string;
  mapping_exists: boolean;
  platform_exists: boolean;
  cpu_quota_percent: number;
  reason?: string;
  updated_by?: string;
  updated_at: string;
};

export type NodeUserMemoryLimit = {
  node_id: string;
  local_username: string;
  billing_username?: string;
  mapping_exists: boolean;
  platform_exists: boolean;
  memory_limit_gb: number;
  reason?: string;
  updated_by?: string;
  updated_at: string;
};

export type NodeUserGPUVisibility = {
  node_id: string;
  local_username: string;
  billing_username?: string;
  mapping_exists: boolean;
  platform_exists: boolean;
  admin_mapping?: boolean;
  admin_username?: string;
  gpu_indices: number[];
  reason?: string;
  updated_by?: string;
  updated_at: string;
};

export type NodeRuntimeSnapshot = {
  report_id: string;
  node_id: string;
  report_ts: string;
  cpu_percent_sum: number;
  memory_mb_sum: number;
  gpu_process_count: number;
  cpu_process_count: number;
  ssh_user_count: number;
  ssh_users: string[];
  cost_total: number;
};

export type NodeDetailResp = {
  node: NodeStatus;
  latest: NodeRuntimeSnapshot;
  history: NodeRuntimeSnapshot[];
  local_users: NodeLocalUser[];
  monthly_user_costs?: NodeMonthlyUserCost[];
  monthly_from?: string;
  monthly_to?: string;
  security_events?: NodeSecurityEvent[];
  suspicious_users?: NodeSuspiciousUser[];
  from: string;
  to: string;
};

export type NodeMonthlyUserCost = {
  node_id: string;
  platform_username: string;
  usage_records: number;
  total_cost: number;
  last_usage_at: string;
};

export type NodeSecurityEvent = {
  event_id: number;
  report_id: string;
  node_id: string;
  event_type: string;
  severity: string;
  reason: string;
  related_usernames: string[];
  details: string;
  created_at: string;
};

export type NodeSecurityEventSummary = {
  event_type: string;
  severity: string;
  normalized_reason: string;
  event_count: number;
  affected_users: number;
  first_seen_at: string;
  last_seen_at: string;
};

export type NodeSuspiciousUser = {
  node_id: string;
  username: string;
  hit_count: number;
  last_seen_at: string;
  reason_hints: string;
  phenomena?: string;
  mining_suspected?: boolean;
};

export type NodeSSHUserPlatformResp = {
  node_id: string;
  local_username: string;
  mapping_exists: boolean;
  mapped_username?: string;
  platform_exists: boolean;
  platform_username?: string;
  admin_mapping?: boolean;
  admin_username?: string;
  real_name?: string;
  message: string;
};

export type PointsUser = {
  username: string;
  email?: string;
  real_name?: string;
  student_id: string;
  advisor?: string;
  phone?: string;
  expected_graduation_year?: number;
  expected_graduation_month?: number;
  role?: string;
  balance: number;
  general_balance?: number;
  carryover_balance?: number;
  exclusive_balance?: number;
  total_balance?: number;
  status: string;
};

export type SpecialMonthlyPointsRule = {
  username: string;
  monthly_points: number;
  enabled: boolean;
  note: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
};

export type MonthlyPointsResetRun = {
  month_key: string;
  run_at: string;
  run_by: string;
  total_users: number;
  changed_users: number;
  forced: boolean;
};

export type MonthlyPointsConfig = {
  doctor_points: number;
  master_points: number;
  other_points: number;
  carryover_limit: number;
  max_overdraft_limit: number;
};

export type PointsOperationRecord = {
  recharge_id: number;
  username: string;
  target_account?: string;
  amount: number;
  method: string;
  points_scope?: "general" | "carryover" | "node_exclusive" | string;
  node_id?: string;
  reason?: string;
  created_at: string;
};

export type UserNodeAccount = {
  node_id: string;
  local_username: string;
  billing_username: string;
  platform_uid?: number;
  node_uid?: number;
  node_primary_gid?: number;
  identity_aligned?: boolean;
  identity_initializing?: boolean;
  created_at: string;
  updated_at: string;
};

export type AdminAccountReadinessResp = {
  status: "all" | "initializing" | "failed" | string;
  total_not_ready: number;
  total_initializing: number;
  total_failed: number;
  accounts: UserNodeAccount[];
};

export type UserNodeBindChallengeInfo = {
  challenge_id: number;
  request_id: number;
  node_id: string;
  local_username: string;
  status: string;
  expires_at: string;
  challenge_token: string;
  claim_command: string;
};

export type UserNodeBindCooldown = {
  billing_username: string;
  failure_streak: number;
  cooldown_until?: string;
  last_failed_at?: string;
  last_succeeded_at?: string;
  updated_at: string;
};

export type UserMappedNodeInfo = {
  node_id: string;
  local_username: string;
  cpu_model?: string;
  cpu_count?: number;
  gpu_model?: string;
  gpu_count?: number;
  effective_gpu_price_per_minute: number;
  gpu_price_source: string;
  effective_cpu_price_per_core_minute: number;
  cpu_price_source: string;
  home_quota_mountpoint?: string;
  home_quota_used_mb?: number;
  home_quota_soft_mb?: number;
  home_quota_hard_mb?: number;
  home_quota_updated_at?: string;
  home_quota_enforced?: boolean;
};

export type UserNodeUnbindRecord = {
  record_id: number;
  source_type: "user_request" | "admin_forced" | string;
  request_id?: number;
  billing_username: string;
  node_id: string;
  local_username: string;
  status: "pending" | "approved" | "rejected" | "forced" | string;
  reason: string;
  initiated_by: string;
  reviewed_by?: string;
  reviewed_at?: string;
  executed_at?: string;
  created_at: string;
  updated_at: string;
};

export type NodeBindSecurityPolicy = {
  challenge_window_seconds: number;
  trial_cpu_quota_percent: number;
  trial_memory_limit_gb: number;
  trial_gpu_blocked: boolean;
  single_active_challenge_per_billing: boolean;
  first_failure_cooldown_minutes: number;
  repeat_failure_cooldown_minutes: number;
  contention_freeze_minutes: number;
};

export type AdminUserNodeBindCooldownRow = {
  billing_username: string;
  failure_streak: number;
  cooldown_until?: string;
  last_failed_at?: string;
  last_succeeded_at?: string;
  updated_at: string;
  remaining_cooldown_seconds: number;
  active_challenge_id?: number;
  active_challenge_node_id?: string;
  active_challenge_local_username?: string;
  active_challenge_expires_at?: string;
};

export type UserNodeAccountMappingRisk = {
  node_id: string;
  local_username: string;
  current_billing_username: string;
  switch_count: number;
  distinct_billing_count: number;
  platform_usernames: string[];
  switch_history?: string[];
  last_changed_at: string;
  risk_reason: string;
};

export type AdminAccountProvisionResp = {
  ok: boolean;
  node_id: string;
  local_username: string;
  billing_username: string;
  reissued_key?: boolean;
  local_user_existed?: boolean;
  email: string;
  ssh_host: string;
  ssh_port: number;
  download_filename: string;
  ssh_command: string;
  decrypt_url: string;
  decrypt_code: string;
  encrypted_payload: string;
  mail_sent: boolean;
  mail_error?: string;
  log_error?: string;
  notice_error?: string;
  message: string;
};

export type UserProvisionMessage = {
  message_id: number;
  billing_username: string;
  node_id: string;
  local_username: string;
  encrypted_payload: string;
  decrypt_url: string;
  ssh_host: string;
  ssh_port: number;
  download_filename: string;
  ssh_command: string;
  mail_to: string;
  created_by: string;
  created_at: string;
  first_decrypted_at?: string;
  destroy_after_at?: string;
  destroyed_at?: string;
};

export type AdminAccountProvisionLog = {
  provision_id: number;
  billing_username: string;
  node_id: string;
  local_username: string;
  email: string;
  ssh_host: string;
  ssh_port: number;
  download_filename: string;
  mail_sent: boolean;
  mail_error: string;
  created_by: string;
  created_at: string;
};

export type ProvisionKeyEnvelope = {
  v: string;
  alg: string;
  salt?: string;
  nonce?: string;
  ciphertext?: string;
  age_ciphertext?: string;
  node_id: string;
  local_username: string;
  billing_username: string;
  ssh_host: string;
  ssh_port: number;
  file_name: string;
  issued_at: string;
};

export type SSHWhitelistEntry = {
  node_id: string;
  local_username: string;
  billing_username: string;
  source_type: string;
  source_platform_username: string;
  created_by: string;
  reason: string;
  created_at: string;
  updated_at: string;
};

export type SSHBlacklistEntry = {
  node_id: string;
  local_username: string;
  billing_username: string;
  source_type: string;
  source_platform_username: string;
  created_by: string;
  reason: string;
  created_at: string;
  updated_at: string;
};

export type SSHExemptionEntry = {
  node_id: string;
  local_username: string;
  billing_username: string;
  source_type: string;
  source_platform_username: string;
  created_by: string;
  reason: string;
  created_at: string;
  updated_at: string;
};

export type SSHTemporaryUserEntry = {
  node_id: string;
  local_username: string;
  billing_username: string;
  source_type: string;
  source_platform_username: string;
  created_by: string;
  reason: string;
  created_at: string;
  updated_at: string;
};

export type PriceRow = { Model?: string; Price?: number; model?: string; price?: number };
export type UsageDayStat = { date: string; record_count: number; estimated_csv_bytes: number };
export type UsageRetentionStatus = {
  retention_days: number;
  enabled: boolean;
  last_deleted_at?: string;
  last_deleted_day?: string;
  last_deleted_from?: string;
  last_deleted_to?: string;
  last_deleted_records: number;
  last_deleted_mode?: string;
  last_deleted_by?: string;
};

export type RegistryResolveResp = {
  registered: boolean;
  billing_username?: string;
  blacklisted?: boolean;
  exempted?: boolean;
};

export type UserRequest = {
  request_id: number;
  request_type: "bind" | "open" | string;
  billing_username: string;
  node_id: string;
  local_username: string;
  message: string;
  status: "pending" | "approved" | "rejected" | string;
  reviewed_by?: string;
  reviewed_at?: string;
  reject_reason?: string;
  created_at: string;
  updated_at: string;
  apply_count_by_billing?: number;
  duplicate_flag?: boolean;
  duplicate_reason?: string;
};

export type RegistrationRequest = {
  request_id: number;
  username: string;
  email: string;
  real_name: string;
  student_id: string;
  advisor: string;
  expected_graduation_year: number;
  expected_graduation_month: number;
  phone: string;
  status: "pending" | "approved" | "rejected" | string;
  reviewed_by?: string;
  reviewed_at?: string;
  reject_reason?: string;
  reject_notify_mail_checked?: boolean;
  reject_notify_mail_sent?: boolean;
  reject_notify_mail_error?: string;
  created_at: string;
  updated_at: string;
};

export type RegistrationRequestView = RegistrationRequest & {
  conflict_fields?: string[];
  conflict_reason?: string;
};

export type RegisterCaptchaChallenge = {
  captcha_id: string;
  question: string;
  options: number[];
  expire_at: string;
};

export type RegistrationDisposableEmailDomain = {
  domain: string;
  enabled: boolean;
  note: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
};

export type RegistrationSecurityEvent = {
  event_id: number;
  action: string;
  decision: string;
  reason: string;
  client_ip: string;
  username: string;
  email: string;
  student_id: string;
  user_agent: string;
  retry_at?: string;
  created_at: string;
};

export type Announcement = {
  announcement_id: number;
  title: string;
  content: string;
  pinned: boolean;
  attachment_filename?: string;
  attachment_content_type?: string;
  attachment_size_bytes?: number;
  attachment_sha256?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type AdminNote = {
  note_id: number;
  note_date: string;
  title: string;
  content: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
};

export type AdminUserDetail = {
  username: string;
  platform_uid?: number;
  is_platform_user?: boolean;
  role: string;
  created_at: string;
  can_view_board: boolean;
  can_view_nodes: boolean;
  can_review_requests: boolean;
  can_manage_platform_users?: boolean;
  two_factor_enabled?: boolean;
  email: string;
  student_id: string;
  real_name: string;
  advisor: string;
  expected_graduation_year: number;
  expected_graduation_month: number;
  phone: string;
  balance: number;
  carryover_balance?: number;
  exclusive_balance?: number;
  total_balance?: number;
  status: string;
  usage_records: number;
  total_cost: number;
  last_usage_at: string;
  node_accounts: UserNodeAccount[];
};

export type PlatformUserDetail = {
  username: string;
  platform_uid?: number;
  email: string;
  real_name: string;
  student_id: string;
  advisor: string;
  expected_graduation_year: number;
  expected_graduation_month: number;
  phone: string;
  role: string;
  two_factor_enabled?: boolean;
  balance: number;
  general_balance?: number;
  carryover_balance?: number;
  exclusive_balance?: number;
  total_balance?: number;
  exclusive_balances?: NodeExclusivePointsBalance[];
  month_used_points?: number;
  total_used_points?: number;
  status: string;
  created_at?: string;
  updated_at?: string;
  node_accounts: UserNodeAccount[];
};

export type DeletedUserAccount = {
  deleted_id: number;
  username: string;
  platform_uid?: number;
  email: string;
  student_id: string;
  real_name: string;
  advisor: string;
  expected_graduation_year: number;
  expected_graduation_month: number;
  phone: string;
  role: string;
  balance: number;
  carryover_balance?: number;
  user_status: string;
  deleted_at: string;
  uid_release_at?: string;
  uid_release_remaining_seconds?: number;
  deleted_by: string;
  delete_reason: string;
  restored_at?: string;
  restored_by?: string;
};

export type GraduationDueUser = {
  username: string;
  email: string;
  student_id: string;
  real_name: string;
  advisor: string;
  expected_graduation_year: number;
  expected_graduation_month: number;
  overdue_months: number;
};

export type GraduationReminderResult = {
  username: string;
  email: string;
  success: boolean;
  error?: string;
};

export type ProfileChangeRequest = {
  request_id: number;
  billing_username: string;
  old_username: string;
  old_email: string;
  old_student_id: string;
  new_username: string;
  new_email: string;
  new_student_id: string;
  reason: string;
  status: "pending" | "approved" | "rejected" | string;
  reviewed_by?: string;
  reviewed_at?: string;
  created_at: string;
  updated_at: string;
};

export type PlatformUsageUserSummary = {
  platform_username: string;
  usage_records: number;
  cpu_usage_seconds: number;
  gpu_usage_seconds: number;
  cpu_util_percent: number;
  gpu_util_percent: number;
  total_cost: number;
  general_balance: number;
};

export type PlatformUsageNodeDetail = {
  node_id: string;
  cpu_model: string;
  cpu_count: number;
  gpu_model: string;
  gpu_count: number;
  last_seen_at: string;
  usage_records: number;
  total_cpu_percent: number;
  total_memory_mb: number;
  cpu_cost: number;
  gpu_cost: number;
  total_cost: number;
  last_usage_at: string;
};

export type PowerUser = {
  username: string;
  is_platform_user: boolean;
  can_view_board: boolean;
  can_view_nodes: boolean;
  can_manage_nodes: boolean;
  can_manage_points: boolean;
  can_points_users: boolean;
  can_points_batch_filtered: boolean;
  can_points_batch_all: boolean;
  can_points_records: boolean;
  can_points_monthly: boolean;
  can_points_special_rules: boolean;
  can_review_requests: boolean;
  can_manage_platform_users: boolean;
  created_by: string;
  updated_by: string;
  last_login_at?: string;
  created_at: string;
  updated_at: string;
};

export type HANodeSummary = {
  users_count: number;
  user_accounts_count: number;
  node_accounts_count: number;
  whitelist_count: number;
  blacklist_count: number;
  exemptions_count: number;
  usage_records_count: number;
  pending_requests_count: number;
  profile_pending_count: number;
  latest_usage_at: string;
  latest_node_seen_at: string;
  digest: string;
};

export type HANodeStatus = {
  enabled: boolean;
  node: string;
  role: string;
  listen_addr: string;
  checked_at: string;
  app_version?: string;
  app_commit?: string;
  app_build_at?: string;
  app_binary_sha256?: string;
  started_at?: string;
  uptime_seconds?: number;
  summary: HANodeSummary;
};

export type HAStatusResp = {
  enabled: boolean;
  local: HANodeStatus;
  peer: null | {
    reachable: boolean;
    url: string;
    error?: string;
    status?: HANodeStatus;
  };
  in_sync: boolean;
  version_match?: boolean;
  note?: string;
  checked?: string;
  peer_url?: string;
  message?: string;
};

export type HASyncConfig = {
  enabled: boolean;
  interval_days: number;
  start_hour: number;
  dr_node_id: string;
  dr_host: string;
  dr_ssh_port: number;
  dr_ssh_user: string;
  dr_key_file: string;
  dr_controller_port: number;
  primary_host: string;
  primary_controller_port: number;
  script_path: string;
  sync_web_dist: boolean;
  sync_database: boolean;
  auto_failover: boolean;
};

export type HASyncStep = {
  name: string;
  success: boolean;
  message: string;
};

export type HASyncRun = {
  run_id: number;
  trigger_mode: string;
  direction: string;
  status: string;
  started_by: string;
  started_at: string;
  finished_at?: string;
  summary: string;
  detail: HASyncStep[];
};

export type HASyncConfigResp = {
  config: HASyncConfig;
  runs: HASyncRun[];
  running: boolean;
  last_run?: HASyncRun;
  next_run_at?: string;
};

function trimSlashRight(v: string): string {
  return v.replace(/\/+$/, "");
}

export class ApiClient {
  private readonly adminToken: string;
  private csrfToken: string;

  constructor(
    private readonly baseUrl: string,
    private readonly opts: { adminToken?: string; csrfToken?: string } = {},
  ) {
    this.adminToken = this.opts.adminToken?.trim() ?? "";
    this.csrfToken = this.opts.csrfToken?.trim() ?? "";
  }

  private url(path: string): string {
    const base = this.baseUrl?.trim() ? trimSlashRight(this.baseUrl.trim()) : window.location.origin;
    // 兼容：若 baseUrl 已包含 /api，避免拼接成 /api/api/...
    if (base.endsWith("/api") && (path === "/api" || path.startsWith("/api/"))) {
      const suffix = path === "/api" ? "" : path.slice(4);
      return base + suffix;
    }
    return base + path;
  }

  private adminHeaders(): Record<string, string> {
    if (!this.adminToken) return {};
    return { Authorization: `Bearer ${this.adminToken}` };
  }

  private csrfHeaders(): Record<string, string> {
    if (!this.csrfToken) return {};
    return { "X-CSRF-Token": this.csrfToken };
  }

  private async readText(res: Response): Promise<string> {
    try {
      return await res.text();
    } catch {
      return "";
    }
  }

  private async getJson<T>(path: string, headers: Record<string, string> = {}): Promise<T> {
    const res = await fetch(this.url(path), { headers, credentials: "include" });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as T;
  }

  private async postJson<T>(path: string, body: unknown, headers: Record<string, string> = {}): Promise<T> {
    const doReq = async (): Promise<Response> => {
      return await fetch(this.url(path), {
        method: "POST",
        headers: { "Content-Type": "application/json", ...this.csrfHeaders(), ...headers },
        body: JSON.stringify(body),
        credentials: "include",
      });
    };

    let res = await doReq();
    if (!res.ok) {
      const text = await this.readText(res);

      // Web 登录会话下：可能是 CSRF token 过期（服务端会返回 csrf_required）。
      // 仅在“未使用 Bearer admin_token”时尝试刷新一次 CSRF。
      if (res.status === 403 && !this.adminToken && text.includes("csrf_required")) {
        try {
          const me = await this.authMe();
          if (me.authenticated && me.csrf_token) {
            this.csrfToken = me.csrf_token;
            res = await doReq();
            if (res.ok) return (await res.json()) as T;
          }
        } catch {
          // 忽略刷新失败，走原始错误输出
        }
      }

      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as T;
  }

  private async postJsonTryPaths<T>(paths: string[], body: unknown, headers: Record<string, string> = {}): Promise<T> {
    let lastErr: any = null;
    for (const path of paths) {
      try {
        return await this.postJson<T>(path, body, headers);
      } catch (e: any) {
        lastErr = e;
        // 仅在明确 404 not_found 时继续兜底其他路径。
        if (e?.status === 404 && String(e?.message || "").trim() === "请求的资源不存在") {
          continue;
        }
        throw e;
      }
    }
    throw lastErr ?? new Error("请求失败");
  }

  private async getJsonTryPaths<T>(paths: string[], headers: Record<string, string> = {}): Promise<T> {
    let lastErr: any = null;
    for (const path of paths) {
      try {
        return await this.getJson<T>(path, headers);
      } catch (e: any) {
        lastErr = e;
        if (e?.status === 404 && String(e?.message || "").trim() === "请求的资源不存在") {
          continue;
        }
        throw e;
      }
    }
    throw lastErr ?? new Error("请求失败");
  }

  async healthz(): Promise<{ ok: boolean }> {
    return await this.getJson("/healthz");
  }

  async metricsText(): Promise<string> {
    const res = await fetch(this.url("/metrics"), { credentials: "include" });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return await res.text();
  }

  async authMe(): Promise<AuthMeResp> {
    return await this.getJson("/api/auth/me");
  }

  async authLogin(username: string, password: string, captchaID: string, captchaOption: number, totpCode = ""): Promise<{ ok: boolean }> {
    return await this.postJson("/api/auth/login", {
      username,
      password,
      totp_code: totpCode,
      captcha_id: captchaID,
      captcha_option: captchaOption,
    });
  }

  async authLoginCaptcha(): Promise<RegisterCaptchaChallenge> {
    return await this.getJson("/api/auth/login/captcha");
  }

  async authRegister(payload: {
    email: string;
    username: string;
    password: string;
    captcha_id: string;
    captcha_option: number;
    real_name: string;
    student_id: string;
    advisor: string;
    expected_graduation_year: number;
    expected_graduation_month: number;
    phone: string;
    accept_guideline: boolean;
  }): Promise<{ ok: boolean; message: string }> {
    return await this.postJson("/api/auth/register", payload);
  }

  async authRegisterCheck(payload: { username?: string; email?: string; student_id?: string }): Promise<{ ok: boolean; errors: Record<string, string> }> {
    const q = new URLSearchParams();
    if ((payload.username ?? "").trim()) q.set("username", (payload.username ?? "").trim());
    if ((payload.email ?? "").trim()) q.set("email", (payload.email ?? "").trim());
    if ((payload.student_id ?? "").trim()) q.set("student_id", (payload.student_id ?? "").trim());
    return await this.getJson(`/api/auth/register/check?${q.toString()}`);
  }

  async authRegisterCaptcha(): Promise<RegisterCaptchaChallenge> {
    return await this.getJson("/api/auth/register/captcha");
  }

  async authForgotPassword(email: string): Promise<{ ok: boolean }> {
    return await this.postJson("/api/auth/forgot-password", { email });
  }

  async authResetPassword(payload: { username: string; token: string; new_password: string }): Promise<{ ok: boolean }> {
    return await this.postJson("/api/auth/reset-password", payload);
  }

  async authChangePassword(currentPassword: string, newPassword: string): Promise<{ ok: boolean }> {
    return await this.postJson("/api/auth/change-password", {
      current_password: currentPassword,
      new_password: newPassword,
    });
  }

  async auth2faStatus(): Promise<TwoFactorState> {
    return await this.getJson("/api/auth/2fa");
  }

  async auth2faSetup(): Promise<TwoFactorSetup> {
    return await this.postJson("/api/auth/2fa/setup", {});
  }

  async auth2faEnable(code: string): Promise<{ ok: boolean; state?: TwoFactorState }> {
    return await this.postJson("/api/auth/2fa/enable", { code });
  }

  async auth2faDisable(password: string, code: string): Promise<{ ok: boolean }> {
    return await this.postJson("/api/auth/2fa/disable", { password, code });
  }

  async authLogout(): Promise<{ ok: boolean }> {
    return await this.postJson("/api/auth/logout", {});
  }

  async announcements(limit = 20): Promise<{ announcements: Announcement[] }> {
    return await this.getJson(`/api/announcements?limit=${limit}`);
  }

  announcementAttachmentUrl(id: number): string {
    return this.url(`/api/announcements/${encodeURIComponent(String(id))}/attachment`);
  }

  async guideline(): Promise<UserGuidelineResp> {
    return await this.getJson("/api/guideline");
  }

  async userBalance(username: string): Promise<BalanceResp> {
    return await this.getJson(`/api/users/${encodeURIComponent(username)}/balance`);
  }

  async userUsage(username: string, limit: number): Promise<{ records: UsageRecord[] }> {
    return await this.getJson(`/api/users/${encodeURIComponent(username)}/usage?limit=${limit}`);
  }

  async userMe(): Promise<UserProfile> {
    return await this.getJson("/api/user/me");
  }

  async userUpdateProfile(payload: {
    email: string;
    username: string;
    student_id: string;
    real_name: string;
    advisor: string;
    expected_graduation_year: number;
    expected_graduation_month: number;
    phone: string;
    change_reason: string;
  }): Promise<{ ok: boolean; profile_updated: boolean; request_submitted: boolean; message: string }> {
    const res = await fetch(this.url("/api/user/me/profile"), {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...this.csrfHeaders() },
      credentials: "include",
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean; profile_updated: boolean; request_submitted: boolean; message: string };
  }

  async userProfileChangeRequests(limit: number): Promise<{ requests: ProfileChangeRequest[] }> {
    return await this.getJson(`/api/user/me/profile-change-requests?limit=${limit}`);
  }

  async userMyBalance(): Promise<BalanceResp> {
    return await this.getJson("/api/user/me/balance");
  }

  async userMyPointsIncrements(params?: { sinceId?: number; limit?: number }): Promise<{
    records: PointsOperationRecord[];
    unread_count: number;
    unread_amount: number;
    latest_recharge_id: number;
  }> {
    const q = new URLSearchParams();
    if (Number.isFinite(Number(params?.sinceId)) && Number(params?.sinceId) > 0) {
      q.set("since_id", String(Math.floor(Number(params?.sinceId))));
    }
    q.set("limit", String(params?.limit ?? 200));
    return await this.getJson(`/api/user/me/points-increments?${q.toString()}`);
  }

  async userMyUsage(limit: number): Promise<{ records: UsageRecord[] }> {
    return await this.getJson(`/api/user/me/usage?limit=${limit}`);
  }

  async userAccounts(): Promise<{
    accounts: UserNodeAccount[];
    active_challenge?: UserNodeBindChallengeInfo;
    bind_cooldown?: UserNodeBindCooldown;
    mapped_node_infos?: UserMappedNodeInfo[];
  }> {
    return await this.getJson("/api/user/accounts");
  }

  async userProvisionMessages(limit = 100): Promise<{ messages: UserProvisionMessage[] }> {
    return await this.getJson(`/api/user/provision-messages?limit=${limit}`);
  }

  async userProvisionMessageDecryptStart(messageId: number): Promise<{ ok: boolean; message: UserProvisionMessage }> {
    return await this.postJson(`/api/user/provision-messages/${Math.floor(Number(messageId))}/decrypt-start`, {});
  }

  async userUpsertAccount(
    nodeId: string,
    localUsername: string,
    message = "",
  ): Promise<{ ok: boolean; challenge: UserNodeBindChallengeInfo; policy: NodeBindSecurityPolicy }> {
    return await this.postJson("/api/user/accounts", { node_id: nodeId, local_username: localUsername, message });
  }

  async userUpdateAccount(payload: {
    old_node_id: string;
    old_local_username: string;
    new_node_id: string;
    new_local_username: string;
  }): Promise<{ ok: boolean }> {
    const res = await fetch(this.url("/api/user/accounts"), {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...this.csrfHeaders() },
      credentials: "include",
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async userDeleteAccount(nodeId: string, localUsername: string, reason: string): Promise<{ ok: boolean; request_id: number; message: string }> {
    const q = new URLSearchParams({ node_id: nodeId, local_username: localUsername, reason });
    const res = await fetch(this.url(`/api/user/accounts?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean; request_id: number; message: string };
  }

  async registryResolve(nodeId: string, localUsername: string): Promise<RegistryResolveResp> {
    const q = new URLSearchParams();
    q.set("node_id", nodeId.trim());
    q.set("local_username", localUsername.trim());
    return await this.getJson(`/api/registry/resolve?${q.toString()}`);
  }

  async userRequests(limit: number): Promise<{ requests: UserRequest[] }> {
    const q = new URLSearchParams();
    q.set("limit", String(limit));
    return await this.getJson(`/api/user/requests?${q.toString()}`);
  }

  async createBindRequests(
    items: Array<{ node_id: string; local_username: string }>,
    message: string,
  ): Promise<{ ok: boolean; request_ids: number[] }> {
    return await this.postJson("/api/user/requests/bind", { items, message });
  }

  async createOpenRequest(
    message: string,
    nodeId = "待分配",
    localUsername = "待分配",
  ): Promise<{ ok: boolean; request_id: number }> {
    return await this.postJson("/api/user/requests/open", {
      node_id: nodeId,
      local_username: localUsername,
      message,
    });
  }

  async adminUsers(): Promise<{ users: Array<{ Username?: string; Balance?: number; Status?: string; username?: string; balance?: number; status?: string }> }> {
    return await this.getJson("/api/admin/users", this.adminHeaders());
  }

  async adminMyProfile(): Promise<{ profile: AdminProfile }> {
    return await this.getJson("/api/admin/me/profile", this.adminHeaders());
  }

  async adminSetMyProfile(payload: { real_name: string; email: string; phone: string }): Promise<{ ok: boolean; profile: AdminProfile }> {
    const res = await fetch(this.url("/api/admin/me/profile"), {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean; profile: AdminProfile };
  }

  async adminUsersDetails(limit = 1000): Promise<{ users: AdminUserDetail[]; registered_count?: number }> {
    return await this.getJson(`/api/admin/users/details?limit=${limit}`, this.adminHeaders());
  }

  async adminExportPlatformUsersCSV(): Promise<Blob> {
    const res = await fetch(this.url("/api/admin/users/export.csv"), {
      headers: { ...this.adminHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return await res.blob();
  }

  async adminImportPlatformUsersCSV(file: File): Promise<{
    ok: boolean;
    imported: number;
    created: number;
    updated: number;
    promoted: number;
    demoted: number;
    registered_cnt: number;
  }> {
    const form = new FormData();
    form.append("file", file);
    const res = await fetch(this.url("/api/admin/users/import.csv"), {
      method: "POST",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
      body: form,
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as {
      ok: boolean;
      imported: number;
      created: number;
      updated: number;
      promoted: number;
      demoted: number;
      registered_cnt: number;
    };
  }

  async adminDeletedUsers(limit = 1000, includeRestored = false): Promise<{ users: DeletedUserAccount[] }> {
    return await this.getJson(`/api/admin/users/deleted?limit=${limit}&include_restored=${includeRestored ? 1 : 0}`, this.adminHeaders());
  }

  async adminUserProfile(username: string): Promise<{ user: PlatformUserDetail }> {
    return await this.getJson(`/api/admin/users/${encodeURIComponent(username)}/profile`, this.adminHeaders());
  }

  async adminUserTwoFactorEnable(username: string, role: string): Promise<{ ok: boolean; setup: TwoFactorSetup }> {
    return await this.postJson(`/api/admin/users/${encodeURIComponent(username)}/2fa/enable`, { role }, this.adminHeaders());
  }

  async adminUserTwoFactorDisable(username: string, role: string): Promise<{ ok: boolean }> {
    return await this.postJson(`/api/admin/users/${encodeURIComponent(username)}/2fa/disable`, { role }, this.adminHeaders());
  }

  async adminPlatformUserDetail(username: string): Promise<{ user: PlatformUserDetail }> {
    const u = String(username || "").trim();
    if (!u) throw { message: "username 不能为空", status: 400 } as ApiError;
    let user: PlatformUserDetail | null = null;
    try {
      const r = await this.adminUserProfile(u);
      user = {
        ...r.user,
        node_accounts: Array.isArray(r.user?.node_accounts) ? r.user.node_accounts : [],
      };
    } catch (e: any) {
      if (e?.status !== 404) throw e;
      const r = await this.adminUsersDetails(5000);
      const row = (r.users ?? []).find((x) => String(x.username || "").trim() === u);
      if (!row) throw e;
      user = {
        username: row.username,
        platform_uid: row.platform_uid,
        email: row.email,
        real_name: row.real_name,
        student_id: row.student_id,
        advisor: row.advisor,
        expected_graduation_year: row.expected_graduation_year,
        expected_graduation_month: row.expected_graduation_month,
        phone: row.phone,
        role: row.role,
        two_factor_enabled: row.two_factor_enabled,
        balance: row.balance,
        status: row.status,
        node_accounts: row.node_accounts ?? [],
      };
    }
    if (!user) throw { message: "平台账号不存在", status: 404 } as ApiError;
    if (!Array.isArray(user.node_accounts) || user.node_accounts.length === 0) {
      try {
        const accounts = await this.adminAccounts(u);
        user.node_accounts = accounts.accounts ?? [];
      } catch {
        user.node_accounts = user.node_accounts ?? [];
      }
    }
    return { user };
  }

  async adminBlockUser(username: string, reason = ""): Promise<{ ok: boolean; username: string; status: string }> {
    return await this.postJson(`/api/admin/users/${encodeURIComponent(username)}/block`, { reason }, this.adminHeaders());
  }

  async adminUnblockUser(username: string): Promise<{ ok: boolean; username: string; status: string }> {
    return await this.postJson(`/api/admin/users/${encodeURIComponent(username)}/unblock`, {}, this.adminHeaders());
  }

  async adminDeleteUser(username: string, reason = ""): Promise<{ ok: boolean; deleted: DeletedUserAccount }> {
    return await this.postJson(`/api/admin/users/${encodeURIComponent(username)}/delete`, { reason }, this.adminHeaders());
  }

  async adminDeleteUserArchive(username: string, reason = ""): Promise<{ ok: boolean; deleted: DeletedUserAccount }> {
    return await this.postJson(`/api/admin/users/${encodeURIComponent(username)}/archive`, { reason }, this.adminHeaders());
  }

  async adminDeleteUserRemove(username: string, reason = ""): Promise<{ ok: boolean; deleted: DeletedUserAccount }> {
    return await this.postJson(`/api/admin/users/${encodeURIComponent(username)}/remove`, { reason }, this.adminHeaders());
  }

  async adminDeleteUserCompat(username: string, reason = ""): Promise<{ ok: boolean; deleted: DeletedUserAccount }> {
    const u = encodeURIComponent(username);
    return await this.postJsonTryPaths<{ ok: boolean; deleted: DeletedUserAccount }>([
      `/api/admin/users/${u}/delete`,
      `/api/admin/users/${u}/archive`,
      `/api/admin/users/${u}/remove`,
      `/admin/users/${u}/delete`,
      `/admin/users/${u}/archive`,
      `/admin/users/${u}/remove`,
    ], { reason }, this.adminHeaders());
  }

  async adminRestoreDeletedUser(id: number): Promise<{ ok: boolean; restored: DeletedUserAccount }> {
    return await this.postJson(`/api/admin/users/deleted/${id}/restore`, {}, this.adminHeaders());
  }

  async adminFindUserDuplicates(params: { username?: string; email?: string; student_id?: string; limit?: number }): Promise<{ active_users: UserProfile[]; deleted_users: DeletedUserAccount[] }> {
    const q = new URLSearchParams();
    if ((params.username ?? "").trim()) q.set("username", (params.username ?? "").trim());
    if ((params.email ?? "").trim()) q.set("email", (params.email ?? "").trim());
    if ((params.student_id ?? "").trim()) q.set("student_id", (params.student_id ?? "").trim());
    q.set("limit", String(params.limit ?? 200));
    return await this.getJson(`/api/admin/users/duplicates?${q.toString()}`, this.adminHeaders());
  }

  async adminGraduationDueUsers(limit = 2000): Promise<{ users: GraduationDueUser[] }> {
    return await this.getJson(`/api/admin/users/graduation-due?limit=${limit}`, this.adminHeaders());
  }

  async adminSendGraduationReminders(usernames: string[]): Promise<{
    ok: boolean;
    total: number;
    success: number;
    failed: number;
    results: GraduationReminderResult[];
  }> {
    return await this.postJson("/api/admin/users/graduation-reminders/send", { usernames }, this.adminHeaders());
  }

  async adminPrices(): Promise<{ prices: Array<{ Model?: string; Price?: number; model?: string; price?: number }> }> {
    return await this.getJson("/api/admin/prices", this.adminHeaders());
  }

  async adminSetPrice(model: string, pricePerMinute: number): Promise<{ ok: boolean }> {
    return await this.postJson(
      "/api/admin/prices",
      { gpu_model: model, price_per_minute: pricePerMinute },
      this.adminHeaders(),
    );
  }

  async adminRecharge(username: string, amount: number, method: string): Promise<BalanceResp> {
    return await this.postJson(
      `/api/users/${encodeURIComponent(username)}/recharge`,
      { amount, method },
      this.adminHeaders(),
    );
  }

  async adminUsage(params: { billingUsername?: string; localUsername?: string; unregisteredOnly?: boolean; limit: number }): Promise<{ records: UsageRecord[]; kill_records: ProcessKillRecord[] }> {
    const q = new URLSearchParams();
    if ((params.billingUsername ?? "").trim()) q.set("billing_username", (params.billingUsername ?? "").trim());
    if ((params.localUsername ?? "").trim()) q.set("local_username", (params.localUsername ?? "").trim());
    if (params.unregisteredOnly) q.set("unregistered_only", "1");
    q.set("limit", String(params.limit));
    return await this.getJson(`/api/admin/usage?${q.toString()}`, this.adminHeaders());
  }

  async adminUsageDays(params: {
    billingUsername?: string;
    localUsername?: string;
    unregisteredOnly?: boolean;
    from?: string;
    to?: string;
  }): Promise<{ days: UsageDayStat[] }> {
    const q = new URLSearchParams();
    if ((params.billingUsername ?? "").trim()) q.set("billing_username", (params.billingUsername ?? "").trim());
    if ((params.localUsername ?? "").trim()) q.set("local_username", (params.localUsername ?? "").trim());
    if (params.unregisteredOnly) q.set("unregistered_only", "1");
    if ((params.from ?? "").trim()) q.set("from", (params.from ?? "").trim());
    if ((params.to ?? "").trim()) q.set("to", (params.to ?? "").trim());
    return await this.getJson(`/api/admin/usage/days?${q.toString()}`, this.adminHeaders());
  }

  async adminUsageRangeEstimate(params: {
    from: string;
    to: string;
    billingUsername?: string;
    localUsername?: string;
    unregisteredOnly?: boolean;
  }): Promise<{
    from: string;
    to: string;
    records: number;
    estimated_csv_bytes: number;
    estimated_db_bytes: number;
  }> {
    const q = new URLSearchParams();
    q.set("from", params.from);
    q.set("to", params.to);
    if ((params.billingUsername ?? "").trim()) q.set("billing_username", (params.billingUsername ?? "").trim());
    if ((params.localUsername ?? "").trim()) q.set("local_username", (params.localUsername ?? "").trim());
    if (params.unregisteredOnly) q.set("unregistered_only", "1");
    return await this.getJson(`/api/admin/usage/range-estimate?${q.toString()}`, this.adminHeaders());
  }

  async adminUsageRetentionGet(): Promise<UsageRetentionStatus> {
    return await this.getJson("/api/admin/usage/retention", this.adminHeaders());
  }

  async adminUsageRetentionSet(payload: { retention_days: number }): Promise<UsageRetentionStatus> {
    return await this.postJson("/api/admin/usage/retention", payload, this.adminHeaders());
  }

  async adminUsageDeleteRange(payload: {
    from: string;
    to: string;
    billing_username?: string;
    local_username?: string;
    unregistered_only?: boolean;
    confirm: boolean;
  }): Promise<{
    ok: boolean;
    from: string;
    to: string;
    records_before: number;
    deleted_records: number;
    estimated_csv_bytes: number;
    estimated_db_bytes: number;
  }> {
    return await this.postJson("/api/admin/usage/delete-range", payload, this.adminHeaders());
  }

  async adminNodes(limit: number): Promise<{ nodes: NodeStatus[] }> {
    return await this.getJson(`/api/admin/nodes?limit=${limit}`, this.adminHeaders());
  }

  async adminNodeMonitor(limit = 200): Promise<{ nodes: NodeMonitorStatus[]; generated_at: string }> {
    return await this.getJson(`/api/admin/node-monitor?limit=${limit}`, this.adminHeaders());
  }

  async adminNodeDetail(nodeId: string, params?: { minutes?: number; limit?: number }): Promise<NodeDetailResp> {
    const q = new URLSearchParams();
    if (params?.minutes) q.set("minutes", String(params.minutes));
    if (params?.limit) q.set("limit", String(params.limit));
    const suffix = q.toString() ? `?${q.toString()}` : "";
    return await this.getJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/detail${suffix}`, this.adminHeaders());
  }

  async adminNodeSSHUserPlatform(nodeId: string, localUsername: string): Promise<NodeSSHUserPlatformResp> {
    return await this.getJson(
      `/api/admin/nodes/${encodeURIComponent(nodeId)}/ssh-user/${encodeURIComponent(localUsername)}/platform`,
      this.adminHeaders(),
    );
  }

  async adminDisconnectNodeSSH(nodeId: string): Promise<{ ok: boolean; node_id: string; ssh_active_count: number; message: string }> {
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/ssh/disconnect-all`, {}, this.adminHeaders());
  }

  async adminKillAllUserProcesses(nodeId: string): Promise<{ ok: boolean; node_id: string; message: string }> {
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/processes/kill-all-users`, {}, this.adminHeaders());
  }

  async adminKillNodeUserProcesses(nodeId: string, localUsername: string): Promise<{
    ok: boolean;
    node_id: string;
    local_username: string;
    message: string;
  }> {
    return await this.postJson(
      `/api/admin/nodes/${encodeURIComponent(nodeId)}/processes/kill-user`,
      { local_username: localUsername },
      this.adminHeaders(),
    );
  }

  async adminDisconnectNodeSSHUser(nodeId: string, localUsername: string): Promise<{ ok: boolean; node_id: string; local_username: string; message: string }> {
    return await this.postJson(
      `/api/admin/nodes/${encodeURIComponent(nodeId)}/ssh/disconnect-user`,
      { local_username: localUsername },
      this.adminHeaders(),
    );
  }

  async adminSyncNodeNow(nodeId: string): Promise<{ ok: boolean; node_id: string; message: string }> {
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/sync-now`, {}, this.adminHeaders());
  }

  async adminSetNodeSSHGuard(nodeId: string, enabled: boolean): Promise<{
    ok: boolean;
    node_id: string;
    enabled: boolean;
    previous: boolean;
    kick_triggered: boolean;
  }> {
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/ssh-guard`, { enabled }, this.adminHeaders());
  }

  async adminNodePointsIntercept(nodeId: string): Promise<{
    ok: boolean;
    node_id: string;
    enabled: boolean;
    throttle_threshold_points?: number;
    limited_cpu_quota_percent?: number;
    blocked_cpu_quota_percent?: number;
    overdraft_memory_limit_gb?: number;
    effective_threshold_points?: number;
    effective_limited_cpu_quota?: number;
    effective_blocked_cpu_quota?: number;
    effective_overdraft_memory_gb?: number;
    default_threshold_points?: number;
    default_limited_cpu_quota?: number;
    default_blocked_cpu_quota?: number;
    default_overdraft_memory_gb?: number;
    cpu_control_enabled_on_server?: boolean;
  }> {
    return await this.getJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/points-intercept`, this.adminHeaders());
  }

  async adminSetNodePointsIntercept(
    nodeId: string,
    payloadOrEnabled:
      | boolean
      | {
          enabled: boolean;
          throttle_threshold_points?: number;
          limited_cpu_quota_percent?: number;
          blocked_cpu_quota_percent?: number;
          overdraft_memory_limit_gb?: number;
        },
  ): Promise<{
    ok: boolean;
    node_id: string;
    enabled: boolean;
    previous: boolean;
    throttle_threshold_points?: number;
    limited_cpu_quota_percent?: number;
    blocked_cpu_quota_percent?: number;
    overdraft_memory_limit_gb?: number;
    effective_threshold_points?: number;
    effective_limited_cpu_quota?: number;
    effective_blocked_cpu_quota?: number;
    effective_overdraft_memory_gb?: number;
    memory_sync_targets?: number;
  }> {
    const payload =
      typeof payloadOrEnabled === "boolean"
        ? { enabled: payloadOrEnabled }
        : {
            enabled: !!payloadOrEnabled.enabled,
            throttle_threshold_points: payloadOrEnabled.throttle_threshold_points,
            limited_cpu_quota_percent: payloadOrEnabled.limited_cpu_quota_percent,
            blocked_cpu_quota_percent: payloadOrEnabled.blocked_cpu_quota_percent,
            overdraft_memory_limit_gb: payloadOrEnabled.overdraft_memory_limit_gb,
          };
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/points-intercept`, payload, this.adminHeaders());
  }

  async adminNodeDiskQuota(nodeId: string): Promise<{
    ok: boolean;
    node_id: string;
    quota_installed: boolean;
    quota_mounts: string[];
    preferred_mountpoint?: string;
    effective_mountpoint?: string;
    enabled: boolean;
    mountpoint?: string;
    default_soft_mb?: number;
    default_hard_mb?: number;
    effective_soft_mb?: number;
    effective_hard_mb?: number;
    users: NodeLocalUser[];
    checked_at?: string;
  }> {
    return await this.getJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/disk-quota`, this.adminHeaders());
  }

  async adminSetNodeDiskQuota(
    nodeId: string,
    payload: {
      enabled: boolean;
      mountpoint?: string;
      default_soft_mb?: number;
      default_hard_mb?: number;
      apply_to_all?: boolean;
    },
  ): Promise<{
    ok: boolean;
    node_id: string;
    enabled: boolean;
    mountpoint?: string;
    default_soft_mb?: number;
    default_hard_mb?: number;
    effective_mountpoint?: string;
    effective_soft_mb?: number;
    effective_hard_mb?: number;
    quota_installed?: boolean;
    quota_mounts?: string[];
    applied_users?: number;
    warning?: string;
  }> {
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/disk-quota`, payload, this.adminHeaders());
  }

  async adminApplyNodeDiskQuota(
    nodeId: string,
    payload: {
      mountpoint?: string;
      all_users?: boolean;
      soft_mb?: number;
      hard_mb?: number;
      users?: Array<{ local_username: string; soft_mb: number; hard_mb: number }>;
    },
  ): Promise<{
    ok: boolean;
    node_id: string;
    mountpoint?: string;
    applied_users: number;
    quota_installed?: boolean;
    quota_mounts?: string[];
    message?: string;
  }> {
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/disk-quota/apply`, payload, this.adminHeaders());
  }

  async adminNodeCPULimits(nodeId: string, limit = 1000): Promise<{ node_id: string; rows: NodeUserCPULimit[] }> {
    return await this.getJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/cpu-limits?limit=${limit}`, this.adminHeaders());
  }

  async adminSetNodeUserCPULimit(
    nodeId: string,
    payload: { local_username: string; cpu_quota_percent: number; reason?: string },
  ): Promise<{ ok: boolean; node_id: string; local_username: string; row: NodeUserCPULimit }> {
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/cpu-limits`, payload, this.adminHeaders());
  }

  async adminDeleteNodeUserCPULimit(
    nodeId: string,
    localUsername: string,
  ): Promise<{ ok: boolean; node_id: string; local_username: string }> {
    const q = new URLSearchParams({ local_username: localUsername });
    const res = await fetch(this.url(`/api/admin/nodes/${encodeURIComponent(nodeId)}/cpu-limits?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean; node_id: string; local_username: string };
  }

  async adminCPULimits(params: { billingUsername?: string; nodeId?: string; limit?: number }): Promise<{
    billing_username: string;
    node_id: string;
    rows: NodeUserCPULimit[];
  }> {
    const q = new URLSearchParams();
    if (params.billingUsername) q.set("billing_username", params.billingUsername);
    if (params.nodeId) q.set("node_id", params.nodeId);
    q.set("limit", String(params.limit ?? 1000));
    return await this.getJson(`/api/admin/cpu-limits?${q.toString()}`, this.adminHeaders());
  }

  async adminNodeMemoryLimits(nodeId: string, limit = 1000): Promise<{ node_id: string; rows: NodeUserMemoryLimit[] }> {
    return await this.getJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/memory-limits?limit=${limit}`, this.adminHeaders());
  }

  async adminSetNodeUserMemoryLimit(
    nodeId: string,
    payload: { local_username: string; memory_limit_gb: number; reason?: string },
  ): Promise<{ ok: boolean; node_id: string; local_username: string; row: NodeUserMemoryLimit }> {
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/memory-limits`, payload, this.adminHeaders());
  }

  async adminDeleteNodeUserMemoryLimit(
    nodeId: string,
    localUsername: string,
  ): Promise<{ ok: boolean; node_id: string; local_username: string }> {
    const q = new URLSearchParams({ local_username: localUsername });
    const res = await fetch(this.url(`/api/admin/nodes/${encodeURIComponent(nodeId)}/memory-limits?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean; node_id: string; local_username: string };
  }

  async adminMemoryLimits(params: { billingUsername?: string; nodeId?: string; limit?: number }): Promise<{
    billing_username: string;
    node_id: string;
    rows: NodeUserMemoryLimit[];
  }> {
    const q = new URLSearchParams();
    if (params.billingUsername) q.set("billing_username", params.billingUsername);
    if (params.nodeId) q.set("node_id", params.nodeId);
    q.set("limit", String(params.limit ?? 1000));
    return await this.getJson(`/api/admin/memory-limits?${q.toString()}`, this.adminHeaders());
  }

  async adminSetNodeUserGPUVisibility(
    nodeId: string,
    payload: { local_username: string; gpu_indices: number[]; reason?: string },
  ): Promise<{
    ok: boolean;
    node_id: string;
    local_username: string;
    gpu_count?: number;
    row: {
      node_id: string;
      local_username: string;
      gpu_indices: number[];
      reason?: string;
      updated_by?: string;
      updated_at: string;
    };
  }> {
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/gpu-visibility`, payload, this.adminHeaders());
  }

  async adminDeleteNodeUserGPUVisibility(
    nodeId: string,
    localUsername: string,
  ): Promise<{ ok: boolean; node_id: string; local_username: string }> {
    const q = new URLSearchParams({ local_username: localUsername });
    const res = await fetch(this.url(`/api/admin/nodes/${encodeURIComponent(nodeId)}/gpu-visibility?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean; node_id: string; local_username: string };
  }

  async adminGPUVisibility(params: { billingUsername?: string; nodeId?: string; limit?: number }): Promise<{
    billing_username: string;
    node_id: string;
    rows: NodeUserGPUVisibility[];
  }> {
    const q = new URLSearchParams();
    if (params.billingUsername) q.set("billing_username", params.billingUsername);
    if (params.nodeId) q.set("node_id", params.nodeId);
    q.set("limit", String(params.limit ?? 1000));
    return await this.getJson(`/api/admin/gpu-visibility?${q.toString()}`, this.adminHeaders());
  }

  async adminNodePrice(nodeId: string): Promise<{
    ok: boolean;
    node_id: string;
    price_per_minute: number | null;
    cpu_price_per_core_minute: number | null;
    effective_price_per_minute: number;
    effective_cpu_price_per_core_minute: number;
    model_price_overrides: NodeModelPriceOverride[];
    mode: "default" | "custom" | string;
    default_price_per_minute: number;
    default_cpu_price_per_core_minute: number;
    billing_rules?: {
      gpu_formula?: string;
      cpu_formula?: string;
      combined_formula?: string;
      sample_interval_seconds?: number;
      billing_interval_formula?: string;
      gpu_price_priority?: string[];
      cpu_price_priority?: string[];
    };
  }> {
    return await this.getJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/price`, this.adminHeaders());
  }

  async adminSetNodePrice(
    nodeId: string,
    payload: { price_per_minute: number; cpu_price_per_core_minute?: number },
  ): Promise<{
    ok: boolean;
    node_id: string;
    previous_price_per_minute: number | null;
    previous_cpu_price_per_core_minute: number | null;
    previous_model_price_overrides?: NodeModelPriceOverride[];
    price_per_minute: number | null;
    cpu_price_per_core_minute: number | null;
    model_price_overrides: NodeModelPriceOverride[];
    mode: "default" | "custom" | string;
    default_price_per_minute: number;
    default_cpu_price_per_core_minute: number;
  }> {
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/price`, payload, this.adminHeaders());
  }

  async adminNodeSSHExclusive(nodeId: string): Promise<NodeSSHExclusiveResp> {
    return await this.getJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/ssh-exclusive`, this.adminHeaders());
  }

  async adminSetNodeSSHExclusive(
    nodeId: string,
    payload: {
      enabled: boolean;
      block_other_ssh: boolean;
      exclusive_users: string[];
      gpu_assignments: Array<{ local_username: string; gpu_indices: number[] }>;
    },
  ): Promise<{
    ok: boolean;
    node_id: string;
    enabled: boolean;
    block_other_ssh: boolean;
    exclusive_users: string[];
    gpu_assignments: Array<{ local_username: string; gpu_indices: number[] }>;
    exempt_ignored_users?: string[];
  }> {
    return await this.postJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/ssh-exclusive`, payload, this.adminHeaders());
  }

  async adminNodeViewAccess(nodeId: string): Promise<NodeViewAccessResp> {
    return await this.getJsonTryPaths<NodeViewAccessResp>([
      `/api/admin/nodes/${encodeURIComponent(nodeId)}/view-access`,
      `/admin/nodes/${encodeURIComponent(nodeId)}/view-access`,
    ], this.adminHeaders());
  }

  async adminSetNodeViewAccess(nodeId: string, payload: { restricted: boolean; allowed_power_users: string[] }): Promise<{
    ok: boolean;
    node_id: string;
    restricted: boolean;
    allowed_power_users: string[];
  }> {
    return await this.postJsonTryPaths<{
      ok: boolean;
      node_id: string;
      restricted: boolean;
      allowed_power_users: string[];
    }>([
      `/api/admin/nodes/${encodeURIComponent(nodeId)}/view-access`,
      `/admin/nodes/${encodeURIComponent(nodeId)}/view-access`,
    ], payload, this.adminHeaders());
  }

  async adminNodeSecurityEvents(
    nodeId: string,
    params?: { eventType?: string; limit?: number; summaryLimit?: number; from?: string; to?: string },
  ): Promise<{
    node_id: string;
    event_type: string;
    events: NodeSecurityEvent[];
    event_summaries: NodeSecurityEventSummary[];
    suspicious_users: NodeSuspiciousUser[];
    suspicious_days?: number;
    from?: string;
    to?: string;
    summary_normalizer?: string;
  }> {
    const q = new URLSearchParams();
    if ((params?.eventType ?? "").trim()) q.set("event_type", (params?.eventType ?? "").trim());
    q.set("limit", String(params?.limit ?? 200));
    q.set("summary_limit", String(params?.summaryLimit ?? 120));
    if ((params?.from ?? "").trim()) q.set("from", (params?.from ?? "").trim());
    if ((params?.to ?? "").trim()) q.set("to", (params?.to ?? "").trim());
    return await this.getJson(`/api/admin/nodes/${encodeURIComponent(nodeId)}/security-events?${q.toString()}`, this.adminHeaders());
  }

  async adminPointsUsers(params?: {
    keyword?: string;
    keywordField?: "all" | "username" | "real_name" | "student_id" | "advisor" | "email" | "phone";
    limit?: number;
  }): Promise<{ users: PointsUser[] }> {
    const q = new URLSearchParams();
    if ((params?.keyword ?? "").trim()) q.set("keyword", (params?.keyword ?? "").trim());
    if ((params?.keywordField ?? "").trim()) q.set("keyword_field", (params?.keywordField ?? "").trim());
    q.set("limit", String(params?.limit ?? 1000));
    return await this.getJson(`/api/admin/points/users?${q.toString()}`, this.adminHeaders());
  }

  async adminPointsAdjust(payload: {
    username: string;
    delta: number;
    reason?: string;
    scope?: "general" | "carryover" | "node_exclusive";
    node_id?: string;
  }): Promise<{
    ok: boolean;
    scope: "general" | "carryover" | "node_exclusive" | string;
    node_id?: string;
    username: string;
    balance: number;
    general_balance?: number;
    carryover_balance?: number;
    exclusive_balance?: number;
    total_balance?: number;
    node_exclusive_balance_current?: number;
    status: string;
    interrupt_targets: number;
  }> {
    return await this.postJson("/api/admin/points/adjust", payload, this.adminHeaders());
  }

  async adminPointsBatchGrant(payload: {
    amount: number;
    reason?: string;
    scope?: "general" | "carryover" | "node_exclusive";
    node_id?: string;
  }): Promise<{
    ok: boolean;
    scope: "general" | "carryover" | "node_exclusive" | string;
    node_id?: string;
    granted_users: number;
    adjusted_users: number;
    amount: number;
    total_granted: number;
    total_adjusted: number;
    interrupted_users: number;
    interrupted_nodes: number;
  }> {
    return await this.postJson("/api/admin/points/batch-grant", payload, this.adminHeaders());
  }

  async adminPointsBatchAdjustUsers(payload: {
    amount: number;
    usernames: string[];
    reason?: string;
    scope?: "general" | "carryover" | "node_exclusive";
    node_id?: string;
  }): Promise<{
    ok: boolean;
    scope: "general" | "carryover" | "node_exclusive" | string;
    node_id?: string;
    amount: number;
    requested_users: number;
    matched_users: number;
    adjusted_users: number;
    skipped_users: number;
    total_adjusted: number;
    interrupted_users: number;
    interrupted_nodes: number;
    quota_refresh_users: number;
    quota_refresh_nodes: number;
    quota_refresh_errors: number;
  }> {
    return await this.postJson("/api/admin/points/batch-adjust-users", payload, this.adminHeaders());
  }

  async adminPointsBatchSetUsers(payload: {
    value: number;
    usernames: string[];
    reason?: string;
    scope?: "general" | "carryover" | "node_exclusive";
    node_id?: string;
  }): Promise<{
    ok: boolean;
    scope: "general" | "carryover" | "node_exclusive" | string;
    node_id?: string;
    value: number;
    requested_users: number;
    matched_users: number;
    changed_users: number;
    unchanged_users: number;
    skipped_users: number;
    total_delta: number;
    interrupted_users: number;
    interrupted_nodes: number;
    quota_refresh_users: number;
    quota_refresh_nodes: number;
    quota_refresh_errors: number;
  }> {
    return await this.postJson("/api/admin/points/batch-set-users", payload, this.adminHeaders());
  }

  async adminPointsRecords(params?: { username?: string; method?: string; limit?: number }): Promise<{ records: PointsOperationRecord[] }> {
    const q = new URLSearchParams();
    if ((params?.username ?? "").trim()) q.set("username", (params?.username ?? "").trim());
    if ((params?.method ?? "").trim()) q.set("method", (params?.method ?? "").trim());
    q.set("limit", String(params?.limit ?? 500));
    return await this.getJson(`/api/admin/points/records?${q.toString()}`, this.adminHeaders());
  }

  async adminPointsSpecialRules(): Promise<{ rules: SpecialMonthlyPointsRule[] }> {
    return await this.getJson("/api/admin/points/special-rules", this.adminHeaders());
  }

  async adminPointsUpsertSpecialRule(payload: { username: string; monthly_points: number; enabled: boolean; note: string }): Promise<{ ok: boolean }> {
    return await this.postJson("/api/admin/points/special-rules", payload, this.adminHeaders());
  }

  async adminPointsDeleteSpecialRule(username: string): Promise<{ ok: boolean }> {
    const res = await fetch(this.url(`/api/admin/points/special-rules/${encodeURIComponent(username)}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminPointsMonthlyConfig(): Promise<{ config: MonthlyPointsConfig }> {
    return await this.getJson("/api/admin/points/monthly-config", this.adminHeaders());
  }

  async adminPointsSetMonthlyConfig(payload: {
    doctor_points: number;
    master_points: number;
    other_points: number;
    carryover_limit: number;
    max_overdraft_limit: number;
  }): Promise<{ ok: boolean }> {
    return await this.postJson("/api/admin/points/monthly-config", payload, this.adminHeaders());
  }

  async adminPointsMonthlyResetStatus(month?: string): Promise<{ month_key: string; has_run: boolean; run: MonthlyPointsResetRun | null; last_run: MonthlyPointsResetRun | null }> {
    const q = new URLSearchParams();
    if ((month ?? "").trim()) q.set("month", (month ?? "").trim());
    q.set("_t", String(Date.now()));
    return await this.getJson(`/api/admin/points/monthly-reset/status?${q.toString()}`, this.adminHeaders());
  }

  async adminPointsMonthlyReset(force = false): Promise<{ ok: boolean; message: string; month_key: string; already_run: boolean; total_users: number; changed_users: number; interrupted_users: number; interrupted_nodes: number; run?: MonthlyPointsResetRun }> {
    return await this.postJson("/api/admin/points/monthly-reset", { force }, this.adminHeaders());
  }

  async adminRequests(params: { status?: string; limit?: number }): Promise<{ requests: UserRequest[] }> {
    const q = new URLSearchParams();
    if (params.status?.trim()) q.set("status", params.status.trim());
    q.set("limit", String(params.limit ?? 200));
    return await this.getJson(`/api/admin/requests?${q.toString()}`, this.adminHeaders());
  }

  async adminRegistrationRequestsOverview(params: { limit?: number; field?: string; keyword?: string } = {}): Promise<{
    pending: RegistrationRequestView[];
    conflicts: RegistrationRequestView[];
    rejected: RegistrationRequest[];
  }> {
    const q = new URLSearchParams();
    q.set("limit", String(params.limit ?? 1000));
    if ((params.field ?? "").trim()) q.set("field", (params.field ?? "").trim());
    if ((params.keyword ?? "").trim()) q.set("keyword", (params.keyword ?? "").trim());
    return await this.getJson(`/api/admin/registration-requests/overview?${q.toString()}`, this.adminHeaders());
  }

  async adminRegisterSecurityPolicy(): Promise<{
    ip_window_seconds: number;
    ip_limit: number;
    email_window_seconds: number;
    email_limit: number;
    ip_cooldown_seconds: number;
    ip_cooldown_failures: number;
    email_cooldown_seconds: number;
    captcha_ttl_seconds: number;
    allowed_email_domains: string[];
    allowed_email_suffix_tip: string;
  }> {
    return await this.getJson("/api/admin/register-security/policy", this.adminHeaders());
  }

  async adminRegisterSecurityEvents(params: {
    keyword?: string;
    field?: string;
    action?: string;
    decision?: string;
    limit?: number;
  }): Promise<{ events: RegistrationSecurityEvent[] }> {
    const q = new URLSearchParams();
    if ((params.keyword ?? "").trim()) q.set("keyword", (params.keyword ?? "").trim());
    if ((params.field ?? "").trim()) q.set("field", (params.field ?? "").trim());
    if ((params.action ?? "").trim()) q.set("action", (params.action ?? "").trim());
    if ((params.decision ?? "").trim()) q.set("decision", (params.decision ?? "").trim());
    q.set("limit", String(params.limit ?? 500));
    return await this.getJson(`/api/admin/register-security/events?${q.toString()}`, this.adminHeaders());
  }

  async adminDisposableEmailDomains(params: { keyword?: string; limit?: number } = {}): Promise<{ domains: RegistrationDisposableEmailDomain[] }> {
    const q = new URLSearchParams();
    if ((params.keyword ?? "").trim()) q.set("keyword", (params.keyword ?? "").trim());
    q.set("limit", String(params.limit ?? 1000));
    return await this.getJson(`/api/admin/register-security/disposable-domains?${q.toString()}`, this.adminHeaders());
  }

  async adminUpsertDisposableEmailDomain(payload: {
    domain: string;
    enabled: boolean;
    note?: string;
  }): Promise<{ ok: boolean; domain: RegistrationDisposableEmailDomain }> {
    return await this.postJson("/api/admin/register-security/disposable-domains", payload, this.adminHeaders());
  }

  async adminDeleteDisposableEmailDomain(domain: string): Promise<{ ok: boolean }> {
    const res = await fetch(this.url(`/api/admin/register-security/disposable-domains/${encodeURIComponent(domain)}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminApproveRegistrationRequest(requestId: number): Promise<{ ok: boolean; request: RegistrationRequest }> {
    return await this.postJson(`/api/admin/registration-requests/${requestId}/approve`, {}, this.adminHeaders());
  }

  async adminRejectRegistrationRequest(requestId: number, reason?: string): Promise<{
    ok: boolean;
    request: RegistrationRequest;
    mail_sent: boolean;
    mail_error?: string;
  }> {
    return await this.postJson(`/api/admin/registration-requests/${requestId}/reject`, { reason: reason ?? "" }, this.adminHeaders());
  }

  async adminApproveRequest(requestId: number): Promise<{ ok: boolean; request: UserRequest }> {
    return await this.postJson(`/api/admin/requests/${requestId}/approve`, {}, this.adminHeaders());
  }

  async adminRejectRequest(requestId: number, reason = ""): Promise<{ ok: boolean; request: UserRequest }> {
    return await this.postJson(`/api/admin/requests/${requestId}/reject`, { reason }, this.adminHeaders());
  }

  async adminReopenRequest(requestId: number): Promise<{ ok: boolean; request: UserRequest }> {
    return await this.postJson(`/api/admin/requests/${requestId}/reopen`, {}, this.adminHeaders());
  }

  async adminBatchReview(requestIds: number[], newStatus: "approved" | "rejected", reason = ""): Promise<{ ok: boolean; ok_count: number; fail_count: number; fail_items: Array<{request_id:number;error:string}> }> {
    return await this.postJson(`/api/admin/requests/batch-review`, { request_ids: requestIds, new_status: newStatus, reason }, this.adminHeaders());
  }

  async adminProfileChangeRequests(params: { status?: string; username?: string; limit?: number }): Promise<{ requests: ProfileChangeRequest[] }> {
    const q = new URLSearchParams();
    if (params.status?.trim()) q.set("status", params.status.trim());
    if (params.username?.trim()) q.set("username", params.username.trim());
    q.set("limit", String(params.limit ?? 500));
    return await this.getJson(`/api/admin/profile-change-requests?${q.toString()}`, this.adminHeaders());
  }

  async adminApproveProfileChange(requestId: number): Promise<{ ok: boolean; request: ProfileChangeRequest }> {
    return await this.postJson(`/api/admin/profile-change-requests/${requestId}/approve`, {}, this.adminHeaders());
  }

  async adminRejectProfileChange(requestId: number): Promise<{ ok: boolean; request: ProfileChangeRequest }> {
    return await this.postJson(`/api/admin/profile-change-requests/${requestId}/reject`, {}, this.adminHeaders());
  }

  async adminCreateAnnouncement(payload: { title: string; content: string; pinned: boolean; attachment?: File | null }): Promise<{ ok: boolean }> {
    if (!payload.attachment) {
      return await this.postJson(`/api/admin/announcements`, payload, this.adminHeaders());
    }
    const form = new FormData();
    form.set("title", payload.title);
    form.set("content", payload.content);
    form.set("pinned", payload.pinned ? "1" : "0");
    form.set("attachment", payload.attachment);
    const res = await fetch(this.url(`/api/admin/announcements`), {
      method: "POST",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      body: form,
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminDeleteAnnouncement(id: number): Promise<{ ok: boolean }> {
    const res = await fetch(this.url(`/api/admin/announcements/${id}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminNotes(params: { from?: string; to?: string; limit?: number }): Promise<{ notes: AdminNote[] }> {
    const q = new URLSearchParams();
    if (params.from?.trim()) q.set("from", params.from.trim());
    if (params.to?.trim()) q.set("to", params.to.trim());
    q.set("limit", String(params.limit ?? 500));
    return await this.getJson(`/api/admin/notes?${q.toString()}`, this.adminHeaders());
  }

  async adminCreateNote(payload: { note_date: string; title: string; content: string }): Promise<{ ok: boolean; note: AdminNote }> {
    return await this.postJson("/api/admin/notes", payload, this.adminHeaders());
  }

  async adminUpdateNote(noteId: number, payload: { note_date: string; title: string; content: string }): Promise<{ ok: boolean; note: AdminNote }> {
    const res = await fetch(this.url(`/api/admin/notes/${noteId}`), {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean; note: AdminNote };
  }

  async adminDeleteNote(noteId: number): Promise<{ ok: boolean }> {
    const res = await fetch(this.url(`/api/admin/notes/${noteId}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminGuideline(): Promise<UserGuidelineResp> {
    return await this.getJson("/api/admin/guideline", this.adminHeaders());
  }

  async adminSetGuideline(content: string): Promise<{ ok: boolean; content: string; updated_by?: string; updated_at?: string | null }> {
    return await this.postJson("/api/admin/guideline", { content }, this.adminHeaders());
  }

  async adminExportUsageCSV(params: {
    username?: string;
    billingUsername?: string;
    localUsername?: string;
    unregisteredOnly?: boolean;
    from?: string;
    to?: string;
    limit?: number;
  }): Promise<Blob> {
    const q = new URLSearchParams();
    if (params.billingUsername?.trim()) q.set("billing_username", params.billingUsername.trim());
    else if (params.username?.trim()) q.set("username", params.username.trim());
    if (params.localUsername?.trim()) q.set("local_username", params.localUsername.trim());
    if (params.unregisteredOnly) q.set("unregistered_only", "1");
    if (params.from?.trim()) q.set("from", params.from.trim());
    if (params.to?.trim()) q.set("to", params.to.trim());
    q.set("limit", String(params.limit ?? 20000));
    const res = await fetch(this.url(`/api/admin/usage/export.csv?${q.toString()}`), {
      headers: { ...this.adminHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return await res.blob();
  }

  async adminGetMailSettings(): Promise<{
    smtp_host: string;
    smtp_port: number;
    smtp_user: string;
    smtp_password_set: boolean;
    from_email: string;
    from_name: string;
  }> {
    return await this.getJson("/api/admin/mail/settings", this.adminHeaders());
  }

  async adminSetMailSettings(payload: {
    smtp_host: string;
    smtp_port: number;
    smtp_user: string;
    smtp_pass: string;
    update_pass: boolean;
    from_email: string;
    from_name: string;
  }): Promise<{ ok: boolean }> {
    return await this.postJson("/api/admin/mail/settings", payload, this.adminHeaders());
  }

  async adminMailTest(username: string): Promise<{ ok: boolean; email: string }> {
    return await this.postJsonTryPaths<{ ok: boolean; email: string }>([
      "/api/admin/mail/test",
      "/admin/mail/test",
    ], { username }, this.adminHeaders());
  }

  async adminMailSend(payload: {
    all_users: boolean;
    usernames: string[];
    subject: string;
    body: string;
  }): Promise<{
    ok: boolean;
    total: number;
    success: number;
    failed: number;
    results: Array<{ username: string; email: string; success: boolean; error?: string }>;
  }> {
    return await this.postJson("/api/admin/mail/send", payload, this.adminHeaders());
  }

  async adminAccounts(
    filters: string | { billing_username?: string; node_id?: string; local_username?: string } = "",
  ): Promise<{ accounts: UserNodeAccount[] }> {
    const q = new URLSearchParams();
    if (typeof filters === "string") {
      const billingUsername = filters.trim();
      if (billingUsername) q.set("billing_username", billingUsername);
    } else {
      const billingUsername = String(filters.billing_username || "").trim();
      const nodeID = String(filters.node_id || "").trim();
      const localUsername = String(filters.local_username || "").trim();
      if (billingUsername) q.set("billing_username", billingUsername);
      if (nodeID) q.set("node_id", nodeID);
      if (localUsername) q.set("local_username", localUsername);
    }
    return await this.getJson(`/api/admin/accounts?${q.toString()}`, this.adminHeaders());
  }

  async adminAccountsNotReady(params: {
    status?: "all" | "initializing" | "failed" | string;
    limit?: number;
  } = {}): Promise<AdminAccountReadinessResp> {
    const q = new URLSearchParams();
    if (String(params.status || "").trim()) q.set("status", String(params.status).trim());
    if (Number.isFinite(params.limit) && Number(params.limit) > 0) q.set("limit", String(Math.floor(Number(params.limit))));
    return await this.getJson(`/api/admin/accounts/not-ready?${q.toString()}`, this.adminHeaders());
  }

  async adminGetBindPolicy(): Promise<NodeBindSecurityPolicy> {
    return await this.getJson("/api/admin/accounts/bind-policy", this.adminHeaders());
  }

  async adminSetBindPolicy(payload: NodeBindSecurityPolicy): Promise<{ ok: boolean; policy: NodeBindSecurityPolicy }> {
    return await this.postJson("/api/admin/accounts/bind-policy", payload, this.adminHeaders());
  }

  async adminBindCooldowns(params: {
    active_only?: boolean;
    limit?: number;
  } = {}): Promise<{ rows: AdminUserNodeBindCooldownRow[] }> {
    const q = new URLSearchParams();
    if (typeof params.active_only === "boolean") q.set("active_only", String(params.active_only));
    if (Number.isFinite(params.limit) && Number(params.limit) > 0) q.set("limit", String(Math.floor(Number(params.limit))));
    return await this.getJson(`/api/admin/accounts/bind-cooldowns?${q.toString()}`, this.adminHeaders());
  }

  async adminUnfreezeBindCooldown(payload: {
    billing_username: string;
  }): Promise<{ ok: boolean; cooldown: UserNodeBindCooldown }> {
    return await this.postJson("/api/admin/accounts/bind-cooldowns/unfreeze", payload, this.adminHeaders());
  }

  async adminAccountMappingRisks(params: {
    days?: number;
    min_switches?: number;
    limit?: number;
  } = {}): Promise<{
    days: number;
    min_switches: number;
    total_risky: number;
    risky_accounts: UserNodeAccountMappingRisk[];
  }> {
    const q = new URLSearchParams();
    if (Number.isFinite(params.days) && Number(params.days) > 0) q.set("days", String(Math.floor(Number(params.days))));
    if (Number.isFinite(params.min_switches) && Number(params.min_switches) > 0) q.set("min_switches", String(Math.floor(Number(params.min_switches))));
    if (Number.isFinite(params.limit) && Number(params.limit) > 0) q.set("limit", String(Math.floor(Number(params.limit))));
    return await this.getJson(`/api/admin/accounts/mapping-risks?${q.toString()}`, this.adminHeaders());
  }

  async adminClearAccountMappingRisk(payload: {
    node_id: string;
    local_username: string;
  }): Promise<{ ok: boolean; node_id: string; local_username: string }> {
    return await this.postJson("/api/admin/accounts/mapping-risks/clear", payload, this.adminHeaders());
  }

  async adminProvisionLogs(params: {
    billing_username?: string;
    node_id?: string;
    local_username?: string;
    limit?: number;
  } = {}): Promise<{ logs: AdminAccountProvisionLog[] }> {
    const q = new URLSearchParams();
    if (String(params.billing_username || "").trim()) q.set("billing_username", String(params.billing_username).trim());
    if (String(params.node_id || "").trim()) q.set("node_id", String(params.node_id).trim());
    if (String(params.local_username || "").trim()) q.set("local_username", String(params.local_username).trim());
    if (Number.isFinite(params.limit) && Number(params.limit) > 0) q.set("limit", String(Math.floor(Number(params.limit))));
    return await this.getJson(`/api/admin/accounts/provision-logs?${q.toString()}`, this.adminHeaders());
  }

  async adminUnbindRecords(params: {
    billing_username?: string;
    node_id?: string;
    local_username?: string;
    status?: string;
    source_type?: string;
    limit?: number;
  } = {}): Promise<{ records: UserNodeUnbindRecord[] }> {
    const q = new URLSearchParams();
    if (String(params.billing_username || "").trim()) q.set("billing_username", String(params.billing_username).trim());
    if (String(params.node_id || "").trim()) q.set("node_id", String(params.node_id).trim());
    if (String(params.local_username || "").trim()) q.set("local_username", String(params.local_username).trim());
    if (String(params.status || "").trim()) q.set("status", String(params.status).trim());
    if (String(params.source_type || "").trim()) q.set("source_type", String(params.source_type).trim());
    if (Number.isFinite(params.limit) && Number(params.limit) > 0) q.set("limit", String(Math.floor(Number(params.limit))));
    return await this.getJson(`/api/admin/accounts/unbind-records?${q.toString()}`, this.adminHeaders());
  }

  async adminCreateUnbindRequest(payload: {
    billing_username?: string;
    node_id: string;
    local_username: string;
    reason: string;
  }): Promise<{
    ok: boolean;
    request_id: number;
    billing_username: string;
    node_id: string;
    local_username: string;
    message: string;
  }> {
    return await this.postJson("/api/admin/accounts/unbind-request", payload, this.adminHeaders());
  }

  async adminUpsertAccount(payload: { billing_username: string; node_id: string; local_username: string }): Promise<{ ok: boolean }> {
    return await this.postJson("/api/admin/accounts", payload, this.adminHeaders());
  }

  async adminProvisionAccount(payload: {
    billing_username: string;
    node_id: string;
    local_username: string;
    ssh_host?: string;
    ssh_port?: number;
    rotate_key?: boolean;
    confirm_existing_local_user?: boolean;
  }): Promise<AdminAccountProvisionResp> {
    return await this.postJson("/api/admin/accounts/provision", payload, this.adminHeaders());
  }

  async adminDisableAccountMapping(payload: {
    billing_username: string;
    node_id: string;
    local_username: string;
    reason?: string;
  }): Promise<{
    ok: boolean;
    billing_username: string;
    node_id: string;
    local_username: string;
    blacklisted: boolean;
    killed_processes: boolean;
    kicked_ssh: boolean;
    removed_from_whitelist?: number;
    removed_from_exemptions?: number;
    message?: string;
  }> {
    return await this.postJson("/api/admin/accounts/disable", payload, this.adminHeaders());
  }

  async adminEnableAccountMapping(payload: {
    billing_username: string;
    node_id: string;
    local_username: string;
    reason?: string;
  }): Promise<{
    ok: boolean;
    billing_username: string;
    node_id: string;
    local_username: string;
    unblacklisted: boolean;
    message?: string;
  }> {
    return await this.postJson("/api/admin/accounts/enable", payload, this.adminHeaders());
  }

  async adminUpdateAccount(payload: {
    old_billing_username: string;
    old_node_id: string;
    old_local_username: string;
    new_billing_username: string;
    new_node_id: string;
    new_local_username: string;
  }): Promise<{ ok: boolean }> {
    const res = await fetch(this.url("/api/admin/accounts"), {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminDeleteAccount(params: { billing_username: string; node_id: string; local_username: string; reason: string }): Promise<{
    ok: boolean;
    billing_username: string;
    node_id: string;
    local_username: string;
    message: string;
  }> {
    const q = new URLSearchParams(params);
    const res = await fetch(this.url(`/api/admin/accounts?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as {
      ok: boolean;
      billing_username: string;
      node_id: string;
      local_username: string;
      message: string;
    };
  }

  async decryptProvisionKey(encryptedPayload: string, decryptCode: string): Promise<{
    ok: boolean;
    private_key: string;
    envelope: ProvisionKeyEnvelope;
  }> {
    return await this.postJson("/api/tools/provision/decrypt", {
      encrypted_payload: encryptedPayload,
      decrypt_code: decryptCode,
    });
  }

  async adminWhitelist(nodeId = ""): Promise<{ entries: SSHWhitelistEntry[] }> {
    const q = new URLSearchParams();
    if (nodeId) q.set("node_id", nodeId);
    return await this.getJson(`/api/admin/whitelist?${q.toString()}`, this.adminHeaders());
  }

  async adminUpsertWhitelist(
    nodeId: string,
    usernames: string[],
    billingUsernames: string[] = [],
    reason = "",
    platformAccounts: Array<{ billing_username: string; node_id: string; local_username: string }> = [],
  ): Promise<{
    ok: boolean;
    entries: number;
    applied: number;
    skipped_due_blacklist?: string[];
    message?: string;
  }> {
    return await this.postJson(
      "/api/admin/whitelist",
      { node_id: nodeId, usernames, billing_usernames: billingUsernames, platform_accounts: platformAccounts, reason },
      this.adminHeaders(),
    );
  }

  async adminDeleteWhitelist(nodeId: string, localUsername: string): Promise<{ ok: boolean }> {
    const q = new URLSearchParams({ node_id: nodeId, local_username: localUsername });
    const res = await fetch(this.url(`/api/admin/whitelist?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminBlacklist(nodeId = ""): Promise<{ entries: SSHBlacklistEntry[] }> {
    const q = new URLSearchParams();
    if (nodeId) q.set("node_id", nodeId);
    return await this.getJson(`/api/admin/blacklist?${q.toString()}`, this.adminHeaders());
  }

  async adminUpsertBlacklist(
    nodeId: string,
    usernames: string[],
    billingUsernames: string[] = [],
    reason = "",
    platformAccounts: Array<{ billing_username: string; node_id: string; local_username: string }> = [],
  ): Promise<{
    ok: boolean;
    entries: number;
    applied: number;
    kicked?: boolean;
    removed_from_whitelist?: number;
    removed_from_exemptions?: number;
    removed_from_temporary_users?: number;
    message?: string;
  }> {
    return await this.postJson(
      "/api/admin/blacklist",
      { node_id: nodeId, usernames, billing_usernames: billingUsernames, platform_accounts: platformAccounts, reason },
      this.adminHeaders(),
    );
  }

  async adminDeleteBlacklist(nodeId: string, localUsername: string): Promise<{ ok: boolean }> {
    const q = new URLSearchParams({ node_id: nodeId, local_username: localUsername });
    const res = await fetch(this.url(`/api/admin/blacklist?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminExemptions(nodeId = ""): Promise<{ entries: SSHExemptionEntry[] }> {
    const q = new URLSearchParams();
    if (nodeId) q.set("node_id", nodeId);
    return await this.getJson(`/api/admin/exemptions?${q.toString()}`, this.adminHeaders());
  }

  async adminUpsertExemptions(
    nodeId: string,
    usernames: string[],
    billingUsernames: string[] = [],
    reason = "",
    platformAccounts: Array<{ billing_username: string; node_id: string; local_username: string }> = [],
  ): Promise<{
    ok: boolean;
    entries: number;
    applied: number;
    skipped_due_blacklist?: string[];
    message?: string;
  }> {
    return await this.postJson(
      "/api/admin/exemptions",
      { node_id: nodeId, usernames, billing_usernames: billingUsernames, platform_accounts: platformAccounts, reason },
      this.adminHeaders(),
    );
  }

  async adminDeleteExemptions(nodeId: string, localUsername: string): Promise<{ ok: boolean }> {
    const q = new URLSearchParams({ node_id: nodeId, local_username: localUsername });
    const res = await fetch(this.url(`/api/admin/exemptions?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminTemporaryUsers(nodeId = ""): Promise<{ entries: SSHTemporaryUserEntry[] }> {
    const q = new URLSearchParams();
    if (nodeId) q.set("node_id", nodeId);
    return await this.getJson(`/api/admin/temporary-users?${q.toString()}`, this.adminHeaders());
  }

  async adminUpsertTemporaryUsers(
    nodeId: string,
    usernames: string[],
    billingUsernames: string[] = [],
    reason = "",
    platformAccounts: Array<{ billing_username: string; node_id: string; local_username: string }> = [],
  ): Promise<{
    ok: boolean;
    entries: number;
    applied: number;
    skipped_due_blacklist?: string[];
    message?: string;
  }> {
    return await this.postJson(
      "/api/admin/temporary-users",
      { node_id: nodeId, usernames, billing_usernames: billingUsernames, platform_accounts: platformAccounts, reason },
      this.adminHeaders(),
    );
  }

  async adminDeleteTemporaryUsers(nodeId: string, localUsername: string): Promise<{ ok: boolean; kicked?: boolean }> {
    const q = new URLSearchParams({ node_id: nodeId, local_username: localUsername });
    const res = await fetch(this.url(`/api/admin/temporary-users?${q.toString()}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean; kicked?: boolean };
  }

  async adminPowerUsers(limit = 1000): Promise<{ users: PowerUser[] }> {
    return await this.getJson(`/api/admin/power-users?limit=${limit}`, this.adminHeaders());
  }

  async adminCreatePowerUser(payload: {
    username: string;
    password: string;
    can_view_board: boolean;
    can_view_nodes: boolean;
    can_manage_nodes: boolean;
    can_manage_points: boolean;
    can_points_users: boolean;
    can_points_batch_filtered: boolean;
    can_points_batch_all: boolean;
    can_points_records: boolean;
    can_points_monthly: boolean;
    can_points_special_rules: boolean;
    can_review_requests: boolean;
    can_manage_platform_users: boolean;
  }): Promise<{ ok: boolean }> {
    return await this.postJson("/api/admin/power-users", payload, this.adminHeaders());
  }

  async adminPromotePowerUser(payload: {
    username: string;
    can_view_board: boolean;
    can_view_nodes: boolean;
    can_manage_nodes: boolean;
    can_manage_points: boolean;
    can_points_users: boolean;
    can_points_batch_filtered: boolean;
    can_points_batch_all: boolean;
    can_points_records: boolean;
    can_points_monthly: boolean;
    can_points_special_rules: boolean;
    can_review_requests: boolean;
    can_manage_platform_users: boolean;
  }): Promise<{ ok: boolean }> {
    return await this.postJson("/api/admin/power-users/promote", payload, this.adminHeaders());
  }

  async adminUpdatePowerUserPermissions(
    username: string,
    payload: {
      can_view_board: boolean;
      can_view_nodes: boolean;
      can_manage_nodes: boolean;
      can_manage_points: boolean;
      can_points_users: boolean;
      can_points_batch_filtered: boolean;
      can_points_batch_all: boolean;
      can_points_records: boolean;
      can_points_monthly: boolean;
      can_points_special_rules: boolean;
      can_review_requests: boolean;
      can_manage_platform_users: boolean;
    },
  ): Promise<{ ok: boolean }> {
    const res = await fetch(this.url(`/api/admin/power-users/${encodeURIComponent(username)}/permissions`), {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminDeletePowerUser(username: string): Promise<{ ok: boolean }> {
    const res = await fetch(this.url(`/api/admin/power-users/${encodeURIComponent(username)}`), {
      method: "DELETE",
      headers: { ...this.adminHeaders(), ...this.csrfHeaders() },
      credentials: "include",
    });
    if (!res.ok) {
      const text = await this.readText(res);
      throw normalizeServerError(res.status, text);
    }
    return (await res.json()) as { ok: boolean };
  }

  async adminDemotePowerUser(username: string): Promise<{ ok: boolean }> {
    return await this.postJson(`/api/admin/power-users/${encodeURIComponent(username)}/demote`, {}, this.adminHeaders());
  }

  async adminHAStatus(): Promise<HAStatusResp> {
    return await this.getJson("/api/admin/ha/status", this.adminHeaders());
  }

  async adminHASyncConfig(limit = 20): Promise<HASyncConfigResp> {
    return await this.getJson(`/api/admin/ha/sync/config?limit=${encodeURIComponent(String(limit))}`, this.adminHeaders());
  }

  async adminSetHASyncConfig(payload: HASyncConfig): Promise<{ ok: boolean; config: HASyncConfig }> {
    return await this.postJson("/api/admin/ha/sync/config", payload, this.adminHeaders());
  }

  async adminHASyncRuns(limit = 50): Promise<{ runs: HASyncRun[]; running: boolean }> {
    return await this.getJson(`/api/admin/ha/sync/runs?limit=${encodeURIComponent(String(limit))}`, this.adminHeaders());
  }

  async adminHASyncNow(payload: { direction: "primary_to_standby" | "standby_to_primary"; trigger_mode?: string }): Promise<{ ok: boolean; run_id: number; message: string }> {
    return await this.postJson("/api/admin/ha/sync/now", payload, this.adminHeaders());
  }

  async adminHAFailoverActivate(): Promise<{ ok: boolean; message: string; steps: HASyncStep[] }> {
    return await this.postJson("/api/admin/ha/failover/activate", {}, this.adminHeaders());
  }

  async adminStatsUsers(params: { from?: string; to?: string; limit?: number }): Promise<{ from: string; to: string; rows: UsageUserSummary[] }> {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    q.set("limit", String(params.limit ?? 1000));
    return await this.getJson(`/api/admin/stats/users?${q.toString()}`, this.adminHeaders());
  }

  async adminStatsPlatformUsers(params: { from?: string; to?: string; limit?: number }): Promise<{ from: string; to: string; rows: PlatformUsageUserSummary[] }> {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    q.set("limit", String(params.limit ?? 1000));
    return await this.getJson(`/api/admin/stats/platform-users?${q.toString()}`, this.adminHeaders());
  }

  async adminStatsPlatformUserNodes(username: string, params: { from?: string; to?: string; limit?: number }): Promise<{ from: string; to: string; username: string; rows: PlatformUsageNodeDetail[] }> {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    q.set("limit", String(params.limit ?? 2000));
    return await this.getJson(`/api/admin/stats/platform-users/${encodeURIComponent(username)}/nodes?${q.toString()}`, this.adminHeaders());
  }

  async adminStatsMonthly(params: { from?: string; to?: string; limit?: number; offset?: number }): Promise<{ from: string; to: string; rows: UsageMonthlySummary[]; has_more?: boolean }> {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    q.set("limit", String(params.limit ?? 20000));
    q.set("offset", String(params.offset ?? 0));
    return await this.getJson(`/api/admin/stats/monthly?${q.toString()}`, this.adminHeaders());
  }

  async adminStatsRecharges(params: { from?: string; to?: string; limit?: number }): Promise<{ from: string; to: string; rows: RechargeSummary[] }> {
    const q = new URLSearchParams();
    if (params.from) q.set("from", params.from);
    if (params.to) q.set("to", params.to);
    q.set("limit", String(params.limit ?? 1000));
    return await this.getJson(`/api/admin/stats/recharges?${q.toString()}`, this.adminHeaders());
  }
}
