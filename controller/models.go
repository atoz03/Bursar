package main

import "time"

// MetricsData 为 Agent 每次上报的数据结构。
type MetricsData struct {
	NodeID    string `json:"node_id"`
	Timestamp string `json:"timestamp"` // RFC3339
	// ReportID 为单次上报的全局唯一 ID，用于幂等：控制器只处理一次，避免重试导致重复扣费。
	ReportID string `json:"report_id"`
	// IntervalSeconds 为 Agent 上报周期（秒）。为空时由控制器用 sample_interval_seconds 兜底。
	IntervalSeconds    int                 `json:"interval_seconds,omitempty"`
	CPUModel           string              `json:"cpu_model,omitempty"`
	CPUCount           int                 `json:"cpu_count,omitempty"`
	GPUModel           string              `json:"gpu_model,omitempty"`
	GPUCount           int                 `json:"gpu_count,omitempty"`
	DiskTotalGB        float64             `json:"disk_total_gb,omitempty"`
	DiskUsedGB         float64             `json:"disk_used_gb,omitempty"`
	HomeTotalGB        float64             `json:"home_total_gb,omitempty"`
	HomeUsedGB         float64             `json:"home_used_gb,omitempty"`
	MntTotalGB         float64             `json:"mnt_total_gb,omitempty"`
	MntUsedGB          float64             `json:"mnt_used_gb,omitempty"`
	OSVersion          string              `json:"os_version,omitempty"`
	KernelVersion      string              `json:"kernel_version,omitempty"`
	AgentVersion       string              `json:"agent_version,omitempty"`
	NodeIP             string              `json:"node_ip,omitempty"`
	NodeMAC            string              `json:"node_mac,omitempty"`
	NetRxBytes         uint64              `json:"net_rx_bytes,omitempty"`
	NetTxBytes         uint64              `json:"net_tx_bytes,omitempty"`
	SecuritySignals    *SecuritySignals    `json:"security_signals,omitempty"`
	SSHUsers           []string            `json:"ssh_users,omitempty"`
	DiskQuotaInstalled bool                `json:"disk_quota_installed,omitempty"`
	DiskQuotaMounts    []string            `json:"disk_quota_mounts,omitempty"`
	UserDiskQuotas     []NodeUserDiskQuota `json:"user_disk_quotas,omitempty"`
	LocalUsers         []NodeLocalUser     `json:"local_users,omitempty"`
	Users              []UserProcess       `json:"users"`
}

type SecuritySignals struct {
	SSHFailedCount5m      int      `json:"ssh_failed_count_5m,omitempty"`
	SSHFailedUsernames    []string `json:"ssh_failed_usernames,omitempty"`
	SSHFailedSourceIPs    []string `json:"ssh_failed_source_ips,omitempty"`
	SSHBruteforceDetected bool     `json:"ssh_bruteforce_detected,omitempty"`
	SSHBruteforceSources  []string `json:"ssh_bruteforce_sources,omitempty"`
	PortScanDetected      bool     `json:"port_scan_detected,omitempty"`
	PortScanSources       []string `json:"port_scan_sources,omitempty"`
	PortScanTargetPorts   []int    `json:"port_scan_target_ports,omitempty"`
}

type UserProcess struct {
	Username   string     `json:"username"`
	PID        int32      `json:"pid"`
	CPUPercent float64    `json:"cpu_percent"`
	MemoryMB   float64    `json:"memory_mb"`
	GPUUsage   []GPUUsage `json:"gpu_usage"`
	Command    string     `json:"command,omitempty"`
}

type GPUUsage struct {
	GPUID    int32   `json:"gpu_id"`
	GPUModel string  `json:"gpu_model"`
	GPUBusID string  `json:"gpu_bus_id"`
	MemoryMB float64 `json:"memory_mb"`
}

type ControllerResponse struct {
	Actions []Action `json:"actions"`
}

type GPUExclusiveAssignment struct {
	Username   string `json:"username"`
	GPUIndices []int  `json:"gpu_indices"`
}

// Action 为控制器下发到节点的动作。
type Action struct {
	Type                    string                   `json:"type"` // notify, block_user, unblock_user, kill_process, kick_ssh_all, kick_ssh_user, kill_all_processes, force_sync, create_local_account, set_cpu_quota, set_memory_limit, set_disk_quota, set_gpu_exclusive, set_gpu_visibility
	Username                string                   `json:"username"`
	PIDs                    []int32                  `json:"pids,omitempty"`
	Reason                  string                   `json:"reason,omitempty"`
	Message                 string                   `json:"message,omitempty"`
	PublicKey               string                   `json:"public_key,omitempty"`                // create_local_account 使用（ssh-ed25519 公钥）
	CPUQuotaPercent         float64                  `json:"cpu_quota_percent,omitempty"`         // set_cpu_quota 使用
	MemoryLimitGB           float64                  `json:"memory_limit_gb,omitempty"`           // set_memory_limit 使用，0 表示解除限制
	DiskQuotaMountpoint     string                   `json:"disk_quota_mountpoint,omitempty"`     // set_disk_quota 使用
	DiskQuotaSoftMB         float64                  `json:"disk_quota_soft_mb,omitempty"`        // set_disk_quota 使用
	DiskQuotaHardMB         float64                  `json:"disk_quota_hard_mb,omitempty"`        // set_disk_quota 使用
	GPUExclusiveEnabled     bool                     `json:"gpu_exclusive_enabled,omitempty"`     // set_gpu_exclusive 使用
	GPUExclusiveAssignments []GPUExclusiveAssignment `json:"gpu_exclusive_assignments,omitempty"` // set_gpu_exclusive 使用
	GPUIndices              []int                    `json:"gpu_indices,omitempty"`               // set_gpu_visibility 使用（仅允许该用户访问这些 GPU）
}

