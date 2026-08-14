package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

const (
	appSettingHASyncEnabled               = "ha_sync_enabled"
	appSettingHASyncIntervalDays          = "ha_sync_interval_days"
	appSettingHASyncStartHour             = "ha_sync_start_hour"
	appSettingHASyncDRNodeID              = "ha_sync_dr_node_id"
	appSettingHASyncDRHost                = "ha_sync_dr_host"
	appSettingHASyncDRSSHPort             = "ha_sync_dr_ssh_port"
	appSettingHASyncDRSSHUser             = "ha_sync_dr_ssh_user"
	appSettingHASyncDRKeyFile             = "ha_sync_dr_key_file"
	appSettingHASyncDRControllerPort      = "ha_sync_dr_controller_port"
	appSettingHASyncPrimaryHost           = "ha_sync_primary_host"
	appSettingHASyncPrimaryControllerPort = "ha_sync_primary_controller_port"
	appSettingHASyncScriptPath            = "ha_sync_script_path"
	appSettingHASyncSyncWebDist           = "ha_sync_sync_web_dist"
	appSettingHASyncSyncDatabase          = "ha_sync_sync_database"
	appSettingHASyncAutoFailover          = "ha_sync_auto_failover"
)

type HASyncConfig struct {
	Enabled               bool   `json:"enabled"`
	IntervalDays          int    `json:"interval_days"`
	StartHour             int    `json:"start_hour"`
	DRNodeID              string `json:"dr_node_id"`
	DRHost                string `json:"dr_host"`
	DRSSHPort             int    `json:"dr_ssh_port"`
	DRSSHUser             string `json:"dr_ssh_user"`
	DRKeyFile             string `json:"dr_key_file"`
	DRControllerPort      int    `json:"dr_controller_port"`
	PrimaryHost           string `json:"primary_host"`
	PrimaryControllerPort int    `json:"primary_controller_port"`
	ScriptPath            string `json:"script_path"`
	SyncWebDist           bool   `json:"sync_web_dist"`
	SyncDatabase          bool   `json:"sync_database"`
	AutoFailover          bool   `json:"auto_failover"`
}

type HASyncStep struct {
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type HASyncRun struct {
	RunID       int64        `json:"run_id"`
	TriggerMode string       `json:"trigger_mode"`
	Direction   string       `json:"direction"`
	Status      string       `json:"status"`
	StartedBy   string       `json:"started_by"`
	StartedAt   time.Time    `json:"started_at"`
	FinishedAt  *time.Time   `json:"finished_at,omitempty"`
	Summary     string       `json:"summary"`
	Detail      []HASyncStep `json:"detail"`
}

func parseListenPort(addr string, fallback int) int {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return fallback
	}
	_, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return fallback
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil || port <= 0 || port > 65535 {
		return fallback
	}
	return port
}

func parseListenHost(addr string, fallback string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return fallback
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fallback
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		return fallback
	}
	return host
}

func defaultHASyncConfig(cfg Config) HASyncConfig {
	const (
		defaultPrimaryPort = 8080
	)
	return HASyncConfig{
		Enabled:               false,
		IntervalDays:          1,
		StartHour:             3,
		DRNodeID:              "",
		DRHost:                "",
		DRSSHPort:             22,
		DRSSHUser:             "",
		DRKeyFile:             "",
		DRControllerPort:      defaultPrimaryPort,
		PrimaryHost:           parseListenHost(cfg.ListenAddr, "127.0.0.1"),
		PrimaryControllerPort: parseListenPort(cfg.ListenAddr, defaultPrimaryPort),
		ScriptPath:            "/opt/gpu-ops/scripts/ha_sync_worker.sh",
		SyncWebDist:           true,
		SyncDatabase:          true,
		AutoFailover:          false,
	}
}

func parseSettingBool(v string, fallback bool) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func parseSettingInt(v string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return fallback
	}
	return n
}

