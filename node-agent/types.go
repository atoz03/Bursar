package main

// 注意：这些结构体与 controller/models.go 的 JSON 字段保持一致，便于直接通信。

type MetricsData struct {
	NodeID                  string                    `json:"node_id"`
	Timestamp               string                    `json:"timestamp"` // RFC3339
	ReportID                string                    `json:"report_id"`
	IntervalSeconds         int                       `json:"interval_seconds,omitempty"`
	CPUModel                string                    `json:"cpu_model,omitempty"`
	CPUCount                int                       `json:"cpu_count,omitempty"`
	GPUModel                string                    `json:"gpu_model,omitempty"`
	GPUCount                int                       `json:"gpu_count,omitempty"`
	DiskTotalGB             float64                   `json:"disk_total_gb,omitempty"`
	DiskUsedGB              float64                   `json:"disk_used_gb,omitempty"`
	HomeTotalGB             float64                   `json:"home_total_gb,omitempty"`
	HomeUsedGB              float64                   `json:"home_used_gb,omitempty"`
	MntTotalGB              float64                   `json:"mnt_total_gb,omitempty"`
	MntUsedGB               float64                   `json:"mnt_used_gb,omitempty"`
	OSVersion               string                    `json:"os_version,omitempty"`
	KernelVersion           string                    `json:"kernel_version,omitempty"`
	AgentVersion            string                    `json:"agent_version,omitempty"`
	NodeIP                  string                    `json:"node_ip,omitempty"`
	NodeMAC                 string                    `json:"node_mac,omitempty"`
	NetRxBytes              uint64                    `json:"net_rx_bytes,omitempty"`
	NetTxBytes              uint64                    `json:"net_tx_bytes,omitempty"`
	SecuritySignals         *SecuritySignals          `json:"security_signals,omitempty"`
	SSHUsers                []string                  `json:"ssh_users,omitempty"`
	DiskQuotaInstalled      bool                      `json:"disk_quota_installed,omitempty"`
	DiskQuotaMounts         []string                  `json:"disk_quota_mounts,omitempty"`
	SystemServicesCheckedAt string                    `json:"system_services_checked_at,omitempty"`
	SystemServices          []NodeSystemServiceStatus `json:"system_services,omitempty"`
	UserDiskQuotas          []NodeUserDiskQuota       `json:"user_disk_quotas,omitempty"`
	LocalUsers              []NodeLocalUser           `json:"local_users,omitempty"`
	Users                   []UserProcess             `json:"users"`
}

type NodeSystemServiceStatus struct {
	Name          string `json:"name"`
	Deployed      bool   `json:"deployed"`
	LoadState     string `json:"load_state,omitempty"`
	UnitFileState string `json:"unit_file_state,omitempty"`
	ActiveState   string `json:"active_state,omitempty"`
	SubState      string `json:"sub_state,omitempty"`
	Healthy       bool   `json:"healthy"`
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

type Action struct {
	Type                    string                   `json:"type"`
	Username                string                   `json:"username"`
	PIDs                    []int32                  `json:"pids,omitempty"`
	Reason                  string                   `json:"reason,omitempty"`
	Message                 string                   `json:"message,omitempty"`
	DelaySeconds            int                      `json:"delay_seconds,omitempty"`
	PublicKey               string                   `json:"public_key,omitempty"`
	SharedWorkspaceUsername string                   `json:"shared_workspace_username,omitempty"`
	TargetUID               int                      `json:"target_uid,omitempty"`
	TargetPrimaryGID        int                      `json:"target_primary_gid,omitempty"`
	CPUQuotaPercent         float64                  `json:"cpu_quota_percent,omitempty"`
	MemoryLimitGB           float64                  `json:"memory_limit_gb,omitempty"`
	DiskQuotaMountpoint     string                   `json:"disk_quota_mountpoint,omitempty"`
	DiskQuotaSoftMB         float64                  `json:"disk_quota_soft_mb,omitempty"`
	DiskQuotaHardMB         float64                  `json:"disk_quota_hard_mb,omitempty"`
	GPUExclusiveEnabled     bool                     `json:"gpu_exclusive_enabled,omitempty"`
	GPUExclusiveAssignments []GPUExclusiveAssignment `json:"gpu_exclusive_assignments,omitempty"`
	GPUIndices              []int                    `json:"gpu_indices,omitempty"`
}

type NodeLocalUser struct {
	LocalUsername   string   `json:"local_username"`
	UID             int      `json:"uid,omitempty"`
	PrimaryGID      int      `json:"primary_gid,omitempty"`
	HomeCreatedAt   *string  `json:"home_created_at,omitempty"`
	LastLoginAt     *string  `json:"last_login_at,omitempty"`
	HomeUsedGB      float64  `json:"home_used_gb"`
	QuotaMountpoint string   `json:"quota_mountpoint,omitempty"`
	QuotaUsedMB     *float64 `json:"quota_used_mb,omitempty"`
	QuotaSoftMB     *float64 `json:"quota_soft_mb,omitempty"`
	QuotaHardMB     *float64 `json:"quota_hard_mb,omitempty"`
	HasSudo         bool     `json:"has_sudo"`
	HasDocker       bool     `json:"has_docker"`
}

type NodeUserDiskQuota struct {
	LocalUsername string  `json:"local_username"`
	Mountpoint    string  `json:"mountpoint"`
	UsedMB        float64 `json:"used_mb"`
	SoftMB        float64 `json:"soft_mb"`
	HardMB        float64 `json:"hard_mb"`
}