type User struct {
	Username         string
	Balance          float64
	CarryoverBalance float64
	Status           string
	BlockedAt        *time.Time
}

type PriceRow struct {
	Model string
	Price float64
}

type UsageRecord struct {
	NodeID      string    `json:"node_id"`
	Username    string    `json:"username"` // 兼容字段：等同 billing_username
	LocalUser   string    `json:"local_username"`
	BillingUser string    `json:"billing_username"`
	Registered  bool      `json:"registered"`
	Timestamp   time.Time `json:"timestamp"`
	PID         int32     `json:"pid"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryMB    float64   `json:"memory_mb"`
	GPUCount    int       `json:"gpu_count"`
	Command     string    `json:"command"`
	GPUUsage    string    `json:"gpu_usage"` // JSON 字符串（原样返回）
	Cost        float64   `json:"cost"`
}

type ProcessKillRecord struct {
	RecordID        int64     `json:"record_id"`
	NodeID          string    `json:"node_id"`
	LocalUsername   string    `json:"local_username"`
	BillingUsername string    `json:"billing_username"`
	Registered      bool      `json:"registered"`
	ActionType      string    `json:"action_type"`
	Reason          string    `json:"reason"`
	PIDs            []int32   `json:"pids"`
	Timestamp       time.Time `json:"timestamp"`
}

type NodeStatus struct {
	NodeID                  string             `json:"node_id"`
	LastSeenAt              time.Time          `json:"last_seen_at"`
	LastReportID            string             `json:"last_report_id"`
	LastReportTS            time.Time          `json:"last_report_ts"`
	IntervalSeconds         int                `json:"interval_seconds"`
	CPUModel                string             `json:"cpu_model"`
	CPUCount                int                `json:"cpu_count"`
	GPUModel                string             `json:"gpu_model"`
	GPUCount                int                `json:"gpu_count"`
	DiskTotalGB             float64            `json:"disk_total_gb"`
	DiskUsedGB              float64            `json:"disk_used_gb"`
	HomeTotalGB             float64            `json:"home_total_gb"`
	HomeUsedGB              float64            `json:"home_used_gb"`
	MntTotalGB              float64            `json:"mnt_total_gb"`
	MntUsedGB               float64            `json:"mnt_used_gb"`
	OSVersion               string             `json:"os_version"`
	KernelVersion           string             `json:"kernel_version"`
	AgentVersion            string             `json:"agent_version"`
	NodeIP                  string             `json:"node_ip"`
	NodeMAC                 string             `json:"node_mac"`
	NetRxMBMonth            float64            `json:"net_rx_mb_month"`
	NetTxMBMonth            float64            `json:"net_tx_mb_month"`
	GPUProcessCount         int                `json:"gpu_process_count"`
	CPUProcessCount         int                `json:"cpu_process_count"`
	UsageRecordsCount       int                `json:"usage_records_count"`
	SSHActiveCount          int                `json:"ssh_active_count"`
	DiskQuotaInstalled      bool               `json:"disk_quota_installed"`
	DiskQuotaMounts         []string           `json:"disk_quota_mounts,omitempty"`
	SSHGuardEnabled         bool               `json:"ssh_guard_enabled"`
	SSHExclusiveEnabled     bool               `json:"ssh_exclusive_enabled"`
	PointsInterceptEnabled  bool               `json:"points_intercept_enabled"`
	PointsThrottleThreshold *float64           `json:"points_throttle_threshold,omitempty"`
	PointsLimitedCPUQuota   *float64           `json:"points_limited_cpu_quota_percent,omitempty"`
	PointsBlockedCPUQuota   *float64           `json:"points_blocked_cpu_quota_percent,omitempty"`
	PointsOverdraftMemoryGB *float64           `json:"points_overdraft_memory_limit_gb,omitempty"`
	DiskQuotaEnabled        bool               `json:"disk_quota_enabled"`
	DiskQuotaMountpoint     string             `json:"disk_quota_mountpoint,omitempty"`
	DiskQuotaSoftMB         *float64           `json:"disk_quota_soft_mb,omitempty"`
	DiskQuotaHardMB         *float64           `json:"disk_quota_hard_mb,omitempty"`
	NodePricePerMinute      *float64           `json:"node_price_per_minute,omitempty"`
	NodeModelPriceOverrides map[string]float64 `json:"node_model_price_overrides,omitempty"`
	SecurityEventCount7d    int                `json:"security_event_count_7d,omitempty"`
	SuspiciousUserCount7d   int                `json:"suspicious_user_count_7d,omitempty"`
	CostTotal               float64            `json:"cost_total"`
	UpdatedAt               time.Time          `json:"updated_at"`
}

type NodePointsInterceptPolicy struct {
	NodeID                  string   `json:"node_id,omitempty"`
	Enabled                 bool     `json:"enabled"`
	ThrottleThresholdPoints *float64 `json:"throttle_threshold_points,omitempty"`
	LimitedCPUQuotaPercent  *float64 `json:"limited_cpu_quota_percent,omitempty"`
	BlockedCPUQuotaPercent  *float64 `json:"blocked_cpu_quota_percent,omitempty"`
	OverdraftMemoryLimitGB  *float64 `json:"overdraft_memory_limit_gb,omitempty"`
}

type NodeExclusiveGPUAssignment struct {
	LocalUsername string `json:"local_username"`
	GPUIndices    []int  `json:"gpu_indices"`
}

type NodeLocalUser struct {
	NodeID                 string     `json:"node_id,omitempty"`
	LocalUsername          string     `json:"local_username"`
	PlatformUsername       string     `json:"platform_username,omitempty"`
	MappingExists          bool       `json:"mapping_exists"`
	PlatformExists         bool       `json:"platform_exists"`
	AdminMapping           bool       `json:"admin_mapping"`
	AdminUsername          string     `json:"admin_username,omitempty"`
	CPUQuotaPercent        *float64   `json:"cpu_quota_percent,omitempty"`
	CPUQuotaReason         string     `json:"cpu_quota_reason,omitempty"`
	CPUQuotaUpdatedAt      *time.Time `json:"cpu_quota_updated_at,omitempty"`
	MemoryLimitGB          *float64   `json:"memory_limit_gb,omitempty"`
	MemoryLimitReason      string     `json:"memory_limit_reason,omitempty"`
	MemoryLimitUpdatedAt   *time.Time `json:"memory_limit_updated_at,omitempty"`
	GPUVisibleIndices      []int      `json:"gpu_visible_indices,omitempty"`
	GPUVisibilityReason    string     `json:"gpu_visibility_reason,omitempty"`
	GPUVisibilityUpdatedAt *time.Time `json:"gpu_visibility_updated_at,omitempty"`
	HomeCreatedAt          *time.Time `json:"home_created_at,omitempty"`
	LastLoginAt            *time.Time `json:"last_login_at,omitempty"`
	HomeUsedGB             float64    `json:"home_used_gb"`
	QuotaMountpoint        string     `json:"quota_mountpoint,omitempty"`
	QuotaUsedMB            *float64   `json:"quota_used_mb,omitempty"`
	QuotaSoftMB            *float64   `json:"quota_soft_mb,omitempty"`
	QuotaHardMB            *float64   `json:"quota_hard_mb,omitempty"`
	HasSudo                bool       `json:"has_sudo"`
	HasDocker              bool       `json:"has_docker"`
	UpdatedAt              time.Time  `json:"updated_at,omitempty"`
}

type NodeUserDiskQuota struct {
	NodeID        string    `json:"node_id,omitempty"`
	LocalUsername string    `json:"local_username"`
	Mountpoint    string    `json:"mountpoint"`
	UsedMB        float64   `json:"used_mb"`
	SoftMB        float64   `json:"soft_mb"`
	HardMB        float64   `json:"hard_mb"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

type NodeUserCPULimit struct {
	NodeID          string    `json:"node_id"`
	LocalUsername   string    `json:"local_username"`
	BillingUsername string    `json:"billing_username,omitempty"`
	MappingExists   bool      `json:"mapping_exists"`
	PlatformExists  bool      `json:"platform_exists"`
	CPUQuotaPercent float64   `json:"cpu_quota_percent"`
	Reason          string    `json:"reason,omitempty"`
	UpdatedBy       string    `json:"updated_by,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type NodeUserMemoryLimit struct {
	NodeID          string    `json:"node_id"`
	LocalUsername   string    `json:"local_username"`
	BillingUsername string    `json:"billing_username,omitempty"`
	MappingExists   bool      `json:"mapping_exists"`
	PlatformExists  bool      `json:"platform_exists"`
	MemoryLimitGB   float64   `json:"memory_limit_gb"`
	Reason          string    `json:"reason,omitempty"`
	UpdatedBy       string    `json:"updated_by,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type NodeUserGPUVisibility struct {
	NodeID          string    `json:"node_id"`
	LocalUsername   string    `json:"local_username"`
	BillingUsername string    `json:"billing_username,omitempty"`
	MappingExists   bool      `json:"mapping_exists"`
	PlatformExists  bool      `json:"platform_exists"`
	AdminMapping    bool      `json:"admin_mapping"`
	AdminUsername   string    `json:"admin_username,omitempty"`
	GPUIndices      []int     `json:"gpu_indices"`
	Reason          string    `json:"reason,omitempty"`
	UpdatedBy       string    `json:"updated_by,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type NodeDiskQuotaPolicy struct {
	NodeID      string   `json:"node_id,omitempty"`
	Enabled     bool     `json:"enabled"`
	Mountpoint  string   `json:"mountpoint,omitempty"`
	DefaultSoft *float64 `json:"default_soft_mb,omitempty"`
	DefaultHard *float64 `json:"default_hard_mb,omitempty"`
}

type NodeSecurityEvent struct {
	EventID          int64     `json:"event_id"`
	ReportID         string    `json:"report_id"`
	NodeID           string    `json:"node_id"`
	EventType        string    `json:"event_type"`
	Severity         string    `json:"severity"`
	Reason           string    `json:"reason"`
	RelatedUsernames []string  `json:"related_usernames"`
	Details          string    `json:"details"`
	CreatedAt        time.Time `json:"created_at"`
}

type NodeSecurityEventSummary struct {
	EventType        string    `json:"event_type"`
	Severity         string    `json:"severity"`
	NormalizedReason string    `json:"normalized_reason"`
	EventCount       int       `json:"event_count"`
	AffectedUsers    int       `json:"affected_users"`
	FirstSeenAt      time.Time `json:"first_seen_at"`
	LastSeenAt       time.Time `json:"last_seen_at"`
}

type NodeSuspiciousUser struct {
	NodeID          string    `json:"node_id"`
	Username        string    `json:"username"`
	HitCount        int       `json:"hit_count"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	ReasonHints     string    `json:"reason_hints"`
	Phenomena       string    `json:"phenomena"`
	MiningSuspected bool      `json:"mining_suspected"`
}

type NodeMonthlyUserCost struct {
	NodeID           string    `json:"node_id"`
	PlatformUsername string    `json:"platform_username"`
	UsageRecords     int       `json:"usage_records"`
	TotalCost        float64   `json:"total_cost"`
	LastUsageAt      time.Time `json:"last_usage_at"`
}

// UserNodeAccount 表示“节点本地账号”到“计费账号”的映射。
// 约定：node_id 为机器编号（可用端口号，例如 60000）；local_username 为该节点上的 Linux 用户名。
type UserNodeAccount struct {
	NodeID          string    `json:"node_id"`
	LocalUsername   string    `json:"local_username"`
	BillingUsername string    `json:"billing_username"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type UserMappedNodeInfo struct {
	NodeID                         string     `json:"node_id"`
	LocalUsername                  string     `json:"local_username"`
	CPUModel                       string     `json:"cpu_model,omitempty"`
	CPUCount                       int        `json:"cpu_count,omitempty"`
	GPUModel                       string     `json:"gpu_model,omitempty"`
	GPUCount                       int        `json:"gpu_count,omitempty"`
	EffectiveGPUPricePerMinute     float64    `json:"effective_gpu_price_per_minute"`
	GPUPriceSource                 string     `json:"gpu_price_source"`
	EffectiveCPUPricePerCoreMinute float64    `json:"effective_cpu_price_per_core_minute"`
	CPUPriceSource                 string     `json:"cpu_price_source"`
	HomeQuotaMountpoint            string     `json:"home_quota_mountpoint,omitempty"`
	HomeQuotaUsedMB                *float64   `json:"home_quota_used_mb,omitempty"`
	HomeQuotaSoftMB                *float64   `json:"home_quota_soft_mb,omitempty"`
	HomeQuotaHardMB                *float64   `json:"home_quota_hard_mb,omitempty"`
	HomeQuotaUpdatedAt             *time.Time `json:"home_quota_updated_at,omitempty"`
	HomeQuotaEnforced              bool       `json:"home_quota_enforced"`
}

type UserNodeAccountAudit struct {
	AuditID            int64     `json:"audit_id"`
	NodeID             string    `json:"node_id"`
	LocalUsername      string    `json:"local_username"`
	OldBillingUsername string    `json:"old_billing_username"`
	NewBillingUsername string    `json:"new_billing_username"`
	Action             string    `json:"action"`
	Operator           string    `json:"operator"`
	Source             string    `json:"source"`
	Reason             string    `json:"reason"`
	CreatedAt          time.Time `json:"created_at"`
}

type UserNodeAccountMappingRisk struct {
	NodeID               string    `json:"node_id"`
	LocalUsername        string    `json:"local_username"`
	CurrentBilling       string    `json:"current_billing_username"`
	SwitchCount          int       `json:"switch_count"`
	DistinctBillingCount int       `json:"distinct_billing_count"`
	PlatformUsernames    []string  `json:"platform_usernames"`
	SwitchHistory        []string  `json:"switch_history,omitempty"`
	LastChangedAt        time.Time `json:"last_changed_at"`
	RiskReason           string    `json:"risk_reason"`
}

type AccountProvisionLog struct {
	ProvisionID      int64     `json:"provision_id"`
	BillingUsername  string    `json:"billing_username"`
	NodeID           string    `json:"node_id"`
	LocalUsername    string    `json:"local_username"`
	Email            string    `json:"email"`
	SSHHost          string    `json:"ssh_host"`
	SSHPort          int       `json:"ssh_port"`
	DownloadFilename string    `json:"download_filename"`
	MailSent         bool      `json:"mail_sent"`
	MailError        string    `json:"mail_error"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
}

type UserProvisionMessage struct {
	MessageID        int64      `json:"message_id"`
	BillingUsername  string     `json:"billing_username"`
	NodeID           string     `json:"node_id"`
	LocalUsername    string     `json:"local_username"`
	EncryptedPayload string     `json:"encrypted_payload"`
	DecryptURL       string     `json:"decrypt_url"`
	SSHHost          string     `json:"ssh_host"`
	SSHPort          int        `json:"ssh_port"`
	DownloadFilename string     `json:"download_filename"`
	SSHCommand       string     `json:"ssh_command"`
	MailTo           string     `json:"mail_to"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	FirstDecryptedAt *time.Time `json:"first_decrypted_at,omitempty"`
	DestroyAfterAt   *time.Time `json:"destroy_after_at,omitempty"`
	DestroyedAt      *time.Time `json:"destroyed_at,omitempty"`
}

type SSHWhitelistEntry struct {
	NodeID                 string    `json:"node_id"`
	LocalUsername          string    `json:"local_username"`
	BillingUsername        string    `json:"billing_username"`
	SourceType             string    `json:"source_type"`
	SourcePlatformUsername string    `json:"source_platform_username"`
	CreatedBy              string    `json:"created_by"`
	Reason                 string    `json:"reason"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type SSHBlacklistEntry struct {
	NodeID                 string    `json:"node_id"`
	LocalUsername          string    `json:"local_username"`
	BillingUsername        string    `json:"billing_username"`
	SourceType             string    `json:"source_type"`
	SourcePlatformUsername string    `json:"source_platform_username"`
	CreatedBy              string    `json:"created_by"`
	Reason                 string    `json:"reason"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type SSHExemptionEntry struct {
	NodeID                 string    `json:"node_id"`
	LocalUsername          string    `json:"local_username"`
	BillingUsername        string    `json:"billing_username"`
	SourceType             string    `json:"source_type"`
	SourcePlatformUsername string    `json:"source_platform_username"`
	CreatedBy              string    `json:"created_by"`
	Reason                 string    `json:"reason"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type SSHListSource struct {
	ListType               string    `json:"list_type"`
	NodeID                 string    `json:"node_id"`
	LocalUsername          string    `json:"local_username"`
	SourceType             string    `json:"source_type"`
	SourcePlatformUsername string    `json:"source_platform_username"`
	UpdatedBy              string    `json:"updated_by"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type UserRequest struct {
	RequestID       int        `json:"request_id"`
	RequestType     string     `json:"request_type"` // bind/open/unbind
	BillingUsername string     `json:"billing_username"`
	NodeID          string     `json:"node_id"`
	LocalUsername   string     `json:"local_username"`
	Message         string     `json:"message"`
	Status          string     `json:"status"` // pending/challenge_active/verified/approved/rejected
	ReviewedBy      *string    `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type UserNodeUnbindRecord struct {
	RecordID        int64      `json:"record_id"`
	SourceType      string     `json:"source_type"` // user_request/admin_forced
	RequestID       *int       `json:"request_id,omitempty"`
	BillingUsername string     `json:"billing_username"`
	NodeID          string     `json:"node_id"`
	LocalUsername   string     `json:"local_username"`
	Status          string     `json:"status"` // pending/approved/rejected/forced
	Reason          string     `json:"reason"`
	InitiatedBy     string     `json:"initiated_by"`
	ReviewedBy      *string    `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`
	ExecutedAt      *time.Time `json:"executed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type NodeBindSecurityPolicy struct {
	ChallengeWindowSeconds       int     `json:"challenge_window_seconds"`
	TrialCPUQuotaPercent         float64 `json:"trial_cpu_quota_percent"`
	TrialMemoryLimitGB           float64 `json:"trial_memory_limit_gb"`
	TrialGPUBlocked              bool    `json:"trial_gpu_blocked"`
	SingleActivePerBilling       bool    `json:"single_active_challenge_per_billing"`
	AllFailureCooldownMinutes    int     `json:"all_failure_cooldown_minutes"`
	FirstFailureCooldownMinutes  int     `json:"first_failure_cooldown_minutes"`
	RepeatFailureCooldownMinutes int     `json:"repeat_failure_cooldown_minutes"`
	ContentionFreezeMinutes      int     `json:"contention_freeze_minutes"`
}

type AdminUserNodeBindCooldownRow struct {
	BillingUsername          string     `json:"billing_username"`
	FailureStreak            int        `json:"failure_streak"`
	CooldownUntil            *time.Time `json:"cooldown_until,omitempty"`
	LastFailedAt             *time.Time `json:"last_failed_at,omitempty"`
	LastSucceededAt          *time.Time `json:"last_succeeded_at,omitempty"`
	UpdatedAt                time.Time  `json:"updated_at"`
	RemainingCooldownSeconds int64      `json:"remaining_cooldown_seconds"`
	ActiveChallengeID        *int64     `json:"active_challenge_id,omitempty"`
	ActiveChallengeNodeID    string     `json:"active_challenge_node_id,omitempty"`
	ActiveChallengeLocalUser string     `json:"active_challenge_local_username,omitempty"`
	ActiveChallengeExpiresAt *time.Time `json:"active_challenge_expires_at,omitempty"`
}

type UserNodeBindChallenge struct {
	ChallengeID      int64      `json:"challenge_id"`
	RequestID        int        `json:"request_id"`
	BillingUsername  string     `json:"billing_username"`
	NodeID           string     `json:"node_id"`
	LocalUsername    string     `json:"local_username"`
	ChallengeToken   string     `json:"challenge_token,omitempty"`
	Status           string     `json:"status"`
	FailReason       string     `json:"fail_reason,omitempty"`
	ExpiresAt        time.Time  `json:"expires_at"`
	ClaimedAt        *time.Time `json:"claimed_at,omitempty"`
	VerifiedAt       *time.Time `json:"verified_at,omitempty"`
	CooldownUntil    *time.Time `json:"cooldown_until,omitempty"`
	FailureProcessed bool       `json:"failure_processed,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type UserNodeBindCooldown struct {
	BillingUsername string     `json:"billing_username"`
	FailureStreak   int        `json:"failure_streak"`
	CooldownUntil   *time.Time `json:"cooldown_until,omitempty"`
	LastFailedAt    *time.Time `json:"last_failed_at,omitempty"`
	LastSucceededAt *time.Time `json:"last_succeeded_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type RegistrationRequest struct {
	RequestID               int        `json:"request_id"`
	Username                string     `json:"username"`
	Email                   string     `json:"email"`
	PasswordHash            string     `json:"-"`
	RealName                string     `json:"real_name"`
	StudentID               string     `json:"student_id"`
	Advisor                 string     `json:"advisor"`
	ExpectedGraduationYear  int        `json:"expected_graduation_year"`
	ExpectedGraduationMonth int        `json:"expected_graduation_month"`
	Phone                   string     `json:"phone"`
	Status                  string     `json:"status"` // pending/approved/rejected
	ReviewedBy              *string    `json:"reviewed_by,omitempty"`
	ReviewedAt              *time.Time `json:"reviewed_at,omitempty"`
	RejectReason            string     `json:"reject_reason,omitempty"`
	RejectNotifyMailChecked bool       `json:"reject_notify_mail_checked"`
	RejectNotifyMailSent    bool       `json:"reject_notify_mail_sent"`
	RejectNotifyMailError   string     `json:"reject_notify_mail_error,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type RegistrationDisposableEmailDomain struct {
	Domain    string    `json:"domain"`
	Enabled   bool      `json:"enabled"`
	Note      string    `json:"note"`
	UpdatedBy string    `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RegistrationSecurityEvent struct {
	EventID   int64     `json:"event_id"`
	Action    string    `json:"action"`
	Decision  string    `json:"decision"`
	Reason    string    `json:"reason"`
	ClientIP  string    `json:"client_ip"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	StudentID string    `json:"student_id"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
}

type RegistrationRateStats struct {
	IPCount     int
	EmailCount  int
	LastIPAt    *time.Time
	LastEmailAt *time.Time
}

type DeletedUserAccount struct {
	DeletedID               int64      `json:"deleted_id"`
	Username                string     `json:"username"`
	Email                   string     `json:"email"`
	StudentID               string     `json:"student_id"`
	RealName                string     `json:"real_name"`
	Advisor                 string     `json:"advisor"`
	ExpectedGraduationYear  int        `json:"expected_graduation_year"`
	ExpectedGraduationMonth int        `json:"expected_graduation_month"`
	Phone                   string     `json:"phone"`
	Role                    string     `json:"role"`
	Balance                 float64    `json:"balance"`
	CarryoverBalance        float64    `json:"carryover_balance"`
	UserStatus              string     `json:"user_status"`
	DeletedAt               time.Time  `json:"deleted_at"`
	DeletedBy               string     `json:"deleted_by"`
	DeleteReason            string     `json:"delete_reason"`
	RestoredAt              *time.Time `json:"restored_at,omitempty"`
	RestoredBy              *string    `json:"restored_by,omitempty"`
}

type AdminProfile struct {
	Username  string    `json:"username"`
	RealName  string    `json:"real_name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserAccount struct {
	Username                string     `json:"username"`
	Email                   string     `json:"email"`
	RealName                string     `json:"real_name"`
	StudentID               string     `json:"student_id"`
	Advisor                 string     `json:"advisor"`
	ExpectedGraduationYear  int        `json:"expected_graduation_year"`
	ExpectedGraduationMonth int        `json:"expected_graduation_month"`
	Phone                   string     `json:"phone"`
	Role                    string     `json:"role"`
	LastLoginAt             *time.Time `json:"last_login_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type MailSettings struct {
	SMTPHost  string `json:"smtp_host"`
	SMTPPort  int    `json:"smtp_port"`
	SMTPUser  string `json:"smtp_user"`
	SMTPPass  string `json:"smtp_pass,omitempty"`
	FromEmail string `json:"from_email"`
	FromName  string `json:"from_name"`
}

type PowerUser struct {
	Username          string     `json:"username"`
	IsPlatformUser    bool       `json:"is_platform_user"`
	CanViewBoard      bool       `json:"can_view_board"`
	CanViewNodes      bool       `json:"can_view_nodes"`
	CanManageNodes    bool       `json:"can_manage_nodes"`
	CanManagePoints   bool       `json:"can_manage_points"`
	CanReviewRequests bool       `json:"can_review_requests"`
	CreatedBy         string     `json:"created_by"`
	UpdatedBy         string     `json:"updated_by"`
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type UsageUserSummary struct {
	Username          string  `json:"username"`
	UsageRecords      int     `json:"usage_records"`
	GPUProcessRecords int     `json:"gpu_process_records"`
	CPUProcessRecords int     `json:"cpu_process_records"`
	TotalCPUPercent   float64 `json:"total_cpu_percent"`
	TotalMemoryMB     float64 `json:"total_memory_mb"`
	TotalCost         float64 `json:"total_cost"`
}

type UsageMonthlySummary struct {
	Month             string  `json:"month"`
	Username          string  `json:"username"`
	UsageRecords      int     `json:"usage_records"`
	GPUProcessRecords int     `json:"gpu_process_records"`
	CPUProcessRecords int     `json:"cpu_process_records"`
	TotalCPUPercent   float64 `json:"total_cpu_percent"`
	TotalMemoryMB     float64 `json:"total_memory_mb"`
	TotalCost         float64 `json:"total_cost"`
}

type RechargeSummary struct {
	Username      string    `json:"username"`
	RechargeCount int       `json:"recharge_count"`
	RechargeTotal float64   `json:"recharge_total"`
	LastRecharge  time.Time `json:"last_recharge"`
}

type PointsOperationRecord struct {
	RechargeID    int64     `json:"recharge_id"`
	Username      string    `json:"username"`
	TargetAccount string    `json:"target_account"`
	Amount        float64   `json:"amount"`
	Method        string    `json:"method"`
	PointsScope   string    `json:"points_scope"`
	NodeID        string    `json:"node_id"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

type Announcement struct {
	AnnouncementID int       `json:"announcement_id"`
	Title          string    `json:"title"`
	Content        string    `json:"content"`
	Pinned         bool      `json:"pinned"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type AdminNote struct {
	NoteID    int       `json:"note_id"`
	NoteDate  time.Time `json:"note_date"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	UpdatedBy string    `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AdminUserDetail struct {
	Username          string            `json:"username"`
	Role              string            `json:"role"`
	CanViewBoard      bool              `json:"can_view_board"`
	CanViewNodes      bool              `json:"can_view_nodes"`
	CanReviewRequest  bool              `json:"can_review_requests"`
	Email             string            `json:"email"`
	StudentID         string            `json:"student_id"`
	RealName          string            `json:"real_name"`
	Advisor           string            `json:"advisor"`
	ExpectedGradYear  int               `json:"expected_graduation_year"`
	ExpectedGradMonth int               `json:"expected_graduation_month"`
	Phone             string            `json:"phone"`
	Balance           float64           `json:"balance"`
	CarryoverBalance  float64           `json:"carryover_balance"`
	ExclusiveBalance  float64           `json:"exclusive_balance"`
	TotalBalance      float64           `json:"total_balance"`
	Status            string            `json:"status"`
	UsageRecords      int               `json:"usage_records"`
	TotalCost         float64           `json:"total_cost"`
	LastUsageAt       time.Time         `json:"last_usage_at"`
	NodeAccounts      []UserNodeAccount `json:"node_accounts"`
}

type GraduationReminderUser struct {
	Username                string `json:"username"`
	Email                   string `json:"email"`
	StudentID               string `json:"student_id"`
	RealName                string `json:"real_name"`
	Advisor                 string `json:"advisor"`
	ExpectedGraduationYear  int    `json:"expected_graduation_year"`
	ExpectedGraduationMonth int    `json:"expected_graduation_month"`
	OverdueMonths           int    `json:"overdue_months"`
}

type ProfileChangeRequest struct {
	RequestID       int        `json:"request_id"`
	BillingUsername string     `json:"billing_username"`
	OldUsername     string     `json:"old_username"`
	OldEmail        string     `json:"old_email"`
	OldStudentID    string     `json:"old_student_id"`
	NewUsername     string     `json:"new_username"`
	NewEmail        string     `json:"new_email"`
	NewStudentID    string     `json:"new_student_id"`
	Reason          string     `json:"reason"`
	Status          string     `json:"status"`
	ReviewedBy      *string    `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type PlatformUsageUserSummary struct {
	PlatformUsername string  `json:"platform_username"`
	UsageRecords     int     `json:"usage_records"`
	CPUUsageSeconds  int64   `json:"cpu_usage_seconds"`
	GPUUsageSeconds  int64   `json:"gpu_usage_seconds"`
	CPUUtilPercent   float64 `json:"cpu_util_percent"`
	GPUUtilPercent   float64 `json:"gpu_util_percent"`
	TotalCost        float64 `json:"total_cost"`
	GeneralBalance   float64 `json:"general_balance"`
}

type PlatformUsageNodeDetail struct {
	NodeID          string    `json:"node_id"`
	CPUModel        string    `json:"cpu_model"`
	CPUCount        int       `json:"cpu_count"`
	GPUModel        string    `json:"gpu_model"`
	GPUCount        int       `json:"gpu_count"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	UsageRecords    int       `json:"usage_records"`
	TotalCPUPercent float64   `json:"total_cpu_percent"`
	TotalMemoryMB   float64   `json:"total_memory_mb"`
	CPUCost         float64   `json:"cpu_cost"`
	GPUCost         float64   `json:"gpu_cost"`
	TotalCost       float64   `json:"total_cost"`
	LastUsageAt     time.Time `json:"last_usage_at"`
}

type NodeRuntimeSnapshot struct {
	ReportID        string    `json:"report_id"`
	NodeID          string    `json:"node_id"`
	ReportTS        time.Time `json:"report_ts"`
	CPUPercentSum   float64   `json:"cpu_percent_sum"`
	MemoryMBSum     float64   `json:"memory_mb_sum"`
	GPUProcessCount int       `json:"gpu_process_count"`
	CPUProcessCount int       `json:"cpu_process_count"`
	SSHUserCount    int       `json:"ssh_user_count"`
	SSHUsers        []string  `json:"ssh_users"`
	CostTotal       float64   `json:"cost_total"`
}

type PointsUser struct {
	Username          string  `json:"username"`
	Email             string  `json:"email"`
	RealName          string  `json:"real_name"`
	StudentID         string  `json:"student_id"`
	Advisor           string  `json:"advisor"`
	Phone             string  `json:"phone"`
	ExpectedGradYear  int     `json:"expected_graduation_year"`
	ExpectedGradMonth int     `json:"expected_graduation_month"`
	Role              string  `json:"role"`
	Balance           float64 `json:"balance"` // 兼容字段：等同普通积分
	GeneralBalance    float64 `json:"general_balance"`
	CarryoverBalance  float64 `json:"carryover_balance"`
	ExclusiveBalance  float64 `json:"exclusive_balance"`
	TotalBalance      float64 `json:"total_balance"`
	Status            string  `json:"status"`
}

type NodeExclusivePointsBalance struct {
	Username  string    `json:"username"`
	NodeID    string    `json:"node_id"`
	Balance   float64   `json:"balance"`
	UpdatedBy string    `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SpecialMonthlyPointsRule struct {
	Username      string    `json:"username"`
	MonthlyPoints float64   `json:"monthly_points"`
	Enabled       bool      `json:"enabled"`
	UpdatedBy     string    `json:"updated_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type MonthlyPointsResetRun struct {
	MonthKey     string    `json:"month_key"`
	RunAt        time.Time `json:"run_at"`
	RunBy        string    `json:"run_by"`
	TotalUsers   int       `json:"total_users"`
	ChangedUsers int       `json:"changed_users"`
	Forced       bool      `json:"forced"`
}

type MonthlyPointsConfig struct {
	DoctorPoints      float64 `json:"doctor_points"`
	MasterPoints      float64 `json:"master_points"`
	OtherPoints       float64 `json:"other_points"`
	CarryoverLimit    float64 `json:"carryover_limit"`
	MaxOverdraftLimit float64 `json:"max_overdraft_limit"`
}

type UsageRetentionSettings struct {
	RetentionDays      int        `json:"retention_days"`
	LastDeletedAt      *time.Time `json:"last_deleted_at,omitempty"`
	LastDeletedFrom    *time.Time `json:"last_deleted_from,omitempty"`
	LastDeletedTo      *time.Time `json:"last_deleted_to,omitempty"`
	LastDeletedRecords int64      `json:"last_deleted_records"`
	LastDeletedMode    string     `json:"last_deleted_mode,omitempty"`
	LastDeletedBy      string     `json:"last_deleted_by,omitempty"`
}