func normalizeHASyncConfig(cfg HASyncConfig) HASyncConfig {
	cfg.IntervalDays = max(1, min(30, cfg.IntervalDays))
	cfg.StartHour = max(0, min(23, cfg.StartHour))
	if cfg.DRSSHPort <= 0 || cfg.DRSSHPort > 65535 {
		cfg.DRSSHPort = 22
	}
	if cfg.DRControllerPort <= 0 || cfg.DRControllerPort > 65535 {
		cfg.DRControllerPort = 8080
	}
	if cfg.PrimaryControllerPort <= 0 || cfg.PrimaryControllerPort > 65535 {
		cfg.PrimaryControllerPort = 8080
	}
	cfg.DRNodeID = strings.TrimSpace(cfg.DRNodeID)
	cfg.DRHost = strings.TrimSpace(cfg.DRHost)
	cfg.DRSSHUser = strings.TrimSpace(cfg.DRSSHUser)
	cfg.DRKeyFile = strings.TrimSpace(cfg.DRKeyFile)
	cfg.PrimaryHost = strings.TrimSpace(cfg.PrimaryHost)
	if cfg.PrimaryHost == "" {
		cfg.PrimaryHost = "127.0.0.1"
	}
	cfg.ScriptPath = strings.TrimSpace(cfg.ScriptPath)
	if cfg.ScriptPath == "" {
		cfg.ScriptPath = "/opt/gpu-ops/scripts/ha_sync_worker.sh"
	}
	return cfg
}

func isPlaceholderHAValue(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "" || strings.ContainsAny(v, "<>") || strings.Contains(v, "replace-with") || strings.Contains(v, "example")
}

func localInterfaceIPs() map[string]struct{} {
	out := map[string]struct{}{"127.0.0.1": {}, "::1": {}}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil {
			out[ip.String()] = struct{}{}
		}
	}
	return out
}

func validateRemoteHAHost(host string) error {
	host = strings.TrimSpace(host)
	if isPlaceholderHAValue(host) {
		return errors.New("容灾节点地址尚未配置")
	}
	if strings.EqualFold(host, "localhost") {
		return errors.New("容灾节点不能指向本机")
	}
	local := localInterfaceIPs()
	resolved := []net.IP{}
	if ip := net.ParseIP(host); ip != nil {
		resolved = append(resolved, ip)
	} else {
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			return errors.New("容灾节点地址无法解析")
		}
		resolved = append(resolved, ips...)
	}
	for _, ip := range resolved {
		if _, ok := local[ip.String()]; ok {
			return errors.New("容灾节点解析到本机，必须使用独立故障域的主机")
		}
	}
	return nil
}

func validateHASyncConfigForRun(cfg HASyncConfig) error {
	if err := validateRemoteHAHost(cfg.DRHost); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.DRSSHUser) == "" {
		return errors.New("dr_ssh_user 不能为空")
	}
	if strings.TrimSpace(cfg.DRKeyFile) == "" {
		return errors.New("dr_key_file 不能为空")
	}
	if strings.TrimSpace(cfg.ScriptPath) == "" {
		return errors.New("script_path 不能为空")
	}
	return nil
}

func (s *Store) GetHASyncConfig(ctx context.Context, baseCfg Config) (HASyncConfig, error) {
	out := defaultHASyncConfig(baseCfg)
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM app_settings WHERE key = ANY($1)`, pq.Array([]string{
		appSettingHASyncEnabled,
		appSettingHASyncIntervalDays,
		appSettingHASyncStartHour,
		appSettingHASyncDRNodeID,
		appSettingHASyncDRHost,
		appSettingHASyncDRSSHPort,
		appSettingHASyncDRSSHUser,
		appSettingHASyncDRKeyFile,
		appSettingHASyncDRControllerPort,
		appSettingHASyncPrimaryHost,
		appSettingHASyncPrimaryControllerPort,
		appSettingHASyncScriptPath,
		appSettingHASyncSyncWebDist,
		appSettingHASyncSyncDatabase,
		appSettingHASyncAutoFailover,
	}))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return out, err
		}
		value = strings.TrimSpace(value)
		switch key {
		case appSettingHASyncEnabled:
			out.Enabled = parseSettingBool(value, out.Enabled)
		case appSettingHASyncIntervalDays:
			out.IntervalDays = parseSettingInt(value, out.IntervalDays)
		case appSettingHASyncStartHour:
			out.StartHour = parseSettingInt(value, out.StartHour)
		case appSettingHASyncDRNodeID:
			out.DRNodeID = value
		case appSettingHASyncDRHost:
			out.DRHost = value
		case appSettingHASyncDRSSHPort:
			out.DRSSHPort = parseSettingInt(value, out.DRSSHPort)
		case appSettingHASyncDRSSHUser:
			out.DRSSHUser = value
		case appSettingHASyncDRKeyFile:
			out.DRKeyFile = value
		case appSettingHASyncDRControllerPort:
			out.DRControllerPort = parseSettingInt(value, out.DRControllerPort)
		case appSettingHASyncPrimaryHost:
			out.PrimaryHost = value
		case appSettingHASyncPrimaryControllerPort:
			out.PrimaryControllerPort = parseSettingInt(value, out.PrimaryControllerPort)
		case appSettingHASyncScriptPath:
			out.ScriptPath = value
		case appSettingHASyncSyncWebDist:
			out.SyncWebDist = parseSettingBool(value, out.SyncWebDist)
		case appSettingHASyncSyncDatabase:
			out.SyncDatabase = parseSettingBool(value, out.SyncDatabase)
		case appSettingHASyncAutoFailover:
			out.AutoFailover = parseSettingBool(value, out.AutoFailover)
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return normalizeHASyncConfig(out), nil
}

func (s *Store) UpsertHASyncConfig(ctx context.Context, cfg HASyncConfig) error {
	cfg = normalizeHASyncConfig(cfg)
	if cfg.Enabled {
		if err := validateHASyncConfigForRun(cfg); err != nil {
			return err
		}
	}
	items := map[string]string{
		appSettingHASyncEnabled:               strconv.FormatBool(cfg.Enabled),
		appSettingHASyncIntervalDays:          strconv.Itoa(cfg.IntervalDays),
		appSettingHASyncStartHour:             strconv.Itoa(cfg.StartHour),
		appSettingHASyncDRNodeID:              strings.TrimSpace(cfg.DRNodeID),
		appSettingHASyncDRHost:                strings.TrimSpace(cfg.DRHost),
		appSettingHASyncDRSSHPort:             strconv.Itoa(cfg.DRSSHPort),
		appSettingHASyncDRSSHUser:             strings.TrimSpace(cfg.DRSSHUser),
		appSettingHASyncDRKeyFile:             strings.TrimSpace(cfg.DRKeyFile),
		appSettingHASyncDRControllerPort:      strconv.Itoa(cfg.DRControllerPort),
		appSettingHASyncPrimaryHost:           strings.TrimSpace(cfg.PrimaryHost),
		appSettingHASyncPrimaryControllerPort: strconv.Itoa(cfg.PrimaryControllerPort),
		appSettingHASyncScriptPath:            strings.TrimSpace(cfg.ScriptPath),
		appSettingHASyncSyncWebDist:           strconv.FormatBool(cfg.SyncWebDist),
		appSettingHASyncSyncDatabase:          strconv.FormatBool(cfg.SyncDatabase),
		appSettingHASyncAutoFailover:          strconv.FormatBool(cfg.AutoFailover),
	}
	for k, v := range items {
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO app_settings(key, value, updated_at)
VALUES($1,$2,NOW())
ON CONFLICT (key) DO UPDATE
SET value=EXCLUDED.value, updated_at=NOW()`, k, v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) InsertHASyncRun(ctx context.Context, triggerMode string, direction string, startedBy string) (int64, error) {
	triggerMode = strings.TrimSpace(triggerMode)
	if triggerMode == "" {
		triggerMode = "manual"
	}
	direction = strings.TrimSpace(direction)
	if direction == "" {
		direction = "primary_to_standby"
	}
	startedBy = strings.TrimSpace(startedBy)
	if startedBy == "" {
		startedBy = "system"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	// 多个控制器共用数据库时也只允许一个同步任务，避免双主并发覆盖。
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(741906401)`); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE ha_sync_runs
SET status='failed', summary='任务超过 3 小时未结束，已自动标记失败', finished_at=NOW()
WHERE status='running' AND started_at < NOW() - INTERVAL '3 hours'`); err != nil {
		return 0, err
	}
	var running bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ha_sync_runs WHERE status='running')`).Scan(&running); err != nil {
		return 0, err
	}
	if running {
		return 0, errHASyncRunning
	}
	var runID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO ha_sync_runs(trigger_mode, direction, status, started_by, started_at)
VALUES($1, $2, 'running', $3, NOW())
RETURNING run_id`, triggerMode, direction, startedBy).Scan(&runID)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return runID, nil
}

func (s *Store) FinishHASyncRun(ctx context.Context, runID int64, status string, summary string, detail []HASyncStep) error {
	if runID <= 0 {
		return errors.New("run_id 不合法")
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "failed"
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "执行完成"
	}
	if detail == nil {
		detail = []HASyncStep{}
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE ha_sync_runs
SET status=$2,
    summary=$3,
    detail_json=$4,
    finished_at=NOW()
WHERE run_id=$1`, runID, status, summary, string(detailJSON))
	return err
}

func decodeHASyncDetail(raw string) []HASyncStep {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []HASyncStep{}
	}
	out := []HASyncStep{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []HASyncStep{}
	}
	return out
}

func (s *Store) ListHASyncRuns(ctx context.Context, limit int) ([]HASyncRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT run_id, trigger_mode, direction, status, started_by, started_at, finished_at, summary, detail_json::text
FROM ha_sync_runs
ORDER BY run_id DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]HASyncRun, 0, limit)
	for rows.Next() {
		var x HASyncRun
		var finished sql.NullTime
		var detailRaw string
		if err := rows.Scan(&x.RunID, &x.TriggerMode, &x.Direction, &x.Status, &x.StartedBy, &x.StartedAt, &finished, &x.Summary, &detailRaw); err != nil {
			return nil, err
		}
		x.StartedAt = asBeijingWallTime(x.StartedAt)
		if finished.Valid {
			t := asBeijingWallTime(finished.Time)
			x.FinishedAt = &t
		}
		x.Detail = decodeHASyncDetail(detailRaw)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetLatestHASyncRun(ctx context.Context, triggerMode string) (HASyncRun, bool, error) {
	var out HASyncRun
	base := `
SELECT run_id, trigger_mode, direction, status, started_by, started_at, finished_at, summary, detail_json::text
FROM ha_sync_runs`
	args := []any{}
	if strings.TrimSpace(triggerMode) != "" {
		base += ` WHERE trigger_mode=$1`
		args = append(args, strings.TrimSpace(triggerMode))
	}
	base += ` ORDER BY run_id DESC LIMIT 1`

	var finished sql.NullTime
	var detailRaw string
	err := s.db.QueryRowContext(ctx, base, args...).Scan(
		&out.RunID,
		&out.TriggerMode,
		&out.Direction,
		&out.Status,
		&out.StartedBy,
		&out.StartedAt,
		&finished,
		&out.Summary,
		&detailRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HASyncRun{}, false, nil
		}
		return HASyncRun{}, false, err
	}
	out.StartedAt = asBeijingWallTime(out.StartedAt)
	if finished.Valid {
		t := asBeijingWallTime(finished.Time)
		out.FinishedAt = &t
	}
	out.Detail = decodeHASyncDetail(detailRaw)
	return out, true, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
