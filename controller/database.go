package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

const (
	appSettingSMTPHost                   = "smtp_host"
	appSettingSMTPPort                   = "smtp_port"
	appSettingSMTPUser                   = "smtp_user"
	appSettingSMTPPass                   = "smtp_pass"
	appSettingFromEmail                  = "from_email"
	appSettingFromName                   = "from_name"
	appSettingUserGuideline              = "user_guideline_markdown"
	appSettingUserGuidelineBy            = "user_guideline_updated_by"
	appSettingMonthlyDoctorPoints        = "monthly_points_doctor"
	appSettingMonthlyMasterPoints        = "monthly_points_master"
	appSettingMonthlyOtherPoints         = "monthly_points_other"
	appSettingMonthlyCarryoverLimit      = "monthly_points_carryover_limit"
	appSettingMonthlyMaxOverdraft        = "monthly_points_max_overdraft_limit"
	appSettingUsageRetentionDays         = "usage_retention_days"
	appSettingUsageLastDeleteAt          = "usage_last_delete_at"
	appSettingUsageLastDeleteFrom        = "usage_last_delete_from"
	appSettingUsageLastDeleteTo          = "usage_last_delete_to"
	appSettingUsageLastDeleteCount       = "usage_last_delete_count"
	appSettingUsageLastDeleteMode        = "usage_last_delete_mode"
	appSettingUsageLastDeleteBy          = "usage_last_delete_by"
	appSettingBindChallengeWindowSeconds = "bind_challenge_window_seconds"
	appSettingBindTrialCPUQuotaPercent   = "bind_trial_cpu_quota_percent"
	appSettingBindTrialMemoryLimitGB     = "bind_trial_memory_limit_gb"
	appSettingBindTrialGPUBlocked        = "bind_trial_gpu_blocked"
	appSettingBindSingleActivePerBilling = "bind_single_active_challenge_per_billing"
	appSettingBindFirstCooldownMinutes   = "bind_first_cooldown_minutes"
	appSettingBindRepeatCooldownMinutes  = "bind_repeat_cooldown_minutes"
	appSettingBindContentionFreezeMinute = "bind_contention_freeze_minutes"
	platformUIDPreferredMin              = 20000
	platformUIDPreferredMax              = 60000
	platformUIDFallbackMin               = 1000
	platformUIDHardMax                   = 65533
	platformUIDReleaseRetentionYears     = 3
)

var (
	errRegistrationVerifyTokenInvalid = errors.New("registration_verify_token_invalid")
	errRegistrationVerifyTokenExpired = errors.New("registration_verify_token_expired")
	errRegistrationVerifyTokenUsed    = errors.New("registration_verify_token_used")
	errRegisterCaptchaInvalid         = errors.New("register_captcha_invalid")
	errRegisterCaptchaExpired         = errors.New("register_captcha_expired")
	errRegisterCaptchaUsed            = errors.New("register_captcha_used")
	errProvisionMessageDestroyed      = errors.New("provision_message_destroyed")
	errNodeBindCooldownActive         = errors.New("node_bind_cooldown_active")
	errNodeBindTargetFrozen           = errors.New("node_bind_target_frozen")
	errNodeBindChallengeExpired       = errors.New("node_bind_challenge_expired")
	errNodeBindChallengeNotFound      = errors.New("node_bind_challenge_not_found")
	errNodeBindChallengeMismatch      = errors.New("node_bind_challenge_mismatch")
	errNodeBindAlreadyBound           = errors.New("node_bind_already_bound")
	platformUIDSensitiveValues        = map[int]struct{}{
		65534: {},
		65535: {},
	}
	platformUIDInventoryFiles = []string{
		"/home/gpuops/gpu-ops/my_ssh_keys/node_uid_gid_used_ids.txt",
		"my_ssh_keys/node_uid_gid_used_ids.txt",
	}
)

type Store struct {
	db *sql.DB
}

func NewStore(cfg Config) (*Store, error) {
	dsn := ensurePostgresURLTimezone(cfg.DatabaseDSN, beijingTZName)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	// 连接池参数可按实际压测调优
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func ensurePostgresURLTimezone(rawDSN string, tz string) string {
	rawDSN = strings.TrimSpace(rawDSN)
	if rawDSN == "" || tz == "" {
		return rawDSN
	}
	// 本项目默认使用 URL DSN；若解析失败则保持原样，避免破坏非 URL 形态。
	u, err := url.Parse(rawDSN)
	if err != nil || u == nil {
		return rawDSN
	}
	q := u.Query()
	if strings.TrimSpace(q.Get("timezone")) != "" {
		return rawDSN
	}
	q.Set("timezone", tz)
	u.RawQuery = q.Encode()
	return u.String()
}

func defaultNodeBindSecurityPolicy() NodeBindSecurityPolicy {
	return NodeBindSecurityPolicy{
		ChallengeWindowSeconds:       300,
		TrialCPUQuotaPercent:         20,
		TrialMemoryLimitGB:           8,
		TrialGPUBlocked:              true,
		SingleActivePerBilling:       true,
		FirstFailureCooldownMinutes:  30,
		RepeatFailureCooldownMinutes: 180,
		ContentionFreezeMinutes:      30,
	}
}

func normalizeNodeBindSecurityPolicy(in NodeBindSecurityPolicy) NodeBindSecurityPolicy {
	out := in
	def := defaultNodeBindSecurityPolicy()
	if out.ChallengeWindowSeconds < 60 || out.ChallengeWindowSeconds > 1800 {
		out.ChallengeWindowSeconds = def.ChallengeWindowSeconds
	}
	if out.TrialCPUQuotaPercent < 1 || out.TrialCPUQuotaPercent > 100 {
		out.TrialCPUQuotaPercent = def.TrialCPUQuotaPercent
	}
	if out.TrialMemoryLimitGB < 0.5 || out.TrialMemoryLimitGB > 1024 {
		out.TrialMemoryLimitGB = def.TrialMemoryLimitGB
	}
	if out.FirstFailureCooldownMinutes < 1 || out.FirstFailureCooldownMinutes > 24*60 {
		out.FirstFailureCooldownMinutes = def.FirstFailureCooldownMinutes
	}
	if out.RepeatFailureCooldownMinutes < 1 || out.RepeatFailureCooldownMinutes > 72*60 {
		out.RepeatFailureCooldownMinutes = def.RepeatFailureCooldownMinutes
	}
	if out.ContentionFreezeMinutes < 1 || out.ContentionFreezeMinutes > 24*60 {
		out.ContentionFreezeMinutes = def.ContentionFreezeMinutes
	}
	return out
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ApplyMigrations(ctx context.Context, migrationDir string) error {
	dir, err := resolveMigrationDir(migrationDir)
	if err != nil {
		return err
	}

	if err := ensureMigrationsTable(ctx, s.db); err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".sql") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	sort.Strings(files)

	for _, f := range files {
		filename := filepath.Base(f)
		applied, err := isMigrationApplied(ctx, s.db, filename)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		sqlBytes, err := os.ReadFile(filepath.Clean(f))
		if err != nil {
			return err
		}
		sqlText := strings.TrimSpace(string(sqlBytes))
		if sqlText == "" {
			return fmt.Errorf("迁移文件为空：%s", filename)
		}

		if _, err := s.db.ExecContext(ctx, sqlText); err != nil {
			return fmt.Errorf("执行迁移失败 %s: %w", filename, err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_migrations(filename) VALUES ($1)`, filename); err != nil {
			return fmt.Errorf("记录迁移失败 %s: %w", filename, err)
		}
	}
	return nil
}

func ensureMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
	filename TEXT PRIMARY KEY,
	applied_at TIMESTAMP NOT NULL DEFAULT NOW()
);`)
	return err
}

func isMigrationApplied(ctx context.Context, db *sql.DB, filename string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename=$1)`, filename).Scan(&exists)
	return exists, err
}

func resolveMigrationDir(cfgValue string) (string, error) {
	if cfgValue != "" {
		if dirExists(cfgValue) {
			return cfgValue, nil
		}
		return "", fmt.Errorf("migration_dir 不存在：%s", cfgValue)
	}

	candidates := []string{
		filepath.FromSlash("../database/migrations"),
		filepath.FromSlash("database/migrations"),
	}
	for _, c := range candidates {
		if dirExists(c) {
			return c, nil
		}
	}
	return "", errors.New("未找到迁移目录：请配置 migration_dir 或在 ../database/migrations 下放置迁移文件")
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func maxNonZeroTime(vals ...time.Time) time.Time {
	var best time.Time
	for _, v := range vals {
		if v.IsZero() {
			continue
		}
		if best.IsZero() || v.After(best) {
			best = v
		}
	}
	return best
}

// 数据库存储大量 TIMESTAMP（无时区）字段。读取后需要按“北京时间墙上时间”解释，
// 不能做时刻换算（In），否则会出现 +8 小时偏移。
func asBeijingWallTime(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	y, m, d := t.Date()
	hh, mm, ss := t.Clock()
	return time.Date(y, m, d, hh, mm, ss, t.Nanosecond(), beijingLocation)
}

func normalizeNodeHeartbeatTimes(n *NodeStatus) {
	if n == nil {
		return
	}
	n.LastSeenAt = asBeijingWallTime(n.LastSeenAt)
	n.LastReportTS = asBeijingWallTime(n.LastReportTS)
	n.UpdatedAt = asBeijingWallTime(n.UpdatedAt)
	best := maxNonZeroTime(n.LastSeenAt, n.LastReportTS, n.UpdatedAt)
	if !best.IsZero() {
		n.LastSeenAt = best
	}
}

func normalizeNodeRuntimeSnapshotTimes(x *NodeRuntimeSnapshot) {
	if x == nil {
		return
	}
	x.ReportTS = asBeijingWallTime(x.ReportTS)
}

func normalizeNodeLocalUserTimes(u *NodeLocalUser) {
	if u == nil {
		return
	}
	if u.CPUQuotaUpdatedAt != nil {
		t := asBeijingWallTime(*u.CPUQuotaUpdatedAt)
		u.CPUQuotaUpdatedAt = &t
	}
	if u.MemoryLimitUpdatedAt != nil {
		t := asBeijingWallTime(*u.MemoryLimitUpdatedAt)
		u.MemoryLimitUpdatedAt = &t
	}
	if u.GPUVisibilityUpdatedAt != nil {
		t := asBeijingWallTime(*u.GPUVisibilityUpdatedAt)
		u.GPUVisibilityUpdatedAt = &t
	}
	if u.HomeCreatedAt != nil {
		t := asBeijingWallTime(*u.HomeCreatedAt)
		u.HomeCreatedAt = &t
	}
	if u.LastLoginAt != nil {
		t := asBeijingWallTime(*u.LastLoginAt)
		u.LastLoginAt = &t
	}
	u.UpdatedAt = asBeijingWallTime(u.UpdatedAt)
}

func normalizeNodeUserDiskQuotaTimes(q *NodeUserDiskQuota) {
	if q == nil {
		return
	}
	q.UpdatedAt = asBeijingWallTime(q.UpdatedAt)
}

func normalizeNodeUserCPULimitTimes(x *NodeUserCPULimit) {
	if x == nil {
		return
	}
	x.UpdatedAt = asBeijingWallTime(x.UpdatedAt)
}

func normalizeNodeUserMemoryLimitTimes(x *NodeUserMemoryLimit) {
	if x == nil {
		return
	}
	x.UpdatedAt = asBeijingWallTime(x.UpdatedAt)
}

func normalizeNodeUserGPUVisibilityTimes(x *NodeUserGPUVisibility) {
	if x == nil {
		return
	}
	x.UpdatedAt = asBeijingWallTime(x.UpdatedAt)
}

func normalizeSSHWhitelistEntryTimes(x *SSHWhitelistEntry) {
	if x == nil {
		return
	}
	x.CreatedAt = asBeijingWallTime(x.CreatedAt)
	x.UpdatedAt = asBeijingWallTime(x.UpdatedAt)
}

func normalizeSSHBlacklistEntryTimes(x *SSHBlacklistEntry) {
	if x == nil {
		return
	}
	x.CreatedAt = asBeijingWallTime(x.CreatedAt)
	x.UpdatedAt = asBeijingWallTime(x.UpdatedAt)
}

func normalizeSSHExemptionEntryTimes(x *SSHExemptionEntry) {
	if x == nil {
		return
	}
	x.CreatedAt = asBeijingWallTime(x.CreatedAt)
	x.UpdatedAt = asBeijingWallTime(x.UpdatedAt)
}

func normalizeAnnouncementTimes(x *Announcement) {
	if x == nil {
		return
	}
	x.CreatedAt = asBeijingWallTime(x.CreatedAt)
	x.UpdatedAt = asBeijingWallTime(x.UpdatedAt)
}

func normalizeAdminNoteTimes(x *AdminNote) {
	if x == nil {
		return
	}
	x.NoteDate = asBeijingWallTime(x.NoteDate)
	x.CreatedAt = asBeijingWallTime(x.CreatedAt)
	x.UpdatedAt = asBeijingWallTime(x.UpdatedAt)
}

func normalizeSpecialMonthlyPointsRuleTimes(x *SpecialMonthlyPointsRule) {
	if x == nil {
		return
	}
	x.CreatedAt = asBeijingWallTime(x.CreatedAt)
	x.UpdatedAt = asBeijingWallTime(x.UpdatedAt)
}

func normalizeNodeSecurityEventTimes(e *NodeSecurityEvent) {
	if e == nil {
		return
	}
	e.CreatedAt = asBeijingWallTime(e.CreatedAt)
}

func normalizeNodeSecurityEventSummaryTimes(x *NodeSecurityEventSummary) {
	if x == nil {
		return
	}
	x.FirstSeenAt = asBeijingWallTime(x.FirstSeenAt)
	x.LastSeenAt = asBeijingWallTime(x.LastSeenAt)
}

func normalizeNodeSuspiciousUserTimes(u *NodeSuspiciousUser) {
	if u == nil {
		return
	}
	u.LastSeenAt = asBeijingWallTime(u.LastSeenAt)
}

func normalizePlatformUsageNodeDetailTimes(x *PlatformUsageNodeDetail) {
	if x == nil {
		return
	}
	x.LastSeenAt = asBeijingWallTime(x.LastSeenAt)
	x.LastUsageAt = asBeijingWallTime(x.LastUsageAt)
}

func normalizeNodeMonthlyUserCostTimes(x *NodeMonthlyUserCost) {
	if x == nil {
		return
	}
	x.LastUsageAt = asBeijingWallTime(x.LastUsageAt)
}

func (s *Store) EnsureUserTx(ctx context.Context, tx *sql.Tx, username string, defaultBalance float64) (User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return User{}, errors.New("username 不能为空")
	}

	_, err := tx.ExecContext(ctx, `
INSERT INTO users(username, balance, status)
VALUES($1, $2, 'normal')
ON CONFLICT (username) DO NOTHING`, username, defaultBalance)
	if err != nil {
		return User{}, err
	}

	return s.GetUserTx(ctx, tx, username)
}

// TryInsertReportTx 尝试写入上报记录（用于幂等）。
// 返回 inserted=false 表示该 report_id 已处理过，应跳过扣费与落库。
func (s *Store) TryInsertReportTx(ctx context.Context, tx *sql.Tx, reportID string, nodeID string, ts time.Time, intervalSeconds int) (bool, error) {
	reportID = strings.TrimSpace(reportID)
	if reportID == "" {
		return false, errors.New("report_id 不能为空")
	}
	if intervalSeconds <= 0 {
		intervalSeconds = 60
	}
	res, err := tx.ExecContext(ctx, `
INSERT INTO metric_reports(report_id, node_id, timestamp, interval_seconds)
VALUES($1,$2,$3,$4)
ON CONFLICT (report_id) DO NOTHING`, reportID, nodeID, ts, intervalSeconds)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

func (s *Store) GetUser(ctx context.Context, username string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
SELECT username, balance, carryover_balance, status, blocked_at
FROM users
WHERE username=$1`, username).Scan(&u.Username, &u.Balance, &u.CarryoverBalance, &u.Status, &u.BlockedAt)
	return u, err
}

func (s *Store) GetUserTx(ctx context.Context, tx *sql.Tx, username string) (User, error) {
	var u User
	err := tx.QueryRowContext(ctx, `
SELECT username, balance, carryover_balance, status, blocked_at
FROM users
WHERE username=$1`, username).Scan(&u.Username, &u.Balance, &u.CarryoverBalance, &u.Status, &u.BlockedAt)
	return u, err
}

type BalanceUpdateResult struct {
	PrevStatus           string
	PrevEffectiveBalance float64
	EffectiveBalance     float64
	User                 User
}

type rechargeReasonCtxKey struct{}

func withRechargeReason(ctx context.Context, reason string) context.Context {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ctx
	}
	return context.WithValue(ctx, rechargeReasonCtxKey{}, reason)
}

func rechargeReasonFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v := strings.TrimSpace(fmt.Sprintf("%v", ctx.Value(rechargeReasonCtxKey{})))
	return v
}

func (s *Store) insertRechargeRecordTx(ctx context.Context, tx *sql.Tx, username string, amount float64, method string, pointsScope string, nodeID string) error {
	username = strings.TrimSpace(username)
	method = strings.TrimSpace(method)
	pointsScope = strings.TrimSpace(pointsScope)
	nodeID = strings.TrimSpace(nodeID)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if method == "" {
		return errors.New("method 不能为空")
	}
	if pointsScope == "" {
		pointsScope = "general"
	}
	if pointsScope != "general" && pointsScope != "carryover" && pointsScope != "node_exclusive" {
		return errors.New("points_scope 不合法")
	}
	if pointsScope != "node_exclusive" {
		nodeID = ""
	}
	reason := rechargeReasonFromContext(ctx)
	_, err := tx.ExecContext(ctx, `
INSERT INTO recharge_records(username, amount, method, points_scope, node_id, reason)
VALUES($1, $2, $3, $4, NULLIF($5, ''), $6)`, username, amount, method, pointsScope, nodeID, reason)
	return err
}

func (s *Store) ensureNodeExclusivePointsRowTx(ctx context.Context, tx *sql.Tx, username string, nodeID string) error {
	username = strings.TrimSpace(username)
	nodeID = strings.TrimSpace(nodeID)
	if username == "" || nodeID == "" {
		return errors.New("username/node_id 不能为空")
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO user_node_exclusive_points(username, node_id, balance, updated_by)
VALUES($1, $2, 0, 'system')
ON CONFLICT (username, node_id) DO NOTHING`, username, nodeID)
	return err
}

func (s *Store) GetUserExclusiveBalanceTotal(ctx context.Context, username string) (float64, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return 0, errors.New("username 不能为空")
	}
	var total float64
	if err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(balance), 0)
FROM user_node_exclusive_points
WHERE username=$1`, username).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (s *Store) GetUserExclusiveBalanceTotalTx(ctx context.Context, tx *sql.Tx, username string) (float64, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return 0, errors.New("username 不能为空")
	}
	var total float64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(SUM(balance), 0)
FROM user_node_exclusive_points
WHERE username=$1`, username).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (s *Store) ListNodeExclusivePointsByUser(ctx context.Context, username string, limit int) ([]NodeExclusivePointsBalance, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username 不能为空")
	}
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT username, node_id, balance, updated_by, created_at, updated_at
FROM user_node_exclusive_points
WHERE username=$1
ORDER BY node_id ASC
LIMIT $2`, username, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeExclusivePointsBalance, 0)
	for rows.Next() {
		var x NodeExclusivePointsBalance
		if err := rows.Scan(&x.Username, &x.NodeID, &x.Balance, &x.UpdatedBy, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) DeductBalanceTx(
	ctx context.Context,
	tx *sql.Tx,
	username string,
	amount float64,
	now time.Time,
	cfg Config,
) (BalanceUpdateResult, error) {
	return s.DeductBalanceOnNodeTx(ctx, tx, username, "", amount, now, cfg)
}

func (s *Store) DeductBalanceOnNodeTx(
	ctx context.Context,
	tx *sql.Tx,
	username string,
	nodeID string,
	amount float64,
	now time.Time,
	cfg Config,
) (BalanceUpdateResult, error) {
	username = strings.TrimSpace(username)
	nodeID = strings.TrimSpace(nodeID)
	if username == "" {
		return BalanceUpdateResult{}, errors.New("username 不能为空")
	}
	if amount < 0 {
		return BalanceUpdateResult{}, errors.New("amount 不能为负数")
	}
	_, err := s.EnsureUserTx(ctx, tx, username, cfg.DefaultBalance)
	if err != nil {
		return BalanceUpdateResult{}, err
	}

	var balance float64
	var carryoverBalance float64
	var prevStatus string
	var blockedAt *time.Time
	if err := tx.QueryRowContext(ctx, `
SELECT balance, carryover_balance, status, blocked_at
FROM users
WHERE username=$1
FOR UPDATE`, username).Scan(&balance, &carryoverBalance, &prevStatus, &blockedAt); err != nil {
		return BalanceUpdateResult{}, err
	}

	nodeExclusiveBalance := 0.0
	newNodeExclusiveBalance := 0.0
	if nodeID != "" {
		if err := s.ensureNodeExclusivePointsRowTx(ctx, tx, username, nodeID); err != nil {
			return BalanceUpdateResult{}, err
		}
		if err := tx.QueryRowContext(ctx, `
SELECT balance
FROM user_node_exclusive_points
WHERE username=$1 AND node_id=$2
FOR UPDATE`, username, nodeID).Scan(&nodeExclusiveBalance); err != nil {
			return BalanceUpdateResult{}, err
		}
		newNodeExclusiveBalance = nodeExclusiveBalance
	}

	newBalance := balance
	newCarryoverBalance := carryoverBalance
	remainingDeduct := amount
	if !cfg.DryRun {
		if nodeID != "" && nodeExclusiveBalance > 0 && remainingDeduct > 0 {
			deductExclusive := remainingDeduct
			if deductExclusive > nodeExclusiveBalance {
				deductExclusive = nodeExclusiveBalance
			}
			newNodeExclusiveBalance = nodeExclusiveBalance - deductExclusive
			remainingDeduct -= deductExclusive
		}
		if newCarryoverBalance > 0 && remainingDeduct > 0 {
			deductCarryover := remainingDeduct
			if deductCarryover > newCarryoverBalance {
				deductCarryover = newCarryoverBalance
			}
			newCarryoverBalance -= deductCarryover
			remainingDeduct -= deductCarryover
		}
		if remainingDeduct > 0 {
			newBalance = balance - remainingDeduct
		}
	}
	if !cfg.DryRun && nodeID != "" && newNodeExclusiveBalance != nodeExclusiveBalance {
		if _, err := tx.ExecContext(ctx, `
UPDATE user_node_exclusive_points
SET balance=$3, updated_by='system', updated_at=NOW()
WHERE username=$1 AND node_id=$2`, username, nodeID, newNodeExclusiveBalance); err != nil {
			return BalanceUpdateResult{}, err
		}
	}
	usableBalance := newBalance + newCarryoverBalance
	newStatus := StatusForBalance(usableBalance, cfg.WarningThreshold, cfg.LimitedThreshold)
	prevUsableBalance := balance + carryoverBalance
	newBlockedAt := blockedAt
	if newStatus == "blocked" {
		if newBlockedAt == nil {
			newBlockedAt = &now
		}
	} else {
		newBlockedAt = nil
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE users
SET balance=$2, carryover_balance=$3, status=$4, blocked_at=$5
WHERE username=$1`, username, newBalance, newCarryoverBalance, newStatus, newBlockedAt); err != nil {
		return BalanceUpdateResult{}, err
	}

	return BalanceUpdateResult{
		PrevStatus:           prevStatus,
		PrevEffectiveBalance: prevUsableBalance,
		EffectiveBalance:     usableBalance,
		User: User{
			Username:         username,
			Balance:          newBalance,
			CarryoverBalance: newCarryoverBalance,
			Status:           newStatus,
			BlockedAt:        newBlockedAt,
		},
	}, nil
}

func (s *Store) RechargeTx(ctx context.Context, tx *sql.Tx, username string, amount float64, method string, now time.Time, cfg Config) (BalanceUpdateResult, error) {
	if amount <= 0 {
		return BalanceUpdateResult{}, errors.New("amount 必须为正数")
	}
	return s.AdjustBalanceTx(ctx, tx, username, amount, method, now, cfg)
}

func (s *Store) AdjustBalanceTx(ctx context.Context, tx *sql.Tx, username string, delta float64, method string, now time.Time, cfg Config) (BalanceUpdateResult, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return BalanceUpdateResult{}, errors.New("username 不能为空")
	}
	if delta == 0 {
		return BalanceUpdateResult{}, errors.New("delta 不能为 0")
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return BalanceUpdateResult{}, errors.New("method 不能为空")
	}

	_, err := s.EnsureUserTx(ctx, tx, username, cfg.DefaultBalance)
	if err != nil {
		return BalanceUpdateResult{}, err
	}

	var balance float64
	var carryoverBalance float64
	var prevStatus string
	var blockedAt *time.Time
	if err := tx.QueryRowContext(ctx, `
SELECT balance, carryover_balance, status, blocked_at
FROM users
WHERE username=$1
FOR UPDATE`, username).Scan(&balance, &carryoverBalance, &prevStatus, &blockedAt); err != nil {
		return BalanceUpdateResult{}, err
	}

	newBalance := balance + delta
	usableBalance := newBalance + carryoverBalance
	prevUsableBalance := balance + carryoverBalance
	newStatus := StatusForBalance(usableBalance, cfg.WarningThreshold, cfg.LimitedThreshold)
	var newBlockedAt *time.Time
	if newStatus == "blocked" {
		newBlockedAt = blockedAt
		if newBlockedAt == nil {
			newBlockedAt = &now
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE users
SET balance=$2, status=$3, blocked_at=$4, last_charge_time=NOW()
WHERE username=$1`, username, newBalance, newStatus, newBlockedAt); err != nil {
		return BalanceUpdateResult{}, err
	}

	if err := s.insertRechargeRecordTx(ctx, tx, username, delta, method, "general", ""); err != nil {
		return BalanceUpdateResult{}, err
	}

	return BalanceUpdateResult{
		PrevStatus:           prevStatus,
		PrevEffectiveBalance: prevUsableBalance,
		EffectiveBalance:     usableBalance,
		User: User{
			Username:         username,
			Balance:          newBalance,
			CarryoverBalance: carryoverBalance,
			Status:           newStatus,
			BlockedAt:        newBlockedAt,
		},
	}, nil
}

func (s *Store) AdjustCarryoverBalanceTx(ctx context.Context, tx *sql.Tx, username string, delta float64, method string, now time.Time, cfg Config) (BalanceUpdateResult, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return BalanceUpdateResult{}, errors.New("username 不能为空")
	}
	if delta == 0 {
		return BalanceUpdateResult{}, errors.New("delta 不能为 0")
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return BalanceUpdateResult{}, errors.New("method 不能为空")
	}

	_, err := s.EnsureUserTx(ctx, tx, username, cfg.DefaultBalance)
	if err != nil {
		return BalanceUpdateResult{}, err
	}

	var balance float64
	var carryoverBalance float64
	var prevStatus string
	var blockedAt *time.Time
	if err := tx.QueryRowContext(ctx, `
SELECT balance, carryover_balance, status, blocked_at
FROM users
WHERE username=$1
FOR UPDATE`, username).Scan(&balance, &carryoverBalance, &prevStatus, &blockedAt); err != nil {
		return BalanceUpdateResult{}, err
	}

	nextCarryover := carryoverBalance + delta
	if nextCarryover < 0 {
		return BalanceUpdateResult{}, errors.New("结转积分不足，无法扣减")
	}
	usableBalance := balance + nextCarryover
	prevUsableBalance := balance + carryoverBalance
	newStatus := StatusForBalance(usableBalance, cfg.WarningThreshold, cfg.LimitedThreshold)
	var newBlockedAt *time.Time
	if newStatus == "blocked" {
		newBlockedAt = blockedAt
		if newBlockedAt == nil {
			newBlockedAt = &now
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE users
SET carryover_balance=$2, status=$3, blocked_at=$4, last_charge_time=NOW()
WHERE username=$1`, username, nextCarryover, newStatus, newBlockedAt); err != nil {
		return BalanceUpdateResult{}, err
	}
	if err := s.insertRechargeRecordTx(ctx, tx, username, delta, method, "carryover", ""); err != nil {
		return BalanceUpdateResult{}, err
	}
	return BalanceUpdateResult{
		PrevStatus:           prevStatus,
		PrevEffectiveBalance: prevUsableBalance,
		EffectiveBalance:     usableBalance,
		User: User{
			Username:         username,
			Balance:          balance,
			CarryoverBalance: nextCarryover,
			Status:           newStatus,
			BlockedAt:        newBlockedAt,
		},
	}, nil
}

func (s *Store) SetBalanceTx(ctx context.Context, tx *sql.Tx, username string, targetBalance float64, method string, now time.Time, cfg Config) (BalanceUpdateResult, float64, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return BalanceUpdateResult{}, 0, errors.New("username 不能为空")
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return BalanceUpdateResult{}, 0, errors.New("method 不能为空")
	}

	_, err := s.EnsureUserTx(ctx, tx, username, cfg.DefaultBalance)
	if err != nil {
		return BalanceUpdateResult{}, 0, err
	}

	var balance float64
	var carryoverBalance float64
	var prevStatus string
	var blockedAt *time.Time
	if err := tx.QueryRowContext(ctx, `
SELECT balance, carryover_balance, status, blocked_at
FROM users
WHERE username=$1
FOR UPDATE`, username).Scan(&balance, &carryoverBalance, &prevStatus, &blockedAt); err != nil {
		return BalanceUpdateResult{}, 0, err
	}

	delta := targetBalance - balance
	usableBalance := targetBalance + carryoverBalance
	prevUsableBalance := balance + carryoverBalance
	newStatus := StatusForBalance(usableBalance, cfg.WarningThreshold, cfg.LimitedThreshold)
	var newBlockedAt *time.Time
	if newStatus == "blocked" {
		newBlockedAt = blockedAt
		if newBlockedAt == nil {
			newBlockedAt = &now
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE users
SET balance=$2, status=$3, blocked_at=$4, last_charge_time=NOW()
WHERE username=$1`, username, targetBalance, newStatus, newBlockedAt); err != nil {
		return BalanceUpdateResult{}, 0, err
	}

	if delta != 0 {
		if err := s.insertRechargeRecordTx(ctx, tx, username, delta, method, "general", ""); err != nil {
			return BalanceUpdateResult{}, 0, err
		}
	}

	return BalanceUpdateResult{
		PrevStatus:           prevStatus,
		PrevEffectiveBalance: prevUsableBalance,
		EffectiveBalance:     usableBalance,
		User: User{
			Username:         username,
			Balance:          targetBalance,
			CarryoverBalance: carryoverBalance,
			Status:           newStatus,
			BlockedAt:        newBlockedAt,
		},
	}, delta, nil
}

func (s *Store) SetCarryoverBalanceTx(ctx context.Context, tx *sql.Tx, username string, targetCarryover float64, method string, now time.Time, cfg Config) (BalanceUpdateResult, float64, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return BalanceUpdateResult{}, 0, errors.New("username 不能为空")
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return BalanceUpdateResult{}, 0, errors.New("method 不能为空")
	}
	if targetCarryover < 0 {
		return BalanceUpdateResult{}, 0, errors.New("target_carryover 不能为负数")
	}

	_, err := s.EnsureUserTx(ctx, tx, username, cfg.DefaultBalance)
	if err != nil {
		return BalanceUpdateResult{}, 0, err
	}

	var balance float64
	var carryoverBalance float64
	var prevStatus string
	var blockedAt *time.Time
	if err := tx.QueryRowContext(ctx, `
SELECT balance, carryover_balance, status, blocked_at
FROM users
WHERE username=$1
FOR UPDATE`, username).Scan(&balance, &carryoverBalance, &prevStatus, &blockedAt); err != nil {
		return BalanceUpdateResult{}, 0, err
	}

	delta := targetCarryover - carryoverBalance
	usableBalance := balance + targetCarryover
	prevUsableBalance := balance + carryoverBalance
	newStatus := StatusForBalance(usableBalance, cfg.WarningThreshold, cfg.LimitedThreshold)
	var newBlockedAt *time.Time
	if newStatus == "blocked" {
		newBlockedAt = blockedAt
		if newBlockedAt == nil {
			newBlockedAt = &now
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE users
SET carryover_balance=$2, status=$3, blocked_at=$4, last_charge_time=NOW()
WHERE username=$1`, username, targetCarryover, newStatus, newBlockedAt); err != nil {
		return BalanceUpdateResult{}, 0, err
	}

	if delta != 0 {
		if err := s.insertRechargeRecordTx(ctx, tx, username, delta, method, "carryover", ""); err != nil {
			return BalanceUpdateResult{}, 0, err
		}
	}

	return BalanceUpdateResult{
		PrevStatus:           prevStatus,
		PrevEffectiveBalance: prevUsableBalance,
		EffectiveBalance:     usableBalance,
		User: User{
			Username:         username,
			Balance:          balance,
			CarryoverBalance: targetCarryover,
			Status:           newStatus,
			BlockedAt:        newBlockedAt,
		},
	}, delta, nil
}

func (s *Store) SetGeneralAndCarryoverBalanceTx(
	ctx context.Context,
	tx *sql.Tx,
	username string,
	targetGeneral float64,
	targetCarryover float64,
	generalMethod string,
	carryoverMethod string,
	now time.Time,
	cfg Config,
) (BalanceUpdateResult, float64, float64, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return BalanceUpdateResult{}, 0, 0, errors.New("username 不能为空")
	}
	generalMethod = strings.TrimSpace(generalMethod)
	carryoverMethod = strings.TrimSpace(carryoverMethod)
	if generalMethod == "" || carryoverMethod == "" {
		return BalanceUpdateResult{}, 0, 0, errors.New("method 不能为空")
	}
	if targetCarryover < 0 {
		return BalanceUpdateResult{}, 0, 0, errors.New("target_carryover 不能为负数")
	}

	_, err := s.EnsureUserTx(ctx, tx, username, cfg.DefaultBalance)
	if err != nil {
		return BalanceUpdateResult{}, 0, 0, err
	}

	var balance float64
	var carryoverBalance float64
	var prevStatus string
	var blockedAt *time.Time
	if err := tx.QueryRowContext(ctx, `
SELECT balance, carryover_balance, status, blocked_at
FROM users
WHERE username=$1
FOR UPDATE`, username).Scan(&balance, &carryoverBalance, &prevStatus, &blockedAt); err != nil {
		return BalanceUpdateResult{}, 0, 0, err
	}

	generalDelta := targetGeneral - balance
	carryoverDelta := targetCarryover - carryoverBalance
	usableBalance := targetGeneral + targetCarryover
	prevUsableBalance := balance + carryoverBalance
	newStatus := StatusForBalance(usableBalance, cfg.WarningThreshold, cfg.LimitedThreshold)
	var newBlockedAt *time.Time
	if newStatus == "blocked" {
		newBlockedAt = blockedAt
		if newBlockedAt == nil {
			newBlockedAt = &now
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE users
SET balance=$2, carryover_balance=$3, status=$4, blocked_at=$5, last_charge_time=NOW()
WHERE username=$1`, username, targetGeneral, targetCarryover, newStatus, newBlockedAt); err != nil {
		return BalanceUpdateResult{}, 0, 0, err
	}
	if generalDelta != 0 {
		if err := s.insertRechargeRecordTx(ctx, tx, username, generalDelta, generalMethod, "general", ""); err != nil {
			return BalanceUpdateResult{}, 0, 0, err
		}
	}
	if carryoverDelta != 0 {
		if err := s.insertRechargeRecordTx(ctx, tx, username, carryoverDelta, carryoverMethod, "carryover", ""); err != nil {
			return BalanceUpdateResult{}, 0, 0, err
		}
	}

	return BalanceUpdateResult{
		PrevStatus:           prevStatus,
		PrevEffectiveBalance: prevUsableBalance,
		EffectiveBalance:     usableBalance,
		User: User{
			Username:         username,
			Balance:          targetGeneral,
			CarryoverBalance: targetCarryover,
			Status:           newStatus,
			BlockedAt:        newBlockedAt,
		},
	}, generalDelta, carryoverDelta, nil
}

func (s *Store) SetNodeExclusiveBalanceTx(
	ctx context.Context,
	tx *sql.Tx,
	username string,
	nodeID string,
	targetBalance float64,
	method string,
	updatedBy string,
	cfg Config,
) (NodeExclusivePointsBalance, float64, error) {
	username = strings.TrimSpace(username)
	nodeID = strings.TrimSpace(nodeID)
	method = strings.TrimSpace(method)
	updatedBy = strings.TrimSpace(updatedBy)
	if username == "" {
		return NodeExclusivePointsBalance{}, 0, errors.New("username 不能为空")
	}
	if nodeID == "" {
		return NodeExclusivePointsBalance{}, 0, errors.New("node_id 不能为空")
	}
	if method == "" {
		return NodeExclusivePointsBalance{}, 0, errors.New("method 不能为空")
	}
	if targetBalance < 0 {
		return NodeExclusivePointsBalance{}, 0, errors.New("target_balance 不能为负数")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}

	if _, err := s.EnsureUserTx(ctx, tx, username, cfg.DefaultBalance); err != nil {
		return NodeExclusivePointsBalance{}, 0, err
	}
	if err := s.ensureNodeExclusivePointsRowTx(ctx, tx, username, nodeID); err != nil {
		return NodeExclusivePointsBalance{}, 0, err
	}

	var current float64
	if err := tx.QueryRowContext(ctx, `
SELECT balance
FROM user_node_exclusive_points
WHERE username=$1 AND node_id=$2
FOR UPDATE`, username, nodeID).Scan(&current); err != nil {
		return NodeExclusivePointsBalance{}, 0, err
	}
	delta := targetBalance - current

	var out NodeExclusivePointsBalance
	if delta == 0 {
		if err := tx.QueryRowContext(ctx, `
SELECT username, node_id, balance, updated_by, created_at, updated_at
FROM user_node_exclusive_points
WHERE username=$1 AND node_id=$2`,
			username, nodeID,
		).Scan(&out.Username, &out.NodeID, &out.Balance, &out.UpdatedBy, &out.CreatedAt, &out.UpdatedAt); err != nil {
			return NodeExclusivePointsBalance{}, 0, err
		}
		return out, 0, nil
	}

	if err := tx.QueryRowContext(ctx, `
UPDATE user_node_exclusive_points
SET balance=$3, updated_by=$4, updated_at=NOW()
WHERE username=$1 AND node_id=$2
RETURNING username, node_id, balance, updated_by, created_at, updated_at`,
		username, nodeID, targetBalance, updatedBy,
	).Scan(&out.Username, &out.NodeID, &out.Balance, &out.UpdatedBy, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return NodeExclusivePointsBalance{}, 0, err
	}
	if err := s.insertRechargeRecordTx(ctx, tx, username, delta, method, "node_exclusive", nodeID); err != nil {
		return NodeExclusivePointsBalance{}, 0, err
	}
	return out, delta, nil
}

func (s *Store) AdjustNodeExclusiveBalanceTx(
	ctx context.Context,
	tx *sql.Tx,
	username string,
	nodeID string,
	delta float64,
	method string,
	updatedBy string,
	cfg Config,
) (NodeExclusivePointsBalance, error) {
	username = strings.TrimSpace(username)
	nodeID = strings.TrimSpace(nodeID)
	method = strings.TrimSpace(method)
	updatedBy = strings.TrimSpace(updatedBy)
	if username == "" {
		return NodeExclusivePointsBalance{}, errors.New("username 不能为空")
	}
	if nodeID == "" {
		return NodeExclusivePointsBalance{}, errors.New("node_id 不能为空")
	}
	if method == "" {
		return NodeExclusivePointsBalance{}, errors.New("method 不能为空")
	}
	if delta == 0 {
		return NodeExclusivePointsBalance{}, errors.New("delta 不能为 0")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}

	if _, err := s.EnsureUserTx(ctx, tx, username, cfg.DefaultBalance); err != nil {
		return NodeExclusivePointsBalance{}, err
	}
	if err := s.ensureNodeExclusivePointsRowTx(ctx, tx, username, nodeID); err != nil {
		return NodeExclusivePointsBalance{}, err
	}

	var current float64
	if err := tx.QueryRowContext(ctx, `
SELECT balance
FROM user_node_exclusive_points
WHERE username=$1 AND node_id=$2
FOR UPDATE`, username, nodeID).Scan(&current); err != nil {
		return NodeExclusivePointsBalance{}, err
	}
	next := current + delta
	if next < 0 {
		return NodeExclusivePointsBalance{}, errors.New("专属积分不足，无法扣减")
	}
	var out NodeExclusivePointsBalance
	if err := tx.QueryRowContext(ctx, `
UPDATE user_node_exclusive_points
SET balance=$3, updated_by=$4, updated_at=NOW()
WHERE username=$1 AND node_id=$2
RETURNING username, node_id, balance, updated_by, created_at, updated_at`,
		username, nodeID, next, updatedBy,
	).Scan(&out.Username, &out.NodeID, &out.Balance, &out.UpdatedBy, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return NodeExclusivePointsBalance{}, err
	}
	if err := s.insertRechargeRecordTx(ctx, tx, username, delta, method, "node_exclusive", nodeID); err != nil {
		return NodeExclusivePointsBalance{}, err
	}
	return out, nil
}

func (s *Store) InsertUsageRecordTx(ctx context.Context, tx *sql.Tx, nodeID string, localUsername string, ts time.Time, proc UserProcess, cost float64) error {
	gpuUsage := proc.GPUUsage
	if gpuUsage == nil {
		// 保持 JSONB 非空且语义一致：CPU-only 记录也用空数组而非 null
		gpuUsage = []GPUUsage{}
	}
	gpuJSON, err := json.Marshal(gpuUsage)
	if err != nil {
		return err
	}
	localUsername = strings.TrimSpace(localUsername)
	if localUsername == "" {
		localUsername = strings.TrimSpace(proc.Username)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO usage_records(node_id, local_username, username, timestamp, pid, cpu_percent, memory_mb, gpu_count, command, gpu_usage, cost)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		nodeID, localUsername, proc.Username, ts, proc.PID, proc.CPUPercent, proc.MemoryMB, len(proc.GPUUsage), strings.TrimSpace(proc.Command), string(gpuJSON), cost)
	return err
}

func (s *Store) InsertProcessKillRecord(
	ctx context.Context,
	nodeID string,
	localUsername string,
	billingUsername string,
	actionType string,
	reason string,
	pids []int32,
) error {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	billingUsername = strings.TrimSpace(billingUsername)
	actionType = strings.TrimSpace(actionType)
	reason = strings.TrimSpace(reason)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if actionType == "" {
		return errors.New("action_type 不能为空")
	}
	if pids == nil {
		pids = []int32{}
	}
	pidsJSON, err := json.Marshal(pids)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO process_kill_records(node_id, local_username, billing_username, action_type, reason, pids_json)
VALUES($1,$2,$3,$4,$5,$6)`,
		nodeID, localUsername, billingUsername, actionType, reason, string(pidsJSON))
	return err
}

func (s *Store) LoadPricesTx(ctx context.Context, tx *sql.Tx) ([]PriceRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT gpu_model, price_per_minute FROM resource_prices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PriceRow
	for rows.Next() {
		var model string
		var price float64
		if err := rows.Scan(&model, &price); err != nil {
			return nil, err
		}
		out = append(out, PriceRow{Model: model, Price: price})
	}
	return out, rows.Err()
}

func (s *Store) UpsertPrice(ctx context.Context, model string, price float64) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("gpu_model 不能为空")
	}
	if price < 0 {
		return errors.New("price_per_minute 不能为负数")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO resource_prices(gpu_model, price_per_minute)
VALUES($1,$2)
ON CONFLICT (gpu_model) DO UPDATE
SET price_per_minute=EXCLUDED.price_per_minute, updated_at=NOW()`, model, price)
	return err
}

func (s *Store) ListPrices(ctx context.Context) ([]PriceRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT gpu_model, price_per_minute FROM resource_prices ORDER BY gpu_model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PriceRow
	for rows.Next() {
		var r PriceRow
		if err := rows.Scan(&r.Model, &r.Price); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListUsers(ctx context.Context, limit int) ([]User, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT username, balance, carryover_balance, status, blocked_at
FROM users
ORDER BY username
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.Username, &u.Balance, &u.CarryoverBalance, &u.Status, &u.BlockedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) ListUsageByUser(ctx context.Context, username string, limit int) ([]UsageRecord, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username 不能为空")
	}
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT ur.node_id,
       CASE
         WHEN ur.local_username = ur.username
              AND EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username = ur.username)
              AND NOT EXISTS(SELECT 1 FROM user_node_accounts una2 WHERE una2.node_id=ur.node_id AND una2.local_username=ur.local_username)
         THEN ''
         ELSE ur.local_username
       END AS local_username,
       ur.username,
       (
         EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username = ur.username)
         OR EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username = ur.username)
         OR EXISTS(SELECT 1 FROM power_users pu WHERE pu.username = ur.username)
       ) AS registered,
       ur.timestamp, ur.pid, ur.cpu_percent, ur.memory_mb, ur.gpu_count, ur.command, ur.gpu_usage, ur.cost
FROM usage_records ur
WHERE ur.username=$1
ORDER BY timestamp DESC
LIMIT $2`, username, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UsageRecord
	for rows.Next() {
		var r UsageRecord
		if err := rows.Scan(&r.NodeID, &r.LocalUser, &r.BillingUser, &r.Registered, &r.Timestamp, &r.PID, &r.CPUPercent, &r.MemoryMB, &r.GPUCount, &r.Command, &r.GPUUsage, &r.Cost); err != nil {
			return nil, err
		}
		r.Username = r.BillingUser
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListUsageAdmin(ctx context.Context, billingUsername string, localUsername string, unregisteredOnly bool, limit int) ([]UsageRecord, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	localUsername = strings.TrimSpace(localUsername)
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	conds := make([]string, 0, 3)
	args := make([]any, 0, 6)
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if billingUsername != "" {
		conds = append(conds, "ur.username="+addArg(billingUsername))
	}
	if localUsername != "" {
		conds = append(conds, "ur.local_username="+addArg(localUsername))
	}
	if unregisteredOnly {
		conds = append(conds, `
NOT (
  EXISTS(SELECT 1 FROM user_accounts ua2 WHERE ua2.username = ur.username)
  OR EXISTS(SELECT 1 FROM admin_accounts aa2 WHERE aa2.username = ur.username)
  OR EXISTS(SELECT 1 FROM power_users pu2 WHERE pu2.username = ur.username)
)`)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	query := `
SELECT ur.node_id,
       CASE
         WHEN ur.local_username = ur.username
              AND EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username = ur.username)
              AND NOT EXISTS(SELECT 1 FROM user_node_accounts una2 WHERE una2.node_id=ur.node_id AND una2.local_username=ur.local_username)
         THEN ''
         ELSE ur.local_username
       END AS local_username,
       ur.username,
       (
         EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username = ur.username)
         OR EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username = ur.username)
         OR EXISTS(SELECT 1 FROM power_users pu WHERE pu.username = ur.username)
       ) AS registered,
       ur.timestamp, ur.pid, ur.cpu_percent, ur.memory_mb, ur.gpu_count, ur.command, ur.gpu_usage, ur.cost
FROM usage_records ur
` + where + `
ORDER BY ur.timestamp DESC
LIMIT ` + addArg(limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UsageRecord
	for rows.Next() {
		var r UsageRecord
		if err := rows.Scan(&r.NodeID, &r.LocalUser, &r.BillingUser, &r.Registered, &r.Timestamp, &r.PID, &r.CPUPercent, &r.MemoryMB, &r.GPUCount, &r.Command, &r.GPUUsage, &r.Cost); err != nil {
			return nil, err
		}
		r.Username = r.BillingUser
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListProcessKillRecordsAdmin(
	ctx context.Context,
	billingUsername string,
	localUsername string,
	unregisteredOnly bool,
	limit int,
) ([]ProcessKillRecord, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	localUsername = strings.TrimSpace(localUsername)
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	conds := make([]string, 0, 3)
	args := make([]any, 0, 6)
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if billingUsername != "" {
		conds = append(conds, "pkr.billing_username="+addArg(billingUsername))
	}
	if localUsername != "" {
		conds = append(conds, "pkr.local_username="+addArg(localUsername))
	}
	if unregisteredOnly {
		conds = append(conds, `
NOT (
  pkr.billing_username <> ''
  AND (
    EXISTS(SELECT 1 FROM user_accounts ua2 WHERE ua2.username = pkr.billing_username)
    OR EXISTS(SELECT 1 FROM admin_accounts aa2 WHERE aa2.username = pkr.billing_username)
    OR EXISTS(SELECT 1 FROM power_users pu2 WHERE pu2.username = pkr.billing_username)
  )
)`)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	query := `
SELECT pkr.record_id,
       pkr.node_id,
       pkr.local_username,
       pkr.billing_username,
       (
         pkr.billing_username <> ''
         AND (
           EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username = pkr.billing_username)
           OR EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username = pkr.billing_username)
           OR EXISTS(SELECT 1 FROM power_users pu WHERE pu.username = pkr.billing_username)
         )
       ) AS registered,
       pkr.action_type,
       pkr.reason,
       pkr.pids_json::text,
       pkr.created_at
FROM process_kill_records pkr
` + where + `
ORDER BY pkr.created_at DESC
LIMIT ` + addArg(limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProcessKillRecord
	for rows.Next() {
		var r ProcessKillRecord
		var pidsJSON string
		if err := rows.Scan(
			&r.RecordID,
			&r.NodeID,
			&r.LocalUsername,
			&r.BillingUsername,
			&r.Registered,
			&r.ActionType,
			&r.Reason,
			&pidsJSON,
			&r.Timestamp,
		); err != nil {
			return nil, err
		}
		r.Timestamp = asBeijingWallTime(r.Timestamp)
		if strings.TrimSpace(pidsJSON) == "" {
			r.PIDs = []int32{}
		} else if err := json.Unmarshal([]byte(pidsJSON), &r.PIDs); err != nil {
			r.PIDs = []int32{}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpsertUserNodeAccountTx(ctx context.Context, tx *sql.Tx, nodeID string, localUsername string, billingUsername string) error {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	billingUsername = strings.TrimSpace(billingUsername)
	if nodeID == "" || localUsername == "" || billingUsername == "" {
		return errors.New("node_id/local_username/billing_username 不能为空")
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO user_node_accounts(node_id, local_username, billing_username)
VALUES($1,$2,$3)
ON CONFLICT (node_id, local_username) DO UPDATE
SET billing_username=EXCLUDED.billing_username,
    updated_at=NOW()`, nodeID, localUsername, billingUsername)
	return err
}

func (s *Store) getUserNodeAccountBillingTx(ctx context.Context, tx *sql.Tx, nodeID string, localUsername string) (string, bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return "", false, errors.New("node_id/local_username 不能为空")
	}
	var billing string
	err := tx.QueryRowContext(ctx, `
SELECT billing_username
FROM user_node_accounts
WHERE node_id=$1 AND local_username=$2
FOR UPDATE`, nodeID, localUsername).Scan(&billing)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(billing), true, nil
}

type NodeAccountOwnershipConflictError struct {
	NodeID          string
	LocalUsername   string
	ExistingBilling string
	RequestedBy     string
}

func (e *NodeAccountOwnershipConflictError) Error() string {
	nodeID := strings.TrimSpace(e.NodeID)
	localUsername := strings.TrimSpace(e.LocalUsername)
	existing := strings.TrimSpace(e.ExistingBilling)
	if existing == "" {
		existing = "未知账号"
	}
	return fmt.Sprintf(
		"节点 %s 的账号 %s 已绑定到平台账号 %s，禁止冒充绑定；如需调整请联系管理员处理",
		nodeID, localUsername, existing,
	)
}

type NodeBindCooldownError struct {
	BillingUsername string
	CooldownUntil   time.Time
}

func (e *NodeBindCooldownError) Error() string {
	return fmt.Sprintf("平台账号 %s 处于绑定冷却期，结束时间 %s", strings.TrimSpace(e.BillingUsername), formatDisplayTimeInBeijing(e.CooldownUntil))
}

type NodeBindActiveChallengeError struct {
	BillingUsername string
	Challenge       UserNodeBindChallenge
}

func (e *NodeBindActiveChallengeError) Error() string {
	billing := strings.TrimSpace(e.BillingUsername)
	if billing == "" {
		billing = strings.TrimSpace(e.Challenge.BillingUsername)
	}
	return fmt.Sprintf(
		"平台账号 %s 已有进行中的绑定 challenge（%s/%s），到期时间 %s",
		billing,
		strings.TrimSpace(e.Challenge.NodeID),
		strings.TrimSpace(e.Challenge.LocalUsername),
		formatDisplayTimeInBeijing(e.Challenge.ExpiresAt),
	)
}

type NodeBindTargetFrozenError struct {
	NodeID        string
	LocalUsername string
	FreezeUntil   time.Time
}

func (e *NodeBindTargetFrozenError) Error() string {
	return fmt.Sprintf("节点 %s 账号 %s 处于争抢冻结期，结束时间 %s", strings.TrimSpace(e.NodeID), strings.TrimSpace(e.LocalUsername), formatDisplayTimeInBeijing(e.FreezeUntil))
}

type NodeBindTargetBusyError struct {
	NodeID        string
	LocalUsername string
	Challenge     UserNodeBindChallenge
}

func (e *NodeBindTargetBusyError) Error() string {
	return fmt.Sprintf("节点 %s 账号 %s 正在被 challenge 占用，结束时间 %s", strings.TrimSpace(e.NodeID), strings.TrimSpace(e.LocalUsername), formatDisplayTimeInBeijing(e.Challenge.ExpiresAt))
}

func isUserMappingSource(source string) bool {
	return strings.EqualFold(strings.TrimSpace(source), "user")
}

func normalizeMappingAuditText(v string) string {
	return strings.TrimSpace(v)
}

func (s *Store) insertUserNodeAccountAuditTx(
	ctx context.Context,
	tx *sql.Tx,
	nodeID string,
	localUsername string,
	oldBilling string,
	newBilling string,
	action string,
	operator string,
	source string,
	reason string,
) error {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	oldBilling = normalizeMappingAuditText(oldBilling)
	newBilling = normalizeMappingAuditText(newBilling)
	action = normalizeMappingAuditText(action)
	operator = normalizeMappingAuditText(operator)
	source = normalizeMappingAuditText(source)
	reason = normalizeMappingAuditText(reason)
	if nodeID == "" || localUsername == "" {
		return errors.New("node_id/local_username 不能为空")
	}
	if action == "" {
		action = "mapping_update"
	}
	if operator == "" {
		operator = "system"
	}
	if source == "" {
		source = "system"
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO user_node_account_audits(
  node_id, local_username, old_billing_username, new_billing_username, action, operator, source, reason, created_at
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,NOW())`,
		nodeID, localUsername, oldBilling, newBilling, action, operator, source, reason,
	)
	return err
}

func (s *Store) UpsertUserNodeAccountWithAudit(
	ctx context.Context,
	nodeID string,
	localUsername string,
	billingUsername string,
	operator string,
	source string,
	reason string,
) error {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	billingUsername = strings.TrimSpace(billingUsername)
	if nodeID == "" || localUsername == "" || billingUsername == "" {
		return errors.New("node_id/local_username/billing_username 不能为空")
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		oldBilling, existed, err := s.getUserNodeAccountBillingTx(ctx, tx, nodeID, localUsername)
		if err != nil {
			return err
		}
		if existed && oldBilling != billingUsername {
			return &NodeAccountOwnershipConflictError{
				NodeID:          nodeID,
				LocalUsername:   localUsername,
				ExistingBilling: oldBilling,
				RequestedBy:     billingUsername,
			}
		}
		if err := s.UpsertUserNodeAccountTx(ctx, tx, nodeID, localUsername, billingUsername); err != nil {
			return err
		}
		if existed && oldBilling == billingUsername {
			return nil
		}
		action := "mapping_create"
		if existed {
			action = "mapping_rebind"
		}
		return s.insertUserNodeAccountAuditTx(ctx, tx, nodeID, localUsername, oldBilling, billingUsername, action, operator, source, reason)
	})
}

func (s *Store) ListUserNodeAccountsByBilling(ctx context.Context, billingUsername string, limit int) ([]UserNodeAccount, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return nil, errors.New("billing_username 不能为空")
	}
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT una.node_id,
       una.local_username,
       una.billing_username,
       nlu.last_login_at,
       nlu.updated_at,
       una.created_at,
       una.updated_at
FROM user_node_accounts una
LEFT JOIN node_local_users nlu
  ON nlu.node_id=una.node_id
 AND nlu.local_username=una.local_username
WHERE una.billing_username=$1
ORDER BY una.node_id, una.local_username
LIMIT $2`, billingUsername, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserNodeAccount, 0)
	for rows.Next() {
		var v UserNodeAccount
		var lastLoginAt sql.NullTime
		var nodeLocalUpdatedAt sql.NullTime
		if err := rows.Scan(&v.NodeID, &v.LocalUsername, &v.BillingUsername, &lastLoginAt, &nodeLocalUpdatedAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		if lastLoginAt.Valid {
			t := asBeijingWallTime(lastLoginAt.Time)
			v.NodeLastLoginAt = &t
		}
		if nodeLocalUpdatedAt.Valid {
			t := asBeijingWallTime(nodeLocalUpdatedAt.Time)
			v.NodeLocalUpdatedAt = &t
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ListUserNodeAccountsWithFilter(
	ctx context.Context,
	billingUsername string,
	nodeID string,
	localUsername string,
	limit int,
) ([]UserNodeAccount, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}
	conds := make([]string, 0, 3)
	args := make([]any, 0, 4)
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if billingUsername != "" {
		conds = append(conds, "una.billing_username="+addArg(billingUsername))
	}
	if nodeID != "" {
		conds = append(conds, "una.node_id="+addArg(nodeID))
	}
	if localUsername != "" {
		conds = append(conds, "una.local_username="+addArg(localUsername))
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	query := `
SELECT una.node_id,
       una.local_username,
       una.billing_username,
       nlu.last_login_at,
       nlu.updated_at,
       una.created_at,
       una.updated_at
FROM user_node_accounts una
LEFT JOIN node_local_users nlu
  ON nlu.node_id=una.node_id
 AND nlu.local_username=una.local_username
` + where + `
ORDER BY una.billing_username, una.node_id, una.local_username
LIMIT ` + addArg(limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserNodeAccount, 0)
	for rows.Next() {
		var v UserNodeAccount
		var lastLoginAt sql.NullTime
		var nodeLocalUpdatedAt sql.NullTime
		if err := rows.Scan(&v.NodeID, &v.LocalUsername, &v.BillingUsername, &lastLoginAt, &nodeLocalUpdatedAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		if lastLoginAt.Valid {
			t := asBeijingWallTime(lastLoginAt.Time)
			v.NodeLastLoginAt = &t
		}
		if nodeLocalUpdatedAt.Valid {
			t := asBeijingWallTime(nodeLocalUpdatedAt.Time)
			v.NodeLocalUpdatedAt = &t
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ListUserNodeAccounts(ctx context.Context, billingUsername string, limit int) ([]UserNodeAccount, error) {
	return s.ListUserNodeAccountsWithFilter(ctx, billingUsername, "", "", limit)
}

func (s *Store) ListUserNodeAccountMappingRisks(ctx context.Context, days int, minSwitches int, limit int) ([]UserNodeAccountMappingRisk, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	if minSwitches < 1 {
		minSwitches = 2
	}
	if limit <= 0 || limit > 5000 {
		limit = 300
	}
	from := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	rows, err := s.db.QueryContext(ctx, `
WITH rebinding AS (
  SELECT
    node_id,
    local_username,
    COUNT(*) FILTER (
      WHERE action = 'mapping_rebind'
        AND COALESCE(NULLIF(TRIM(old_billing_username), ''), '') <> ''
        AND COALESCE(NULLIF(TRIM(new_billing_username), ''), '') <> ''
        AND TRIM(old_billing_username) <> TRIM(new_billing_username)
    ) AS switch_count,
    MAX(created_at) AS last_rebind_at
  FROM user_node_account_audits
  WHERE created_at >= $1
  GROUP BY node_id, local_username
),
switches AS (
  SELECT
    node_id,
    local_username,
    ARRAY_AGG(
      TRIM(old_billing_username) || ' → ' || TRIM(new_billing_username)
      ORDER BY created_at ASC
    ) FILTER (
      WHERE action = 'mapping_rebind'
        AND COALESCE(NULLIF(TRIM(old_billing_username), ''), '') <> ''
        AND COALESCE(NULLIF(TRIM(new_billing_username), ''), '') <> ''
        AND TRIM(old_billing_username) <> TRIM(new_billing_username)
    ) AS switch_history
  FROM user_node_account_audits
  WHERE created_at >= $1
  GROUP BY node_id, local_username
),
events AS (
  SELECT
    node_id,
    local_username,
    NULLIF(TRIM(old_billing_username), '') AS billing_username,
    created_at
  FROM user_node_account_audits
  WHERE created_at >= $1
  UNION ALL
  SELECT
    node_id,
    local_username,
    NULLIF(TRIM(new_billing_username), '') AS billing_username,
    created_at
  FROM user_node_account_audits
  WHERE created_at >= $1
  UNION ALL
  SELECT
    node_id,
    local_username,
    NULLIF(TRIM(billing_username), '') AS billing_username,
    created_at
  FROM account_provision_logs
  WHERE created_at >= $1
  UNION ALL
  SELECT
    node_id,
    local_username,
    NULLIF(TRIM(billing_username), '') AS billing_username,
    updated_at AS created_at
  FROM user_node_accounts
),
agg AS (
  SELECT
    node_id,
    local_username,
    COUNT(DISTINCT billing_username) AS distinct_billing_count,
    ARRAY_AGG(DISTINCT billing_username) FILTER (WHERE billing_username IS NOT NULL) AS platform_usernames,
    MAX(created_at) AS last_changed_at
  FROM events
  WHERE billing_username IS NOT NULL
  GROUP BY node_id, local_username
),
risk_keys AS (
  SELECT node_id, local_username FROM rebinding
  UNION
  SELECT node_id, local_username FROM agg
),
joined AS (
  SELECT
    rk.node_id,
    rk.local_username,
    COALESCE(r.switch_count, 0) AS switch_count,
    COALESCE(a.distinct_billing_count, 0) AS distinct_billing_count,
    COALESCE(a.platform_usernames, ARRAY[]::TEXT[]) AS platform_usernames,
    COALESCE(sw.switch_history, ARRAY[]::TEXT[]) AS switch_history,
    COALESCE(
      GREATEST(a.last_changed_at, r.last_rebind_at),
      a.last_changed_at,
      r.last_rebind_at
    ) AS last_changed_at
  FROM risk_keys rk
  LEFT JOIN rebinding r
    ON r.node_id = rk.node_id AND r.local_username = rk.local_username
  LEFT JOIN switches sw
    ON sw.node_id = rk.node_id AND sw.local_username = rk.local_username
  LEFT JOIN agg a
    ON a.node_id = rk.node_id AND a.local_username = rk.local_username
)
SELECT
  j.node_id,
  j.local_username,
  COALESCE(una.billing_username, '') AS current_billing_username,
  j.switch_count,
  j.distinct_billing_count,
  j.platform_usernames,
  j.switch_history,
  j.last_changed_at
FROM joined j
LEFT JOIN user_node_accounts una
  ON una.node_id = j.node_id AND una.local_username = j.local_username
LEFT JOIN user_node_account_risk_clears rc
  ON rc.node_id = j.node_id AND rc.local_username = j.local_username
WHERE (j.switch_count >= $2 OR j.distinct_billing_count >= 2)
  AND (rc.cleared_at IS NULL OR j.last_changed_at IS NULL OR j.last_changed_at > rc.cleared_at)
ORDER BY j.switch_count DESC, j.distinct_billing_count DESC, j.last_changed_at DESC
LIMIT $3`, from, minSwitches, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserNodeAccountMappingRisk, 0, limit)
	for rows.Next() {
		var x UserNodeAccountMappingRisk
		var users []string
		var switchHistory []string
		if err := rows.Scan(
			&x.NodeID,
			&x.LocalUsername,
			&x.CurrentBilling,
			&x.SwitchCount,
			&x.DistinctBillingCount,
			pq.Array(&users),
			pq.Array(&switchHistory),
			&x.LastChangedAt,
		); err != nil {
			return nil, err
		}
		x.PlatformUsernames = uniqTrim(users)
		seenSwitch := map[string]struct{}{}
		x.SwitchHistory = make([]string, 0, len(switchHistory))
		for _, raw := range switchHistory {
			v := strings.TrimSpace(raw)
			if v == "" {
				continue
			}
			if _, ok := seenSwitch[v]; ok {
				continue
			}
			seenSwitch[v] = struct{}{}
			x.SwitchHistory = append(x.SwitchHistory, v)
		}
		reasons := make([]string, 0, 2)
		if x.SwitchCount >= minSwitches {
			reasons = append(reasons, fmt.Sprintf("%d 天内换绑 %d 次", days, x.SwitchCount))
		}
		if x.DistinctBillingCount >= 2 {
			reasons = append(reasons, fmt.Sprintf("涉及 %d 个平台账号", x.DistinctBillingCount))
		}
		x.RiskReason = strings.Join(reasons, "；")
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ClearUserNodeAccountMappingRisk(ctx context.Context, nodeID string, localUsername string, clearedBy string) error {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	clearedBy = strings.TrimSpace(clearedBy)
	if nodeID == "" || localUsername == "" {
		return errors.New("node_id/local_username 不能为空")
	}
	if clearedBy == "" {
		clearedBy = "admin"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO user_node_account_risk_clears(node_id, local_username, cleared_by, cleared_at)
VALUES($1,$2,$3,NOW())
ON CONFLICT (node_id, local_username) DO UPDATE SET
  cleared_by=EXCLUDED.cleared_by,
  cleared_at=NOW()`, nodeID, localUsername, clearedBy)
	return err
}

func (s *Store) InsertAccountProvisionLog(
	ctx context.Context,
	billingUsername string,
	nodeID string,
	localUsername string,
	email string,
	sshHost string,
	sshPort int,
	downloadFilename string,
	mailSent bool,
	mailError string,
	createdBy string,
) error {
	billingUsername = strings.TrimSpace(billingUsername)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	email = strings.TrimSpace(email)
	sshHost = strings.TrimSpace(sshHost)
	downloadFilename = strings.TrimSpace(downloadFilename)
	mailError = strings.TrimSpace(mailError)
	createdBy = strings.TrimSpace(createdBy)
	if createdBy == "" {
		createdBy = "admin"
	}
	if billingUsername == "" || nodeID == "" || localUsername == "" || email == "" {
		return errors.New("billing_username/node_id/local_username/email 不能为空")
	}
	if sshPort <= 0 || sshPort > 65535 {
		sshPort = 22
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO account_provision_logs(
  billing_username, node_id, local_username, email, ssh_host, ssh_port, download_filename, mail_sent, mail_error, created_by, created_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW())`,
		billingUsername, nodeID, localUsername, email, sshHost, sshPort, downloadFilename, mailSent, mailError, createdBy)
	return err
}

func (s *Store) ListAccountProvisionLogs(ctx context.Context, billingUsername string, nodeID string, localUsername string, limit int) ([]AccountProvisionLog, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if limit <= 0 || limit > 5000 {
		limit = 200
	}

	conds := make([]string, 0, 3)
	args := make([]any, 0, 4)
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if billingUsername != "" {
		conds = append(conds, "billing_username="+addArg(billingUsername))
	}
	if nodeID != "" {
		conds = append(conds, "node_id="+addArg(nodeID))
	}
	if localUsername != "" {
		conds = append(conds, "local_username="+addArg(localUsername))
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	query := `
SELECT provision_id, billing_username, node_id, local_username, email, ssh_host, ssh_port, download_filename, mail_sent, mail_error, created_by, created_at
FROM account_provision_logs
` + where + `
ORDER BY created_at DESC
LIMIT ` + addArg(limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AccountProvisionLog, 0)
	for rows.Next() {
		var v AccountProvisionLog
		if err := rows.Scan(
			&v.ProvisionID,
			&v.BillingUsername,
			&v.NodeID,
			&v.LocalUsername,
			&v.Email,
			&v.SSHHost,
			&v.SSHPort,
			&v.DownloadFilename,
			&v.MailSent,
			&v.MailError,
			&v.CreatedBy,
			&v.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) InsertUserProvisionMessage(
	ctx context.Context,
	billingUsername string,
	nodeID string,
	localUsername string,
	encryptedPayload string,
	decryptURL string,
	sshHost string,
	sshPort int,
	downloadFilename string,
	sshCommand string,
	mailTo string,
	createdBy string,
) error {
	billingUsername = strings.TrimSpace(billingUsername)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	encryptedPayload = strings.TrimSpace(encryptedPayload)
	decryptURL = strings.TrimSpace(decryptURL)
	sshHost = strings.TrimSpace(sshHost)
	downloadFilename = strings.TrimSpace(downloadFilename)
	sshCommand = strings.TrimSpace(sshCommand)
	mailTo = strings.TrimSpace(mailTo)
	createdBy = strings.TrimSpace(createdBy)
	if createdBy == "" {
		createdBy = "admin"
	}
	if billingUsername == "" || nodeID == "" || localUsername == "" || encryptedPayload == "" {
		return errors.New("billing_username/node_id/local_username/encrypted_payload 不能为空")
	}
	if decryptURL == "" {
		decryptURL = "/user/accounts?tool=key-decryptor"
	}
	if sshPort <= 0 || sshPort > 65535 {
		sshPort = 22
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO user_provision_messages(
  billing_username, node_id, local_username, encrypted_payload, decrypt_url,
  ssh_host, ssh_port, download_filename, ssh_command, mail_to, created_by, created_at
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW())`,
		billingUsername, nodeID, localUsername, encryptedPayload, decryptURL,
		sshHost, sshPort, downloadFilename, sshCommand, mailTo, createdBy,
	)
	return err
}

func (s *Store) ListUserProvisionMessages(ctx context.Context, billingUsername string, limit int) ([]UserProvisionMessage, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return nil, errors.New("billing_username 不能为空")
	}
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	_ = s.cleanupExpiredProvisionMessages(ctx)
	rows, err := s.db.QueryContext(ctx, `
SELECT message_id, billing_username, node_id, local_username, encrypted_payload, decrypt_url,
       ssh_host, ssh_port, download_filename, ssh_command, mail_to, created_by, created_at,
       first_decrypted_at, destroy_after_at, destroyed_at
FROM user_provision_messages
WHERE billing_username=$1
ORDER BY created_at DESC
LIMIT $2`, billingUsername, limit)
	if err != nil {
		if isColumnMissingErr(err, "first_decrypted_at") || isColumnMissingErr(err, "destroy_after_at") || isColumnMissingErr(err, "destroyed_at") {
			return s.listUserProvisionMessagesLegacy(ctx, billingUsername, limit)
		}
		return nil, err
	}
	defer rows.Close()
	out := make([]UserProvisionMessage, 0)
	for rows.Next() {
		var v UserProvisionMessage
		var firstDecryptedAt sql.NullTime
		var destroyAfterAt sql.NullTime
		var destroyedAt sql.NullTime
		if err := rows.Scan(
			&v.MessageID,
			&v.BillingUsername,
			&v.NodeID,
			&v.LocalUsername,
			&v.EncryptedPayload,
			&v.DecryptURL,
			&v.SSHHost,
			&v.SSHPort,
			&v.DownloadFilename,
			&v.SSHCommand,
			&v.MailTo,
			&v.CreatedBy,
			&v.CreatedAt,
			&firstDecryptedAt,
			&destroyAfterAt,
			&destroyedAt,
		); err != nil {
			return nil, err
		}
		if firstDecryptedAt.Valid {
			t := firstDecryptedAt.Time
			v.FirstDecryptedAt = &t
		}
		if destroyAfterAt.Valid {
			t := destroyAfterAt.Time
			v.DestroyAfterAt = &t
		}
		if destroyedAt.Valid {
			t := destroyedAt.Time
			v.DestroyedAt = &t
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) listUserProvisionMessagesLegacy(ctx context.Context, billingUsername string, limit int) ([]UserProvisionMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT message_id, billing_username, node_id, local_username, encrypted_payload, decrypt_url,
       ssh_host, ssh_port, download_filename, ssh_command, mail_to, created_by, created_at
FROM user_provision_messages
WHERE billing_username=$1
ORDER BY created_at DESC
LIMIT $2`, billingUsername, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserProvisionMessage, 0)
	for rows.Next() {
		var v UserProvisionMessage
		if err := rows.Scan(
			&v.MessageID,
			&v.BillingUsername,
			&v.NodeID,
			&v.LocalUsername,
			&v.EncryptedPayload,
			&v.DecryptURL,
			&v.SSHHost,
			&v.SSHPort,
			&v.DownloadFilename,
			&v.SSHCommand,
			&v.MailTo,
			&v.CreatedBy,
			&v.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) cleanupExpiredProvisionMessages(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE user_provision_messages
SET encrypted_payload='',
    destroyed_at=COALESCE(destroyed_at, NOW())
WHERE destroyed_at IS NULL
  AND destroy_after_at IS NOT NULL
  AND destroy_after_at <= NOW()`)
	if err != nil {
		if isColumnMissingErr(err, "destroy_after_at") || isColumnMissingErr(err, "destroyed_at") {
			return nil
		}
		return err
	}
	return nil
}

func (s *Store) MarkUserProvisionMessageDecryptStarted(ctx context.Context, billingUsername string, messageID int64, now time.Time) (UserProvisionMessage, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return UserProvisionMessage{}, errors.New("billing_username 不能为空")
	}
	if messageID <= 0 {
		return UserProvisionMessage{}, errors.New("message_id 不合法")
	}
	var out UserProvisionMessage
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var firstDecryptedAt sql.NullTime
		var destroyAfterAt sql.NullTime
		var destroyedAt sql.NullTime
		if err := tx.QueryRowContext(ctx, `
SELECT message_id, billing_username, node_id, local_username, encrypted_payload, decrypt_url,
       ssh_host, ssh_port, download_filename, ssh_command, mail_to, created_by, created_at,
       first_decrypted_at, destroy_after_at, destroyed_at
FROM user_provision_messages
WHERE billing_username=$1 AND message_id=$2
FOR UPDATE`, billingUsername, messageID).Scan(
			&out.MessageID,
			&out.BillingUsername,
			&out.NodeID,
			&out.LocalUsername,
			&out.EncryptedPayload,
			&out.DecryptURL,
			&out.SSHHost,
			&out.SSHPort,
			&out.DownloadFilename,
			&out.SSHCommand,
			&out.MailTo,
			&out.CreatedBy,
			&out.CreatedAt,
			&firstDecryptedAt,
			&destroyAfterAt,
			&destroyedAt,
		); err != nil {
			return err
		}
		if firstDecryptedAt.Valid {
			t := firstDecryptedAt.Time
			out.FirstDecryptedAt = &t
		}
		if destroyAfterAt.Valid {
			t := destroyAfterAt.Time
			out.DestroyAfterAt = &t
		}
		if destroyedAt.Valid {
			t := destroyedAt.Time
			out.DestroyedAt = &t
		}
		if out.DestroyedAt == nil && out.DestroyAfterAt != nil && !out.DestroyAfterAt.After(now) {
			if _, err := tx.ExecContext(ctx, `
UPDATE user_provision_messages
SET encrypted_payload='',
    destroyed_at=COALESCE(destroyed_at, $3)
WHERE billing_username=$1 AND message_id=$2`, billingUsername, messageID, now); err != nil {
				return err
			}
			t := now
			out.DestroyedAt = &t
			out.EncryptedPayload = ""
		}
		if out.DestroyedAt != nil {
			return errProvisionMessageDestroyed
		}
		if out.FirstDecryptedAt == nil {
			first := now
			destroyAfter := first.Add(24 * time.Hour)
			if _, err := tx.ExecContext(ctx, `
UPDATE user_provision_messages
SET first_decrypted_at=$3, destroy_after_at=$4
WHERE billing_username=$1 AND message_id=$2`, billingUsername, messageID, first, destroyAfter); err != nil {
				return err
			}
			out.FirstDecryptedAt = &first
			out.DestroyAfterAt = &destroyAfter
		}
		return nil
	})
	if err != nil {
		return UserProvisionMessage{}, err
	}
	return out, nil
}

func (s *Store) MarkUserProvisionMessageDecryptStartedByPayload(ctx context.Context, billingUsername string, encryptedPayload string, now time.Time) (UserProvisionMessage, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	encryptedPayload = strings.TrimSpace(encryptedPayload)
	if billingUsername == "" || encryptedPayload == "" {
		return UserProvisionMessage{}, errors.New("billing_username/encrypted_payload 不能为空")
	}
	var out UserProvisionMessage
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var firstDecryptedAt sql.NullTime
		var destroyAfterAt sql.NullTime
		var destroyedAt sql.NullTime
		if err := tx.QueryRowContext(ctx, `
SELECT message_id, billing_username, node_id, local_username, encrypted_payload, decrypt_url,
       ssh_host, ssh_port, download_filename, ssh_command, mail_to, created_by, created_at,
       first_decrypted_at, destroy_after_at, destroyed_at
FROM user_provision_messages
WHERE billing_username=$1 AND encrypted_payload=$2
ORDER BY message_id DESC
LIMIT 1
FOR UPDATE`, billingUsername, encryptedPayload).Scan(
			&out.MessageID,
			&out.BillingUsername,
			&out.NodeID,
			&out.LocalUsername,
			&out.EncryptedPayload,
			&out.DecryptURL,
			&out.SSHHost,
			&out.SSHPort,
			&out.DownloadFilename,
			&out.SSHCommand,
			&out.MailTo,
			&out.CreatedBy,
			&out.CreatedAt,
			&firstDecryptedAt,
			&destroyAfterAt,
			&destroyedAt,
		); err != nil {
			return err
		}
		if firstDecryptedAt.Valid {
			t := firstDecryptedAt.Time
			out.FirstDecryptedAt = &t
		}
		if destroyAfterAt.Valid {
			t := destroyAfterAt.Time
			out.DestroyAfterAt = &t
		}
		if destroyedAt.Valid {
			t := destroyedAt.Time
			out.DestroyedAt = &t
		}
		if out.DestroyedAt == nil && out.DestroyAfterAt != nil && !out.DestroyAfterAt.After(now) {
			if _, err := tx.ExecContext(ctx, `
UPDATE user_provision_messages
SET encrypted_payload='',
    destroyed_at=COALESCE(destroyed_at, $3)
WHERE billing_username=$1 AND message_id=$2`, billingUsername, out.MessageID, now); err != nil {
				return err
			}
			t := now
			out.DestroyedAt = &t
			out.EncryptedPayload = ""
		}
		if out.DestroyedAt != nil {
			return errProvisionMessageDestroyed
		}
		if out.FirstDecryptedAt == nil {
			first := now
			destroyAfter := first.Add(24 * time.Hour)
			if _, err := tx.ExecContext(ctx, `
UPDATE user_provision_messages
SET first_decrypted_at=$3, destroy_after_at=$4
WHERE billing_username=$1 AND message_id=$2`, billingUsername, out.MessageID, first, destroyAfter); err != nil {
				return err
			}
			out.FirstDecryptedAt = &first
			out.DestroyAfterAt = &destroyAfter
		}
		return nil
	})
	if err != nil {
		return UserProvisionMessage{}, err
	}
	return out, nil
}

func (s *Store) UpsertUserNodeAccount(ctx context.Context, nodeID string, localUsername string, billingUsername string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		return s.UpsertUserNodeAccountTx(ctx, tx, nodeID, localUsername, billingUsername)
	})
}

func (s *Store) DeleteUserNodeAccount(ctx context.Context, nodeID string, localUsername string, billingUsername string) error {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	billingUsername = strings.TrimSpace(billingUsername)
	if nodeID == "" || localUsername == "" || billingUsername == "" {
		return errors.New("node_id/local_username/billing_username 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `
DELETE FROM user_node_accounts
WHERE node_id=$1 AND local_username=$2 AND billing_username=$3`, nodeID, localUsername, billingUsername)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteUserNodeAccountWithAudit(
	ctx context.Context,
	nodeID string,
	localUsername string,
	billingUsername string,
	operator string,
	source string,
	reason string,
) error {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	billingUsername = strings.TrimSpace(billingUsername)
	if nodeID == "" || localUsername == "" || billingUsername == "" {
		return errors.New("node_id/local_username/billing_username 不能为空")
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		oldBilling, existed, err := s.getUserNodeAccountBillingTx(ctx, tx, nodeID, localUsername)
		if err != nil {
			return err
		}
		if !existed || oldBilling != billingUsername {
			return sql.ErrNoRows
		}
		res, err := tx.ExecContext(ctx, `
DELETE FROM user_node_accounts
WHERE node_id=$1 AND local_username=$2 AND billing_username=$3`, nodeID, localUsername, billingUsername)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return sql.ErrNoRows
		}
		return s.insertUserNodeAccountAuditTx(ctx, tx, nodeID, localUsername, oldBilling, "", "mapping_delete", operator, source, reason)
	})
}

func (s *Store) AdminForceUnbindUserNodeAccountWithRecord(
	ctx context.Context,
	nodeID string,
	localUsername string,
	billingUsername string,
	operator string,
	reason string,
) (string, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	billingUsername = strings.TrimSpace(billingUsername)
	operator = strings.TrimSpace(operator)
	reason = strings.TrimSpace(reason)
	if nodeID == "" || localUsername == "" {
		return "", errors.New("node_id/local_username 不能为空")
	}
	if operator == "" {
		operator = "admin"
	}
	if reason == "" {
		reason = fmt.Sprintf("管理员 %s 强制解绑节点账号映射", operator)
	}
	var resolvedBilling string
	if err := s.WithTx(ctx, func(tx *sql.Tx) error {
		oldBilling, existed, err := s.getUserNodeAccountBillingTx(ctx, tx, nodeID, localUsername)
		if err != nil {
			return err
		}
		if !existed {
			return sql.ErrNoRows
		}
		if billingUsername != "" && oldBilling != billingUsername {
			return errors.New("映射校验失败：该节点账号不属于指定平台账号")
		}
		resolvedBilling = oldBilling
		res, err := tx.ExecContext(ctx, `
DELETE FROM user_node_accounts
WHERE node_id=$1 AND local_username=$2 AND billing_username=$3`, nodeID, localUsername, oldBilling)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return sql.ErrNoRows
		}
		if err := s.insertUserNodeAccountAuditTx(
			ctx, tx,
			nodeID, localUsername,
			oldBilling, "",
			"mapping_delete",
			operator, "admin_force_unbind", reason,
		); err != nil {
			return err
		}
		return s.insertAdminForcedUnbindRecordTx(
			ctx, tx,
			oldBilling,
			nodeID,
			localUsername,
			reason,
			operator,
			time.Now(),
		)
	}); err != nil {
		return "", err
	}
	return resolvedBilling, nil
}

func (s *Store) ListUserNodeUnbindRecords(
	ctx context.Context,
	billingUsername string,
	nodeID string,
	localUsername string,
	status string,
	sourceType string,
	limit int,
) ([]UserNodeUnbindRecord, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	status = strings.TrimSpace(status)
	sourceType = strings.TrimSpace(sourceType)
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	conds := make([]string, 0, 5)
	args := make([]any, 0, 8)
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if billingUsername != "" {
		conds = append(conds, "billing_username="+addArg(billingUsername))
	}
	if nodeID != "" {
		conds = append(conds, "node_id="+addArg(nodeID))
	}
	if localUsername != "" {
		conds = append(conds, "local_username="+addArg(localUsername))
	}
	if status != "" {
		conds = append(conds, "status="+addArg(status))
	}
	if sourceType != "" {
		conds = append(conds, "source_type="+addArg(sourceType))
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	query := `
SELECT record_id, source_type, request_id, billing_username, node_id, local_username, status, reason,
       initiated_by, reviewed_by, reviewed_at, executed_at, created_at, updated_at
FROM user_node_unbind_records
` + where + `
ORDER BY created_at DESC, record_id DESC
LIMIT ` + addArg(limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserNodeUnbindRecord, 0, limit)
	for rows.Next() {
		var x UserNodeUnbindRecord
		var requestID sql.NullInt64
		var reviewedBy sql.NullString
		var reviewedAt sql.NullTime
		var executedAt sql.NullTime
		if err := rows.Scan(
			&x.RecordID,
			&x.SourceType,
			&requestID,
			&x.BillingUsername,
			&x.NodeID,
			&x.LocalUsername,
			&x.Status,
			&x.Reason,
			&x.InitiatedBy,
			&reviewedBy,
			&reviewedAt,
			&executedAt,
			&x.CreatedAt,
			&x.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if requestID.Valid {
			v := int(requestID.Int64)
			x.RequestID = &v
		}
		if reviewedBy.Valid {
			v := strings.TrimSpace(reviewedBy.String)
			x.ReviewedBy = &v
		}
		if reviewedAt.Valid {
			t := reviewedAt.Time
			x.ReviewedAt = &t
		}
		if executedAt.Valid {
			t := executedAt.Time
			x.ExecutedAt = &t
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUserNodeAccount(ctx context.Context, oldNodeID string, oldLocalUsername string, oldBillingUsername string, newNodeID string, newLocalUsername string, newBillingUsername string) error {
	oldNodeID = strings.TrimSpace(oldNodeID)
	oldLocalUsername = strings.TrimSpace(oldLocalUsername)
	oldBillingUsername = strings.TrimSpace(oldBillingUsername)
	newNodeID = strings.TrimSpace(newNodeID)
	newLocalUsername = strings.TrimSpace(newLocalUsername)
	newBillingUsername = strings.TrimSpace(newBillingUsername)
	if oldNodeID == "" || oldLocalUsername == "" || oldBillingUsername == "" ||
		newNodeID == "" || newLocalUsername == "" || newBillingUsername == "" {
		return errors.New("参数不能为空")
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
DELETE FROM user_node_accounts
WHERE node_id=$1 AND local_username=$2 AND billing_username=$3`, oldNodeID, oldLocalUsername, oldBillingUsername)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return sql.ErrNoRows
		}
		return s.UpsertUserNodeAccountTx(ctx, tx, newNodeID, newLocalUsername, newBillingUsername)
	})
}

func (s *Store) UpdateUserNodeAccountWithAudit(
	ctx context.Context,
	oldNodeID string,
	oldLocalUsername string,
	oldBillingUsername string,
	newNodeID string,
	newLocalUsername string,
	newBillingUsername string,
	operator string,
	source string,
	reason string,
) error {
	oldNodeID = strings.TrimSpace(oldNodeID)
	oldLocalUsername = strings.TrimSpace(oldLocalUsername)
	oldBillingUsername = strings.TrimSpace(oldBillingUsername)
	newNodeID = strings.TrimSpace(newNodeID)
	newLocalUsername = strings.TrimSpace(newLocalUsername)
	newBillingUsername = strings.TrimSpace(newBillingUsername)
	if oldNodeID == "" || oldLocalUsername == "" || oldBillingUsername == "" ||
		newNodeID == "" || newLocalUsername == "" || newBillingUsername == "" {
		return errors.New("参数不能为空")
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		oldBilling, existed, err := s.getUserNodeAccountBillingTx(ctx, tx, oldNodeID, oldLocalUsername)
		if err != nil {
			return err
		}
		if !existed || oldBilling != oldBillingUsername {
			return sql.ErrNoRows
		}

		newOldBilling, newExisted, err := s.getUserNodeAccountBillingTx(ctx, tx, newNodeID, newLocalUsername)
		if err != nil {
			return err
		}
		if newExisted && newOldBilling != newBillingUsername {
			return &NodeAccountOwnershipConflictError{
				NodeID:          newNodeID,
				LocalUsername:   newLocalUsername,
				ExistingBilling: newOldBilling,
				RequestedBy:     newBillingUsername,
			}
		}

		res, err := tx.ExecContext(ctx, `
DELETE FROM user_node_accounts
WHERE node_id=$1 AND local_username=$2 AND billing_username=$3`, oldNodeID, oldLocalUsername, oldBillingUsername)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return sql.ErrNoRows
		}
		if err := s.UpsertUserNodeAccountTx(ctx, tx, newNodeID, newLocalUsername, newBillingUsername); err != nil {
			return err
		}

		if oldNodeID == newNodeID && oldLocalUsername == newLocalUsername {
			if oldBillingUsername != newBillingUsername {
				return s.insertUserNodeAccountAuditTx(
					ctx, tx,
					newNodeID, newLocalUsername,
					oldBillingUsername, newBillingUsername,
					"mapping_rebind",
					operator, source, reason,
				)
			}
			return nil
		}

		if err := s.insertUserNodeAccountAuditTx(
			ctx, tx,
			oldNodeID, oldLocalUsername,
			oldBillingUsername, "",
			"mapping_delete",
			operator, source, reason,
		); err != nil {
			return err
		}
		if newExisted && newOldBilling == newBillingUsername {
			return nil
		}
		action := "mapping_create"
		if newExisted {
			action = "mapping_rebind"
		}
		return s.insertUserNodeAccountAuditTx(
			ctx, tx,
			newNodeID, newLocalUsername,
			newOldBilling, newBillingUsername,
			action,
			operator, source, reason,
		)
	})
}

func (s *Store) ResolveBillingUsernameTx(ctx context.Context, tx *sql.Tx, nodeID string, localUsername string) (string, bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return "", false, errors.New("node_id/local_username 不能为空")
	}
	var billing string
	err := tx.QueryRowContext(ctx, `
SELECT billing_username
FROM user_node_accounts
WHERE node_id=$1 AND local_username=$2`, nodeID, localUsername).Scan(&billing)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return billing, true, nil
}

func (s *Store) ResolveBillingUsername(ctx context.Context, nodeID string, localUsername string) (string, bool, error) {
	var billing string
	var found bool
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var txErr error
		billing, found, txErr = s.ResolveBillingUsernameTx(ctx, tx, nodeID, localUsername)
		return txErr
	})
	if err != nil {
		return "", false, err
	}
	return billing, found, nil
}

func (s *Store) IsNodeLocalUserKnown(ctx context.Context, nodeID string, localUsername string) (bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return false, errors.New("node_id/local_username 不能为空")
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM node_local_users
  WHERE node_id=$1 AND local_username=$2
)`, nodeID, localUsername).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Store) IsNodeAccountMapped(ctx context.Context, nodeID string, localUsername string) (bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return false, errors.New("node_id/local_username 不能为空")
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM user_node_accounts
  WHERE node_id=$1 AND local_username=$2
)`, nodeID, localUsername).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Store) IsPlatformUserExists(ctx context.Context, username string) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, errors.New("username 不能为空")
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM (
    SELECT username FROM user_accounts
    UNION
    SELECT username FROM admin_accounts
    UNION
    SELECT username FROM power_users
  ) u
  WHERE u.username=$1
)`, username).Scan(&exists)
	return exists, err
}

func (s *Store) IsAdminAccount(ctx context.Context, username string) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, errors.New("username 不能为空")
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Store) IsPrivilegedAccount(ctx context.Context, username string) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, errors.New("username 不能为空")
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM admin_accounts WHERE username=$1
  UNION ALL
  SELECT 1 FROM power_users WHERE username=$1
)`, username).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Store) LookupAnyAccountPlatformUID(ctx context.Context, username string) (int, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return 0, false, errors.New("username 不能为空")
	}
	var uid sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT platform_uid
FROM (
  SELECT platform_uid, 0 AS priority FROM user_accounts WHERE username=$1
  UNION ALL
  SELECT platform_uid, 1 AS priority FROM power_users WHERE username=$1
  UNION ALL
  SELECT platform_uid, 2 AS priority FROM admin_accounts WHERE username=$1
) t
WHERE platform_uid IS NOT NULL
ORDER BY priority ASC
LIMIT 1`, username).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if !uid.Valid || uid.Int64 <= 0 {
		return 0, false, nil
	}
	return int(uid.Int64), true, nil
}

func (s *Store) GetNodeLocalUserIdentity(ctx context.Context, nodeID string, localUsername string) (int, int, bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return 0, 0, false, errors.New("node_id/local_username 不能为空")
	}
	var uid sql.NullInt64
	var gid sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT uid, primary_gid
FROM node_local_users
WHERE node_id=$1 AND local_username=$2`, nodeID, localUsername).Scan(&uid, &gid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	return int(uid.Int64), int(gid.Int64), true, nil
}

func (s *Store) IsNodeLocalMappedToAdmin(ctx context.Context, nodeID string, localUsername string) (bool, string, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return false, "", errors.New("node_id/local_username 不能为空")
	}
	var adminUsername string
	err := s.db.QueryRowContext(ctx, `
SELECT una.billing_username
FROM user_node_accounts una
JOIN admin_accounts aa
  ON aa.username=una.billing_username
WHERE una.node_id=$1 AND una.local_username=$2
LIMIT 1`, nodeID, localUsername).Scan(&adminUsername)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, strings.TrimSpace(adminUsername), nil
}

func (s *Store) ListNodeAdminMappedLocals(ctx context.Context, nodeID string, limit int) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT una.local_username
FROM user_node_accounts una
JOIN admin_accounts aa
  ON aa.username=una.billing_username
WHERE una.node_id=$1
ORDER BY una.local_username
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var local string
		if err := rows.Scan(&local); err != nil {
			return nil, err
		}
		local = strings.TrimSpace(local)
		if local != "" {
			out = append(out, local)
		}
	}
	return out, rows.Err()
}

func (s *Store) ListAdminUsernames(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT username FROM admin_accounts ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return nil, err
		}
		username = strings.TrimSpace(username)
		if username != "" {
			out = append(out, username)
		}
	}
	return out, rows.Err()
}

func (s *Store) ListRegisteredLocalUsersByNode(ctx context.Context, nodeID string, limit int) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 200000 {
		limit = 50000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT local_username
FROM user_node_accounts
WHERE node_id=$1
ORDER BY local_username
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) ListAllowedLocalUsersByNode(ctx context.Context, nodeID string, limit int) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 200000 {
		limit = 50000
	}
	rows, err := s.db.QueryContext(ctx, `
WITH policy AS (
  SELECT COALESCE(np.ssh_guard_enabled, FALSE) AS guard_enabled,
         COALESCE(np.ssh_exclusive_enabled, FALSE) AS exclusive_enabled,
         COALESCE(np.ssh_exclusive_block_others, TRUE) AS exclusive_block_others
  FROM nodes n
  LEFT JOIN node_policies np ON np.node_id=n.node_id
  WHERE n.node_id=$1
),
has_wildcard AS (
  SELECT EXISTS(
    SELECT 1 FROM ssh_whitelist
    WHERE (node_id=$1 OR node_id='*') AND local_username='*'
  ) AS v
)
SELECT DISTINCT local_username FROM (
  SELECT local_username
  FROM user_node_accounts
  WHERE node_id=$1
    AND EXISTS (SELECT 1 FROM policy WHERE (exclusive_enabled=false OR exclusive_block_others=false))
  UNION ALL
  SELECT local_username
  FROM node_exclusive_users
  WHERE node_id=$1
    AND EXISTS (SELECT 1 FROM policy WHERE exclusive_enabled=true AND exclusive_block_others=true)
  UNION ALL
  SELECT nlu.local_username
  FROM node_local_users nlu
  WHERE nlu.node_id=$1
    AND EXISTS (SELECT 1 FROM policy WHERE guard_enabled=false AND (exclusive_enabled=false OR exclusive_block_others=false))
  UNION ALL
  SELECT local_username
  FROM user_node_bind_temp_access
  WHERE node_id=$1
    AND expires_at > NOW()
  UNION ALL
  SELECT local_username
  FROM ssh_whitelist
  WHERE (node_id=$1 OR node_id='*')
    AND local_username <> '*'
  UNION ALL
  SELECT nlu.local_username
  FROM node_local_users nlu
  WHERE nlu.node_id=$1
    AND EXISTS (SELECT 1 FROM has_wildcard WHERE v=true)
) t
WHERE local_username <> ''
ORDER BY local_username
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) ListExemptLocalUsersByNode(ctx context.Context, nodeID string, limit int) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 200000 {
		limit = 50000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT local_username
FROM ssh_exemptions
WHERE node_id=$1 OR node_id='*'
ORDER BY local_username
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) ListDeniedLocalUsersByNode(ctx context.Context, nodeID string, limit int) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 200000 {
		limit = 50000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT local_username
FROM ssh_blacklist
WHERE node_id=$1 OR node_id='*'
ORDER BY local_username
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) IsExempted(ctx context.Context, nodeID string, localUsername string) (bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return false, errors.New("node_id/local_username 不能为空")
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM ssh_exemptions
  WHERE local_username=$2 AND (node_id=$1 OR node_id='*')
)`, nodeID, localUsername).Scan(&exists)
	return exists, err
}

func (s *Store) IsWhitelisted(ctx context.Context, nodeID string, localUsername string) (bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return false, errors.New("node_id/local_username 不能为空")
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM ssh_whitelist
  WHERE (local_username=$2 OR local_username='*')
    AND (node_id=$1 OR node_id='*')
)`, nodeID, localUsername).Scan(&exists)
	return exists, err
}

func (s *Store) IsBlacklisted(ctx context.Context, nodeID string, localUsername string) (bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return false, errors.New("node_id/local_username 不能为空")
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM ssh_blacklist
  WHERE local_username=$2 AND (node_id=$1 OR node_id='*')
)`, nodeID, localUsername).Scan(&exists)
	return exists, err
}

func (s *Store) ListWhitelist(ctx context.Context, nodeID string, limit int) ([]SSHWhitelistEntry, error) {
	nodeID = strings.TrimSpace(nodeID)
	if limit <= 0 || limit > 200000 {
		limit = 1000
	}
	var rows *sql.Rows
	var err error
	if nodeID == "" {
		rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, reason, created_at, updated_at
FROM ssh_whitelist
ORDER BY node_id, local_username
LIMIT $1`, limit)
	} else if nodeID == "*" {
		rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, reason, created_at, updated_at
FROM ssh_whitelist
WHERE node_id='*'
ORDER BY local_username
LIMIT $1`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, reason, created_at, updated_at
FROM ssh_whitelist
WHERE node_id=$1 OR node_id='*'
ORDER BY CASE WHEN node_id='*' THEN 0 ELSE 1 END, local_username
LIMIT $2`, nodeID, limit)
	}
	if err != nil && isReasonColumnMissingErr(err) {
		if nodeID == "" {
			rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, created_at, updated_at
FROM ssh_whitelist
ORDER BY node_id, local_username
LIMIT $1`, limit)
		} else if nodeID == "*" {
			rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, created_at, updated_at
FROM ssh_whitelist
WHERE node_id='*'
ORDER BY local_username
LIMIT $1`, limit)
		} else {
			rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, created_at, updated_at
FROM ssh_whitelist
WHERE node_id=$1 OR node_id='*'
ORDER BY CASE WHEN node_id='*' THEN 0 ELSE 1 END, local_username
LIMIT $2`, nodeID, limit)
		}
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]SSHWhitelistEntry, 0)
		for rows.Next() {
			var v SSHWhitelistEntry
			if err := rows.Scan(&v.NodeID, &v.LocalUsername, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
				return nil, err
			}
			v.Reason = ""
			normalizeSSHWhitelistEntryTimes(&v)
			out = append(out, v)
		}
		return out, rows.Err()
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SSHWhitelistEntry, 0)
	for rows.Next() {
		var v SSHWhitelistEntry
		if err := rows.Scan(&v.NodeID, &v.LocalUsername, &v.CreatedBy, &v.Reason, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		normalizeSSHWhitelistEntryTimes(&v)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) UpsertWhitelist(ctx context.Context, nodeID string, usernames []string, createdBy string, reason string) error {
	nodeID = strings.TrimSpace(nodeID)
	createdBy = strings.TrimSpace(createdBy)
	reason = strings.TrimSpace(reason)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if createdBy == "" {
		createdBy = "admin"
	}
	if len(usernames) == 0 {
		return errors.New("usernames 不能为空")
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		for _, u := range usernames {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO ssh_whitelist(node_id, local_username, created_by, reason, updated_at)
VALUES($1,$2,$3,$4,NOW())
ON CONFLICT (node_id, local_username) DO UPDATE
SET created_by=EXCLUDED.created_by, reason=EXCLUDED.reason, updated_at=NOW()`, nodeID, u, createdBy, reason); err != nil {
				if !isReasonColumnMissingErr(err) {
					return err
				}
				if _, e2 := tx.ExecContext(ctx, `
INSERT INTO ssh_whitelist(node_id, local_username, created_by, updated_at)
VALUES($1,$2,$3,NOW())
ON CONFLICT (node_id, local_username) DO UPDATE
SET created_by=EXCLUDED.created_by, updated_at=NOW()`, nodeID, u, createdBy); e2 != nil {
					return e2
				}
			}
		}
		return nil
	})
}

func (s *Store) DeleteWhitelistWithNodes(ctx context.Context, nodeID string, localUsername string) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return nil, errors.New("node_id/local_username 不能为空")
	}
	deletedNodeSet := map[string]struct{}{}
	if nodeID == "*" {
		deletedNodeSet["*"] = struct{}{}
	} else {
		deletedNodeSet[nodeID] = struct{}{}
		var hasGlobal bool
		if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM ssh_whitelist
  WHERE node_id='*' AND local_username=$1
)`, localUsername).Scan(&hasGlobal); err == nil && hasGlobal {
			deletedNodeSet["*"] = struct{}{}
		}
	}
	var res sql.Result
	var err error
	if nodeID == "*" {
		res, err = s.db.ExecContext(ctx, `
DELETE FROM ssh_whitelist
WHERE node_id='*' AND local_username=$1`, localUsername)
	} else {
		// 删除节点级白名单时，同时删除全局(*)同名白名单，避免“看起来删了但仍能登录”。
		res, err = s.db.ExecContext(ctx, `
DELETE FROM ssh_whitelist
WHERE local_username=$1
  AND (node_id=$2 OR node_id='*')`, localUsername, nodeID)
	}
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, sql.ErrNoRows
	}
	nodes := make([]string, 0, len(deletedNodeSet))
	for k := range deletedNodeSet {
		nodes = append(nodes, k)
	}
	sort.Strings(nodes)
	return nodes, nil
}

func (s *Store) DeleteWhitelist(ctx context.Context, nodeID string, localUsername string) error {
	_, err := s.DeleteWhitelistWithNodes(ctx, nodeID, localUsername)
	return err
}

func (s *Store) ListBlacklist(ctx context.Context, nodeID string, limit int) ([]SSHBlacklistEntry, error) {
	nodeID = strings.TrimSpace(nodeID)
	if limit <= 0 || limit > 200000 {
		limit = 1000
	}
	var rows *sql.Rows
	var err error
	if nodeID == "" {
		rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, reason, created_at, updated_at
FROM ssh_blacklist
ORDER BY node_id, local_username
LIMIT $1`, limit)
	} else if nodeID == "*" {
		rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, reason, created_at, updated_at
FROM ssh_blacklist
WHERE node_id='*'
ORDER BY local_username
LIMIT $1`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, reason, created_at, updated_at
FROM ssh_blacklist
WHERE node_id=$1 OR node_id='*'
ORDER BY CASE WHEN node_id='*' THEN 0 ELSE 1 END, local_username
LIMIT $2`, nodeID, limit)
	}
	if err != nil && isReasonColumnMissingErr(err) {
		if nodeID == "" {
			rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, created_at, updated_at
FROM ssh_blacklist
ORDER BY node_id, local_username
LIMIT $1`, limit)
		} else if nodeID == "*" {
			rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, created_at, updated_at
FROM ssh_blacklist
WHERE node_id='*'
ORDER BY local_username
LIMIT $1`, limit)
		} else {
			rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, created_at, updated_at
FROM ssh_blacklist
WHERE node_id=$1 OR node_id='*'
ORDER BY CASE WHEN node_id='*' THEN 0 ELSE 1 END, local_username
LIMIT $2`, nodeID, limit)
		}
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]SSHBlacklistEntry, 0)
		for rows.Next() {
			var v SSHBlacklistEntry
			if err := rows.Scan(&v.NodeID, &v.LocalUsername, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
				return nil, err
			}
			v.Reason = ""
			normalizeSSHBlacklistEntryTimes(&v)
			out = append(out, v)
		}
		return out, rows.Err()
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SSHBlacklistEntry, 0)
	for rows.Next() {
		var v SSHBlacklistEntry
		if err := rows.Scan(&v.NodeID, &v.LocalUsername, &v.CreatedBy, &v.Reason, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		normalizeSSHBlacklistEntryTimes(&v)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ListExemptions(ctx context.Context, nodeID string, limit int) ([]SSHExemptionEntry, error) {
	nodeID = strings.TrimSpace(nodeID)
	if limit <= 0 || limit > 200000 {
		limit = 1000
	}
	var rows *sql.Rows
	var err error
	if nodeID == "" {
		rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, reason, created_at, updated_at
FROM ssh_exemptions
ORDER BY node_id, local_username
LIMIT $1`, limit)
	} else if nodeID == "*" {
		rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, reason, created_at, updated_at
FROM ssh_exemptions
WHERE node_id='*'
ORDER BY local_username
LIMIT $1`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, reason, created_at, updated_at
FROM ssh_exemptions
WHERE node_id=$1 OR node_id='*'
ORDER BY CASE WHEN node_id='*' THEN 0 ELSE 1 END, local_username
LIMIT $2`, nodeID, limit)
	}
	if err != nil && isReasonColumnMissingErr(err) {
		if nodeID == "" {
			rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, created_at, updated_at
FROM ssh_exemptions
ORDER BY node_id, local_username
LIMIT $1`, limit)
		} else if nodeID == "*" {
			rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, created_at, updated_at
FROM ssh_exemptions
WHERE node_id='*'
ORDER BY local_username
LIMIT $1`, limit)
		} else {
			rows, err = s.db.QueryContext(ctx, `
SELECT node_id, local_username, created_by, created_at, updated_at
FROM ssh_exemptions
WHERE node_id=$1 OR node_id='*'
ORDER BY CASE WHEN node_id='*' THEN 0 ELSE 1 END, local_username
LIMIT $2`, nodeID, limit)
		}
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]SSHExemptionEntry, 0)
		for rows.Next() {
			var v SSHExemptionEntry
			if err := rows.Scan(&v.NodeID, &v.LocalUsername, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
				return nil, err
			}
			v.Reason = ""
			normalizeSSHExemptionEntryTimes(&v)
			out = append(out, v)
		}
		return out, rows.Err()
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SSHExemptionEntry, 0)
	for rows.Next() {
		var v SSHExemptionEntry
		if err := rows.Scan(&v.NodeID, &v.LocalUsername, &v.CreatedBy, &v.Reason, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		normalizeSSHExemptionEntryTimes(&v)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) UpsertExemptions(ctx context.Context, nodeID string, usernames []string, createdBy string, reason string) error {
	nodeID = strings.TrimSpace(nodeID)
	createdBy = strings.TrimSpace(createdBy)
	reason = strings.TrimSpace(reason)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if createdBy == "" {
		createdBy = "admin"
	}
	if len(usernames) == 0 {
		return errors.New("usernames 不能为空")
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		for _, u := range usernames {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO ssh_exemptions(node_id, local_username, created_by, reason, updated_at)
VALUES($1,$2,$3,$4,NOW())
ON CONFLICT (node_id, local_username) DO UPDATE
SET created_by=EXCLUDED.created_by, reason=EXCLUDED.reason, updated_at=NOW()`, nodeID, u, createdBy, reason); err != nil {
				if !isReasonColumnMissingErr(err) {
					return err
				}
				if _, e2 := tx.ExecContext(ctx, `
INSERT INTO ssh_exemptions(node_id, local_username, created_by, updated_at)
VALUES($1,$2,$3,NOW())
ON CONFLICT (node_id, local_username) DO UPDATE
SET created_by=EXCLUDED.created_by, updated_at=NOW()`, nodeID, u, createdBy); e2 != nil {
					return e2
				}
			}
		}
		return nil
	})
}

func (s *Store) DeleteExemptionsWithNodes(ctx context.Context, nodeID string, localUsername string) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return nil, errors.New("node_id/local_username 不能为空")
	}
	deletedNodeSet := map[string]struct{}{}
	if nodeID == "*" {
		deletedNodeSet["*"] = struct{}{}
	} else {
		deletedNodeSet[nodeID] = struct{}{}
		var hasGlobal bool
		if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM ssh_exemptions
  WHERE node_id='*' AND local_username=$1
)`, localUsername).Scan(&hasGlobal); err == nil && hasGlobal {
			deletedNodeSet["*"] = struct{}{}
		}
	}
	var res sql.Result
	var err error
	if nodeID == "*" {
		res, err = s.db.ExecContext(ctx, `
DELETE FROM ssh_exemptions
WHERE node_id='*' AND local_username=$1`, localUsername)
	} else {
		res, err = s.db.ExecContext(ctx, `
DELETE FROM ssh_exemptions
WHERE local_username=$1
  AND (node_id=$2 OR node_id='*')`, localUsername, nodeID)
	}
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, sql.ErrNoRows
	}
	nodes := make([]string, 0, len(deletedNodeSet))
	for k := range deletedNodeSet {
		nodes = append(nodes, k)
	}
	sort.Strings(nodes)
	return nodes, nil
}

func (s *Store) DeleteExemptions(ctx context.Context, nodeID string, localUsername string) error {
	_, err := s.DeleteExemptionsWithNodes(ctx, nodeID, localUsername)
	return err
}

func (s *Store) UpsertSSHListSource(ctx context.Context, listType string, nodeID string, localUsername string, sourceType string, sourcePlatformUsername string, updatedBy string) error {
	listType = strings.TrimSpace(listType)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	sourceType = strings.TrimSpace(sourceType)
	sourcePlatformUsername = strings.TrimSpace(sourcePlatformUsername)
	updatedBy = strings.TrimSpace(updatedBy)
	if listType == "" || nodeID == "" || localUsername == "" {
		return errors.New("list_type/node_id/local_username 不能为空")
	}
	if sourceType != "platform" {
		sourceType = "local"
		sourcePlatformUsername = ""
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO ssh_list_sources(list_type, node_id, local_username, source_type, source_platform_username, updated_by, updated_at)
VALUES($1,$2,$3,$4,$5,$6,NOW())
ON CONFLICT (list_type, node_id, local_username) DO UPDATE
SET source_type=EXCLUDED.source_type,
    source_platform_username=EXCLUDED.source_platform_username,
    updated_by=EXCLUDED.updated_by,
    updated_at=NOW()`, listType, nodeID, localUsername, sourceType, sourcePlatformUsername, updatedBy)
	if err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "relation") && strings.Contains(msg, "ssh_list_sources") && strings.Contains(msg, "does not exist") {
			return nil
		}
	}
	return err
}

func (s *Store) ListSSHListSources(ctx context.Context, listType string, nodeID string, limit int) ([]SSHListSource, error) {
	listType = strings.TrimSpace(listType)
	nodeID = strings.TrimSpace(nodeID)
	if listType == "" {
		return nil, errors.New("list_type 不能为空")
	}
	if limit <= 0 || limit > 200000 {
		limit = 5000
	}
	var rows *sql.Rows
	var err error
	if nodeID == "" {
		rows, err = s.db.QueryContext(ctx, `
SELECT list_type, node_id, local_username, source_type, source_platform_username, updated_by, updated_at
FROM ssh_list_sources
WHERE list_type=$1
ORDER BY node_id, local_username
LIMIT $2`, listType, limit)
	} else if nodeID == "*" {
		rows, err = s.db.QueryContext(ctx, `
SELECT list_type, node_id, local_username, source_type, source_platform_username, updated_by, updated_at
FROM ssh_list_sources
WHERE list_type=$1 AND node_id='*'
ORDER BY local_username
LIMIT $2`, listType, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT list_type, node_id, local_username, source_type, source_platform_username, updated_by, updated_at
FROM ssh_list_sources
WHERE list_type=$1 AND (node_id=$2 OR node_id='*')
ORDER BY CASE WHEN node_id='*' THEN 0 ELSE 1 END, local_username
LIMIT $3`, listType, nodeID, limit)
	}
	if err != nil {
		// 兼容旧库：未升级到 ssh_list_sources 时返回空结果，不阻塞主流程。
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "relation") && strings.Contains(msg, "ssh_list_sources") && strings.Contains(msg, "does not exist") {
			return []SSHListSource{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := make([]SSHListSource, 0)
	for rows.Next() {
		var v SSHListSource
		if err := rows.Scan(&v.ListType, &v.NodeID, &v.LocalUsername, &v.SourceType, &v.SourcePlatformUsername, &v.UpdatedBy, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSSHListSource(ctx context.Context, listType string, nodeID string, localUsername string) error {
	listType = strings.TrimSpace(listType)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if listType == "" || nodeID == "" || localUsername == "" {
		return errors.New("list_type/node_id/local_username 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `
DELETE FROM ssh_list_sources
WHERE list_type=$1 AND node_id=$2 AND local_username=$3`, listType, nodeID, localUsername)
	if err != nil {
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "relation") && strings.Contains(msg, "ssh_list_sources") && strings.Contains(msg, "does not exist") {
			return nil
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpsertBlacklist(ctx context.Context, nodeID string, usernames []string, createdBy string, reason string) error {
	nodeID = strings.TrimSpace(nodeID)
	createdBy = strings.TrimSpace(createdBy)
	reason = strings.TrimSpace(reason)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if createdBy == "" {
		createdBy = "admin"
	}
	if len(usernames) == 0 {
		return errors.New("usernames 不能为空")
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		for _, u := range usernames {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO ssh_blacklist(node_id, local_username, created_by, reason, updated_at)
VALUES($1,$2,$3,$4,NOW())
ON CONFLICT (node_id, local_username) DO UPDATE
SET created_by=EXCLUDED.created_by, reason=EXCLUDED.reason, updated_at=NOW()`, nodeID, u, createdBy, reason); err != nil {
				if !isReasonColumnMissingErr(err) {
					return err
				}
				if _, e2 := tx.ExecContext(ctx, `
INSERT INTO ssh_blacklist(node_id, local_username, created_by, updated_at)
VALUES($1,$2,$3,NOW())
ON CONFLICT (node_id, local_username) DO UPDATE
SET created_by=EXCLUDED.created_by, updated_at=NOW()`, nodeID, u, createdBy); e2 != nil {
					return e2
				}
			}
		}
		return nil
	})
}

func isReasonColumnMissingErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, `column "reason" does not exist`) ||
		(strings.Contains(msg, "reason") && strings.Contains(msg, "does not exist"))
}

func isColumnMissingErr(err error, column string) bool {
	if err == nil {
		return false
	}
	col := strings.ToLower(strings.TrimSpace(column))
	if col == "" {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "column") &&
		strings.Contains(msg, col) &&
		strings.Contains(msg, "does not exist")
}

func (s *Store) DeleteBlacklistWithNodes(ctx context.Context, nodeID string, localUsername string) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return nil, errors.New("node_id/local_username 不能为空")
	}
	deletedNodeSet := map[string]struct{}{}
	if nodeID == "*" {
		deletedNodeSet["*"] = struct{}{}
	} else {
		deletedNodeSet[nodeID] = struct{}{}
		var hasGlobal bool
		if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM ssh_blacklist
  WHERE node_id='*' AND local_username=$1
)`, localUsername).Scan(&hasGlobal); err == nil && hasGlobal {
			deletedNodeSet["*"] = struct{}{}
		}
	}
	var res sql.Result
	var err error
	if nodeID == "*" {
		res, err = s.db.ExecContext(ctx, `
DELETE FROM ssh_blacklist
WHERE node_id='*' AND local_username=$1`, localUsername)
	} else {
		// 删除节点级黑名单时，同时删除全局(*)同名黑名单，避免“看起来删了仍被全局拦截”。
		res, err = s.db.ExecContext(ctx, `
DELETE FROM ssh_blacklist
WHERE local_username=$1
  AND (node_id=$2 OR node_id='*')`, localUsername, nodeID)
	}
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, sql.ErrNoRows
	}
	nodes := make([]string, 0, len(deletedNodeSet))
	for k := range deletedNodeSet {
		nodes = append(nodes, k)
	}
	sort.Strings(nodes)
	return nodes, nil
}

func (s *Store) DeleteBlacklist(ctx context.Context, nodeID string, localUsername string) error {
	_, err := s.DeleteBlacklistWithNodes(ctx, nodeID, localUsername)
	return err
}

// DeleteBlacklistExact 仅删除精确 (node_id, local_username) 黑名单记录。
// 与 DeleteBlacklistWithNodes 不同：不会联动删除 node_id='*' 的同名记录。
func (s *Store) DeleteBlacklistExact(ctx context.Context, nodeID string, localUsername string) error {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return errors.New("node_id/local_username 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `
DELETE FROM ssh_blacklist
WHERE node_id=$1 AND local_username=$2`, nodeID, localUsername)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func normalizeUnbindRecordSourceType(v string) string {
	switch strings.TrimSpace(v) {
	case "admin_forced":
		return "admin_forced"
	default:
		return "user_request"
	}
}

func normalizeUnbindRecordStatus(v string) string {
	switch strings.TrimSpace(v) {
	case "approved":
		return "approved"
	case "rejected":
		return "rejected"
	case "forced":
		return "forced"
	default:
		return "pending"
	}
}

func (s *Store) upsertUserNodeUnbindRecordByRequestTx(
	ctx context.Context,
	tx *sql.Tx,
	requestID int,
	billingUsername string,
	nodeID string,
	localUsername string,
	status string,
	reason string,
	initiatedBy string,
	reviewedBy *string,
	reviewedAt *time.Time,
	executedAt *time.Time,
) error {
	if requestID <= 0 {
		return errors.New("request_id 不合法")
	}
	billingUsername = strings.TrimSpace(billingUsername)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	reason = strings.TrimSpace(reason)
	initiatedBy = strings.TrimSpace(initiatedBy)
	status = normalizeUnbindRecordStatus(status)
	if billingUsername == "" || nodeID == "" || localUsername == "" {
		return errors.New("billing_username/node_id/local_username 不能为空")
	}
	if initiatedBy == "" {
		initiatedBy = billingUsername
	}
	var reviewedByArg any = nil
	if reviewedBy != nil {
		v := strings.TrimSpace(*reviewedBy)
		if v != "" {
			reviewedByArg = v
		}
	}
	var reviewedAtArg any = nil
	if reviewedAt != nil {
		reviewedAtArg = *reviewedAt
	}
	var executedAtArg any = nil
	if executedAt != nil {
		executedAtArg = *executedAt
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO user_node_unbind_records(
  source_type, request_id, billing_username, node_id, local_username, status, reason,
  initiated_by, reviewed_by, reviewed_at, executed_at, created_at, updated_at
)
VALUES('user_request',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW(),NOW())
ON CONFLICT (request_id) DO UPDATE SET
  billing_username=EXCLUDED.billing_username,
  node_id=EXCLUDED.node_id,
  local_username=EXCLUDED.local_username,
  status=EXCLUDED.status,
  reason=COALESCE(NULLIF(EXCLUDED.reason, ''), user_node_unbind_records.reason),
  initiated_by=COALESCE(NULLIF(EXCLUDED.initiated_by, ''), user_node_unbind_records.initiated_by),
  reviewed_by=COALESCE(EXCLUDED.reviewed_by, user_node_unbind_records.reviewed_by),
  reviewed_at=COALESCE(EXCLUDED.reviewed_at, user_node_unbind_records.reviewed_at),
  executed_at=COALESCE(EXCLUDED.executed_at, user_node_unbind_records.executed_at),
  updated_at=NOW()`,
		requestID, billingUsername, nodeID, localUsername, status, reason, initiatedBy,
		reviewedByArg, reviewedAtArg, executedAtArg,
	)
	return err
}

func (s *Store) insertAdminForcedUnbindRecordTx(
	ctx context.Context,
	tx *sql.Tx,
	billingUsername string,
	nodeID string,
	localUsername string,
	reason string,
	operator string,
	now time.Time,
) error {
	billingUsername = strings.TrimSpace(billingUsername)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	reason = strings.TrimSpace(reason)
	operator = strings.TrimSpace(operator)
	if billingUsername == "" || nodeID == "" || localUsername == "" {
		return errors.New("billing_username/node_id/local_username 不能为空")
	}
	if operator == "" {
		operator = "admin"
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO user_node_unbind_records(
  source_type, request_id, billing_username, node_id, local_username, status, reason,
  initiated_by, reviewed_by, reviewed_at, executed_at, created_at, updated_at
)
VALUES('admin_forced',NULL,$1,$2,$3,'forced',$4,$5,$5,$6,$6,NOW(),NOW())`,
		billingUsername, nodeID, localUsername, reason, operator, now,
	)
	return err
}

func (s *Store) CreateUserRequestTx(
	ctx context.Context,
	tx *sql.Tx,
	requestType string,
	billingUsername string,
	nodeID string,
	localUsername string,
	message string,
) (int, error) {
	requestType = strings.TrimSpace(requestType)
	billingUsername = strings.TrimSpace(billingUsername)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	message = strings.TrimSpace(message)

	if requestType != "bind" && requestType != "open" && requestType != "unbind" {
		return 0, errors.New("request_type 仅支持 bind/open/unbind")
	}
	if billingUsername == "" {
		return 0, errors.New("billing_username 不能为空")
	}
	if (requestType == "bind" || requestType == "unbind") && (nodeID == "" || localUsername == "") {
		return 0, errors.New("node_id/local_username 不能为空")
	}
	if requestType == "open" {
		if nodeID == "" {
			nodeID = "待分配"
		}
		if localUsername == "" {
			localUsername = "待分配"
		}
	}
	if requestType == "open" {
		if err := validateOpenRequestReason(message); err != nil {
			return 0, err
		}
		// 锁定当前平台账号，避免并发提交多个 open 申请。
		var lockUsername string
		if err := tx.QueryRowContext(ctx, `
SELECT username
FROM user_accounts
WHERE username=$1
FOR UPDATE`, billingUsername).Scan(&lockUsername); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, errors.New("平台账号不存在，无法提交节点开通申请")
			}
			return 0, err
		}
		var mappingCount int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(1)
FROM user_node_accounts
WHERE billing_username=$1`, billingUsername).Scan(&mappingCount); err != nil {
			return 0, err
		}
		if mappingCount > 0 {
			return 0, errors.New("你已有节点账号映射，不能提交节点开通申请")
		}
		var pendingOpenCount int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(1)
FROM user_requests
WHERE billing_username=$1
  AND request_type='open'
  AND status='pending'`, billingUsername).Scan(&pendingOpenCount); err != nil {
			return 0, err
		}
		if pendingOpenCount > 0 {
			return 0, errors.New("你已有待审核的节点开通申请，请勿重复提交")
		}
	}
	if requestType == "unbind" {
		if utf8RuneCount := len([]rune(message)); utf8RuneCount < 10 {
			return 0, errors.New("解绑申请理由至少 10 个字")
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM user_node_accounts
  WHERE node_id=$1 AND local_username=$2 AND billing_username=$3
)`, nodeID, localUsername, billingUsername).Scan(&exists); err != nil {
			return 0, err
		}
		if !exists {
			return 0, errors.New("当前映射不存在，无法申请解绑")
		}
		var blockedCount int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(1)
FROM user_requests
WHERE request_type='unbind'
  AND billing_username=$1
  AND node_id=$2
  AND local_username=$3
  AND status='pending'`, billingUsername, nodeID, localUsername).Scan(&blockedCount); err != nil {
			return 0, err
		}
		if blockedCount > 0 {
			return 0, errors.New("该映射已有待审核解绑申请，不可重复提交")
		}
	}

	var id int
	err := tx.QueryRowContext(ctx, `
INSERT INTO user_requests(request_type, billing_username, node_id, local_username, message, status)
VALUES($1,$2,$3,$4,$5,'pending')
RETURNING request_id`, requestType, billingUsername, nodeID, localUsername, message).Scan(&id)
	if err != nil {
		return 0, err
	}
	if requestType == "unbind" {
		if err := s.upsertUserNodeUnbindRecordByRequestTx(
			ctx, tx, id,
			billingUsername, nodeID, localUsername,
			"pending",
			message,
			billingUsername,
			nil, nil, nil,
		); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func (s *Store) ListUserRequestsByBilling(ctx context.Context, billingUsername string, limit int) ([]UserRequest, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return nil, errors.New("billing_username 不能为空")
	}
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT request_id, request_type, billing_username, node_id, local_username, message, status,
       reviewed_by, reviewed_at, reject_reason, created_at, updated_at
FROM user_requests
WHERE billing_username=$1
ORDER BY created_at DESC
LIMIT $2`, billingUsername, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserRequest
	for rows.Next() {
		var r UserRequest
		var reviewedBy sql.NullString
		var reviewedAt sql.NullTime
		if err := rows.Scan(
			&r.RequestID, &r.RequestType, &r.BillingUsername, &r.NodeID, &r.LocalUsername,
			&r.Message, &r.Status, &reviewedBy, &reviewedAt, &r.RejectReason, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if reviewedBy.Valid {
			v := reviewedBy.String
			r.ReviewedBy = &v
		}
		if reviewedAt.Valid {
			v := reviewedAt.Time
			r.ReviewedAt = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListUserRequestsAdmin(ctx context.Context, status string, limit int) ([]UserRequest, error) {
	status = strings.TrimSpace(status)
	if limit <= 0 || limit > 5000 {
		limit = 200
	}

	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = s.db.QueryContext(ctx, `
SELECT request_id, request_type, billing_username, node_id, local_username, message, status,
       reviewed_by, reviewed_at, reject_reason, created_at, updated_at
FROM user_requests
ORDER BY created_at DESC
LIMIT $1`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT request_id, request_type, billing_username, node_id, local_username, message, status,
       reviewed_by, reviewed_at, reject_reason, created_at, updated_at
FROM user_requests
WHERE status=$1
ORDER BY created_at DESC
LIMIT $2`, status, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserRequest
	for rows.Next() {
		var r UserRequest
		var reviewedBy sql.NullString
		var reviewedAt sql.NullTime
		if err := rows.Scan(
			&r.RequestID, &r.RequestType, &r.BillingUsername, &r.NodeID, &r.LocalUsername,
			&r.Message, &r.Status, &reviewedBy, &reviewedAt, &r.RejectReason, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if reviewedBy.Valid {
			v := reviewedBy.String
			r.ReviewedBy = &v
		}
		if reviewedAt.Valid {
			v := reviewedAt.Time
			r.ReviewedAt = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ReviewUserRequestTx(
	ctx context.Context,
	tx *sql.Tx,
	requestID int,
	newStatus string,
	reviewedBy string,
	reviewedAt time.Time,
	rejectReason string,
) (UserRequest, error) {
	if requestID <= 0 {
		return UserRequest{}, errors.New("request_id 不合法")
	}
	newStatus = strings.TrimSpace(newStatus)
	reviewedBy = strings.TrimSpace(reviewedBy)
	if newStatus != "approved" && newStatus != "rejected" {
		return UserRequest{}, errors.New("status 仅支持 approved/rejected")
	}
	if reviewedBy == "" {
		reviewedBy = "admin"
	}
	rejectReason = strings.TrimSpace(rejectReason)

	// 锁住记录，避免并发重复审批
	var r UserRequest
	var reviewedByPrev sql.NullString
	var reviewedAtPrev sql.NullTime
	if err := tx.QueryRowContext(ctx, `
SELECT request_id, request_type, billing_username, node_id, local_username, message, status,
       reviewed_by, reviewed_at, reject_reason, created_at, updated_at
FROM user_requests
WHERE request_id=$1
FOR UPDATE`, requestID).Scan(
		&r.RequestID, &r.RequestType, &r.BillingUsername, &r.NodeID, &r.LocalUsername,
		&r.Message, &r.Status, &reviewedByPrev, &reviewedAtPrev, &r.RejectReason, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return UserRequest{}, err
	}
	if r.Status != "pending" && r.Status != "challenge_active" && r.Status != "verified" {
		return UserRequest{}, errors.New("该申请已处理，不能重复审核")
	}
	if r.RequestType == "bind" && newStatus == "approved" && r.Status != "verified" {
		return UserRequest{}, errors.New("绑定申请需先完成节点侧 challenge 校验，当前不能直接审批通过")
	}
	if newStatus == "rejected" && (r.RequestType == "open" || r.RequestType == "unbind") && rejectReason == "" {
		return UserRequest{}, errors.New("拒绝该申请时必须填写理由")
	}
	if newStatus != "rejected" {
		rejectReason = ""
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE user_requests
SET status=$2, reviewed_by=$3, reviewed_at=$4, reject_reason=$5, updated_at=NOW()
WHERE request_id=$1`, requestID, newStatus, reviewedBy, reviewedAt, rejectReason); err != nil {
		return UserRequest{}, err
	}
	if r.RequestType == "unbind" && newStatus == "rejected" {
		if rejectReason == "" {
			return UserRequest{}, errors.New("拒绝解绑申请时必须填写理由")
		}
		rb := reviewedBy
		if err := s.upsertUserNodeUnbindRecordByRequestTx(
			ctx, tx, r.RequestID,
			r.BillingUsername, r.NodeID, r.LocalUsername,
			"rejected",
			rejectReason,
			r.BillingUsername,
			&rb, &reviewedAt, nil,
		); err != nil {
			return UserRequest{}, err
		}
	}

	// bind 申请在 approved 时，写入映射表，供计费与 SSH 校验使用
	if newStatus == "approved" && r.RequestType == "bind" {
		oldBilling, existed, err := s.getUserNodeAccountBillingTx(ctx, tx, r.NodeID, r.LocalUsername)
		if err != nil {
			return UserRequest{}, err
		}
		if existed && oldBilling != r.BillingUsername {
			return UserRequest{}, &NodeAccountOwnershipConflictError{
				NodeID:          r.NodeID,
				LocalUsername:   r.LocalUsername,
				ExistingBilling: oldBilling,
				RequestedBy:     r.BillingUsername,
			}
		}
		if err := s.UpsertUserNodeAccountTx(ctx, tx, r.NodeID, r.LocalUsername, r.BillingUsername); err != nil {
			return UserRequest{}, err
		}
	}
	if newStatus == "approved" && r.RequestType == "unbind" {
		oldBilling, existed, err := s.getUserNodeAccountBillingTx(ctx, tx, r.NodeID, r.LocalUsername)
		if err != nil {
			return UserRequest{}, err
		}
		if !existed || oldBilling != r.BillingUsername {
			return UserRequest{}, errors.New("解绑失败：当前映射不存在或归属已变化")
		}
		res, err := tx.ExecContext(ctx, `
DELETE FROM user_node_accounts
WHERE node_id=$1 AND local_username=$2 AND billing_username=$3`, r.NodeID, r.LocalUsername, r.BillingUsername)
		if err != nil {
			return UserRequest{}, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return UserRequest{}, err
		}
		if affected == 0 {
			return UserRequest{}, sql.ErrNoRows
		}
		if err := s.insertUserNodeAccountAuditTx(
			ctx, tx,
			r.NodeID, r.LocalUsername,
			r.BillingUsername, "",
			"mapping_delete",
			reviewedBy, "admin_request", "管理员审批通过解绑申请",
		); err != nil {
			return UserRequest{}, err
		}
		rb := reviewedBy
		if err := s.upsertUserNodeUnbindRecordByRequestTx(
			ctx, tx, r.RequestID,
			r.BillingUsername, r.NodeID, r.LocalUsername,
			"approved",
			r.Message,
			r.BillingUsername,
			&rb, &reviewedAt, &reviewedAt,
		); err != nil {
			return UserRequest{}, err
		}
	}

	r.Status = newStatus
	r.ReviewedBy = &reviewedBy
	r.ReviewedAt = &reviewedAt
	r.RejectReason = rejectReason
	r.UpdatedAt = reviewedAt
	return r, nil
}

func (s *Store) ReopenUserOpenRequestTx(
	ctx context.Context,
	tx *sql.Tx,
	requestID int,
	reviewedBy string,
	reviewedAt time.Time,
) (UserRequest, error) {
	if requestID <= 0 {
		return UserRequest{}, errors.New("request_id 不合法")
	}
	reviewedBy = strings.TrimSpace(reviewedBy)
	if reviewedBy == "" {
		reviewedBy = "admin"
	}

	var r UserRequest
	var reviewedByPrev sql.NullString
	var reviewedAtPrev sql.NullTime
	if err := tx.QueryRowContext(ctx, `
SELECT request_id, request_type, billing_username, node_id, local_username, message, status,
       reviewed_by, reviewed_at, reject_reason, created_at, updated_at
FROM user_requests
WHERE request_id=$1
FOR UPDATE`, requestID).Scan(
		&r.RequestID, &r.RequestType, &r.BillingUsername, &r.NodeID, &r.LocalUsername,
		&r.Message, &r.Status, &reviewedByPrev, &reviewedAtPrev, &r.RejectReason, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return UserRequest{}, err
	}

	if strings.TrimSpace(r.RequestType) != "open" {
		return UserRequest{}, errors.New("仅 open 申请支持恢复为待处理")
	}
	if strings.TrimSpace(r.Status) == "pending" {
		return UserRequest{}, errors.New("该申请已是待处理状态")
	}
	if strings.TrimSpace(r.Status) != "approved" && strings.TrimSpace(r.Status) != "rejected" {
		return UserRequest{}, errors.New("当前状态不支持恢复为待处理")
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE user_requests
SET status='pending', reviewed_by=NULL, reviewed_at=NULL, reject_reason='', updated_at=NOW()
WHERE request_id=$1`, requestID); err != nil {
		return UserRequest{}, err
	}
	r.Status = "pending"
	r.ReviewedBy = nil
	r.ReviewedAt = nil
	r.RejectReason = ""
	r.UpdatedAt = reviewedAt
	return r, nil
}

func nullableTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := asBeijingWallTime(v.Time)
	return &t
}

func scanUserNodeBindChallengeRow(
	ch *UserNodeBindChallenge,
	row interface {
		Scan(dest ...any) error
	},
) error {
	var claimedAt sql.NullTime
	var verifiedAt sql.NullTime
	var cooldownUntil sql.NullTime
	if err := row.Scan(
		&ch.ChallengeID,
		&ch.RequestID,
		&ch.BillingUsername,
		&ch.NodeID,
		&ch.LocalUsername,
		&ch.ChallengeToken,
		&ch.Status,
		&ch.FailReason,
		&ch.ExpiresAt,
		&claimedAt,
		&verifiedAt,
		&cooldownUntil,
		&ch.FailureProcessed,
		&ch.CreatedAt,
		&ch.UpdatedAt,
	); err != nil {
		return err
	}
	ch.ExpiresAt = asBeijingWallTime(ch.ExpiresAt)
	ch.CreatedAt = asBeijingWallTime(ch.CreatedAt)
	ch.UpdatedAt = asBeijingWallTime(ch.UpdatedAt)
	ch.ClaimedAt = nullableTimePtr(claimedAt)
	ch.VerifiedAt = nullableTimePtr(verifiedAt)
	ch.CooldownUntil = nullableTimePtr(cooldownUntil)
	return nil
}

func (s *Store) GetUserNodeBindCooldown(ctx context.Context, billingUsername string) (UserNodeBindCooldown, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return UserNodeBindCooldown{}, errors.New("billing_username 不能为空")
	}
	var out UserNodeBindCooldown
	out.BillingUsername = billingUsername
	var cooldownUntil sql.NullTime
	var lastFailedAt sql.NullTime
	var lastSucceededAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
SELECT billing_username, failure_streak, cooldown_until, last_failed_at, last_succeeded_at, updated_at
FROM user_node_bind_cooldowns
WHERE billing_username=$1`, billingUsername).Scan(
		&out.BillingUsername,
		&out.FailureStreak,
		&cooldownUntil,
		&lastFailedAt,
		&lastSucceededAt,
		&out.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		out.UpdatedAt = nowInBeijing()
		return out, nil
	}
	if err != nil {
		return UserNodeBindCooldown{}, err
	}
	out.UpdatedAt = asBeijingWallTime(out.UpdatedAt)
	out.CooldownUntil = nullableTimePtr(cooldownUntil)
	out.LastFailedAt = nullableTimePtr(lastFailedAt)
	out.LastSucceededAt = nullableTimePtr(lastSucceededAt)
	return out, nil
}

func (s *Store) ListUserNodeBindCooldowns(ctx context.Context, activeOnly bool, limit int, now time.Time) ([]AdminUserNodeBindCooldownRow, error) {
	if now.IsZero() {
		now = nowInBeijing()
	}
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	queryLimit := limit
	if activeOnly && queryLimit < 5000 {
		queryLimit = 5000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT c.billing_username,
       c.failure_streak,
       c.cooldown_until,
       c.last_failed_at,
       c.last_succeeded_at,
       c.updated_at,
       ch.challenge_id,
       ch.node_id,
       ch.local_username,
       ch.expires_at
FROM user_node_bind_cooldowns c
LEFT JOIN LATERAL (
  SELECT challenge_id, node_id, local_username, expires_at
  FROM user_node_bind_challenges
  WHERE billing_username=c.billing_username
    AND status='active'
    AND expires_at>$1
  ORDER BY created_at DESC
  LIMIT 1
) ch ON TRUE
ORDER BY COALESCE(c.cooldown_until, to_timestamp(0)) DESC, c.failure_streak DESC, c.updated_at DESC
LIMIT $2`, now, queryLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AdminUserNodeBindCooldownRow, 0, limit)
	for rows.Next() {
		var x AdminUserNodeBindCooldownRow
		var cooldownUntil sql.NullTime
		var lastFailedAt sql.NullTime
		var lastSucceededAt sql.NullTime
		var activeChallengeID sql.NullInt64
		var activeNodeID sql.NullString
		var activeLocal sql.NullString
		var activeExpires sql.NullTime
		if err := rows.Scan(
			&x.BillingUsername,
			&x.FailureStreak,
			&cooldownUntil,
			&lastFailedAt,
			&lastSucceededAt,
			&x.UpdatedAt,
			&activeChallengeID,
			&activeNodeID,
			&activeLocal,
			&activeExpires,
		); err != nil {
			return nil, err
		}
		x.UpdatedAt = asBeijingWallTime(x.UpdatedAt)
		x.CooldownUntil = nullableTimePtr(cooldownUntil)
		x.LastFailedAt = nullableTimePtr(lastFailedAt)
		x.LastSucceededAt = nullableTimePtr(lastSucceededAt)
		if x.CooldownUntil != nil && x.CooldownUntil.After(now) {
			x.RemainingCooldownSeconds = int64(x.CooldownUntil.Sub(now).Seconds())
		}
		if activeOnly && x.RemainingCooldownSeconds <= 0 {
			continue
		}
		if activeChallengeID.Valid {
			v := activeChallengeID.Int64
			x.ActiveChallengeID = &v
		}
		if activeNodeID.Valid {
			x.ActiveChallengeNodeID = strings.TrimSpace(activeNodeID.String)
		}
		if activeLocal.Valid {
			x.ActiveChallengeLocalUser = strings.TrimSpace(activeLocal.String)
		}
		x.ActiveChallengeExpiresAt = nullableTimePtr(activeExpires)
		out = append(out, x)
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

func (s *Store) ClearUserNodeBindCooldown(ctx context.Context, billingUsername string, now time.Time) (UserNodeBindCooldown, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return UserNodeBindCooldown{}, errors.New("billing_username 不能为空")
	}
	if now.IsZero() {
		now = nowInBeijing()
	}
	if err := s.WithTx(ctx, func(tx *sql.Tx) error {
		return s.clearNodeBindCooldownTx(ctx, tx, billingUsername, now)
	}); err != nil {
		return UserNodeBindCooldown{}, err
	}
	return s.GetUserNodeBindCooldown(ctx, billingUsername)
}

func (s *Store) applyNodeBindFailureCooldownTx(
	ctx context.Context,
	tx *sql.Tx,
	billingUsername string,
	now time.Time,
	policy NodeBindSecurityPolicy,
) (UserNodeBindCooldown, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return UserNodeBindCooldown{}, errors.New("billing_username 不能为空")
	}
	policy = normalizeNodeBindSecurityPolicy(policy)

	var streak int
	var cooldownUntil sql.NullTime
	if err := tx.QueryRowContext(ctx, `
SELECT failure_streak, cooldown_until
FROM user_node_bind_cooldowns
WHERE billing_username=$1
FOR UPDATE`, billingUsername).Scan(&streak, &cooldownUntil); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return UserNodeBindCooldown{}, err
	}
	streak++
	cdMinutes := policy.RepeatFailureCooldownMinutes
	if streak <= 1 {
		cdMinutes = policy.FirstFailureCooldownMinutes
	}
	if cdMinutes <= 0 {
		cdMinutes = 30
	}
	until := now.Add(time.Duration(cdMinutes) * time.Minute)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO user_node_bind_cooldowns(
  billing_username, failure_streak, cooldown_until, last_failed_at, updated_at
)
VALUES($1,$2,$3,$4,$4)
ON CONFLICT (billing_username) DO UPDATE
SET failure_streak=EXCLUDED.failure_streak,
    cooldown_until=EXCLUDED.cooldown_until,
    last_failed_at=EXCLUDED.last_failed_at,
    updated_at=EXCLUDED.updated_at`,
		billingUsername, streak, until, now,
	); err != nil {
		return UserNodeBindCooldown{}, err
	}
	out := UserNodeBindCooldown{
		BillingUsername: billingUsername,
		FailureStreak:   streak,
		CooldownUntil:   &until,
		LastFailedAt:    &now,
		UpdatedAt:       now,
	}
	return out, nil
}

func (s *Store) applyNodeBindCooldownUntilTx(
	ctx context.Context,
	tx *sql.Tx,
	billingUsername string,
	until time.Time,
	now time.Time,
) (UserNodeBindCooldown, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return UserNodeBindCooldown{}, errors.New("billing_username 不能为空")
	}
	if now.IsZero() {
		now = nowInBeijing()
	}
	if until.IsZero() || until.Before(now) {
		until = now
	}

	var streak int
	var existingUntil sql.NullTime
	if err := tx.QueryRowContext(ctx, `
SELECT failure_streak, cooldown_until
FROM user_node_bind_cooldowns
WHERE billing_username=$1
FOR UPDATE`, billingUsername).Scan(&streak, &existingUntil); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return UserNodeBindCooldown{}, err
	}
	streak++
	if existingUntil.Valid && existingUntil.Time.After(until) {
		until = existingUntil.Time
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO user_node_bind_cooldowns(
  billing_username, failure_streak, cooldown_until, last_failed_at, updated_at
)
VALUES($1,$2,$3,$4,$4)
ON CONFLICT (billing_username) DO UPDATE
SET failure_streak=EXCLUDED.failure_streak,
    cooldown_until=EXCLUDED.cooldown_until,
    last_failed_at=EXCLUDED.last_failed_at,
    updated_at=EXCLUDED.updated_at`,
		billingUsername, streak, until, now,
	); err != nil {
		return UserNodeBindCooldown{}, err
	}
	out := UserNodeBindCooldown{
		BillingUsername: billingUsername,
		FailureStreak:   streak,
		CooldownUntil:   &until,
		LastFailedAt:    &now,
		UpdatedAt:       now,
	}
	return out, nil
}

func (s *Store) clearNodeBindCooldownTx(ctx context.Context, tx *sql.Tx, billingUsername string, now time.Time) error {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO user_node_bind_cooldowns(
  billing_username, failure_streak, cooldown_until, last_succeeded_at, updated_at
)
VALUES($1,0,NULL,$2,$2)
ON CONFLICT (billing_username) DO UPDATE
SET failure_streak=0,
    cooldown_until=NULL,
    last_succeeded_at=EXCLUDED.last_succeeded_at,
    updated_at=EXCLUDED.updated_at`,
		billingUsername, now,
	)
	return err
}

func (s *Store) CreateUserBindChallengeTx(
	ctx context.Context,
	tx *sql.Tx,
	billingUsername string,
	nodeID string,
	localUsername string,
	message string,
	challengeToken string,
	now time.Time,
	policy NodeBindSecurityPolicy,
) (UserNodeBindChallenge, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	message = strings.TrimSpace(message)
	challengeToken = strings.TrimSpace(challengeToken)
	if billingUsername == "" || nodeID == "" || localUsername == "" || challengeToken == "" {
		return UserNodeBindChallenge{}, errors.New("billing_username/node_id/local_username/challenge_token 不能为空")
	}
	policy = normalizeNodeBindSecurityPolicy(policy)
	if now.IsZero() {
		now = nowInBeijing()
	}

	var accountExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE username=$1)`, billingUsername).Scan(&accountExists); err != nil {
		return UserNodeBindChallenge{}, err
	}
	if !accountExists {
		return UserNodeBindChallenge{}, errors.New("平台账号不存在")
	}

	oldBilling, existed, err := s.getUserNodeAccountBillingTx(ctx, tx, nodeID, localUsername)
	if err != nil {
		return UserNodeBindChallenge{}, err
	}
	if existed {
		if oldBilling == billingUsername {
			return UserNodeBindChallenge{}, errNodeBindAlreadyBound
		}
		return UserNodeBindChallenge{}, &NodeAccountOwnershipConflictError{
			NodeID:          nodeID,
			LocalUsername:   localUsername,
			ExistingBilling: oldBilling,
			RequestedBy:     billingUsername,
		}
	}

	var failureStreak int
	var cooldownUntil sql.NullTime
	if err := tx.QueryRowContext(ctx, `
SELECT failure_streak, cooldown_until
FROM user_node_bind_cooldowns
WHERE billing_username=$1
FOR UPDATE`, billingUsername).Scan(&failureStreak, &cooldownUntil); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return UserNodeBindChallenge{}, err
	}
	if cooldownUntil.Valid && cooldownUntil.Time.After(now) {
		return UserNodeBindChallenge{}, &NodeBindCooldownError{
			BillingUsername: billingUsername,
			CooldownUntil:   cooldownUntil.Time,
		}
	}

	if policy.SingleActivePerBilling {
		var activeByBilling UserNodeBindChallenge
		activeErr := scanUserNodeBindChallengeRow(&activeByBilling, tx.QueryRowContext(ctx, `
SELECT challenge_id, request_id, billing_username, node_id, local_username, challenge_token, status, fail_reason,
       expires_at, claimed_at, verified_at, cooldown_until, failure_processed, created_at, updated_at
FROM user_node_bind_challenges
WHERE billing_username=$1
  AND status='active'
  AND expires_at>$2
ORDER BY created_at DESC
LIMIT 1
FOR UPDATE`, billingUsername, now))
		if activeErr == nil {
			if activeByBilling.NodeID == nodeID && activeByBilling.LocalUsername == localUsername {
				return activeByBilling, nil
			}
			return UserNodeBindChallenge{}, &NodeBindActiveChallengeError{
				BillingUsername: billingUsername,
				Challenge:       activeByBilling,
			}
		}
		if !errors.Is(activeErr, sql.ErrNoRows) {
			return UserNodeBindChallenge{}, activeErr
		}
	}

	var existing UserNodeBindChallenge
	err = scanUserNodeBindChallengeRow(&existing, tx.QueryRowContext(ctx, `
SELECT challenge_id, request_id, billing_username, node_id, local_username, challenge_token, status, fail_reason,
       expires_at, claimed_at, verified_at, cooldown_until, failure_processed, created_at, updated_at
FROM user_node_bind_challenges
WHERE billing_username=$1
  AND node_id=$2
  AND local_username=$3
  AND status='active'
  AND expires_at>$4
ORDER BY created_at DESC
LIMIT 1
FOR UPDATE`, billingUsername, nodeID, localUsername, now))
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return UserNodeBindChallenge{}, err
	}

	var occupied UserNodeBindChallenge
	occupiedErr := scanUserNodeBindChallengeRow(&occupied, tx.QueryRowContext(ctx, `
SELECT challenge_id, request_id, billing_username, node_id, local_username, challenge_token, status, fail_reason,
       expires_at, claimed_at, verified_at, cooldown_until, failure_processed, created_at, updated_at
FROM user_node_bind_challenges
WHERE node_id=$1
  AND local_username=$2
  AND status='active'
  AND expires_at>$3
ORDER BY created_at ASC
LIMIT 1
FOR UPDATE`, nodeID, localUsername, now))
	if occupiedErr == nil {
		if occupied.BillingUsername == billingUsername {
			return occupied, nil
		}
		return UserNodeBindChallenge{}, &NodeBindTargetBusyError{
			NodeID:        nodeID,
			LocalUsername: localUsername,
			Challenge:     occupied,
		}
	}
	if !errors.Is(occupiedErr, sql.ErrNoRows) {
		return UserNodeBindChallenge{}, occupiedErr
	}

	expiresAt := now.Add(time.Duration(policy.ChallengeWindowSeconds) * time.Second)
	requestID := 0
	if err := tx.QueryRowContext(ctx, `
INSERT INTO user_requests(request_type, billing_username, node_id, local_username, message, status)
VALUES('bind',$1,$2,$3,$4,'pending')
RETURNING request_id`, billingUsername, nodeID, localUsername, message).Scan(&requestID); err != nil {
		return UserNodeBindChallenge{}, err
	}

	var out UserNodeBindChallenge
	if err := scanUserNodeBindChallengeRow(&out, tx.QueryRowContext(ctx, `
INSERT INTO user_node_bind_challenges(
  request_id, billing_username, node_id, local_username, challenge_token, status, expires_at, created_at, updated_at
)
VALUES($1,$2,$3,$4,$5,'active',$6,$7,$7)
RETURNING challenge_id, request_id, billing_username, node_id, local_username, challenge_token, status, fail_reason,
          expires_at, claimed_at, verified_at, cooldown_until, failure_processed, created_at, updated_at`,
		requestID, billingUsername, nodeID, localUsername, challengeToken, expiresAt, now,
	)); err != nil {
		return UserNodeBindChallenge{}, err
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO user_node_bind_temp_access(node_id, local_username, billing_username, challenge_id, expires_at, created_at, updated_at)
VALUES($1,$2,$3,$4,$5,$6,$6)
ON CONFLICT (node_id, local_username) DO UPDATE
SET billing_username=EXCLUDED.billing_username,
    challenge_id=EXCLUDED.challenge_id,
    expires_at=EXCLUDED.expires_at,
    updated_at=EXCLUDED.updated_at`,
		nodeID, localUsername, billingUsername, out.ChallengeID, expiresAt, now,
	); err != nil {
		return UserNodeBindChallenge{}, err
	}
	return out, nil
}

func (s *Store) ClaimUserBindChallengeTx(
	ctx context.Context,
	tx *sql.Tx,
	challengeToken string,
	nodeID string,
	localUsername string,
	now time.Time,
) (UserNodeBindChallenge, error) {
	challengeToken = strings.TrimSpace(challengeToken)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if challengeToken == "" {
		return UserNodeBindChallenge{}, errNodeBindChallengeNotFound
	}
	if now.IsZero() {
		now = nowInBeijing()
	}
	var ch UserNodeBindChallenge
	if err := scanUserNodeBindChallengeRow(&ch, tx.QueryRowContext(ctx, `
SELECT challenge_id, request_id, billing_username, node_id, local_username, challenge_token, status, fail_reason,
       expires_at, claimed_at, verified_at, cooldown_until, failure_processed, created_at, updated_at
FROM user_node_bind_challenges
WHERE challenge_token=$1
FOR UPDATE`, challengeToken)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserNodeBindChallenge{}, errNodeBindChallengeNotFound
		}
		return UserNodeBindChallenge{}, err
	}
	if nodeID != "" && ch.NodeID != nodeID {
		return UserNodeBindChallenge{}, errNodeBindChallengeMismatch
	}
	if localUsername != "" && ch.LocalUsername != localUsername {
		return UserNodeBindChallenge{}, errNodeBindChallengeMismatch
	}
	if ch.Status != "active" {
		if ch.Status == "expired" || ch.Status == "failed" || ch.Status == "cancelled" {
			return UserNodeBindChallenge{}, errNodeBindChallengeExpired
		}
		if ch.Status == "approved" {
			return ch, nil
		}
		return UserNodeBindChallenge{}, errors.New("challenge 状态不合法")
	}
	if !ch.ExpiresAt.After(now) {
		return UserNodeBindChallenge{}, errNodeBindChallengeExpired
	}

	oldBilling, existed, err := s.getUserNodeAccountBillingTx(ctx, tx, ch.NodeID, ch.LocalUsername)
	if err != nil {
		return UserNodeBindChallenge{}, err
	}
	if existed && oldBilling != ch.BillingUsername {
		return UserNodeBindChallenge{}, &NodeAccountOwnershipConflictError{
			NodeID:          ch.NodeID,
			LocalUsername:   ch.LocalUsername,
			ExistingBilling: oldBilling,
			RequestedBy:     ch.BillingUsername,
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE user_node_bind_challenges
SET status='verified',
    claimed_at=$2,
    verified_at=$2,
    updated_at=$2
WHERE challenge_id=$1`, ch.ChallengeID, now); err != nil {
		return UserNodeBindChallenge{}, err
	}
	if ch.RequestID > 0 {
		if _, err := tx.ExecContext(ctx, `
UPDATE user_requests
SET status='verified', updated_at=$2
WHERE request_id=$1
  AND status IN ('pending','challenge_active')`, ch.RequestID, now); err != nil {
			return UserNodeBindChallenge{}, err
		}
	}

	if !existed {
		if err := s.UpsertUserNodeAccountTx(ctx, tx, ch.NodeID, ch.LocalUsername, ch.BillingUsername); err != nil {
			return UserNodeBindChallenge{}, err
		}
		if err := s.insertUserNodeAccountAuditTx(
			ctx, tx,
			ch.NodeID, ch.LocalUsername,
			"", ch.BillingUsername,
			"mapping_create",
			ch.BillingUsername, "user_claim", "用户完成 challenge 校验，自动生效映射",
		); err != nil {
			return UserNodeBindChallenge{}, err
		}
	}

	if ch.RequestID > 0 {
		if _, err := tx.ExecContext(ctx, `
UPDATE user_requests
SET status='approved', reviewed_by='self-verify', reviewed_at=$2, updated_at=$2
WHERE request_id=$1`, ch.RequestID, now); err != nil {
			return UserNodeBindChallenge{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE user_node_bind_challenges
SET status='approved', updated_at=$2
WHERE challenge_id=$1`, ch.ChallengeID, now); err != nil {
		return UserNodeBindChallenge{}, err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM user_node_bind_temp_access
WHERE node_id=$1 AND local_username=$2`, ch.NodeID, ch.LocalUsername); err != nil {
		return UserNodeBindChallenge{}, err
	}
	if err := s.clearNodeBindCooldownTx(ctx, tx, ch.BillingUsername, now); err != nil {
		return UserNodeBindChallenge{}, err
	}

	return s.GetUserBindChallengeByIDTx(ctx, tx, ch.ChallengeID)
}

func (s *Store) ValidateUserBindChallengeClaim(
	ctx context.Context,
	challengeToken string,
	nodeID string,
	localUsername string,
	now time.Time,
) (UserNodeBindChallenge, error) {
	challengeToken = strings.TrimSpace(challengeToken)
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if challengeToken == "" {
		return UserNodeBindChallenge{}, errNodeBindChallengeNotFound
	}
	if now.IsZero() {
		now = nowInBeijing()
	}
	var ch UserNodeBindChallenge
	if err := scanUserNodeBindChallengeRow(&ch, s.db.QueryRowContext(ctx, `
SELECT challenge_id, request_id, billing_username, node_id, local_username, challenge_token, status, fail_reason,
       expires_at, claimed_at, verified_at, cooldown_until, failure_processed, created_at, updated_at
FROM user_node_bind_challenges
WHERE challenge_token=$1`, challengeToken)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserNodeBindChallenge{}, errNodeBindChallengeNotFound
		}
		return UserNodeBindChallenge{}, err
	}
	if nodeID != "" && ch.NodeID != nodeID {
		return UserNodeBindChallenge{}, errNodeBindChallengeMismatch
	}
	if localUsername != "" && ch.LocalUsername != localUsername {
		return UserNodeBindChallenge{}, errNodeBindChallengeMismatch
	}
	if ch.Status != "active" {
		if ch.Status == "expired" || ch.Status == "failed" || ch.Status == "cancelled" {
			return UserNodeBindChallenge{}, errNodeBindChallengeExpired
		}
		if ch.Status == "approved" {
			return ch, nil
		}
		return UserNodeBindChallenge{}, errors.New("challenge 状态不合法")
	}
	if !ch.ExpiresAt.After(now) {
		return UserNodeBindChallenge{}, errNodeBindChallengeExpired
	}

	var oldBilling string
	err := s.db.QueryRowContext(ctx, `
SELECT billing_username
FROM user_node_accounts
WHERE node_id=$1 AND local_username=$2`, ch.NodeID, ch.LocalUsername).Scan(&oldBilling)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ch, nil
	case err != nil:
		return UserNodeBindChallenge{}, err
	}
	oldBilling = strings.TrimSpace(oldBilling)
	if oldBilling != "" && oldBilling != ch.BillingUsername {
		return UserNodeBindChallenge{}, &NodeAccountOwnershipConflictError{
			NodeID:          ch.NodeID,
			LocalUsername:   ch.LocalUsername,
			ExistingBilling: oldBilling,
			RequestedBy:     ch.BillingUsername,
		}
	}
	return ch, nil
}

func (s *Store) GetUserBindChallengeByIDTx(ctx context.Context, tx *sql.Tx, challengeID int64) (UserNodeBindChallenge, error) {
	if challengeID <= 0 {
		return UserNodeBindChallenge{}, errors.New("challenge_id 不合法")
	}
	var ch UserNodeBindChallenge
	if err := scanUserNodeBindChallengeRow(&ch, tx.QueryRowContext(ctx, `
SELECT challenge_id, request_id, billing_username, node_id, local_username, challenge_token, status, fail_reason,
       expires_at, claimed_at, verified_at, cooldown_until, failure_processed, created_at, updated_at
FROM user_node_bind_challenges
WHERE challenge_id=$1`, challengeID)); err != nil {
		return UserNodeBindChallenge{}, err
	}
	return ch, nil
}

func (s *Store) GetLatestActiveUserBindChallenge(ctx context.Context, billingUsername string, now time.Time) (UserNodeBindChallenge, bool, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return UserNodeBindChallenge{}, false, errors.New("billing_username 不能为空")
	}
	if now.IsZero() {
		now = nowInBeijing()
	}
	var ch UserNodeBindChallenge
	if err := scanUserNodeBindChallengeRow(&ch, s.db.QueryRowContext(ctx, `
SELECT challenge_id, request_id, billing_username, node_id, local_username, challenge_token, status, fail_reason,
       expires_at, claimed_at, verified_at, cooldown_until, failure_processed, created_at, updated_at
FROM user_node_bind_challenges
WHERE billing_username=$1
  AND status='active'
  AND expires_at>$2
ORDER BY created_at DESC
LIMIT 1`, billingUsername, now)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserNodeBindChallenge{}, false, nil
		}
		return UserNodeBindChallenge{}, false, err
	}
	return ch, true, nil
}

func (s *Store) ResolveTemporaryBindAccess(ctx context.Context, nodeID string, localUsername string, now time.Time) (string, bool, *time.Time, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return "", false, nil, errors.New("node_id/local_username 不能为空")
	}
	if now.IsZero() {
		now = nowInBeijing()
	}
	var billing string
	var expiresAt time.Time
	err := s.db.QueryRowContext(ctx, `
SELECT ta.billing_username, ta.expires_at
FROM user_node_bind_temp_access ta
JOIN user_node_bind_challenges ch ON ch.challenge_id=ta.challenge_id
WHERE ta.node_id=$1
  AND ta.local_username=$2
  AND ta.expires_at>$3
  AND ch.status='active'`, nodeID, localUsername, now).Scan(&billing, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil, nil
	}
	if err != nil {
		return "", false, nil, err
	}
	expiresAt = asBeijingWallTime(expiresAt)
	return strings.TrimSpace(billing), true, &expiresAt, nil
}

func (s *Store) PullNodeBindFailureActions(ctx context.Context, nodeID string, now time.Time, policy NodeBindSecurityPolicy) ([]UserNodeBindChallenge, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	policy = normalizeNodeBindSecurityPolicy(policy)
	if now.IsZero() {
		now = nowInBeijing()
	}
	out := make([]UserNodeBindChallenge, 0)
	if err := s.WithTx(ctx, func(tx *sql.Tx) error {
		expiredRows, err := tx.QueryContext(ctx, `
SELECT challenge_id, request_id, billing_username, node_id, local_username, challenge_token, status, fail_reason,
       expires_at, claimed_at, verified_at, cooldown_until, failure_processed, created_at, updated_at
FROM user_node_bind_challenges
WHERE node_id=$1
  AND status='active'
  AND expires_at <= $2
FOR UPDATE`, nodeID, now)
		if err != nil {
			return err
		}
		expired := make([]UserNodeBindChallenge, 0)
		for expiredRows.Next() {
			var ch UserNodeBindChallenge
			if err := scanUserNodeBindChallengeRow(&ch, expiredRows); err != nil {
				_ = expiredRows.Close()
				return err
			}
			expired = append(expired, ch)
		}
		if err := expiredRows.Err(); err != nil {
			_ = expiredRows.Close()
			return err
		}
		_ = expiredRows.Close()

		for _, ch := range expired {
			cooldown, err := s.applyNodeBindFailureCooldownTx(ctx, tx, ch.BillingUsername, now, policy)
			if err != nil {
				return err
			}
			cooldownUntil := now
			if cooldown.CooldownUntil != nil {
				cooldownUntil = *cooldown.CooldownUntil
			}
			if _, err := tx.ExecContext(ctx, `
UPDATE user_node_bind_challenges
SET status='expired',
    fail_reason='challenge_expired',
    cooldown_until=$2,
    updated_at=$3
WHERE challenge_id=$1`, ch.ChallengeID, cooldownUntil, now); err != nil {
				return err
			}
			if ch.RequestID > 0 {
				if _, err := tx.ExecContext(ctx, `
UPDATE user_requests
SET status='rejected', reviewed_by='system', reviewed_at=$2, updated_at=$2
WHERE request_id=$1
  AND status IN ('pending','challenge_active','verified')`, ch.RequestID, now); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, `
DELETE FROM user_node_bind_temp_access
WHERE node_id=$1 AND local_username=$2`, ch.NodeID, ch.LocalUsername); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
DELETE FROM user_node_bind_temp_access
WHERE node_id=$1
  AND expires_at <= $2`, nodeID, now); err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, `
SELECT challenge_id, request_id, billing_username, node_id, local_username, challenge_token, status, fail_reason,
       expires_at, claimed_at, verified_at, cooldown_until, failure_processed, created_at, updated_at
FROM user_node_bind_challenges
WHERE node_id=$1
  AND status IN ('failed','expired')
  AND failure_processed=FALSE
ORDER BY created_at ASC
FOR UPDATE`, nodeID)
		if err != nil {
			return err
		}
		ids := make([]int64, 0)
		for rows.Next() {
			var ch UserNodeBindChallenge
			if err := scanUserNodeBindChallengeRow(&ch, rows); err != nil {
				_ = rows.Close()
				return err
			}
			out = append(out, ch)
			ids = append(ids, ch.ChallengeID)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		_ = rows.Close()
		if len(ids) == 0 {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE user_node_bind_challenges
SET failure_processed=TRUE, updated_at=$2
WHERE challenge_id = ANY($1)`, pq.Array(ids), now); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) UpsertNodeStatusTx(
	ctx context.Context,
	tx *sql.Tx,
	nodeID string,
	lastSeenAt time.Time,
	reportID string,
	reportTS time.Time,
	intervalSeconds int,
	cpuModel string,
	cpuCount int,
	gpuModel string,
	gpuCount int,
	osVersion string,
	kernelVersion string,
	agentVersion string,
	nodeIP string,
	nodeMAC string,
	diskTotalGB float64,
	diskUsedGB float64,
	homeTotalGB float64,
	homeUsedGB float64,
	mntTotalGB float64,
	mntUsedGB float64,
	netRxBytes uint64,
	netTxBytes uint64,
	gpuProcCount int,
	cpuProcCount int,
	usageRecordsCount int,
	sshActiveCount int,
	diskQuotaInstalled bool,
	diskQuotaMounts []string,
	systemServicesCheckedAt *time.Time,
	systemServices []NodeSystemServiceStatus,
	costTotal float64,
) error {
	nodeID = strings.TrimSpace(nodeID)
	reportID = strings.TrimSpace(reportID)
	if nodeID == "" || reportID == "" {
		return errors.New("node_id/report_id 不能为空")
	}
	if intervalSeconds <= 0 {
		intervalSeconds = 60
	}
	cpuModel = strings.TrimSpace(cpuModel)
	gpuModel = strings.TrimSpace(gpuModel)
	osVersion = strings.TrimSpace(osVersion)
	kernelVersion = strings.TrimSpace(kernelVersion)
	agentVersion = strings.TrimSpace(agentVersion)
	nodeIP = strings.TrimSpace(nodeIP)
	nodeMAC = strings.TrimSpace(nodeMAC)
	if cpuCount < 0 {
		cpuCount = 0
	}
	if gpuCount < 0 {
		gpuCount = 0
	}
	cleanQuotaMounts := make([]string, 0, len(diskQuotaMounts))
	for _, x := range uniqTrim(diskQuotaMounts) {
		v := strings.TrimSpace(x)
		if v == "" {
			continue
		}
		cleanQuotaMounts = append(cleanQuotaMounts, v)
	}
	cleanSystemServices := normalizeNodeSystemServiceStatuses(systemServices)
	systemServicesJSON, err := json.Marshal(cleanSystemServices)
	if err != nil {
		return err
	}
	var checkedAt any
	if systemServicesCheckedAt != nil && !systemServicesCheckedAt.IsZero() {
		ts := inBeijing(*systemServicesCheckedAt)
		checkedAt = ts
	}

	month := lastSeenAt.Format("2006-01")
	var prevRx int64
	var prevTx int64
	var prevMonth string
	var prevRxMBMonth float64
	var prevTxMBMonth float64
	_ = tx.QueryRowContext(ctx, `
SELECT net_rx_bytes, net_tx_bytes, traffic_month, net_rx_mb_month, net_tx_mb_month
FROM nodes
WHERE node_id=$1
FOR UPDATE`, nodeID).Scan(&prevRx, &prevTx, &prevMonth, &prevRxMBMonth, &prevTxMBMonth)

	rxMBMonth := prevRxMBMonth
	txMBMonth := prevTxMBMonth
	if prevMonth != month {
		rxMBMonth = 0
		txMBMonth = 0
	}
	if prevRx >= 0 && int64(netRxBytes) >= prevRx {
		rxMBMonth += float64(int64(netRxBytes)-prevRx) / 1024.0 / 1024.0
	}
	if prevTx >= 0 && int64(netTxBytes) >= prevTx {
		txMBMonth += float64(int64(netTxBytes)-prevTx) / 1024.0 / 1024.0
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO nodes(
  node_id, last_seen_at, last_report_id, last_report_ts, interval_seconds,
  cpu_model, cpu_count, gpu_model, gpu_count, os_version, kernel_version, agent_version, node_ip, node_mac, disk_total_gb, disk_used_gb, home_total_gb, home_used_gb, mnt_total_gb, mnt_used_gb,
  net_rx_bytes, net_tx_bytes, net_rx_mb_month, net_tx_mb_month, traffic_month,
	  gpu_process_count, cpu_process_count, usage_records_count, ssh_active_count, disk_quota_installed, disk_quota_mounts, system_services_checked_at, system_services, cost_total, updated_at
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33::jsonb,$34,$35)
ON CONFLICT (node_id) DO UPDATE SET
  last_seen_at=EXCLUDED.last_seen_at,
  last_report_id=EXCLUDED.last_report_id,
  last_report_ts=EXCLUDED.last_report_ts,
  interval_seconds=EXCLUDED.interval_seconds,
  cpu_model=EXCLUDED.cpu_model,
  cpu_count=EXCLUDED.cpu_count,
  gpu_model=EXCLUDED.gpu_model,
  gpu_count=EXCLUDED.gpu_count,
  os_version=EXCLUDED.os_version,
  kernel_version=EXCLUDED.kernel_version,
  agent_version=EXCLUDED.agent_version,
  node_ip=EXCLUDED.node_ip,
  node_mac=EXCLUDED.node_mac,
  disk_total_gb=EXCLUDED.disk_total_gb,
  disk_used_gb=EXCLUDED.disk_used_gb,
  home_total_gb=EXCLUDED.home_total_gb,
  home_used_gb=EXCLUDED.home_used_gb,
  mnt_total_gb=EXCLUDED.mnt_total_gb,
  mnt_used_gb=EXCLUDED.mnt_used_gb,
  net_rx_bytes=EXCLUDED.net_rx_bytes,
  net_tx_bytes=EXCLUDED.net_tx_bytes,
  net_rx_mb_month=EXCLUDED.net_rx_mb_month,
  net_tx_mb_month=EXCLUDED.net_tx_mb_month,
  traffic_month=EXCLUDED.traffic_month,
  gpu_process_count=EXCLUDED.gpu_process_count,
  cpu_process_count=EXCLUDED.cpu_process_count,
  usage_records_count=EXCLUDED.usage_records_count,
  ssh_active_count=EXCLUDED.ssh_active_count,
  disk_quota_installed=EXCLUDED.disk_quota_installed,
  disk_quota_mounts=EXCLUDED.disk_quota_mounts,
  system_services_checked_at=EXCLUDED.system_services_checked_at,
  system_services=EXCLUDED.system_services,
	  cost_total=EXCLUDED.cost_total,
	  updated_at=EXCLUDED.updated_at
		`, nodeID, lastSeenAt, reportID, reportTS, intervalSeconds,
		cpuModel, cpuCount, gpuModel, gpuCount, osVersion, kernelVersion, agentVersion, nodeIP, nodeMAC,
		diskTotalGB, diskUsedGB, homeTotalGB, homeUsedGB, mntTotalGB, mntUsedGB,
		int64(netRxBytes), int64(netTxBytes), rxMBMonth, txMBMonth, month,
		gpuProcCount, cpuProcCount, usageRecordsCount, sshActiveCount, diskQuotaInstalled, pq.Array(cleanQuotaMounts), checkedAt, string(systemServicesJSON), costTotal, inBeijing(lastSeenAt))
	return err
}

func (s *Store) ListNodes(ctx context.Context, limit int) ([]NodeStatus, error) {
	if limit <= 0 || limit > 2000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT n.node_id, n.last_seen_at, n.last_report_id, n.last_report_ts, n.interval_seconds,
       n.cpu_model, n.cpu_count, n.gpu_model, n.gpu_count, n.os_version, n.kernel_version, n.agent_version, n.node_ip, n.node_mac, n.disk_total_gb, n.disk_used_gb, n.home_total_gb, n.home_used_gb, n.mnt_total_gb, n.mnt_used_gb,
       n.net_rx_mb_month, n.net_tx_mb_month,
       n.gpu_process_count, n.cpu_process_count, n.usage_records_count, n.ssh_active_count, n.disk_quota_installed,
       COALESCE(
         (
           SELECT ARRAY(
             SELECT q
             FROM unnest(COALESCE(n.disk_quota_mounts, ARRAY[]::TEXT[])) AS q
             WHERE q IS NOT NULL
           )::TEXT[]
         ),
         ARRAY[]::TEXT[]
       ),
       n.system_services_checked_at,
       COALESCE(n.system_services, '[]'::jsonb) AS system_services,
       COALESCE(np.ssh_guard_enabled, FALSE) AS ssh_guard_enabled,
       COALESCE(np.ssh_exclusive_enabled, FALSE) AS ssh_exclusive_enabled,
       COALESCE(np.points_intercept_enabled, TRUE) AS points_intercept_enabled,
       np.points_throttle_threshold,
       np.points_limited_cpu_quota_percent,
       np.points_blocked_cpu_quota_percent,
       np.points_overdraft_memory_limit_gb,
       COALESCE(np.disk_quota_enabled, FALSE) AS disk_quota_enabled,
       np.disk_quota_mountpoint,
       np.disk_quota_soft_mb,
       np.disk_quota_hard_mb,
       np.node_price_per_minute,
       COALESCE(np.node_model_price_overrides, '{}'::jsonb) AS node_model_price_overrides,
       COALESCE(se.security_event_count_7d, 0) AS security_event_count_7d,
       COALESCE(su.suspicious_user_count_7d, 0) AS suspicious_user_count_7d,
       n.cost_total, n.updated_at
FROM nodes n
LEFT JOIN node_policies np ON np.node_id=n.node_id
LEFT JOIN (
  SELECT node_id, COUNT(1) AS security_event_count_7d
  FROM node_security_events
  WHERE created_at >= NOW() - INTERVAL '7 days'
  GROUP BY node_id
) se ON se.node_id=n.node_id
LEFT JOIN (
  SELECT t.node_id, COUNT(DISTINCT t.username) AS suspicious_user_count_7d
  FROM (
    SELECT e.node_id, NULLIF(TRIM(u.username), '') AS username
    FROM node_security_events e
    CROSS JOIN LATERAL unnest(e.related_usernames) AS u(username)
    WHERE e.created_at >= NOW() - INTERVAL '7 days'
  ) t
  WHERE t.username IS NOT NULL
  GROUP BY t.node_id
) su ON su.node_id=n.node_id
ORDER BY last_seen_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []NodeStatus
	for rows.Next() {
		var n NodeStatus
		var throttleThreshold sql.NullFloat64
		var limitedCPUQuota sql.NullFloat64
		var blockedCPUQuota sql.NullFloat64
		var overdraftMemoryLimit sql.NullFloat64
		var diskQuotaMountpoint sql.NullString
		var diskQuotaSoft sql.NullFloat64
		var diskQuotaHard sql.NullFloat64
		var nodePrice sql.NullFloat64
		var nodeModelPricesRaw []byte
		var diskQuotaMountsRaw []sql.NullString
		var systemServicesCheckedAt sql.NullTime
		var systemServicesRaw []byte
		if err := rows.Scan(
			&n.NodeID,
			&n.LastSeenAt,
			&n.LastReportID,
			&n.LastReportTS,
			&n.IntervalSeconds,
			&n.CPUModel,
			&n.CPUCount,
			&n.GPUModel,
			&n.GPUCount,
			&n.OSVersion,
			&n.KernelVersion,
			&n.AgentVersion,
			&n.NodeIP,
			&n.NodeMAC,
			&n.DiskTotalGB,
			&n.DiskUsedGB,
			&n.HomeTotalGB,
			&n.HomeUsedGB,
			&n.MntTotalGB,
			&n.MntUsedGB,
			&n.NetRxMBMonth,
			&n.NetTxMBMonth,
			&n.GPUProcessCount,
			&n.CPUProcessCount,
			&n.UsageRecordsCount,
			&n.SSHActiveCount,
			&n.DiskQuotaInstalled,
			pq.Array(&diskQuotaMountsRaw),
			&systemServicesCheckedAt,
			&systemServicesRaw,
			&n.SSHGuardEnabled,
			&n.SSHExclusiveEnabled,
			&n.PointsInterceptEnabled,
			&throttleThreshold,
			&limitedCPUQuota,
			&blockedCPUQuota,
			&overdraftMemoryLimit,
			&n.DiskQuotaEnabled,
			&diskQuotaMountpoint,
			&diskQuotaSoft,
			&diskQuotaHard,
			&nodePrice,
			&nodeModelPricesRaw,
			&n.SecurityEventCount7d,
			&n.SuspiciousUserCount7d,
			&n.CostTotal,
			&n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if throttleThreshold.Valid {
			v := throttleThreshold.Float64
			n.PointsThrottleThreshold = &v
		}
		if limitedCPUQuota.Valid {
			v := limitedCPUQuota.Float64
			n.PointsLimitedCPUQuota = &v
		}
		if blockedCPUQuota.Valid {
			v := blockedCPUQuota.Float64
			n.PointsBlockedCPUQuota = &v
		}
		if overdraftMemoryLimit.Valid {
			v := overdraftMemoryLimit.Float64
			n.PointsOverdraftMemoryGB = &v
		}
		n.PointsInterceptEnabled = normalizeImplicitPointsInterceptEnabled(
			n.PointsInterceptEnabled,
			n.PointsThrottleThreshold,
			n.PointsLimitedCPUQuota,
			n.PointsBlockedCPUQuota,
			n.PointsOverdraftMemoryGB,
		)
		n.DiskQuotaMounts = normalizeNullStringArray(diskQuotaMountsRaw)
		if systemServicesCheckedAt.Valid {
			ts := inBeijing(systemServicesCheckedAt.Time)
			n.SystemServicesCheckedAt = &ts
		}
		systemServices, err := parseNodeSystemServicesRaw(systemServicesRaw)
		if err != nil {
			return nil, err
		}
		n.SystemServices = systemServices
		if diskQuotaMountpoint.Valid {
			n.DiskQuotaMountpoint = strings.TrimSpace(diskQuotaMountpoint.String)
		}
		if diskQuotaSoft.Valid {
			v := diskQuotaSoft.Float64
			n.DiskQuotaSoftMB = &v
		}
		if diskQuotaHard.Valid {
			v := diskQuotaHard.Float64
			n.DiskQuotaHardMB = &v
		}
		if nodePrice.Valid {
			v := nodePrice.Float64
			n.NodePricePerMinute = &v
		} else {
			n.NodePricePerMinute = nil
		}
		nodeModelPrices, err := parseNodeModelPriceOverridesRaw(nodeModelPricesRaw)
		if err != nil {
			return nil, err
		}
		if len(nodeModelPrices) > 0 {
			n.NodeModelPriceOverrides = nodeModelPrices
		} else {
			n.NodeModelPriceOverrides = nil
		}
		normalizeNodeHeartbeatTimes(&n)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) GetNodeStatus(ctx context.Context, nodeID string) (NodeStatus, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return NodeStatus{}, errors.New("node_id 不能为空")
	}
	var n NodeStatus
	var throttleThreshold sql.NullFloat64
	var limitedCPUQuota sql.NullFloat64
	var blockedCPUQuota sql.NullFloat64
	var overdraftMemoryLimit sql.NullFloat64
	var diskQuotaMountpoint sql.NullString
	var diskQuotaSoft sql.NullFloat64
	var diskQuotaHard sql.NullFloat64
	var diskQuotaMountsRaw []sql.NullString
	var nodePrice sql.NullFloat64
	var nodeModelPricesRaw []byte
	var systemServicesCheckedAt sql.NullTime
	var systemServicesRaw []byte
	err := s.db.QueryRowContext(ctx, `
SELECT n.node_id, n.last_seen_at, n.last_report_id, n.last_report_ts, n.interval_seconds,
       n.cpu_model, n.cpu_count, n.gpu_model, n.gpu_count, n.os_version, n.kernel_version, n.agent_version, n.node_ip, n.node_mac, n.disk_total_gb, n.disk_used_gb, n.home_total_gb, n.home_used_gb, n.mnt_total_gb, n.mnt_used_gb,
       n.net_rx_mb_month, n.net_tx_mb_month,
       n.gpu_process_count, n.cpu_process_count, n.usage_records_count, n.ssh_active_count, n.disk_quota_installed,
       COALESCE(
         (
           SELECT ARRAY(
             SELECT q
             FROM unnest(COALESCE(n.disk_quota_mounts, ARRAY[]::TEXT[])) AS q
             WHERE q IS NOT NULL
           )::TEXT[]
         ),
         ARRAY[]::TEXT[]
       ),
       n.system_services_checked_at,
       COALESCE(n.system_services, '[]'::jsonb) AS system_services,
       COALESCE(np.ssh_guard_enabled, FALSE) AS ssh_guard_enabled,
       COALESCE(np.ssh_exclusive_enabled, FALSE) AS ssh_exclusive_enabled,
       COALESCE(np.points_intercept_enabled, TRUE) AS points_intercept_enabled,
       np.points_throttle_threshold,
       np.points_limited_cpu_quota_percent,
       np.points_blocked_cpu_quota_percent,
       np.points_overdraft_memory_limit_gb,
       COALESCE(np.disk_quota_enabled, FALSE) AS disk_quota_enabled,
       np.disk_quota_mountpoint,
       np.disk_quota_soft_mb,
       np.disk_quota_hard_mb,
       np.node_price_per_minute,
       COALESCE(np.node_model_price_overrides, '{}'::jsonb) AS node_model_price_overrides,
       n.cost_total, n.updated_at
FROM nodes n
LEFT JOIN node_policies np ON np.node_id=n.node_id
WHERE n.node_id=$1`, nodeID).Scan(
		&n.NodeID,
		&n.LastSeenAt,
		&n.LastReportID,
		&n.LastReportTS,
		&n.IntervalSeconds,
		&n.CPUModel,
		&n.CPUCount,
		&n.GPUModel,
		&n.GPUCount,
		&n.OSVersion,
		&n.KernelVersion,
		&n.AgentVersion,
		&n.NodeIP,
		&n.NodeMAC,
		&n.DiskTotalGB,
		&n.DiskUsedGB,
		&n.HomeTotalGB,
		&n.HomeUsedGB,
		&n.MntTotalGB,
		&n.MntUsedGB,
		&n.NetRxMBMonth,
		&n.NetTxMBMonth,
		&n.GPUProcessCount,
		&n.CPUProcessCount,
		&n.UsageRecordsCount,
		&n.SSHActiveCount,
		&n.DiskQuotaInstalled,
		pq.Array(&diskQuotaMountsRaw),
		&systemServicesCheckedAt,
		&systemServicesRaw,
		&n.SSHGuardEnabled,
		&n.SSHExclusiveEnabled,
		&n.PointsInterceptEnabled,
		&throttleThreshold,
		&limitedCPUQuota,
		&blockedCPUQuota,
		&overdraftMemoryLimit,
		&n.DiskQuotaEnabled,
		&diskQuotaMountpoint,
		&diskQuotaSoft,
		&diskQuotaHard,
		&nodePrice,
		&nodeModelPricesRaw,
		&n.CostTotal,
		&n.UpdatedAt,
	)
	if err != nil {
		return n, err
	}
	if throttleThreshold.Valid {
		v := throttleThreshold.Float64
		n.PointsThrottleThreshold = &v
	}
	if limitedCPUQuota.Valid {
		v := limitedCPUQuota.Float64
		n.PointsLimitedCPUQuota = &v
	}
	if blockedCPUQuota.Valid {
		v := blockedCPUQuota.Float64
		n.PointsBlockedCPUQuota = &v
	}
	if overdraftMemoryLimit.Valid {
		v := overdraftMemoryLimit.Float64
		n.PointsOverdraftMemoryGB = &v
	}
	n.PointsInterceptEnabled = normalizeImplicitPointsInterceptEnabled(
		n.PointsInterceptEnabled,
		n.PointsThrottleThreshold,
		n.PointsLimitedCPUQuota,
		n.PointsBlockedCPUQuota,
		n.PointsOverdraftMemoryGB,
	)
	n.DiskQuotaMounts = normalizeNullStringArray(diskQuotaMountsRaw)
	if systemServicesCheckedAt.Valid {
		ts := inBeijing(systemServicesCheckedAt.Time)
		n.SystemServicesCheckedAt = &ts
	}
	systemServices, err := parseNodeSystemServicesRaw(systemServicesRaw)
	if err != nil {
		return n, err
	}
	n.SystemServices = systemServices
	if diskQuotaMountpoint.Valid {
		n.DiskQuotaMountpoint = strings.TrimSpace(diskQuotaMountpoint.String)
	}
	if diskQuotaSoft.Valid {
		v := diskQuotaSoft.Float64
		n.DiskQuotaSoftMB = &v
	}
	if diskQuotaHard.Valid {
		v := diskQuotaHard.Float64
		n.DiskQuotaHardMB = &v
	}
	if nodePrice.Valid {
		v := nodePrice.Float64
		n.NodePricePerMinute = &v
	} else {
		n.NodePricePerMinute = nil
	}
	nodeModelPrices, err := parseNodeModelPriceOverridesRaw(nodeModelPricesRaw)
	if err != nil {
		return n, err
	}
	if len(nodeModelPrices) > 0 {
		n.NodeModelPriceOverrides = nodeModelPrices
	} else {
		n.NodeModelPriceOverrides = nil
	}
	normalizeNodeHeartbeatTimes(&n)
	return n, nil
}

func (s *Store) IsNodeSSHGuardEnabled(ctx context.Context, nodeID string) (bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return false, errors.New("node_id 不能为空")
	}
	var enabled bool
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(np.ssh_guard_enabled, FALSE)
FROM nodes n
LEFT JOIN node_policies np ON np.node_id=n.node_id
WHERE n.node_id=$1`, nodeID).Scan(&enabled)
	if err != nil {
		return false, err
	}
	return enabled, nil
}

func (s *Store) UpsertNodeSSHGuardPolicy(ctx context.Context, nodeID string, enabled bool, updatedBy string) error {
	nodeID = strings.TrimSpace(nodeID)
	updatedBy = strings.TrimSpace(updatedBy)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO node_policies(node_id, ssh_guard_enabled, updated_by, updated_at)
VALUES($1,$2,$3,NOW())
ON CONFLICT (node_id) DO UPDATE SET
  ssh_guard_enabled=EXCLUDED.ssh_guard_enabled,
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()`,
		nodeID, enabled, updatedBy,
	)
	return err
}

func (s *Store) GetNodeSSHExclusivePolicy(ctx context.Context, nodeID string) (bool, error) {
	enabled, _, err := s.GetNodeSSHExclusiveConfig(ctx, nodeID)
	return enabled, err
}

func (s *Store) GetNodeSSHExclusiveConfig(ctx context.Context, nodeID string) (bool, bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return false, true, errors.New("node_id 不能为空")
	}
	var enabled bool
	var blockOthers bool
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(np.ssh_exclusive_enabled, FALSE),
       COALESCE(np.ssh_exclusive_block_others, TRUE)
FROM nodes n
LEFT JOIN node_policies np ON np.node_id=n.node_id
WHERE n.node_id=$1`, nodeID).Scan(&enabled, &blockOthers)
	if err != nil {
		return false, true, err
	}
	return enabled, blockOthers, nil
}

func (s *Store) GetNodePointsInterceptPolicy(ctx context.Context, nodeID string) (NodePointsInterceptPolicy, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return NodePointsInterceptPolicy{}, errors.New("node_id 不能为空")
	}
	var p NodePointsInterceptPolicy
	var threshold sql.NullFloat64
	var limitedQuota sql.NullFloat64
	var blockedQuota sql.NullFloat64
	var overdraftMemoryLimit sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(np.points_intercept_enabled, TRUE),
       np.points_throttle_threshold,
       np.points_limited_cpu_quota_percent,
       np.points_blocked_cpu_quota_percent,
       np.points_overdraft_memory_limit_gb
FROM nodes n
LEFT JOIN node_policies np ON np.node_id=n.node_id
WHERE n.node_id=$1`, nodeID).Scan(&p.Enabled, &threshold, &limitedQuota, &blockedQuota, &overdraftMemoryLimit)
	if err != nil {
		return NodePointsInterceptPolicy{}, err
	}
	p.NodeID = nodeID
	if threshold.Valid {
		v := threshold.Float64
		p.ThrottleThresholdPoints = &v
	}
	if limitedQuota.Valid {
		v := limitedQuota.Float64
		p.LimitedCPUQuotaPercent = &v
	}
	if blockedQuota.Valid {
		v := blockedQuota.Float64
		p.BlockedCPUQuotaPercent = &v
	}
	if overdraftMemoryLimit.Valid {
		v := overdraftMemoryLimit.Float64
		p.OverdraftMemoryLimitGB = &v
	}
	p.Enabled = normalizeImplicitPointsInterceptEnabled(
		p.Enabled,
		p.ThrottleThresholdPoints,
		p.LimitedCPUQuotaPercent,
		p.BlockedCPUQuotaPercent,
		p.OverdraftMemoryLimitGB,
	)
	return p, nil
}

func (s *Store) GetNodePointsInterceptPolicyTx(ctx context.Context, tx *sql.Tx, nodeID string) (NodePointsInterceptPolicy, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return NodePointsInterceptPolicy{}, errors.New("node_id 不能为空")
	}
	var p NodePointsInterceptPolicy
	var threshold sql.NullFloat64
	var limitedQuota sql.NullFloat64
	var blockedQuota sql.NullFloat64
	var overdraftMemoryLimit sql.NullFloat64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE((SELECT points_intercept_enabled FROM node_policies WHERE node_id=$1), TRUE),
       (SELECT points_throttle_threshold FROM node_policies WHERE node_id=$1),
       (SELECT points_limited_cpu_quota_percent FROM node_policies WHERE node_id=$1),
       (SELECT points_blocked_cpu_quota_percent FROM node_policies WHERE node_id=$1),
       (SELECT points_overdraft_memory_limit_gb FROM node_policies WHERE node_id=$1)`, nodeID).Scan(&p.Enabled, &threshold, &limitedQuota, &blockedQuota, &overdraftMemoryLimit); err != nil {
		return NodePointsInterceptPolicy{}, err
	}
	p.NodeID = nodeID
	if threshold.Valid {
		v := threshold.Float64
		p.ThrottleThresholdPoints = &v
	}
	if limitedQuota.Valid {
		v := limitedQuota.Float64
		p.LimitedCPUQuotaPercent = &v
	}
	if blockedQuota.Valid {
		v := blockedQuota.Float64
		p.BlockedCPUQuotaPercent = &v
	}
	if overdraftMemoryLimit.Valid {
		v := overdraftMemoryLimit.Float64
		p.OverdraftMemoryLimitGB = &v
	}
	p.Enabled = normalizeImplicitPointsInterceptEnabled(
		p.Enabled,
		p.ThrottleThresholdPoints,
		p.LimitedCPUQuotaPercent,
		p.BlockedCPUQuotaPercent,
		p.OverdraftMemoryLimitGB,
	)
	return p, nil
}

func normalizeDiskQuotaPolicyInput(
	mountpoint string,
	defaultSoftMB *float64,
	defaultHardMB *float64,
) (string, *float64, *float64, error) {
	mountpoint = strings.TrimSpace(mountpoint)
	if mountpoint == "." {
		mountpoint = ""
	}
	if mountpoint != "" && !strings.HasPrefix(mountpoint, "/") {
		return "", nil, nil, errors.New("mountpoint 必须以 / 开头")
	}
	if defaultSoftMB != nil {
		v := *defaultSoftMB
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return "", nil, nil, errors.New("disk_quota_soft_mb 必须为非负数")
		}
		defaultSoftMB = &v
	}
	if defaultHardMB != nil {
		v := *defaultHardMB
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return "", nil, nil, errors.New("disk_quota_hard_mb 必须为非负数")
		}
		defaultHardMB = &v
	}
	if defaultSoftMB == nil && defaultHardMB != nil {
		v := *defaultHardMB
		defaultSoftMB = &v
	}
	if defaultHardMB == nil && defaultSoftMB != nil {
		v := *defaultSoftMB
		defaultHardMB = &v
	}
	if defaultSoftMB != nil && defaultHardMB != nil &&
		*defaultSoftMB > 0 && *defaultHardMB > 0 &&
		*defaultHardMB < *defaultSoftMB {
		return "", nil, nil, errors.New("disk_quota_hard_mb 不能小于 disk_quota_soft_mb")
	}
	return mountpoint, defaultSoftMB, defaultHardMB, nil
}

func (s *Store) GetNodeDiskQuotaPolicy(ctx context.Context, nodeID string) (NodeDiskQuotaPolicy, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return NodeDiskQuotaPolicy{}, errors.New("node_id 不能为空")
	}
	var p NodeDiskQuotaPolicy
	var mountpoint sql.NullString
	var soft sql.NullFloat64
	var hard sql.NullFloat64
	if err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(np.disk_quota_enabled, FALSE),
       np.disk_quota_mountpoint,
       np.disk_quota_soft_mb,
       np.disk_quota_hard_mb
FROM nodes n
LEFT JOIN node_policies np ON np.node_id=n.node_id
WHERE n.node_id=$1`, nodeID).Scan(&p.Enabled, &mountpoint, &soft, &hard); err != nil {
		return NodeDiskQuotaPolicy{}, err
	}
	p.NodeID = nodeID
	if mountpoint.Valid {
		p.Mountpoint = strings.TrimSpace(mountpoint.String)
	}
	if soft.Valid {
		v := soft.Float64
		p.DefaultSoft = &v
	}
	if hard.Valid {
		v := hard.Float64
		p.DefaultHard = &v
	}
	return p, nil
}

func (s *Store) UpsertNodeDiskQuotaPolicy(
	ctx context.Context,
	nodeID string,
	enabled bool,
	mountpoint string,
	defaultSoftMB *float64,
	defaultHardMB *float64,
	updatedBy string,
) error {
	nodeID = strings.TrimSpace(nodeID)
	updatedBy = strings.TrimSpace(updatedBy)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	normMount, normSoft, normHard, err := normalizeDiskQuotaPolicyInput(mountpoint, defaultSoftMB, defaultHardMB)
	if err != nil {
		return err
	}
	var mountVal any
	var softVal any
	var hardVal any
	if normMount != "" {
		mountVal = normMount
	}
	if normSoft != nil {
		softVal = *normSoft
	}
	if normHard != nil {
		hardVal = *normHard
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO node_policies(
  node_id, disk_quota_enabled, disk_quota_mountpoint, disk_quota_soft_mb, disk_quota_hard_mb, updated_by, updated_at
)
VALUES($1,$2,$3,$4,$5,$6,NOW())
ON CONFLICT (node_id) DO UPDATE SET
  disk_quota_enabled=EXCLUDED.disk_quota_enabled,
  disk_quota_mountpoint=COALESCE(EXCLUDED.disk_quota_mountpoint, node_policies.disk_quota_mountpoint),
  disk_quota_soft_mb=COALESCE(EXCLUDED.disk_quota_soft_mb, node_policies.disk_quota_soft_mb),
  disk_quota_hard_mb=COALESCE(EXCLUDED.disk_quota_hard_mb, node_policies.disk_quota_hard_mb),
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()`,
		nodeID, enabled, mountVal, softVal, hardVal, updatedBy,
	)
	return err
}

func (s *Store) GetNodePricePolicy(ctx context.Context, nodeID string) (*float64, *float64, map[string]float64, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, nil, nil, errors.New("node_id 不能为空")
	}
	var gpuPrice sql.NullFloat64
	var cpuPrice sql.NullFloat64
	var modelRaw []byte
	err := s.db.QueryRowContext(ctx, `
SELECT np.node_price_per_minute, np.node_cpu_price_per_core_minute, COALESCE(np.node_model_price_overrides, '{}'::jsonb)
FROM nodes n
LEFT JOIN node_policies np ON np.node_id=n.node_id
WHERE n.node_id=$1`, nodeID).Scan(&gpuPrice, &cpuPrice, &modelRaw)
	if err != nil {
		return nil, nil, nil, err
	}
	overrides, err := parseNodeModelPriceOverridesRaw(modelRaw)
	if err != nil {
		return nil, nil, nil, err
	}
	var gpuOut *float64
	if gpuPrice.Valid {
		v := gpuPrice.Float64
		gpuOut = &v
	}
	var cpuOut *float64
	if cpuPrice.Valid {
		v := cpuPrice.Float64
		cpuOut = &v
	}
	return gpuOut, cpuOut, overrides, nil
}

func (s *Store) GetNodePricePolicyTx(ctx context.Context, tx *sql.Tx, nodeID string) (*float64, *float64, map[string]float64, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, nil, nil, errors.New("node_id 不能为空")
	}
	var gpuPrice sql.NullFloat64
	var cpuPrice sql.NullFloat64
	var modelRaw []byte
	if err := tx.QueryRowContext(ctx, `
SELECT node_price_per_minute, node_cpu_price_per_core_minute, COALESCE(node_model_price_overrides, '{}'::jsonb)
FROM node_policies
WHERE node_id=$1`, nodeID).Scan(&gpuPrice, &cpuPrice, &modelRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, err
	}
	overrides, err := parseNodeModelPriceOverridesRaw(modelRaw)
	if err != nil {
		return nil, nil, nil, err
	}
	var gpuOut *float64
	if gpuPrice.Valid {
		v := gpuPrice.Float64
		gpuOut = &v
	}
	var cpuOut *float64
	if cpuPrice.Valid {
		v := cpuPrice.Float64
		cpuOut = &v
	}
	return gpuOut, cpuOut, overrides, nil
}

func (s *Store) GetNodePriceOverride(ctx context.Context, nodeID string) (*float64, error) {
	price, _, _, err := s.GetNodePricePolicy(ctx, nodeID)
	return price, err
}

func (s *Store) GetNodePriceOverrideTx(ctx context.Context, tx *sql.Tx, nodeID string) (*float64, error) {
	price, _, _, err := s.GetNodePricePolicyTx(ctx, tx, nodeID)
	return price, err
}

func (s *Store) UpsertNodePricePolicy(ctx context.Context, nodeID string, pricePerMinute *float64, cpuPricePerCoreMinute *float64, modelOverrides map[string]float64, updatedBy string) error {
	nodeID = strings.TrimSpace(nodeID)
	updatedBy = strings.TrimSpace(updatedBy)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	var price any = nil
	if pricePerMinute != nil {
		if *pricePerMinute < 0 {
			return errors.New("price_per_minute 不能为负数")
		}
		price = *pricePerMinute
	}
	var cpuPrice any = nil
	if cpuPricePerCoreMinute != nil {
		if *cpuPricePerCoreMinute < 0 {
			return errors.New("cpu_price_per_core_minute 不能为负数")
		}
		cpuPrice = *cpuPricePerCoreMinute
	}
	normOverrides, err := normalizeNodeModelPriceOverrides(modelOverrides)
	if err != nil {
		return err
	}
	overridesJSON, err := json.Marshal(normOverrides)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO node_policies(node_id, node_price_per_minute, node_cpu_price_per_core_minute, node_model_price_overrides, updated_by, updated_at)
VALUES($1,$2,$3,$4::jsonb,$5,NOW())
ON CONFLICT (node_id) DO UPDATE SET
  node_price_per_minute=EXCLUDED.node_price_per_minute,
  node_cpu_price_per_core_minute=EXCLUDED.node_cpu_price_per_core_minute,
  node_model_price_overrides=EXCLUDED.node_model_price_overrides,
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()`,
		nodeID, price, cpuPrice, string(overridesJSON), updatedBy,
	)
	return err
}

func (s *Store) UpsertNodePriceOverride(ctx context.Context, nodeID string, pricePerMinute *float64, updatedBy string) error {
	return s.UpsertNodePricePolicy(ctx, nodeID, pricePerMinute, nil, nil, updatedBy)
}

func parseNodeModelPriceOverridesRaw(raw []byte) (map[string]float64, error) {
	if len(raw) == 0 {
		return map[string]float64{}, nil
	}
	var m map[string]float64
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return normalizeNodeModelPriceOverrides(m)
}

func parseNodeSystemServicesRaw(raw []byte) ([]NodeSystemServiceStatus, error) {
	if len(raw) == 0 {
		return []NodeSystemServiceStatus{}, nil
	}
	var items []NodeSystemServiceStatus
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return normalizeNodeSystemServiceStatuses(items), nil
}

func normalizeNodeSystemServiceStatuses(in []NodeSystemServiceStatus) []NodeSystemServiceStatus {
	if len(in) == 0 {
		return []NodeSystemServiceStatus{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]NodeSystemServiceStatus, 0, len(in))
	for _, item := range in {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		item.Name = name
		item.LoadState = strings.TrimSpace(item.LoadState)
		item.UnitFileState = strings.TrimSpace(item.UnitFileState)
		item.ActiveState = strings.TrimSpace(item.ActiveState)
		item.SubState = strings.TrimSpace(item.SubState)
		item.Healthy = item.Deployed && strings.EqualFold(item.ActiveState, "active")
		out = append(out, item)
	}
	return out
}

func normalizeNodeModelPriceOverrides(in map[string]float64) (map[string]float64, error) {
	if len(in) == 0 {
		return map[string]float64{}, nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		model := strings.TrimSpace(k)
		if model == "" {
			continue
		}
		if v < 0 {
			return nil, errors.New("model_price_overrides.price_per_minute 不能为负数")
		}
		out[model] = v
	}
	return out, nil
}

func normalizePointsInterceptPolicyInput(
	throttleThreshold *float64,
	limitedCPUQuota *float64,
	blockedCPUQuota *float64,
	overdraftMemoryLimitGB *float64,
) (*float64, *float64, *float64, *float64, error) {
	if throttleThreshold != nil {
		v := *throttleThreshold
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return nil, nil, nil, nil, errors.New("throttle_threshold_points 必须为非负数")
		}
		throttleThreshold = &v
	}
	if limitedCPUQuota != nil {
		v := *limitedCPUQuota
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 1 || v > 100 {
			return nil, nil, nil, nil, errors.New("limited_cpu_quota_percent 必须在 [1, 100]")
		}
		limitedCPUQuota = &v
	}
	if blockedCPUQuota != nil {
		v := *blockedCPUQuota
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 1 || v > 100 {
			return nil, nil, nil, nil, errors.New("blocked_cpu_quota_percent 必须在 [1, 100]")
		}
		blockedCPUQuota = &v
	}
	if overdraftMemoryLimitGB != nil {
		v := *overdraftMemoryLimitGB
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return nil, nil, nil, nil, errors.New("overdraft_memory_limit_gb 必须为非负数（0 表示关闭）")
		}
		overdraftMemoryLimitGB = &v
	}
	return throttleThreshold, limitedCPUQuota, blockedCPUQuota, overdraftMemoryLimitGB, nil
}

func normalizeImplicitPointsInterceptEnabled(enabled bool, threshold *float64, limited *float64, blocked *float64, memoryLimit *float64) bool {
	_ = threshold
	_ = limited
	_ = blocked
	_ = memoryLimit
	// 以数据库显式配置为准。
	// 历史默认值修复由迁移脚本处理，运行期不再把 false 隐式改回 true，
	// 避免“管理员关闭后又被自动视为开启”的状态回弹。
	return enabled
}

func (s *Store) UpsertNodePointsInterceptPolicy(
	ctx context.Context,
	nodeID string,
	enabled bool,
	throttleThreshold *float64,
	limitedCPUQuota *float64,
	blockedCPUQuota *float64,
	overdraftMemoryLimitGB *float64,
	updatedBy string,
) error {
	nodeID = strings.TrimSpace(nodeID)
	updatedBy = strings.TrimSpace(updatedBy)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	threshold, limited, blocked, memoryLimit, err := normalizePointsInterceptPolicyInput(throttleThreshold, limitedCPUQuota, blockedCPUQuota, overdraftMemoryLimitGB)
	if err != nil {
		return err
	}
	var thresholdVal any
	var limitedVal any
	var blockedVal any
	var memoryLimitVal any
	if threshold != nil {
		thresholdVal = *threshold
	}
	if limited != nil {
		limitedVal = *limited
	}
	if blocked != nil {
		blockedVal = *blocked
	}
	if memoryLimit != nil {
		memoryLimitVal = *memoryLimit
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO node_policies(
  node_id, points_intercept_enabled, points_throttle_threshold, points_limited_cpu_quota_percent, points_blocked_cpu_quota_percent, points_overdraft_memory_limit_gb, updated_by, updated_at
)
VALUES($1,$2,$3,$4,$5,$6,$7,NOW())
ON CONFLICT (node_id) DO UPDATE SET
  points_intercept_enabled=EXCLUDED.points_intercept_enabled,
  points_throttle_threshold=COALESCE(EXCLUDED.points_throttle_threshold, node_policies.points_throttle_threshold),
  points_limited_cpu_quota_percent=COALESCE(EXCLUDED.points_limited_cpu_quota_percent, node_policies.points_limited_cpu_quota_percent),
  points_blocked_cpu_quota_percent=COALESCE(EXCLUDED.points_blocked_cpu_quota_percent, node_policies.points_blocked_cpu_quota_percent),
  points_overdraft_memory_limit_gb=COALESCE(EXCLUDED.points_overdraft_memory_limit_gb, node_policies.points_overdraft_memory_limit_gb),
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()`,
		nodeID, enabled, thresholdVal, limitedVal, blockedVal, memoryLimitVal, updatedBy,
	)
	return err
}

func (s *Store) ListNodeExclusiveUsers(ctx context.Context, nodeID string, limit int) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 50000 {
		limit = 5000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT local_username
FROM node_exclusive_users
WHERE node_id=$1
ORDER BY local_username
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		u = strings.TrimSpace(u)
		if u != "" {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

func (s *Store) IsNodeExclusiveUser(ctx context.Context, nodeID string, localUsername string) (bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return false, errors.New("node_id/local_username 不能为空")
	}
	var ok bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM node_exclusive_users
  WHERE node_id=$1 AND local_username=$2
)`, nodeID, localUsername).Scan(&ok)
	if err != nil {
		return false, err
	}
	return ok, nil
}

func normalizeNodeExclusiveGPUAssignments(assignments []NodeExclusiveGPUAssignment) ([]NodeExclusiveGPUAssignment, error) {
	perUser := map[string]map[int]struct{}{}
	for _, item := range assignments {
		u := strings.TrimSpace(item.LocalUsername)
		if u == "" {
			continue
		}
		if _, ok := perUser[u]; !ok {
			perUser[u] = map[int]struct{}{}
		}
		for _, idx := range item.GPUIndices {
			if idx < 0 {
				return nil, fmt.Errorf("gpu_index 不能为负数：user=%s index=%d", u, idx)
			}
			perUser[u][idx] = struct{}{}
		}
	}
	users := make([]string, 0, len(perUser))
	for u := range perUser {
		users = append(users, u)
	}
	sort.Strings(users)
	out := make([]NodeExclusiveGPUAssignment, 0, len(users))
	for _, u := range users {
		idxSet := perUser[u]
		idxs := make([]int, 0, len(idxSet))
		for idx := range idxSet {
			idxs = append(idxs, idx)
		}
		sort.Ints(idxs)
		out = append(out, NodeExclusiveGPUAssignment{
			LocalUsername: u,
			GPUIndices:    idxs,
		})
	}
	return out, nil
}

func (s *Store) ListNodeExclusiveGPUAssignments(ctx context.Context, nodeID string, limit int) ([]NodeExclusiveGPUAssignment, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 200000 {
		limit = 10000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT local_username, gpu_index
FROM node_exclusive_gpu_users
WHERE node_id=$1
ORDER BY local_username, gpu_index
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	perUser := map[string][]int{}
	order := make([]string, 0)
	seenUser := map[string]struct{}{}
	for rows.Next() {
		var u string
		var idx int
		if err := rows.Scan(&u, &idx); err != nil {
			return nil, err
		}
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := seenUser[u]; !ok {
			seenUser[u] = struct{}{}
			order = append(order, u)
		}
		perUser[u] = append(perUser[u], idx)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]NodeExclusiveGPUAssignment, 0, len(order))
	for _, u := range order {
		idxs := perUser[u]
		sort.Ints(idxs)
		out = append(out, NodeExclusiveGPUAssignment{
			LocalUsername: u,
			GPUIndices:    idxs,
		})
	}
	return out, nil
}

func (s *Store) ReplaceNodeExclusiveUsers(ctx context.Context, nodeID string, enabled bool, blockOtherSSH bool, users []string, gpuAssignments []NodeExclusiveGPUAssignment, updatedBy string) error {
	nodeID = strings.TrimSpace(nodeID)
	updatedBy = strings.TrimSpace(updatedBy)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	cleanUsers := uniqTrim(users)
	cleanGPUAssignments, err := normalizeNodeExclusiveGPUAssignments(gpuAssignments)
	if err != nil {
		return err
	}
	allowedUser := make(map[string]struct{}, len(cleanUsers))
	for _, u := range cleanUsers {
		allowedUser[u] = struct{}{}
	}
	for _, item := range cleanGPUAssignments {
		if _, ok := allowedUser[item.LocalUsername]; !ok {
			return fmt.Errorf("GPU 分配用户 %s 不在独享用户列表中", item.LocalUsername)
		}
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO node_policies(node_id, ssh_exclusive_enabled, ssh_exclusive_block_others, updated_by, updated_at)
VALUES($1,$2,$3,$4,NOW())
ON CONFLICT (node_id) DO UPDATE SET
  ssh_exclusive_enabled=EXCLUDED.ssh_exclusive_enabled,
  ssh_exclusive_block_others=EXCLUDED.ssh_exclusive_block_others,
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()`, nodeID, enabled, blockOtherSSH, updatedBy); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM node_exclusive_users WHERE node_id=$1`, nodeID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM node_exclusive_gpu_users WHERE node_id=$1`, nodeID); err != nil {
			return err
		}
		for _, u := range cleanUsers {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO node_exclusive_users(node_id, local_username, updated_by, updated_at)
VALUES($1,$2,$3,NOW())`, nodeID, u, updatedBy); err != nil {
				return err
			}
		}
		for _, item := range cleanGPUAssignments {
			for _, idx := range item.GPUIndices {
				if _, err := tx.ExecContext(ctx, `
INSERT INTO node_exclusive_gpu_users(node_id, local_username, gpu_index, updated_by, updated_at)
VALUES($1,$2,$3,$4,NOW())`, nodeID, item.LocalUsername, idx, updatedBy); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *Store) ListNodeExclusiveCandidates(ctx context.Context, nodeID string, limit int) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 100000 {
		limit = 5000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT local_username FROM (
  SELECT local_username FROM node_local_users WHERE node_id=$1
  UNION
  SELECT local_username FROM user_node_accounts WHERE node_id=$1
) t
WHERE local_username <> ''
ORDER BY local_username
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		u = strings.TrimSpace(u)
		if u != "" {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

func (s *Store) ListVisibleNodeIDsForPowerUser(ctx context.Context, username string) ([]string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username 不能为空")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT n.node_id
FROM nodes n
WHERE NOT EXISTS (
  SELECT 1 FROM node_view_acl a WHERE a.node_id=n.node_id
)
OR EXISTS (
  SELECT 1 FROM node_view_acl a WHERE a.node_id=n.node_id AND a.power_username=$1
)
ORDER BY n.node_id`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var nodeID string
		if err := rows.Scan(&nodeID); err != nil {
			return nil, err
		}
		out = append(out, strings.TrimSpace(nodeID))
	}
	return out, rows.Err()
}

func (s *Store) CanPowerUserViewNode(ctx context.Context, nodeID string, username string) (bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	username = strings.TrimSpace(username)
	if nodeID == "" || username == "" {
		return false, errors.New("node_id/username 不能为空")
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE node_id=$1)`, nodeID).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, sql.ErrNoRows
	}
	var restricted bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM node_view_acl WHERE node_id=$1)`, nodeID).Scan(&restricted); err != nil {
		return false, err
	}
	if !restricted {
		return true, nil
	}
	var allowed bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM node_view_acl WHERE node_id=$1 AND power_username=$2
)`, nodeID, username).Scan(&allowed); err != nil {
		return false, err
	}
	return allowed, nil
}

func (s *Store) ListNodeViewACL(ctx context.Context, nodeID string, limit int) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT power_username
FROM node_view_acl
WHERE node_id=$1
ORDER BY power_username
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		u = strings.TrimSpace(u)
		if u != "" {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

func (s *Store) ReplaceNodeViewACL(ctx context.Context, nodeID string, powerUsers []string, updatedBy string) error {
	nodeID = strings.TrimSpace(nodeID)
	updatedBy = strings.TrimSpace(updatedBy)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	cleanUsers := uniqTrim(powerUsers)
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM node_view_acl WHERE node_id=$1`, nodeID); err != nil {
			return err
		}
		for _, u := range cleanUsers {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO node_view_acl(node_id, power_username, updated_by, updated_at)
VALUES($1,$2,$3,NOW())`, nodeID, u, updatedBy); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ReplaceNodeLocalUsersTx(ctx context.Context, tx *sql.Tx, nodeID string, users []NodeLocalUser) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM node_local_users WHERE node_id=$1`, nodeID); err != nil {
		return err
	}
	for _, u := range users {
		localUsername := strings.TrimSpace(u.LocalUsername)
		if localUsername == "" {
			continue
		}
		var uidArg any = nil
		if u.UID != nil && *u.UID > 0 {
			uidArg = *u.UID
		}
		var primaryGIDArg any = nil
		if u.PrimaryGID != nil && *u.PrimaryGID > 0 {
			primaryGIDArg = *u.PrimaryGID
		}
		homeUsedGB := u.HomeUsedGB
		if homeUsedGB < 0 {
			homeUsedGB = 0
		}
		hasSudo := u.HasSudo
		hasDocker := u.HasDocker
		if _, err := tx.ExecContext(ctx, `
INSERT INTO node_local_users(node_id, local_username, uid, primary_gid, home_created_at, last_login_at, home_used_gb, has_sudo, has_docker, updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())`,
			nodeID, localUsername, uidArg, primaryGIDArg, u.HomeCreatedAt, u.LastLoginAt, homeUsedGB, hasSudo, hasDocker,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListNodeLocalUsernamesByNodeTx(ctx context.Context, tx *sql.Tx, nodeID string, limit int) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 200000 {
		limit = 50000
	}
	rows, err := tx.QueryContext(ctx, `
SELECT local_username
FROM node_local_users
WHERE node_id=$1
ORDER BY local_username
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var local string
		if err := rows.Scan(&local); err != nil {
			return nil, err
		}
		local = strings.TrimSpace(local)
		if local != "" {
			out = append(out, local)
		}
	}
	return out, rows.Err()
}

func (s *Store) ListRegisteredLocalUsersByNodeTx(ctx context.Context, tx *sql.Tx, nodeID string, limit int) ([]string, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 200000 {
		limit = 50000
	}
	rows, err := tx.QueryContext(ctx, `
SELECT local_username
FROM user_node_accounts
WHERE node_id=$1
ORDER BY local_username
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		u = strings.TrimSpace(u)
		if u != "" {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

func (s *Store) CleanupMissingNodeReportedUsersTx(
	ctx context.Context,
	tx *sql.Tx,
	nodeID string,
	previousLocalUsers []string,
	currentUsers []NodeLocalUser,
) (int, int, int, int, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return 0, 0, 0, 0, errors.New("node_id 不能为空")
	}
	prevSet := make(map[string]struct{}, len(previousLocalUsers))
	for _, local := range previousLocalUsers {
		local = strings.TrimSpace(local)
		if local == "" {
			continue
		}
		prevSet[local] = struct{}{}
	}
	if len(prevSet) == 0 {
		return 0, 0, 0, 0, nil
	}
	currentSet := make(map[string]struct{}, len(currentUsers))
	for _, u := range currentUsers {
		local := strings.TrimSpace(u.LocalUsername)
		if local == "" {
			continue
		}
		currentSet[local] = struct{}{}
	}

	removedMappings := 0
	clearedCPULimits := 0
	clearedMemoryLimits := 0
	clearedGPUVisibility := 0
	for local := range prevSet {
		if _, ok := currentSet[local]; ok {
			continue
		}
		oldBilling, mapped, err := s.getUserNodeAccountBillingTx(ctx, tx, nodeID, local)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		if mapped {
			res, err := tx.ExecContext(ctx, `
DELETE FROM user_node_accounts
WHERE node_id=$1 AND local_username=$2 AND billing_username=$3`, nodeID, local, oldBilling)
			if err != nil {
				return 0, 0, 0, 0, err
			}
			affected, err := res.RowsAffected()
			if err != nil {
				return 0, 0, 0, 0, err
			}
			if affected > 0 {
				if err := s.insertUserNodeAccountAuditTx(
					ctx,
					tx,
					nodeID,
					local,
					oldBilling,
					"",
					"mapping_delete",
					"node_sync",
					"node_report",
					"节点上报该本地账号已不存在，自动清理映射",
				); err != nil {
					return 0, 0, 0, 0, err
				}
				removedMappings++
			}
		}
		if res, err := tx.ExecContext(ctx, `
DELETE FROM node_user_cpu_limits
WHERE node_id=$1 AND local_username=$2`, nodeID, local); err != nil {
			return 0, 0, 0, 0, err
		} else if affected, err := res.RowsAffected(); err != nil {
			return 0, 0, 0, 0, err
		} else if affected > 0 {
			clearedCPULimits += int(affected)
		}
		if res, err := tx.ExecContext(ctx, `
DELETE FROM node_user_memory_limits
WHERE node_id=$1 AND local_username=$2`, nodeID, local); err != nil {
			return 0, 0, 0, 0, err
		} else if affected, err := res.RowsAffected(); err != nil {
			return 0, 0, 0, 0, err
		} else if affected > 0 {
			clearedMemoryLimits += int(affected)
		}
		if res, err := tx.ExecContext(ctx, `
DELETE FROM node_user_gpu_visibility
WHERE node_id=$1 AND local_username=$2`, nodeID, local); err != nil {
			return 0, 0, 0, 0, err
		} else if affected, err := res.RowsAffected(); err != nil {
			return 0, 0, 0, 0, err
		} else if affected > 0 {
			clearedGPUVisibility += int(affected)
		}
	}
	return removedMappings, clearedCPULimits, clearedMemoryLimits, clearedGPUVisibility, nil
}

func (s *Store) ReplaceNodeUserDiskQuotasTx(ctx context.Context, tx *sql.Tx, nodeID string, quotas []NodeUserDiskQuota) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM node_user_disk_quotas WHERE node_id=$1`, nodeID); err != nil {
		return err
	}
	for _, q := range quotas {
		localUsername := strings.TrimSpace(q.LocalUsername)
		mountpoint := strings.TrimSpace(q.Mountpoint)
		if localUsername == "" || mountpoint == "" {
			continue
		}
		usedMB := q.UsedMB
		softMB := q.SoftMB
		hardMB := q.HardMB
		if usedMB < 0 {
			usedMB = 0
		}
		if softMB < 0 {
			softMB = 0
		}
		if hardMB < 0 {
			hardMB = 0
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO node_user_disk_quotas(node_id, local_username, mountpoint, used_mb, soft_mb, hard_mb, updated_at)
VALUES($1,$2,$3,$4,$5,$6,NOW())`,
			nodeID, localUsername, mountpoint, usedMB, softMB, hardMB,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListNodeUserDiskQuotas(ctx context.Context, nodeID string, mountpoint string, limit int) ([]NodeUserDiskQuota, error) {
	nodeID = strings.TrimSpace(nodeID)
	mountpoint = strings.TrimSpace(mountpoint)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 10000 {
		limit = 5000
	}
	baseSQL := `
SELECT node_id, local_username, mountpoint, used_mb, soft_mb, hard_mb, updated_at
FROM node_user_disk_quotas
WHERE node_id=$1`
	args := []any{nodeID}
	if mountpoint != "" {
		baseSQL += ` AND mountpoint=$2`
		args = append(args, mountpoint)
	}
	baseSQL += ` ORDER BY mountpoint, local_username LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, baseSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeUserDiskQuota, 0)
	for rows.Next() {
		var q NodeUserDiskQuota
		if err := rows.Scan(
			&q.NodeID,
			&q.LocalUsername,
			&q.Mountpoint,
			&q.UsedMB,
			&q.SoftMB,
			&q.HardMB,
			&q.UpdatedAt,
		); err != nil {
			return nil, err
		}
		normalizeNodeUserDiskQuotaTimes(&q)
		out = append(out, q)
	}
	return out, rows.Err()
}

func (s *Store) GetNodeUserDiskQuota(
	ctx context.Context,
	nodeID string,
	localUsername string,
	mountpoint string,
) (*NodeUserDiskQuota, bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	mountpoint = strings.TrimSpace(mountpoint)
	if nodeID == "" || localUsername == "" || mountpoint == "" {
		return nil, false, errors.New("node_id/local_username/mountpoint 不能为空")
	}
	var q NodeUserDiskQuota
	if err := s.db.QueryRowContext(ctx, `
SELECT node_id, local_username, mountpoint, used_mb, soft_mb, hard_mb, updated_at
FROM node_user_disk_quotas
WHERE node_id=$1 AND local_username=$2 AND mountpoint=$3`,
		nodeID, localUsername, mountpoint,
	).Scan(
		&q.NodeID,
		&q.LocalUsername,
		&q.Mountpoint,
		&q.UsedMB,
		&q.SoftMB,
		&q.HardMB,
		&q.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	normalizeNodeUserDiskQuotaTimes(&q)
	return &q, true, nil
}

func (s *Store) ListNodeLocalUsersWithPlatformMapping(ctx context.Context, nodeID string, limit int) ([]NodeLocalUser, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT nlu.node_id,
       nlu.local_username,
       nlu.uid,
       nlu.primary_gid,
       COALESCE(una.billing_username, '') AS platform_username,
       (una.billing_username IS NOT NULL) AS mapping_exists,
       (
         EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM power_users pu WHERE pu.username=una.billing_username)
       ) AS platform_exists,
       (adm.username IS NOT NULL) AS admin_mapping,
       COALESCE(adm.username, '') AS admin_username,
       nul.cpu_quota_percent,
       COALESCE(nul.reason, '') AS cpu_quota_reason,
       nul.updated_at AS cpu_quota_updated_at,
       nml.memory_limit_gb,
       COALESCE(nml.reason, '') AS memory_limit_reason,
       nml.updated_at AS memory_limit_updated_at,
       nugv.gpu_indices,
       COALESCE(nugv.reason, '') AS gpu_visibility_reason,
       nugv.updated_at AS gpu_visibility_updated_at,
       nlu.home_created_at,
       nlu.last_login_at,
       nlu.home_used_gb,
       nlu.has_sudo,
       nlu.has_docker,
       nlu.updated_at
FROM node_local_users nlu
LEFT JOIN user_node_accounts una
  ON una.node_id=nlu.node_id
 AND una.local_username=nlu.local_username
LEFT JOIN admin_accounts adm
  ON adm.username=una.billing_username
LEFT JOIN node_user_cpu_limits nul
  ON nul.node_id=nlu.node_id
 AND nul.local_username=nlu.local_username
LEFT JOIN node_user_memory_limits nml
  ON nml.node_id=nlu.node_id
 AND nml.local_username=nlu.local_username
LEFT JOIN (
  SELECT node_id,
         local_username,
         array_agg(gpu_index ORDER BY gpu_index)::BIGINT[] AS gpu_indices,
         (array_agg(reason ORDER BY updated_at DESC, gpu_index ASC))[1] AS reason,
         MAX(updated_at) AS updated_at
  FROM node_user_gpu_visibility
  GROUP BY node_id, local_username
) nugv
  ON nugv.node_id=nlu.node_id
 AND nugv.local_username=nlu.local_username
WHERE nlu.node_id=$1
ORDER BY nlu.local_username
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeLocalUser, 0)
	for rows.Next() {
		var u NodeLocalUser
		var uidRaw sql.NullInt64
		var primaryGIDRaw sql.NullInt64
		var cpuQuotaPercent sql.NullFloat64
		var cpuQuotaUpdatedAt sql.NullTime
		var memoryLimitGB sql.NullFloat64
		var memoryLimitUpdatedAt sql.NullTime
		var gpuVisibleIndicesRaw pq.Int64Array
		var gpuVisibilityUpdatedAt sql.NullTime
		if err := rows.Scan(
			&u.NodeID,
			&u.LocalUsername,
			&uidRaw,
			&primaryGIDRaw,
			&u.PlatformUsername,
			&u.MappingExists,
			&u.PlatformExists,
			&u.AdminMapping,
			&u.AdminUsername,
			&cpuQuotaPercent,
			&u.CPUQuotaReason,
			&cpuQuotaUpdatedAt,
			&memoryLimitGB,
			&u.MemoryLimitReason,
			&memoryLimitUpdatedAt,
			&gpuVisibleIndicesRaw,
			&u.GPUVisibilityReason,
			&gpuVisibilityUpdatedAt,
			&u.HomeCreatedAt,
			&u.LastLoginAt,
			&u.HomeUsedGB,
			&u.HasSudo,
			&u.HasDocker,
			&u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if uidRaw.Valid && uidRaw.Int64 > 0 {
			v := int(uidRaw.Int64)
			u.UID = &v
		}
		if primaryGIDRaw.Valid && primaryGIDRaw.Int64 > 0 {
			v := int(primaryGIDRaw.Int64)
			u.PrimaryGID = &v
		}
		if cpuQuotaPercent.Valid {
			v := cpuQuotaPercent.Float64
			u.CPUQuotaPercent = &v
		}
		if cpuQuotaUpdatedAt.Valid {
			t := cpuQuotaUpdatedAt.Time
			u.CPUQuotaUpdatedAt = &t
		}
		if memoryLimitGB.Valid {
			v := memoryLimitGB.Float64
			u.MemoryLimitGB = &v
		}
		if memoryLimitUpdatedAt.Valid {
			t := memoryLimitUpdatedAt.Time
			u.MemoryLimitUpdatedAt = &t
		}
		if len(gpuVisibleIndicesRaw) > 0 {
			u.GPUVisibleIndices = make([]int, 0, len(gpuVisibleIndicesRaw))
			for _, idx := range gpuVisibleIndicesRaw {
				if idx >= 0 {
					u.GPUVisibleIndices = append(u.GPUVisibleIndices, int(idx))
				}
			}
			sort.Ints(u.GPUVisibleIndices)
		}
		if gpuVisibilityUpdatedAt.Valid {
			t := gpuVisibilityUpdatedAt.Time
			u.GPUVisibilityUpdatedAt = &t
		}
		normalizeNodeLocalUserTimes(&u)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UpsertNodeUserCPULimit(
	ctx context.Context,
	nodeID string,
	localUsername string,
	cpuQuotaPercent float64,
	reason string,
	updatedBy string,
) (NodeUserCPULimit, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	reason = strings.TrimSpace(reason)
	updatedBy = strings.TrimSpace(updatedBy)
	if nodeID == "" || localUsername == "" {
		return NodeUserCPULimit{}, errors.New("node_id/local_username 不能为空")
	}
	if cpuQuotaPercent <= 0 || cpuQuotaPercent > 100 {
		return NodeUserCPULimit{}, errors.New("cpu_quota_percent 必须在 (0, 100] 范围内")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO node_user_cpu_limits(node_id, local_username, cpu_quota_percent, reason, updated_by, updated_at)
VALUES($1,$2,$3,$4,$5,NOW())
ON CONFLICT (node_id, local_username) DO UPDATE
SET cpu_quota_percent=EXCLUDED.cpu_quota_percent,
    reason=EXCLUDED.reason,
    updated_by=EXCLUDED.updated_by,
    updated_at=NOW()`, nodeID, localUsername, cpuQuotaPercent, reason, updatedBy); err != nil {
		return NodeUserCPULimit{}, err
	}
	row, ok, err := s.GetNodeUserCPULimit(ctx, nodeID, localUsername)
	if err != nil {
		return NodeUserCPULimit{}, err
	}
	if !ok || row == nil {
		return NodeUserCPULimit{}, sql.ErrNoRows
	}
	return *row, nil
}

func (s *Store) DeleteNodeUserCPULimit(ctx context.Context, nodeID string, localUsername string) error {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return errors.New("node_id/local_username 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `
DELETE FROM node_user_cpu_limits
WHERE node_id=$1 AND local_username=$2`, nodeID, localUsername)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected <= 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetNodeUserCPULimit(ctx context.Context, nodeID string, localUsername string) (*NodeUserCPULimit, bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return nil, false, errors.New("node_id/local_username 不能为空")
	}
	var out NodeUserCPULimit
	if err := s.db.QueryRowContext(ctx, `
SELECT l.node_id,
       l.local_username,
       COALESCE(una.billing_username, '') AS billing_username,
       (una.billing_username IS NOT NULL) AS mapping_exists,
       (
         EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM power_users pu WHERE pu.username=una.billing_username)
       ) AS platform_exists,
       l.cpu_quota_percent,
       COALESCE(l.reason, '') AS reason,
       l.updated_by,
       l.updated_at
FROM node_user_cpu_limits l
LEFT JOIN user_node_accounts una
  ON una.node_id=l.node_id
 AND una.local_username=l.local_username
WHERE l.node_id=$1 AND l.local_username=$2`, nodeID, localUsername).Scan(
		&out.NodeID,
		&out.LocalUsername,
		&out.BillingUsername,
		&out.MappingExists,
		&out.PlatformExists,
		&out.CPUQuotaPercent,
		&out.Reason,
		&out.UpdatedBy,
		&out.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	normalizeNodeUserCPULimitTimes(&out)
	return &out, true, nil
}

func (s *Store) ListNodeUserCPULimitsByNodeTx(
	ctx context.Context,
	tx *sql.Tx,
	nodeID string,
	limit int,
) ([]NodeUserCPULimit, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 10000 {
		limit = 5000
	}
	rows, err := tx.QueryContext(ctx, `
SELECT node_id, local_username, cpu_quota_percent, COALESCE(reason, '') AS reason, updated_by, updated_at
FROM node_user_cpu_limits
WHERE node_id=$1
ORDER BY updated_at DESC, local_username ASC
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeUserCPULimit, 0)
	for rows.Next() {
		var x NodeUserCPULimit
		if err := rows.Scan(
			&x.NodeID,
			&x.LocalUsername,
			&x.CPUQuotaPercent,
			&x.Reason,
			&x.UpdatedBy,
			&x.UpdatedAt,
		); err != nil {
			return nil, err
		}
		normalizeNodeUserCPULimitTimes(&x)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListNodeUserCPULimits(
	ctx context.Context,
	nodeID string,
	billingUsername string,
	limit int,
) ([]NodeUserCPULimit, error) {
	nodeID = strings.TrimSpace(nodeID)
	billingUsername = strings.TrimSpace(billingUsername)
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	conds := make([]string, 0, 2)
	args := make([]any, 0, 4)
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if nodeID != "" {
		conds = append(conds, "l.node_id="+addArg(nodeID))
	}
	if billingUsername != "" {
		conds = append(conds, "una.billing_username="+addArg(billingUsername))
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	query := `
SELECT l.node_id,
       l.local_username,
       COALESCE(una.billing_username, '') AS billing_username,
       (una.billing_username IS NOT NULL) AS mapping_exists,
       (
         EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM power_users pu WHERE pu.username=una.billing_username)
       ) AS platform_exists,
       l.cpu_quota_percent,
       COALESCE(l.reason, '') AS reason,
       l.updated_by,
       l.updated_at
FROM node_user_cpu_limits l
LEFT JOIN user_node_accounts una
  ON una.node_id=l.node_id
 AND una.local_username=l.local_username
` + where + `
ORDER BY l.updated_at DESC, l.node_id ASC, l.local_username ASC
LIMIT ` + addArg(limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeUserCPULimit, 0)
	for rows.Next() {
		var x NodeUserCPULimit
		if err := rows.Scan(
			&x.NodeID,
			&x.LocalUsername,
			&x.BillingUsername,
			&x.MappingExists,
			&x.PlatformExists,
			&x.CPUQuotaPercent,
			&x.Reason,
			&x.UpdatedBy,
			&x.UpdatedAt,
		); err != nil {
			return nil, err
		}
		normalizeNodeUserCPULimitTimes(&x)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) UpsertNodeUserMemoryLimit(
	ctx context.Context,
	nodeID string,
	localUsername string,
	memoryLimitGB float64,
	reason string,
	updatedBy string,
) (NodeUserMemoryLimit, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	reason = strings.TrimSpace(reason)
	updatedBy = strings.TrimSpace(updatedBy)
	if nodeID == "" || localUsername == "" {
		return NodeUserMemoryLimit{}, errors.New("node_id/local_username 不能为空")
	}
	if memoryLimitGB <= 0 {
		return NodeUserMemoryLimit{}, errors.New("memory_limit_gb 必须大于 0")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO node_user_memory_limits(node_id, local_username, memory_limit_gb, reason, updated_by, updated_at)
VALUES($1,$2,$3,$4,$5,NOW())
ON CONFLICT (node_id, local_username) DO UPDATE
SET memory_limit_gb=EXCLUDED.memory_limit_gb,
    reason=EXCLUDED.reason,
    updated_by=EXCLUDED.updated_by,
    updated_at=NOW()`, nodeID, localUsername, memoryLimitGB, reason, updatedBy); err != nil {
		return NodeUserMemoryLimit{}, err
	}
	row, ok, err := s.GetNodeUserMemoryLimit(ctx, nodeID, localUsername)
	if err != nil {
		return NodeUserMemoryLimit{}, err
	}
	if !ok || row == nil {
		return NodeUserMemoryLimit{}, sql.ErrNoRows
	}
	return *row, nil
}

func (s *Store) DeleteNodeUserMemoryLimit(ctx context.Context, nodeID string, localUsername string) error {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return errors.New("node_id/local_username 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `
DELETE FROM node_user_memory_limits
WHERE node_id=$1 AND local_username=$2`, nodeID, localUsername)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected <= 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetNodeUserMemoryLimit(ctx context.Context, nodeID string, localUsername string) (*NodeUserMemoryLimit, bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return nil, false, errors.New("node_id/local_username 不能为空")
	}
	var out NodeUserMemoryLimit
	if err := s.db.QueryRowContext(ctx, `
SELECT l.node_id,
       l.local_username,
       COALESCE(una.billing_username, '') AS billing_username,
       (una.billing_username IS NOT NULL) AS mapping_exists,
       (
         EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM power_users pu WHERE pu.username=una.billing_username)
       ) AS platform_exists,
       l.memory_limit_gb,
       COALESCE(l.reason, '') AS reason,
       l.updated_by,
       l.updated_at
FROM node_user_memory_limits l
LEFT JOIN user_node_accounts una
  ON una.node_id=l.node_id
 AND una.local_username=l.local_username
WHERE l.node_id=$1 AND l.local_username=$2`, nodeID, localUsername).Scan(
		&out.NodeID,
		&out.LocalUsername,
		&out.BillingUsername,
		&out.MappingExists,
		&out.PlatformExists,
		&out.MemoryLimitGB,
		&out.Reason,
		&out.UpdatedBy,
		&out.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	normalizeNodeUserMemoryLimitTimes(&out)
	return &out, true, nil
}

func (s *Store) ListNodeUserMemoryLimitsByNodeTx(
	ctx context.Context,
	tx *sql.Tx,
	nodeID string,
	limit int,
) ([]NodeUserMemoryLimit, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 10000 {
		limit = 5000
	}
	rows, err := tx.QueryContext(ctx, `
SELECT node_id, local_username, memory_limit_gb, COALESCE(reason, '') AS reason, updated_by, updated_at
FROM node_user_memory_limits
WHERE node_id=$1
ORDER BY updated_at DESC, local_username ASC
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeUserMemoryLimit, 0)
	for rows.Next() {
		var x NodeUserMemoryLimit
		if err := rows.Scan(
			&x.NodeID,
			&x.LocalUsername,
			&x.MemoryLimitGB,
			&x.Reason,
			&x.UpdatedBy,
			&x.UpdatedAt,
		); err != nil {
			return nil, err
		}
		normalizeNodeUserMemoryLimitTimes(&x)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListNodeUserMemoryLimits(
	ctx context.Context,
	nodeID string,
	billingUsername string,
	limit int,
) ([]NodeUserMemoryLimit, error) {
	nodeID = strings.TrimSpace(nodeID)
	billingUsername = strings.TrimSpace(billingUsername)
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	conds := make([]string, 0, 2)
	args := make([]any, 0, 4)
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if nodeID != "" {
		conds = append(conds, "l.node_id="+addArg(nodeID))
	}
	if billingUsername != "" {
		conds = append(conds, "una.billing_username="+addArg(billingUsername))
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	query := `
SELECT l.node_id,
       l.local_username,
       COALESCE(una.billing_username, '') AS billing_username,
       (una.billing_username IS NOT NULL) AS mapping_exists,
       (
         EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM power_users pu WHERE pu.username=una.billing_username)
       ) AS platform_exists,
       l.memory_limit_gb,
       COALESCE(l.reason, '') AS reason,
       l.updated_by,
       l.updated_at
FROM node_user_memory_limits l
LEFT JOIN user_node_accounts una
  ON una.node_id=l.node_id
 AND una.local_username=l.local_username
` + where + `
ORDER BY l.updated_at DESC, l.node_id ASC, l.local_username ASC
LIMIT ` + addArg(limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeUserMemoryLimit, 0)
	for rows.Next() {
		var x NodeUserMemoryLimit
		if err := rows.Scan(
			&x.NodeID,
			&x.LocalUsername,
			&x.BillingUsername,
			&x.MappingExists,
			&x.PlatformExists,
			&x.MemoryLimitGB,
			&x.Reason,
			&x.UpdatedBy,
			&x.UpdatedAt,
		); err != nil {
			return nil, err
		}
		normalizeNodeUserMemoryLimitTimes(&x)
		out = append(out, x)
	}
	return out, rows.Err()
}

func normalizeNodeUserGPUIndices(indices []int) ([]int, error) {
	set := make(map[int]struct{}, len(indices))
	for _, idx := range indices {
		if idx < 0 {
			return nil, fmt.Errorf("gpu_index 不能为负数：%d", idx)
		}
		set[idx] = struct{}{}
	}
	out := make([]int, 0, len(set))
	for idx := range set {
		out = append(out, idx)
	}
	sort.Ints(out)
	return out, nil
}

func (s *Store) UpsertNodeUserGPUVisibility(
	ctx context.Context,
	nodeID string,
	localUsername string,
	gpuIndices []int,
	reason string,
	updatedBy string,
) (NodeUserGPUVisibility, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	reason = strings.TrimSpace(reason)
	updatedBy = strings.TrimSpace(updatedBy)
	if nodeID == "" || localUsername == "" {
		return NodeUserGPUVisibility{}, errors.New("node_id/local_username 不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	normalized, err := normalizeNodeUserGPUIndices(gpuIndices)
	if err != nil {
		return NodeUserGPUVisibility{}, err
	}
	if len(normalized) == 0 {
		return NodeUserGPUVisibility{}, errors.New("gpu_indices 至少包含一个 GPU 编号")
	}
	if err := s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM node_user_gpu_visibility
WHERE node_id=$1 AND local_username=$2`, nodeID, localUsername); err != nil {
			return err
		}
		for _, idx := range normalized {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO node_user_gpu_visibility(node_id, local_username, gpu_index, reason, updated_by, updated_at)
VALUES($1,$2,$3,$4,$5,NOW())`, nodeID, localUsername, idx, reason, updatedBy); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return NodeUserGPUVisibility{}, err
	}
	row, ok, err := s.GetNodeUserGPUVisibility(ctx, nodeID, localUsername)
	if err != nil {
		return NodeUserGPUVisibility{}, err
	}
	if !ok || row == nil {
		return NodeUserGPUVisibility{}, sql.ErrNoRows
	}
	return *row, nil
}

func (s *Store) DeleteNodeUserGPUVisibility(ctx context.Context, nodeID string, localUsername string) error {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return errors.New("node_id/local_username 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `
DELETE FROM node_user_gpu_visibility
WHERE node_id=$1 AND local_username=$2`, nodeID, localUsername)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected <= 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetNodeUserGPUVisibility(ctx context.Context, nodeID string, localUsername string) (*NodeUserGPUVisibility, bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return nil, false, errors.New("node_id/local_username 不能为空")
	}
	var out NodeUserGPUVisibility
	var gpuIndicesRaw pq.Int64Array
	if err := s.db.QueryRowContext(ctx, `
SELECT l.node_id,
       l.local_username,
       COALESCE(una.billing_username, '') AS billing_username,
       (una.billing_username IS NOT NULL) AS mapping_exists,
       (
         EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM power_users pu WHERE pu.username=una.billing_username)
       ) AS platform_exists,
       (adm.username IS NOT NULL) AS admin_mapping,
       COALESCE(adm.username, '') AS admin_username,
       array_agg(l.gpu_index ORDER BY l.gpu_index)::BIGINT[] AS gpu_indices,
       (array_agg(l.reason ORDER BY l.updated_at DESC, l.gpu_index ASC))[1] AS reason,
       (array_agg(l.updated_by ORDER BY l.updated_at DESC, l.gpu_index ASC))[1] AS updated_by,
       MAX(l.updated_at) AS updated_at
FROM node_user_gpu_visibility l
LEFT JOIN user_node_accounts una
  ON una.node_id=l.node_id
 AND una.local_username=l.local_username
LEFT JOIN admin_accounts adm
  ON adm.username=una.billing_username
WHERE l.node_id=$1 AND l.local_username=$2
GROUP BY l.node_id, l.local_username, una.billing_username, adm.username`, nodeID, localUsername).Scan(
		&out.NodeID,
		&out.LocalUsername,
		&out.BillingUsername,
		&out.MappingExists,
		&out.PlatformExists,
		&out.AdminMapping,
		&out.AdminUsername,
		&gpuIndicesRaw,
		&out.Reason,
		&out.UpdatedBy,
		&out.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	out.GPUIndices = make([]int, 0, len(gpuIndicesRaw))
	for _, idx := range gpuIndicesRaw {
		if idx >= 0 {
			out.GPUIndices = append(out.GPUIndices, int(idx))
		}
	}
	sort.Ints(out.GPUIndices)
	normalizeNodeUserGPUVisibilityTimes(&out)
	return &out, true, nil
}

func (s *Store) ListNodeUserGPUVisibilityByNodeTx(
	ctx context.Context,
	tx *sql.Tx,
	nodeID string,
	limit int,
) ([]NodeUserGPUVisibility, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 10000 {
		limit = 5000
	}
	rows, err := tx.QueryContext(ctx, `
SELECT l.node_id,
       l.local_username,
       COALESCE(una.billing_username, '') AS billing_username,
       (una.billing_username IS NOT NULL) AS mapping_exists,
       (
         EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM power_users pu WHERE pu.username=una.billing_username)
       ) AS platform_exists,
       (adm.username IS NOT NULL) AS admin_mapping,
       COALESCE(adm.username, '') AS admin_username,
       array_agg(l.gpu_index ORDER BY l.gpu_index)::BIGINT[] AS gpu_indices,
       (array_agg(l.reason ORDER BY l.updated_at DESC, l.gpu_index ASC))[1] AS reason,
       (array_agg(l.updated_by ORDER BY l.updated_at DESC, l.gpu_index ASC))[1] AS updated_by,
       MAX(l.updated_at) AS updated_at
FROM node_user_gpu_visibility l
LEFT JOIN user_node_accounts una
  ON una.node_id=l.node_id
 AND una.local_username=l.local_username
LEFT JOIN admin_accounts adm
  ON adm.username=una.billing_username
WHERE l.node_id=$1
GROUP BY l.node_id, l.local_username, una.billing_username, adm.username
ORDER BY l.local_username
LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeUserGPUVisibility, 0)
	for rows.Next() {
		var x NodeUserGPUVisibility
		var gpuIndicesRaw pq.Int64Array
		if err := rows.Scan(
			&x.NodeID,
			&x.LocalUsername,
			&x.BillingUsername,
			&x.MappingExists,
			&x.PlatformExists,
			&x.AdminMapping,
			&x.AdminUsername,
			&gpuIndicesRaw,
			&x.Reason,
			&x.UpdatedBy,
			&x.UpdatedAt,
		); err != nil {
			return nil, err
		}
		x.GPUIndices = make([]int, 0, len(gpuIndicesRaw))
		for _, idx := range gpuIndicesRaw {
			if idx >= 0 {
				x.GPUIndices = append(x.GPUIndices, int(idx))
			}
		}
		sort.Ints(x.GPUIndices)
		normalizeNodeUserGPUVisibilityTimes(&x)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListNodeUserGPUVisibility(
	ctx context.Context,
	nodeID string,
	billingUsername string,
	limit int,
) ([]NodeUserGPUVisibility, error) {
	nodeID = strings.TrimSpace(nodeID)
	billingUsername = strings.TrimSpace(billingUsername)
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	conds := make([]string, 0, 2)
	args := make([]any, 0, 4)
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if nodeID != "" {
		conds = append(conds, "l.node_id="+addArg(nodeID))
	}
	if billingUsername != "" {
		conds = append(conds, "una.billing_username="+addArg(billingUsername))
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	query := `
SELECT l.node_id,
       l.local_username,
       COALESCE(una.billing_username, '') AS billing_username,
       (una.billing_username IS NOT NULL) AS mapping_exists,
       (
         EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM power_users pu WHERE pu.username=una.billing_username)
       ) AS platform_exists,
       (adm.username IS NOT NULL) AS admin_mapping,
       COALESCE(adm.username, '') AS admin_username,
       array_agg(l.gpu_index ORDER BY l.gpu_index)::BIGINT[] AS gpu_indices,
       (array_agg(l.reason ORDER BY l.updated_at DESC, l.gpu_index ASC))[1] AS reason,
       (array_agg(l.updated_by ORDER BY l.updated_at DESC, l.gpu_index ASC))[1] AS updated_by,
       MAX(l.updated_at) AS updated_at
FROM node_user_gpu_visibility l
LEFT JOIN user_node_accounts una
  ON una.node_id=l.node_id
 AND una.local_username=l.local_username
LEFT JOIN admin_accounts adm
  ON adm.username=una.billing_username
` + where + `
GROUP BY l.node_id, l.local_username, una.billing_username, adm.username
ORDER BY MAX(l.updated_at) DESC, l.node_id ASC, l.local_username ASC
LIMIT ` + addArg(limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeUserGPUVisibility, 0)
	for rows.Next() {
		var x NodeUserGPUVisibility
		var gpuIndicesRaw pq.Int64Array
		if err := rows.Scan(
			&x.NodeID,
			&x.LocalUsername,
			&x.BillingUsername,
			&x.MappingExists,
			&x.PlatformExists,
			&x.AdminMapping,
			&x.AdminUsername,
			&gpuIndicesRaw,
			&x.Reason,
			&x.UpdatedBy,
			&x.UpdatedAt,
		); err != nil {
			return nil, err
		}
		x.GPUIndices = make([]int, 0, len(gpuIndicesRaw))
		for _, idx := range gpuIndicesRaw {
			if idx >= 0 {
				x.GPUIndices = append(x.GPUIndices, int(idx))
			}
		}
		sort.Ints(x.GPUIndices)
		normalizeNodeUserGPUVisibilityTimes(&x)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) InsertNodeSecurityEventTx(
	ctx context.Context,
	tx *sql.Tx,
	reportID string,
	nodeID string,
	eventType string,
	severity string,
	reason string,
	relatedUsernames []string,
	details any,
	cooldown time.Duration,
) (bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	eventType = strings.TrimSpace(eventType)
	severity = strings.TrimSpace(severity)
	reason = strings.TrimSpace(reason)
	reportID = strings.TrimSpace(reportID)
	if nodeID == "" || eventType == "" {
		return false, errors.New("node_id/event_type 不能为空")
	}
	if severity == "" {
		severity = "info"
	}
	if cooldown > 0 {
		var lastAt sql.NullTime
		if err := tx.QueryRowContext(ctx, `
SELECT MAX(created_at)
FROM node_security_events
WHERE node_id=$1 AND event_type=$2`, nodeID, eventType).Scan(&lastAt); err != nil {
			return false, err
		}
		if lastAt.Valid && time.Since(lastAt.Time) < cooldown {
			return false, nil
		}
	}
	detailsJSON := []byte("{}")
	if details != nil {
		v, err := json.Marshal(details)
		if err != nil {
			return false, err
		}
		if len(v) > 0 {
			detailsJSON = v
		}
	}
	users := uniqTrim(relatedUsernames)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO node_security_events(report_id, node_id, event_type, severity, reason, related_usernames, details, created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,NOW())`,
		reportID, nodeID, eventType, severity, reason, pq.Array(users), string(detailsJSON),
	); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ListNodeSecurityEvents(
	ctx context.Context,
	nodeID string,
	eventType string,
	from *time.Time,
	to *time.Time,
	limit int,
) ([]NodeSecurityEvent, error) {
	nodeID = strings.TrimSpace(nodeID)
	eventType = strings.TrimSpace(eventType)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 2000 {
		limit = 200
	}
	baseSQL := `
SELECT event_id, report_id, node_id, event_type, severity, reason, related_usernames, details::text, created_at
FROM node_security_events
WHERE node_id=$1`
	args := []any{nodeID}
	if eventType != "" {
		baseSQL += ` AND event_type=$2`
		args = append(args, eventType)
	}
	if from != nil {
		baseSQL += ` AND created_at >= $` + strconv.Itoa(len(args)+1)
		args = append(args, *from)
	}
	if to != nil {
		baseSQL += ` AND created_at <= $` + strconv.Itoa(len(args)+1)
		args = append(args, *to)
	}
	baseSQL += ` ORDER BY created_at DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, baseSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeSecurityEvent, 0)
	for rows.Next() {
		var e NodeSecurityEvent
		if err := rows.Scan(
			&e.EventID,
			&e.ReportID,
			&e.NodeID,
			&e.EventType,
			&e.Severity,
			&e.Reason,
			pq.Array(&e.RelatedUsernames),
			&e.Details,
			&e.CreatedAt,
		); err != nil {
			return nil, err
		}
		normalizeNodeSecurityEventTimes(&e)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ListNodeSecurityEventSummaries(
	ctx context.Context,
	nodeID string,
	eventType string,
	from *time.Time,
	to *time.Time,
	limit int,
) ([]NodeSecurityEventSummary, error) {
	nodeID = strings.TrimSpace(nodeID)
	eventType = strings.TrimSpace(eventType)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	baseSQL := `
WITH filtered AS (
  SELECT event_id, event_type, severity, reason, related_usernames, created_at
  FROM node_security_events
  WHERE node_id=$1`
	args := []any{nodeID}
	if eventType != "" {
		baseSQL += ` AND event_type=$2`
		args = append(args, eventType)
	}
	if from != nil {
		baseSQL += ` AND created_at >= $` + strconv.Itoa(len(args)+1)
		args = append(args, *from)
	}
	if to != nil {
		baseSQL += ` AND created_at <= $` + strconv.Itoa(len(args)+1)
		args = append(args, *to)
	}
	baseSQL += `
)
SELECT f.event_type,
       f.severity,
       LEFT(REGEXP_REPLACE(COALESCE(f.reason, ''), '\d+(\.\d+)?', '#', 'g'), 120) AS normalized_reason,
       COUNT(DISTINCT f.event_id) AS event_count,
       COUNT(DISTINCT NULLIF(TRIM(u.username), '')) AS affected_users,
       MIN(f.created_at) AS first_seen_at,
       MAX(f.created_at) AS last_seen_at
FROM filtered f
LEFT JOIN LATERAL unnest(f.related_usernames) AS u(username) ON TRUE
GROUP BY f.event_type, f.severity, LEFT(REGEXP_REPLACE(COALESCE(f.reason, ''), '\d+(\.\d+)?', '#', 'g'), 120)
ORDER BY event_count DESC, last_seen_at DESC
LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, baseSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeSecurityEventSummary, 0)
	for rows.Next() {
		var x NodeSecurityEventSummary
		if err := rows.Scan(
			&x.EventType,
			&x.Severity,
			&x.NormalizedReason,
			&x.EventCount,
			&x.AffectedUsers,
			&x.FirstSeenAt,
			&x.LastSeenAt,
		); err != nil {
			return nil, err
		}
		normalizeNodeSecurityEventSummaryTimes(&x)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListNodeSuspiciousUsers(ctx context.Context, nodeID string, days int, limit int) ([]NodeSuspiciousUser, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if days <= 0 || days > 365 {
		days = 7
	}
	if limit <= 0 || limit > 2000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
WITH expanded AS (
  SELECT node_id, event_type, reason, created_at, unnest(related_usernames) AS username
  FROM node_security_events
  WHERE node_id=$1
    AND created_at >= NOW() - ($2::text || ' days')::interval
)
SELECT node_id,
       username,
       COUNT(1) AS hit_count,
       MAX(created_at) AS last_seen_at,
       STRING_AGG(DISTINCT reason, '；') AS reason_hints,
       STRING_AGG(
         DISTINCT CASE event_type
           WHEN 'suspected_mining' THEN '疑似挖矿'
           WHEN 'high_cpu_load' THEN '异常高CPU占用'
           WHEN 'ssh_failed_login_spike' THEN 'SSH失败登录峰值'
           WHEN 'ssh_bruteforce' THEN 'SSH爆破'
           WHEN 'abnormal_port_scan' THEN '端口扫描'
           WHEN 'disk_full_risk' THEN '磁盘打满风险'
           ELSE '其他异常'
         END,
         '、'
       ) AS phenomena,
       BOOL_OR(event_type='suspected_mining') AS mining_suspected
FROM expanded
WHERE COALESCE(username, '') <> ''
GROUP BY node_id, username
ORDER BY last_seen_at DESC
LIMIT $3`, nodeID, days, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeSuspiciousUser, 0)
	for rows.Next() {
		var u NodeSuspiciousUser
		if err := rows.Scan(&u.NodeID, &u.Username, &u.HitCount, &u.LastSeenAt, &u.ReasonHints, &u.Phenomena, &u.MiningSuspected); err != nil {
			return nil, err
		}
		normalizeNodeSuspiciousUserTimes(&u)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) ListNodeSuspiciousUsersByRange(ctx context.Context, nodeID string, from time.Time, to time.Time, limit int) ([]NodeSuspiciousUser, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if to.Before(from) {
		return nil, errors.New("to 不能早于 from")
	}
	if limit <= 0 || limit > 2000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
WITH expanded AS (
  SELECT node_id, event_type, reason, created_at, unnest(related_usernames) AS username
  FROM node_security_events
  WHERE node_id=$1
    AND created_at >= $2
    AND created_at <= $3
)
SELECT node_id,
       username,
       COUNT(1) AS hit_count,
       MAX(created_at) AS last_seen_at,
       STRING_AGG(DISTINCT reason, '；') AS reason_hints,
       STRING_AGG(
         DISTINCT CASE event_type
           WHEN 'suspected_mining' THEN '疑似挖矿'
           WHEN 'high_cpu_load' THEN '异常高CPU占用'
           WHEN 'ssh_failed_login_spike' THEN 'SSH失败登录峰值'
           WHEN 'ssh_bruteforce' THEN 'SSH爆破'
           WHEN 'abnormal_port_scan' THEN '端口扫描'
           WHEN 'disk_full_risk' THEN '磁盘打满风险'
           ELSE '其他异常'
         END,
         '、'
       ) AS phenomena,
       BOOL_OR(event_type='suspected_mining') AS mining_suspected
FROM expanded
WHERE COALESCE(username, '') <> ''
GROUP BY node_id, username
ORDER BY last_seen_at DESC
LIMIT $4`, nodeID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeSuspiciousUser, 0)
	for rows.Next() {
		var u NodeSuspiciousUser
		if err := rows.Scan(&u.NodeID, &u.Username, &u.HitCount, &u.LastSeenAt, &u.ReasonHints, &u.Phenomena, &u.MiningSuspected); err != nil {
			return nil, err
		}
		normalizeNodeSuspiciousUserTimes(&u)
		out = append(out, u)
	}
	return out, rows.Err()
}

func uniqTrim(items []string) []string {
	set := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, raw := range items {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := set[v]; ok {
			continue
		}
		set[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func normalizeNullStringArray(items []sql.NullString) []string {
	if len(items) == 0 {
		return nil
	}
	raw := make([]string, 0, len(items))
	for _, it := range items {
		if !it.Valid {
			continue
		}
		v := strings.TrimSpace(it.String)
		if v == "" {
			continue
		}
		raw = append(raw, v)
	}
	return uniqTrim(raw)
}

func (s *Store) UpsertNodeRuntimeSnapshotTx(
	ctx context.Context,
	tx *sql.Tx,
	reportID string,
	nodeID string,
	reportTS time.Time,
	cpuPercentSum float64,
	memoryMBSum float64,
	gpuProcCount int,
	cpuProcCount int,
	sshUsers []string,
	costTotal float64,
) error {
	reportID = strings.TrimSpace(reportID)
	nodeID = strings.TrimSpace(nodeID)
	if reportID == "" || nodeID == "" {
		return errors.New("report_id/node_id 不能为空")
	}
	if gpuProcCount < 0 {
		gpuProcCount = 0
	}
	if cpuProcCount < 0 {
		cpuProcCount = 0
	}
	users := uniqTrim(sshUsers)
	raw, err := json.Marshal(users)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO node_runtime_snapshots(
  report_id, node_id, report_ts, cpu_percent_sum, memory_mb_sum,
  gpu_process_count, cpu_process_count, ssh_user_count, ssh_users_json, cost_total
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)
ON CONFLICT (report_id) DO UPDATE SET
  node_id=EXCLUDED.node_id,
  report_ts=EXCLUDED.report_ts,
  cpu_percent_sum=EXCLUDED.cpu_percent_sum,
  memory_mb_sum=EXCLUDED.memory_mb_sum,
  gpu_process_count=EXCLUDED.gpu_process_count,
  cpu_process_count=EXCLUDED.cpu_process_count,
  ssh_user_count=EXCLUDED.ssh_user_count,
  ssh_users_json=EXCLUDED.ssh_users_json,
  cost_total=EXCLUDED.cost_total`,
		reportID, nodeID, reportTS, cpuPercentSum, memoryMBSum,
		gpuProcCount, cpuProcCount, len(users), string(raw), costTotal,
	)
	return err
}

func (s *Store) ListNodeRuntimeSnapshots(ctx context.Context, nodeID string, from time.Time, to time.Time, limit int) ([]NodeRuntimeSnapshot, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT report_id, node_id, report_ts, cpu_percent_sum, memory_mb_sum,
       gpu_process_count, cpu_process_count, ssh_user_count, ssh_users_json::text, cost_total
FROM node_runtime_snapshots
WHERE node_id=$1 AND report_ts >= $2 AND report_ts <= $3
ORDER BY report_ts DESC
LIMIT $4`, nodeID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeRuntimeSnapshot, 0)
	for rows.Next() {
		var x NodeRuntimeSnapshot
		var sshUsersRaw string
		if err := rows.Scan(
			&x.ReportID, &x.NodeID, &x.ReportTS, &x.CPUPercentSum, &x.MemoryMBSum,
			&x.GPUProcessCount, &x.CPUProcessCount, &x.SSHUserCount, &sshUsersRaw, &x.CostTotal,
		); err != nil {
			return nil, err
		}
		if strings.TrimSpace(sshUsersRaw) != "" {
			_ = json.Unmarshal([]byte(sshUsersRaw), &x.SSHUsers)
		}
		normalizeNodeRuntimeSnapshotTimes(&x)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) CountAdminAccounts(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM admin_accounts`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) CreateAdminAccount(ctx context.Context, username string, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if err := validateStrongPassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		uid, _, err := s.allocatePlatformUIDTx(ctx, tx, time.Now(), nil)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `
INSERT INTO admin_accounts(username, password_hash, platform_uid)
VALUES($1,$2,$3)
ON CONFLICT (username) DO NOTHING`, username, string(hash), uid)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			_, err = s.EnsureUserPlatformUIDTx(ctx, tx, username)
			return err
		}
		_, err = s.EnsureUserPlatformUIDTx(ctx, tx, username)
		return err
	})
}

func (s *Store) VerifyAdminPassword(ctx context.Context, username string, password string) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, errors.New("username 不能为空")
	}
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM admin_accounts WHERE username=$1`, username).Scan(&hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return false, nil
	}
	return true, nil
}

func (s *Store) CreatePowerUser(ctx context.Context, username string, password string, canViewBoard bool, canViewNodes bool, canManageNodes bool, canManagePoints bool, canReview bool, createdBy string) error {
	username = strings.TrimSpace(username)
	createdBy = strings.TrimSpace(createdBy)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if err := validateStrongPassword(password); err != nil {
		return err
	}
	if canManageNodes {
		canViewNodes = true
	}
	if createdBy == "" {
		createdBy = "admin"
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return errors.New("该用户名已存在于平台账号，请使用“提升平台用户为高级用户”")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return errors.New("该用户名已被管理员账号占用")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM power_users WHERE username=$1)`, username).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return errors.New("高级用户已存在")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM registration_requests WHERE username=$1 AND status='pending')`, username).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return errors.New("该用户名已被待审核注册申请占用")
	}
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM registration_email_verifications
  WHERE consumed_at IS NULL AND expire_at > NOW() AND username=$1
)`, username).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return errors.New("该用户名已被待验证注册申请占用")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		uid, _, err := s.allocatePlatformUIDTx(ctx, tx, time.Now(), nil)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO power_users(username, password_hash, platform_uid, can_view_board, can_view_nodes, can_manage_nodes, can_manage_points, can_review_requests, created_by, updated_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)`, username, string(hash), uid, canViewBoard, canViewNodes, canManageNodes, canManagePoints, canReview, createdBy)
		if err != nil {
			return err
		}
		_, err = s.EnsureUserPlatformUIDTx(ctx, tx, username)
		return err
	})
}

func (s *Store) VerifyPowerUserPassword(ctx context.Context, username string, password string) (PowerUser, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return PowerUser{}, false, errors.New("username 不能为空")
	}
	var hash string
	var uidRaw sql.NullInt64
	var out PowerUser
	err := s.db.QueryRowContext(ctx, `
SELECT pu.password_hash,
       pu.username,
       COALESCE(ua.platform_uid, pu.platform_uid) AS platform_uid,
       pu.can_view_board,
       pu.can_view_nodes,
       pu.can_manage_nodes,
       COALESCE(pu.can_manage_points,false),
       pu.can_review_requests,
       pu.created_by,
       pu.updated_by,
       pu.last_login_at,
       pu.created_at,
       pu.updated_at
FROM power_users pu
LEFT JOIN user_accounts ua ON ua.username=pu.username
WHERE pu.username=$1`, username).Scan(
		&hash, &out.Username, &uidRaw, &out.CanViewBoard, &out.CanViewNodes, &out.CanManageNodes, &out.CanManagePoints, &out.CanReviewRequests, &out.CreatedBy, &out.UpdatedBy, &out.LastLoginAt, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PowerUser{}, false, nil
		}
		return PowerUser{}, false, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return PowerUser{}, false, nil
	}
	if uidRaw.Valid && uidRaw.Int64 > 0 {
		v := int(uidRaw.Int64)
		out.PlatformUID = &v
	}
	return out, true, nil
}

func (s *Store) ListPowerUsers(ctx context.Context, limit int) ([]PowerUser, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT pu.username,
       COALESCE(ua.platform_uid, pu.platform_uid) AS platform_uid,
       (ua.username IS NOT NULL) AS is_platform_user,
       pu.can_view_board,
       pu.can_view_nodes,
       pu.can_manage_nodes,
       COALESCE(pu.can_manage_points,false) AS can_manage_points,
       pu.can_review_requests,
       pu.created_by,
       pu.updated_by,
       pu.last_login_at,
       pu.created_at,
       pu.updated_at
FROM power_users pu
LEFT JOIN user_accounts ua ON ua.username=pu.username
ORDER BY pu.username
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PowerUser, 0)
	for rows.Next() {
		var p PowerUser
		var uidRaw sql.NullInt64
		if err := rows.Scan(&p.Username, &uidRaw, &p.IsPlatformUser, &p.CanViewBoard, &p.CanViewNodes, &p.CanManageNodes, &p.CanManagePoints, &p.CanReviewRequests, &p.CreatedBy, &p.UpdatedBy, &p.LastLoginAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if uidRaw.Valid && uidRaw.Int64 > 0 {
			v := int(uidRaw.Int64)
			p.PlatformUID = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) PromotePlatformUserToPowerUser(ctx context.Context, username string, canViewBoard bool, canViewNodes bool, canManageNodes bool, canManagePoints bool, canReview bool, updatedBy string) error {
	username = strings.TrimSpace(username)
	updatedBy = strings.TrimSpace(updatedBy)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	if canManageNodes {
		canViewNodes = true
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return errors.New("管理员账号不能提升为高级用户")
		}
		var pwdHash string
		if err := tx.QueryRowContext(ctx, `SELECT password_hash FROM user_accounts WHERE username=$1 FOR UPDATE`, username).Scan(&pwdHash); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("该平台用户不存在")
			}
			return err
		}
		platformUID, err := s.EnsureUserPlatformUIDTx(ctx, tx, username)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO power_users(username, password_hash, platform_uid, can_view_board, can_view_nodes, can_manage_nodes, can_manage_points, can_review_requests, created_by, updated_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)
ON CONFLICT (username) DO UPDATE SET
  password_hash=EXCLUDED.password_hash,
  platform_uid=EXCLUDED.platform_uid,
  can_view_board=EXCLUDED.can_view_board,
  can_view_nodes=EXCLUDED.can_view_nodes,
  can_manage_nodes=EXCLUDED.can_manage_nodes,
  can_manage_points=EXCLUDED.can_manage_points,
  can_review_requests=EXCLUDED.can_review_requests,
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()`, username, pwdHash, platformUID, canViewBoard, canViewNodes, canManageNodes, canManagePoints, canReview, updatedBy); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE user_accounts
SET role='power_user', updated_at=NOW()
WHERE username=$1`, username); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) DemotePowerUserToPlatformUser(ctx context.Context, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM power_users WHERE username=$1`, username)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return sql.ErrNoRows
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE user_accounts
SET role='user', updated_at=NOW()
WHERE username=$1`, username); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) UpdatePowerUserPermissions(ctx context.Context, username string, canViewBoard bool, canViewNodes bool, canManageNodes bool, canManagePoints bool, canReview bool, updatedBy string) error {
	username = strings.TrimSpace(username)
	updatedBy = strings.TrimSpace(updatedBy)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	if canManageNodes {
		canViewNodes = true
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE power_users
SET can_view_board=$2, can_view_nodes=$3, can_manage_nodes=$4, can_manage_points=$5, can_review_requests=$6, updated_by=$7, updated_at=NOW()
WHERE username=$1`, username, canViewBoard, canViewNodes, canManageNodes, canManagePoints, canReview, updatedBy)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ResolveSessionRolePerms(ctx context.Context, username string) (string, uint32, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", 0, false, errors.New("username 不能为空")
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
		return "", 0, false, err
	}
	if exists {
		return "admin", uint32(permViewBoard | permViewNodes | permManageNodes | permReviewRequests | permManagePoints), true, nil
	}
	var canViewBoard bool
	var canViewNodes bool
	var canManageNodes bool
	var canManagePoints bool
	var canReview bool
	err := s.db.QueryRowContext(ctx, `
SELECT can_view_board, can_view_nodes, COALESCE(can_manage_nodes,false), COALESCE(can_manage_points,false), can_review_requests
FROM power_users
WHERE username=$1`, username).Scan(&canViewBoard, &canViewNodes, &canManageNodes, &canManagePoints, &canReview)
	if err == nil {
		perms := uint32(0)
		if canViewBoard {
			perms |= permViewBoard
		}
		if canViewNodes {
			perms |= permViewNodes
		}
		if canManageNodes {
			perms |= permManageNodes
		}
		if canManagePoints {
			perms |= permManagePoints
		}
		if canReview {
			perms |= permReviewRequests
		}
		return "power_user", perms, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
		return "", 0, false, err
	}
	if exists {
		return "user", 0, true, nil
	}
	return "", 0, false, nil
}

func (s *Store) DeletePowerUser(ctx context.Context, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM power_users WHERE username=$1`, username)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) resolveInitialGeneralBalanceTx(
	ctx context.Context,
	tx *sql.Tx,
	username string,
	studentID string,
	fallback float64,
) (float64, error) {
	username = strings.TrimSpace(username)
	studentID = strings.TrimSpace(studentID)
	if username == "" {
		return fallback, errors.New("username 不能为空")
	}
	cfg, err := s.LoadMonthlyPointsConfigTx(ctx, tx)
	if err != nil {
		return fallback, err
	}
	initial := monthlyBasePointsByStudentID(studentID, cfg)
	specialMap, err := s.LoadSpecialMonthlyPointsMapTx(ctx, tx)
	if err != nil {
		return fallback, err
	}
	if v, ok := specialMap[username]; ok {
		initial = v
	}
	return initial, nil
}

func platformUIDInventoryCandidates() []string {
	out := make([]string, 0, len(platformUIDInventoryFiles)+1)
	if envPath := strings.TrimSpace(os.Getenv("PLATFORM_UID_RESERVED_FILE")); envPath != "" {
		out = append(out, envPath)
	}
	for _, p := range platformUIDInventoryFiles {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		dup := false
		for _, existing := range out {
			if existing == p {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	return out
}

func decorateDeletedUserAccountUIDRelease(out *DeletedUserAccount, now time.Time) {
	if out == nil {
		return
	}
	if out.PlatformUID == nil || *out.PlatformUID <= 0 || out.RestoredAt != nil {
		out.UIDReleaseAt = nil
		out.UIDReleaseRemainingSec = nil
		return
	}
	releaseAt := out.DeletedAt.AddDate(platformUIDReleaseRetentionYears, 0, 0)
	out.UIDReleaseAt = &releaseAt
	remainingSec := int64(0)
	if releaseAt.After(now) {
		remainingSec = int64(releaseAt.Sub(now) / time.Second)
	}
	out.UIDReleaseRemainingSec = &remainingSec
}

func loadReservedUIDsFromInventoryFiles() map[int]struct{} {
	out := map[int]struct{}{}
	for _, p := range platformUIDInventoryCandidates() {
		absPath := p
		if !filepath.IsAbs(absPath) {
			if resolved, err := filepath.Abs(absPath); err == nil {
				absPath = resolved
			}
		}
		raw, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		for _, ln := range strings.Split(string(raw), "\n") {
			line := strings.TrimSpace(ln)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			id, err := strconv.Atoi(strings.TrimSpace(fields[0]))
			if err != nil || id <= 0 {
				continue
			}
			out[id] = struct{}{}
		}
	}
	return out
}

func nextAvailablePlatformUID(used map[int]struct{}) (int, bool) {
	if used == nil {
		used = map[int]struct{}{}
	}
	ranges := [][2]int{
		{platformUIDPreferredMin, platformUIDPreferredMax},
		{platformUIDFallbackMin, platformUIDPreferredMin - 1},
		{platformUIDPreferredMax + 1, platformUIDHardMax},
	}
	for _, rg := range ranges {
		start, end := rg[0], rg[1]
		if start > end {
			continue
		}
		if start < platformUIDFallbackMin {
			start = platformUIDFallbackMin
		}
		for id := start; id <= end; id++ {
			if _, sensitive := platformUIDSensitiveValues[id]; sensitive {
				continue
			}
			if _, occupied := used[id]; occupied {
				continue
			}
			return id, true
		}
	}
	return 0, false
}

func (s *Store) lockPlatformUIDTablesTx(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `LOCK TABLE user_accounts IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `LOCK TABLE power_users IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `LOCK TABLE admin_accounts IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return err
	}
	return nil
}

func (s *Store) isPlatformUIDOccupiedByOtherTx(ctx context.Context, tx *sql.Tx, uid int, username string) (bool, error) {
	if uid <= 0 {
		return false, nil
	}
	var occupied bool
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM user_accounts WHERE platform_uid=$1 AND username<>$2
  UNION ALL
  SELECT 1 FROM power_users WHERE platform_uid=$1 AND username<>$2
  UNION ALL
  SELECT 1 FROM admin_accounts WHERE platform_uid=$1 AND username<>$2
)`, uid, username).Scan(&occupied); err != nil {
		return false, err
	}
	return occupied, nil
}

func (s *Store) collectUsedPlatformUIDsTx(ctx context.Context, tx *sql.Tx, now time.Time) (map[int]struct{}, error) {
	used := map[int]struct{}{}
	for id := range platformUIDSensitiveValues {
		used[id] = struct{}{}
	}
	for id := range loadReservedUIDsFromInventoryFiles() {
		used[id] = struct{}{}
	}

	rows, err := tx.QueryContext(ctx, `
SELECT platform_uid FROM user_accounts WHERE platform_uid IS NOT NULL
UNION
SELECT platform_uid FROM power_users WHERE platform_uid IS NOT NULL
UNION
SELECT platform_uid FROM admin_accounts WHERE platform_uid IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if id.Valid && id.Int64 > 0 {
			used[int(id.Int64)] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	cutoff := now.AddDate(-platformUIDReleaseRetentionYears, 0, 0)
	rows, err = tx.QueryContext(ctx, `
SELECT platform_uid
FROM deleted_user_accounts
WHERE platform_uid IS NOT NULL
  AND restored_at IS NULL
  AND deleted_at > $1`, cutoff)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if id.Valid && id.Int64 > 0 {
			used[int(id.Int64)] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = tx.QueryContext(ctx, `
SELECT uid
FROM node_local_users
WHERE uid IS NOT NULL
UNION
SELECT primary_gid
FROM node_local_users
WHERE primary_gid IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if id.Valid && id.Int64 > 0 {
			used[int(id.Int64)] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return used, nil
}

func (s *Store) allocatePlatformUIDTx(ctx context.Context, tx *sql.Tx, now time.Time, used map[int]struct{}) (int, map[int]struct{}, error) {
	if err := s.lockPlatformUIDTablesTx(ctx, tx); err != nil {
		return 0, nil, err
	}
	if used == nil {
		var err error
		used, err = s.collectUsedPlatformUIDsTx(ctx, tx, now)
		if err != nil {
			return 0, nil, err
		}
	}
	uid, ok := nextAvailablePlatformUID(used)
	if !ok {
		return 0, nil, errors.New("无法分配可用 platform_uid：可用 UID 已耗尽")
	}
	used[uid] = struct{}{}
	return uid, used, nil
}

func (s *Store) ensureAnyAccountPlatformUIDTx(ctx context.Context, tx *sql.Tx, username string) (int, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return 0, false, errors.New("username 不能为空")
	}
	if err := s.lockPlatformUIDTablesTx(ctx, tx); err != nil {
		return 0, false, err
	}

	type accountUIDRow struct {
		exists bool
		uid    sql.NullInt64
	}

	readRow := func(query string) (accountUIDRow, error) {
		var out accountUIDRow
		err := tx.QueryRowContext(ctx, query, username).Scan(&out.uid)
		if errors.Is(err, sql.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out.exists = true
		return out, nil
	}

	userRow, err := readRow(`
SELECT platform_uid
FROM user_accounts
WHERE username=$1
FOR UPDATE`)
	if err != nil {
		return 0, false, err
	}
	powerRow, err := readRow(`
SELECT platform_uid
FROM power_users
WHERE username=$1
FOR UPDATE`)
	if err != nil {
		return 0, false, err
	}
	adminRow, err := readRow(`
SELECT platform_uid
FROM admin_accounts
WHERE username=$1
FOR UPDATE`)
	if err != nil {
		return 0, false, err
	}

	if !userRow.exists && !powerRow.exists && !adminRow.exists {
		return 0, false, sql.ErrNoRows
	}

	targetUID := 0
	if userRow.uid.Valid && userRow.uid.Int64 > 0 {
		targetUID = int(userRow.uid.Int64)
	} else if powerRow.uid.Valid && powerRow.uid.Int64 > 0 {
		targetUID = int(powerRow.uid.Int64)
	} else if adminRow.uid.Valid && adminRow.uid.Int64 > 0 {
		targetUID = int(adminRow.uid.Int64)
	}
	if _, sensitive := platformUIDSensitiveValues[targetUID]; sensitive {
		targetUID = 0
	}
	if targetUID > 0 {
		occupied, err := s.isPlatformUIDOccupiedByOtherTx(ctx, tx, targetUID, username)
		if err != nil {
			return 0, false, err
		}
		if occupied {
			targetUID = 0
		}
	}
	if targetUID == 0 {
		uid, _, err := s.allocatePlatformUIDTx(ctx, tx, time.Now(), nil)
		if err != nil {
			return 0, false, err
		}
		targetUID = uid
	}

	updated := false
	apply := func(table string) error {
		res, err := tx.ExecContext(ctx, fmt.Sprintf(`
UPDATE %s
SET platform_uid=$2,
    updated_at=NOW()
WHERE username=$1
  AND platform_uid IS DISTINCT FROM $2`, table), username, targetUID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n > 0 {
			updated = true
		}
		return nil
	}
	if userRow.exists {
		if err := apply("user_accounts"); err != nil {
			return 0, false, err
		}
	}
	if powerRow.exists {
		if err := apply("power_users"); err != nil {
			return 0, false, err
		}
	}
	if adminRow.exists {
		if err := apply("admin_accounts"); err != nil {
			return 0, false, err
		}
	}
	return targetUID, updated, nil
}

func (s *Store) EnsureUserPlatformUIDTx(ctx context.Context, tx *sql.Tx, username string) (int, error) {
	uid, _, err := s.ensureAnyAccountPlatformUIDTx(ctx, tx, username)
	if err != nil {
		return 0, err
	}
	return uid, nil
}

func (s *Store) EnsureUserPlatformUID(ctx context.Context, username string) (int, error) {
	var uid int
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var txErr error
		uid, txErr = s.EnsureUserPlatformUIDTx(ctx, tx, username)
		return txErr
	})
	if err != nil {
		return 0, err
	}
	return uid, nil
}

func (s *Store) BackfillMissingPlatformUIDs(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 200000 {
		limit = 200000
	}
	assigned := 0
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.lockPlatformUIDTablesTx(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `
WITH account_uids AS (
  SELECT username, platform_uid FROM user_accounts
  UNION ALL
  SELECT username, platform_uid FROM power_users
  UNION ALL
  SELECT username, platform_uid FROM admin_accounts
),
dup_uids AS (
  SELECT platform_uid
  FROM account_uids
  WHERE platform_uid IS NOT NULL
    AND platform_uid > 0
  GROUP BY platform_uid
  HAVING COUNT(DISTINCT username) > 1
),
need_fix AS (
  SELECT au.username
  FROM account_uids au
  GROUP BY au.username
  HAVING COUNT(*) FILTER (WHERE au.platform_uid IS NULL OR au.platform_uid <= 0) > 0
     OR COUNT(DISTINCT au.platform_uid) FILTER (WHERE au.platform_uid IS NOT NULL AND au.platform_uid > 0) > 1
     OR BOOL_OR(au.platform_uid IN (65534, 65535))
     OR BOOL_OR(au.platform_uid IN (SELECT platform_uid FROM dup_uids))
)
SELECT username
FROM need_fix
ORDER BY username
LIMIT $1`, limit)
		if err != nil {
			return err
		}
		usernames := make([]string, 0)
		for rows.Next() {
			var username string
			if err := rows.Scan(&username); err != nil {
				_ = rows.Close()
				return err
			}
			username = strings.TrimSpace(username)
			if username != "" {
				usernames = append(usernames, username)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(usernames) == 0 {
			return nil
		}
		for _, username := range usernames {
			_, changed, err := s.ensureAnyAccountPlatformUIDTx(ctx, tx, username)
			if err != nil {
				return err
			}
			if changed {
				assigned++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return assigned, nil
}

func (s *Store) BackfillMissingDeletedPlatformUIDs(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 200000 {
		limit = 200000
	}
	assigned := 0
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.lockPlatformUIDTablesTx(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `LOCK TABLE deleted_user_accounts IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, `
SELECT deleted_id
FROM deleted_user_accounts
WHERE restored_at IS NULL
  AND platform_uid IS NULL
ORDER BY deleted_at ASC, deleted_id ASC
LIMIT $1
FOR UPDATE`, limit)
		if err != nil {
			return err
		}
		ids := make([]int64, 0)
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			if id > 0 {
				ids = append(ids, id)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}

		used, err := s.collectUsedPlatformUIDsTx(ctx, tx, time.Now())
		if err != nil {
			return err
		}
		for _, id := range ids {
			uid, nextUsed, err := s.allocatePlatformUIDTx(ctx, tx, time.Now(), used)
			if err != nil {
				return err
			}
			used = nextUsed
			res, err := tx.ExecContext(ctx, `
UPDATE deleted_user_accounts
SET platform_uid=$2
WHERE deleted_id=$1
  AND restored_at IS NULL
  AND platform_uid IS NULL`, id, uid)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n > 0 {
				assigned++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return assigned, nil
}

func (s *Store) CreateUserAccountTx(ctx context.Context, tx *sql.Tx, in UserAccount, password string, defaultBalance float64) error {
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	in.RealName = strings.TrimSpace(in.RealName)
	in.StudentID = strings.TrimSpace(in.StudentID)
	in.Advisor = strings.TrimSpace(in.Advisor)
	in.Phone = strings.TrimSpace(in.Phone)
	if in.Username == "" || in.Email == "" || in.RealName == "" || in.StudentID == "" || in.Advisor == "" || in.Phone == "" {
		return errors.New("注册信息不完整")
	}
	var dup []string
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE username=$1)`, in.Username).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM power_users WHERE username=$1)`, in.Username).Scan(&exists); err != nil {
			return err
		}
	}
	if !exists {
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_accounts WHERE username=$1)`, in.Username).Scan(&exists); err != nil {
			return err
		}
	}
	if exists {
		dup = append(dup, "用户名")
	}
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE email=$1)`, in.Email).Scan(&exists); err != nil {
		return err
	}
	if exists {
		dup = append(dup, "邮箱")
	}
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE student_id=$1)`, in.StudentID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		dup = append(dup, "学号")
	}
	if len(dup) > 0 {
		return fmt.Errorf("以下信息已存在账号：%s", strings.Join(dup, "、"))
	}
	if err := validateStrongPassword(password); err != nil {
		return err
	}
	if in.ExpectedGraduationYear < 2000 || in.ExpectedGraduationYear > 2200 {
		return errors.New("expected_graduation_year 不合法")
	}
	if in.ExpectedGraduationMonth < 1 || in.ExpectedGraduationMonth > 12 {
		return errors.New("expected_graduation_month 不合法")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	platformUID, _, err := s.allocatePlatformUIDTx(ctx, tx, time.Now(), nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO user_accounts(
  username, email, password_hash, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, platform_uid, role
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'user')`,
		in.Username, in.Email, string(hash), in.RealName, in.StudentID, in.Advisor, in.ExpectedGraduationYear, in.ExpectedGraduationMonth, in.Phone, platformUID); err != nil {
		return err
	}
	initialBalance, err := s.resolveInitialGeneralBalanceTx(ctx, tx, in.Username, in.StudentID, defaultBalance)
	if err != nil {
		return err
	}
	_, err = s.EnsureUserTx(ctx, tx, in.Username, initialBalance)
	return err
}

func (s *Store) registrationConflictFieldsTx(ctx context.Context, tx *sql.Tx, username string, email string, studentID string, excludeRequestID int, excludeVerifyID int64) ([]string, error) {
	dup := make([]string, 0, 3)
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM power_users WHERE username=$1)`, username).Scan(&exists); err != nil {
			return nil, err
		}
	}
	if !exists {
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
			return nil, err
		}
	}
	if !exists {
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM registration_requests
  WHERE status='pending' AND username=$1 AND ($2 <= 0 OR request_id<>$2)
)`, username, excludeRequestID).Scan(&exists); err != nil {
			return nil, err
		}
	}
	if !exists {
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM registration_email_verifications
  WHERE consumed_at IS NULL
    AND expire_at > NOW()
    AND username=$1
    AND ($2 <= 0 OR verify_id<>$2)
)`, username, excludeVerifyID).Scan(&exists); err != nil {
			return nil, err
		}
	}
	if exists {
		dup = append(dup, "用户名")
	}
	exists = false
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE email=$1)`, email).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM registration_requests
  WHERE status='pending' AND email=$1 AND ($2 <= 0 OR request_id<>$2)
)`, email, excludeRequestID).Scan(&exists); err != nil {
			return nil, err
		}
	}
	if !exists {
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM registration_email_verifications
  WHERE consumed_at IS NULL
    AND expire_at > NOW()
    AND email=$1
    AND ($2 <= 0 OR verify_id<>$2)
)`, email, excludeVerifyID).Scan(&exists); err != nil {
			return nil, err
		}
	}
	if exists {
		dup = append(dup, "邮箱")
	}
	exists = false
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE student_id=$1)`, studentID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM registration_requests
  WHERE status='pending' AND student_id=$1 AND ($2 <= 0 OR request_id<>$2)
)`, studentID, excludeRequestID).Scan(&exists); err != nil {
			return nil, err
		}
	}
	if !exists {
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM registration_email_verifications
  WHERE consumed_at IS NULL
    AND expire_at > NOW()
    AND student_id=$1
    AND ($2 <= 0 OR verify_id<>$2)
)`, studentID, excludeVerifyID).Scan(&exists); err != nil {
			return nil, err
		}
	}
	if exists {
		dup = append(dup, "学号")
	}
	return dup, nil
}

func (s *Store) RegistrationAvailability(ctx context.Context, username string, email string, studentID string) (map[string]string, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(strings.ToLower(email))
	studentID = strings.TrimSpace(studentID)
	out := map[string]string{}
	if username == "" && email == "" && studentID == "" {
		return out, nil
	}
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		if username != "" {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM power_users WHERE username=$1)`, username).Scan(&exists); err != nil {
					return err
				}
			}
			if !exists {
				if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
					return err
				}
			}
			if exists {
				out["username"] = "用户名已存在"
			} else if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM registration_requests WHERE status='pending' AND username=$1)`, username).Scan(&exists); err != nil {
				return err
			} else if exists {
				out["username"] = "用户名已被待审核申请占用"
			} else if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM registration_email_verifications
  WHERE consumed_at IS NULL AND expire_at > NOW() AND username=$1
)`, username).Scan(&exists); err != nil {
				return err
			} else if exists {
				out["username"] = "用户名已被待验证申请占用"
			}
		}
		if email != "" {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE email=$1)`, email).Scan(&exists); err != nil {
				return err
			}
			if exists {
				out["email"] = "邮箱已存在"
			} else if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM registration_requests WHERE status='pending' AND email=$1)`, email).Scan(&exists); err != nil {
				return err
			} else if exists {
				out["email"] = "邮箱已被待审核申请占用"
			} else if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM registration_email_verifications
  WHERE consumed_at IS NULL AND expire_at > NOW() AND email=$1
)`, email).Scan(&exists); err != nil {
				return err
			} else if exists {
				out["email"] = "邮箱已被待验证申请占用"
			}
		}
		if studentID != "" {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE student_id=$1)`, studentID).Scan(&exists); err != nil {
				return err
			}
			if exists {
				out["student_id"] = "学号已存在"
			} else if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM registration_requests WHERE status='pending' AND student_id=$1)`, studentID).Scan(&exists); err != nil {
				return err
			} else if exists {
				out["student_id"] = "学号已被待审核申请占用"
			} else if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM registration_email_verifications
  WHERE consumed_at IS NULL AND expire_at > NOW() AND student_id=$1
)`, studentID).Scan(&exists); err != nil {
				return err
			} else if exists {
				out["student_id"] = "学号已被待验证申请占用"
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) RegistrationConflictsWithAccounts(ctx context.Context, username string, email string, studentID string) (map[string]bool, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(strings.ToLower(email))
	studentID = strings.TrimSpace(studentID)
	out := map[string]bool{
		"username":   false,
		"email":      false,
		"student_id": false,
	}
	if username == "" && email == "" && studentID == "" {
		return out, nil
	}
	if username != "" {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM power_users WHERE username=$1)`, username).Scan(&exists); err != nil {
				return nil, err
			}
		}
		if !exists {
			if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
				return nil, err
			}
		}
		out["username"] = exists
	}
	if email != "" {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE email=$1)`, email).Scan(&exists); err != nil {
			return nil, err
		}
		out["email"] = exists
	}
	if studentID != "" {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE student_id=$1)`, studentID).Scan(&exists); err != nil {
			return nil, err
		}
		out["student_id"] = exists
	}
	return out, nil
}

func (s *Store) cleanupExpiredRegistrationEmailVerificationsTx(ctx context.Context, tx *sql.Tx, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
DELETE FROM registration_email_verifications
WHERE consumed_at IS NULL AND expire_at <= $1`, now)
	return err
}

func (s *Store) CreateRegistrationEmailVerificationTx(
	ctx context.Context,
	tx *sql.Tx,
	in UserAccount,
	password string,
	tokenHash string,
	expireAt time.Time,
) error {
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	in.RealName = strings.TrimSpace(in.RealName)
	in.StudentID = strings.TrimSpace(in.StudentID)
	in.Advisor = strings.TrimSpace(in.Advisor)
	in.Phone = strings.TrimSpace(in.Phone)
	tokenHash = strings.TrimSpace(tokenHash)
	if in.Username == "" || in.Email == "" || in.RealName == "" || in.StudentID == "" || in.Advisor == "" || in.Phone == "" {
		return errors.New("注册信息不完整")
	}
	if err := validateStrongPassword(password); err != nil {
		return err
	}
	if tokenHash == "" {
		return errors.New("邮箱验证令牌不能为空")
	}
	if in.ExpectedGraduationYear < 2000 || in.ExpectedGraduationYear > 2200 {
		return errors.New("expected_graduation_year 不合法")
	}
	if in.ExpectedGraduationMonth < 1 || in.ExpectedGraduationMonth > 12 {
		return errors.New("expected_graduation_month 不合法")
	}

	now := time.Now()
	if !expireAt.After(now) {
		return errors.New("邮箱验证链接过期时间不合法")
	}
	if err := s.cleanupExpiredRegistrationEmailVerificationsTx(ctx, tx, now); err != nil {
		return err
	}

	dup, err := s.registrationConflictFieldsTx(ctx, tx, in.Username, in.Email, in.StudentID, 0, 0)
	if err != nil {
		return err
	}
	if len(dup) > 0 {
		return fmt.Errorf("以下信息不可用：%s", strings.Join(dup, "、"))
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO registration_email_verifications(
  token_hash, username, email, password_hash, real_name, student_id, advisor,
  expected_graduation_year, expected_graduation_month, phone, expire_at
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		tokenHash, in.Username, in.Email, string(hash), in.RealName, in.StudentID, in.Advisor,
		in.ExpectedGraduationYear, in.ExpectedGraduationMonth, in.Phone, expireAt,
	)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			switch strings.TrimSpace(pqErr.Constraint) {
			case "uq_registration_email_verifications_pending_username":
				return errors.New("用户名已被待验证申请占用")
			case "uq_registration_email_verifications_pending_email":
				return errors.New("邮箱已被待验证申请占用")
			case "uq_registration_email_verifications_pending_student_id":
				return errors.New("学号已被待验证申请占用")
			}
		}
		return err
	}
	return nil
}

func (s *Store) DeleteRegistrationEmailVerificationByTokenHash(ctx context.Context, tokenHash string) error {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM registration_email_verifications WHERE token_hash=$1`, tokenHash)
	return err
}

func (s *Store) ConsumeRegistrationEmailVerificationTx(ctx context.Context, tx *sql.Tx, tokenHash string, now time.Time) (RegistrationRequest, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return RegistrationRequest{}, errRegistrationVerifyTokenInvalid
	}

	var (
		verifyID int64
		v        RegistrationRequest
		expireAt time.Time
		usedAt   sql.NullTime
	)
	if err := tx.QueryRowContext(ctx, `
SELECT verify_id, username, email, password_hash, real_name, student_id, advisor,
       expected_graduation_year, expected_graduation_month, phone, expire_at, consumed_at
FROM registration_email_verifications
WHERE token_hash=$1
FOR UPDATE`, tokenHash).Scan(
		&verifyID, &v.Username, &v.Email, &v.PasswordHash, &v.RealName, &v.StudentID, &v.Advisor,
		&v.ExpectedGraduationYear, &v.ExpectedGraduationMonth, &v.Phone, &expireAt, &usedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RegistrationRequest{}, errRegistrationVerifyTokenInvalid
		}
		return RegistrationRequest{}, err
	}
	if usedAt.Valid {
		return RegistrationRequest{}, errRegistrationVerifyTokenUsed
	}
	if !expireAt.After(now) {
		return RegistrationRequest{}, errRegistrationVerifyTokenExpired
	}

	dup, err := s.registrationConflictFieldsTx(ctx, tx, v.Username, v.Email, v.StudentID, 0, verifyID)
	if err != nil {
		return RegistrationRequest{}, err
	}
	if len(dup) > 0 {
		return RegistrationRequest{}, fmt.Errorf("以下信息不可用：%s", strings.Join(dup, "、"))
	}

	var (
		reviewedBy sql.NullString
		reviewedAt sql.NullTime
	)
	err = tx.QueryRowContext(ctx, `
INSERT INTO registration_requests(
  username, email, password_hash, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, status
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'pending')
RETURNING request_id, username, email, password_hash, real_name, student_id, advisor,
          expected_graduation_year, expected_graduation_month, phone, status,
          reviewed_by, reviewed_at, reject_reason,
          reject_notify_mail_checked, reject_notify_mail_sent, reject_notify_mail_error,
          created_at, updated_at`,
		v.Username, v.Email, v.PasswordHash, v.RealName, v.StudentID, v.Advisor,
		v.ExpectedGraduationYear, v.ExpectedGraduationMonth, v.Phone,
	).Scan(
		&v.RequestID, &v.Username, &v.Email, &v.PasswordHash, &v.RealName, &v.StudentID, &v.Advisor,
		&v.ExpectedGraduationYear, &v.ExpectedGraduationMonth, &v.Phone, &v.Status,
		&reviewedBy, &reviewedAt, &v.RejectReason,
		&v.RejectNotifyMailChecked, &v.RejectNotifyMailSent, &v.RejectNotifyMailError,
		&v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			switch strings.TrimSpace(pqErr.Constraint) {
			case "uq_registration_requests_pending_username":
				return RegistrationRequest{}, errors.New("用户名已被待审核申请占用")
			case "uq_registration_requests_pending_email":
				return RegistrationRequest{}, errors.New("邮箱已被待审核申请占用")
			case "uq_registration_requests_pending_student_id":
				return RegistrationRequest{}, errors.New("学号已被待审核申请占用")
			}
		}
		return RegistrationRequest{}, err
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE registration_email_verifications
SET consumed_at=$2, consumed_request_id=$3, updated_at=$2
WHERE verify_id=$1`, verifyID, now, v.RequestID); err != nil {
		return RegistrationRequest{}, err
	}
	if reviewedBy.Valid {
		value := reviewedBy.String
		v.ReviewedBy = &value
	}
	if reviewedAt.Valid {
		value := reviewedAt.Time
		v.ReviewedAt = &value
	}
	return v, nil
}

func (s *Store) cleanupExpiredRegisterCaptchaChallengesTx(ctx context.Context, tx *sql.Tx, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
DELETE FROM registration_captcha_challenges
WHERE expire_at <= $1 OR (consumed_at IS NOT NULL AND created_at <= $1 - INTERVAL '1 day')`, now)
	return err
}

func (s *Store) CreateRegisterCaptchaChallenge(
	ctx context.Context,
	captchaID string,
	clientIP string,
	question string,
	options []int,
	answerIndex int,
	expireAt time.Time,
) error {
	captchaID = strings.TrimSpace(captchaID)
	clientIP = strings.TrimSpace(clientIP)
	question = strings.TrimSpace(question)
	if captchaID == "" {
		return errors.New("captcha_id 不能为空")
	}
	if question == "" {
		return errors.New("question 不能为空")
	}
	if answerIndex < 0 {
		return errors.New("answer_index 不合法")
	}
	if len(options) < 2 {
		return errors.New("captcha options 不合法")
	}
	if answerIndex >= len(options) {
		return errors.New("answer_index 越界")
	}
	if !expireAt.After(time.Now()) {
		return errors.New("captcha 过期时间不合法")
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		now := time.Now()
		if err := s.cleanupExpiredRegisterCaptchaChallengesTx(ctx, tx, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO registration_captcha_challenges(
  captcha_id, client_ip, question, options_json, answer_index, expire_at
)
VALUES($1,$2,$3,$4::jsonb,$5,$6)`,
			captchaID, clientIP, question, string(optionsJSON), answerIndex, expireAt,
		)
		return err
	})
}

func (s *Store) VerifyRegisterCaptcha(ctx context.Context, captchaID string, clientIP string, selectedOption int, now time.Time) error {
	return s.verifyRegisterCaptcha(ctx, captchaID, clientIP, selectedOption, now, false, true)
}

func (s *Store) VerifyAndConsumeRegisterCaptcha(ctx context.Context, captchaID string, clientIP string, selectedOption int, now time.Time) error {
	return s.verifyRegisterCaptcha(ctx, captchaID, clientIP, selectedOption, now, true, true)
}

func (s *Store) verifyRegisterCaptcha(
	ctx context.Context,
	captchaID string,
	clientIP string,
	selectedOption int,
	now time.Time,
	consumeOnSuccess bool,
	consumeOnFailure bool,
) error {
	captchaID = strings.TrimSpace(captchaID)
	clientIP = strings.TrimSpace(clientIP)
	if captchaID == "" {
		return errRegisterCaptchaInvalid
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.cleanupExpiredRegisterCaptchaChallengesTx(ctx, tx, now); err != nil {
			return err
		}
		var (
			dbClientIP string
			answerIdx  int
			expireAt   time.Time
			consumedAt sql.NullTime
		)
		if err := tx.QueryRowContext(ctx, `
SELECT client_ip, answer_index, expire_at, consumed_at
FROM registration_captcha_challenges
WHERE captcha_id=$1
FOR UPDATE`, captchaID).Scan(&dbClientIP, &answerIdx, &expireAt, &consumedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errRegisterCaptchaInvalid
			}
			return err
		}
		if consumedAt.Valid {
			return errRegisterCaptchaUsed
		}
		if !expireAt.After(now) {
			return errRegisterCaptchaExpired
		}
		// 绑定客户端 IP，避免挑战被跨端复用。
		if strings.TrimSpace(dbClientIP) != "" && strings.TrimSpace(clientIP) != "" && strings.TrimSpace(dbClientIP) != strings.TrimSpace(clientIP) {
			return errRegisterCaptchaInvalid
		}

		isAnswerCorrect := answerIdx == selectedOption
		shouldConsume := (isAnswerCorrect && consumeOnSuccess) || (!isAnswerCorrect && consumeOnFailure)
		if shouldConsume {
			_, err := tx.ExecContext(ctx, `
UPDATE registration_captcha_challenges
SET consumed_at=$2
WHERE captcha_id=$1`, captchaID, now)
			if err != nil {
				return err
			}
		}
		if !isAnswerCorrect {
			return errRegisterCaptchaInvalid
		}
		return nil
	})
}

func (s *Store) ConsumeRegisterCaptcha(ctx context.Context, captchaID string, now time.Time) error {
	captchaID = strings.TrimSpace(captchaID)
	if captchaID == "" {
		return errRegisterCaptchaInvalid
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.cleanupExpiredRegisterCaptchaChallengesTx(ctx, tx, now); err != nil {
			return err
		}
		var (
			expireAt   time.Time
			consumedAt sql.NullTime
		)
		if err := tx.QueryRowContext(ctx, `
SELECT expire_at, consumed_at
FROM registration_captcha_challenges
WHERE captcha_id=$1
FOR UPDATE`, captchaID).Scan(&expireAt, &consumedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errRegisterCaptchaInvalid
			}
			return err
		}
		if consumedAt.Valid {
			return errRegisterCaptchaUsed
		}
		if !expireAt.After(now) {
			return errRegisterCaptchaExpired
		}
		_, err := tx.ExecContext(ctx, `
UPDATE registration_captcha_challenges
SET consumed_at=$2
WHERE captcha_id=$1`, captchaID, now)
		return err
	})
}

func (s *Store) LoadRegistrationRateStats(
	ctx context.Context,
	clientIP string,
	email string,
	ipSince time.Time,
	emailSince time.Time,
) (RegistrationRateStats, error) {
	clientIP = strings.TrimSpace(clientIP)
	email = strings.TrimSpace(strings.ToLower(email))
	var out RegistrationRateStats
	if clientIP != "" {
		var last sql.NullTime
		if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1), MAX(created_at)
FROM registration_security_events
WHERE action='register_submit'
  AND client_ip=$1
  AND created_at >= $2`, clientIP, ipSince).Scan(&out.IPCount, &last); err != nil {
			return out, err
		}
		if last.Valid {
			v := last.Time
			out.LastIPAt = &v
		}
		if out.IPCount >= registerIPLimit {
			var unlockBase time.Time
			offset := out.IPCount - registerIPLimit
			if err := s.db.QueryRowContext(ctx, `
SELECT created_at
FROM registration_security_events
WHERE action='register_submit'
  AND client_ip=$1
  AND created_at >= $2
ORDER BY created_at ASC
OFFSET $3
LIMIT 1`, clientIP, ipSince, offset).Scan(&unlockBase); err == nil {
				v := unlockBase.Add(registerIPWindow)
				out.IPWindowRetryAt = &v
			} else if !errors.Is(err, sql.ErrNoRows) {
				return out, err
			}
		}
	}
	if email != "" {
		var last sql.NullTime
		if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1), MAX(created_at)
FROM registration_security_events
WHERE action='register_submit'
  AND decision='allow'
  AND reason='verification_mail_sent'
  AND email=$1
  AND created_at >= $2`, email, emailSince).Scan(&out.EmailCount, &last); err != nil {
			return out, err
		}
		if last.Valid {
			v := last.Time
			out.LastEmailAt = &v
		}
		if out.EmailCount >= registerEmailLimit {
			var unlockBase time.Time
			offset := out.EmailCount - registerEmailLimit
			if err := s.db.QueryRowContext(ctx, `
SELECT created_at
FROM registration_security_events
WHERE action='register_submit'
  AND decision='allow'
  AND reason='verification_mail_sent'
  AND email=$1
  AND created_at >= $2
ORDER BY created_at ASC
OFFSET $3
LIMIT 1`, email, emailSince, offset).Scan(&unlockBase); err == nil {
				v := unlockBase.Add(registerEmailWindow)
				out.EmailWindowRetryAt = &v
			} else if !errors.Is(err, sql.ErrNoRows) {
				return out, err
			}
		}
	}
	return out, nil
}

func (s *Store) InsertRegistrationSecurityEvent(ctx context.Context, event RegistrationSecurityEvent) error {
	event.Action = strings.TrimSpace(event.Action)
	event.Decision = strings.TrimSpace(event.Decision)
	event.Reason = strings.TrimSpace(event.Reason)
	event.ClientIP = strings.TrimSpace(event.ClientIP)
	event.Username = strings.TrimSpace(event.Username)
	event.Email = strings.TrimSpace(strings.ToLower(event.Email))
	event.StudentID = strings.TrimSpace(strings.ToUpper(event.StudentID))
	event.UserAgent = strings.TrimSpace(event.UserAgent)
	if event.Action == "" {
		return errors.New("action 不能为空")
	}
	if event.Decision == "" {
		return errors.New("decision 不能为空")
	}
	if event.Reason == "" {
		event.Reason = "-"
	}
	if len(event.UserAgent) > 512 {
		event.UserAgent = event.UserAgent[:512]
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO registration_security_events(
  action, decision, reason, client_ip, username, email, student_id, user_agent
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		event.Action, event.Decision, event.Reason, event.ClientIP, event.Username, event.Email, event.StudentID, event.UserAgent,
	)
	return err
}

func (s *Store) ListRegistrationSecurityEvents(
	ctx context.Context,
	keyword string,
	keywordField string,
	action string,
	decision string,
	limit int,
) ([]RegistrationSecurityEvent, error) {
	keyword = strings.TrimSpace(keyword)
	keywordField = strings.ToLower(strings.TrimSpace(keywordField))
	action = strings.TrimSpace(action)
	decision = strings.TrimSpace(decision)
	switch keywordField {
	case "all", "client_ip", "email", "username", "student_id", "reason", "user_agent":
	default:
		keywordField = "all"
	}
	if limit <= 0 || limit > 5000 {
		limit = registerSecurityDefaultLimit
	}

	query := fmt.Sprintf(`
SELECT
  e.event_id,
  e.action,
  e.decision,
  e.reason,
  e.client_ip,
  e.username,
  e.email,
  e.student_id,
  e.user_agent,
  CASE
    WHEN e.reason='cooldown_by_ip' THEN e.created_at + INTERVAL '%d seconds'
    WHEN e.reason='cooldown_by_email' THEN (
      SELECT x.created_at + INTERVAL '%d seconds'
      FROM registration_security_events x
      WHERE x.action='register_submit'
        AND x.decision='allow'
        AND x.reason='verification_mail_sent'
        AND x.email=e.email
        AND x.event_id < e.event_id
      ORDER BY x.event_id DESC
      LIMIT 1
    )
    WHEN e.reason='rate_limited_by_ip_window' THEN (
      SELECT x.created_at + INTERVAL '%d seconds'
      FROM registration_security_events x
      WHERE x.action='register_submit'
        AND x.client_ip=e.client_ip
        AND x.created_at <= e.created_at
        AND x.created_at >= e.created_at - INTERVAL '%d seconds'
      ORDER BY x.created_at DESC
      OFFSET %d
      LIMIT 1
    )
    WHEN e.reason='rate_limited_by_email_window' THEN (
      SELECT x.created_at + INTERVAL '%d seconds'
      FROM registration_security_events x
      WHERE x.action='register_submit'
        AND x.decision='allow'
        AND x.reason='verification_mail_sent'
        AND x.email=e.email
        AND x.created_at <= e.created_at
        AND x.created_at >= e.created_at - INTERVAL '%d seconds'
      ORDER BY x.created_at DESC
      OFFSET %d
      LIMIT 1
    )
    ELSE NULL
  END AS retry_at,
  e.created_at
FROM registration_security_events e
WHERE (
  $1='' OR
  ($2='all' AND (
    e.action ILIKE '%%' || $1 || '%%' OR
    e.decision ILIKE '%%' || $1 || '%%' OR
    e.reason ILIKE '%%' || $1 || '%%' OR
    e.client_ip ILIKE '%%' || $1 || '%%' OR
    e.username ILIKE '%%' || $1 || '%%' OR
    e.email ILIKE '%%' || $1 || '%%' OR
    e.student_id ILIKE '%%' || $1 || '%%' OR
    e.user_agent ILIKE '%%' || $1 || '%%'
  )) OR
  ($2='client_ip' AND e.client_ip ILIKE '%%' || $1 || '%%') OR
  ($2='email' AND e.email ILIKE '%%' || $1 || '%%') OR
  ($2='username' AND e.username ILIKE '%%' || $1 || '%%') OR
  ($2='student_id' AND e.student_id ILIKE '%%' || $1 || '%%') OR
  ($2='reason' AND e.reason ILIKE '%%' || $1 || '%%') OR
  ($2='user_agent' AND e.user_agent ILIKE '%%' || $1 || '%%')
)
AND ($3='' OR e.action=$3)
AND ($4='' OR e.decision=$4)
ORDER BY e.event_id DESC
LIMIT $5`,
		int(registerIPCooldown/time.Second),
		int(registerEmailCooldown/time.Second),
		int(registerIPWindow/time.Second),
		int(registerIPWindow/time.Second),
		registerIPLimit-1,
		int(registerEmailWindow/time.Second),
		int(registerEmailWindow/time.Second),
		registerEmailLimit-1,
	)
	rows, err := s.db.QueryContext(ctx, query, keyword, keywordField, action, decision, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RegistrationSecurityEvent, 0, limit)
	for rows.Next() {
		var item RegistrationSecurityEvent
		var retryAt sql.NullTime
		if err := rows.Scan(
			&item.EventID,
			&item.Action,
			&item.Decision,
			&item.Reason,
			&item.ClientIP,
			&item.Username,
			&item.Email,
			&item.StudentID,
			&item.UserAgent,
			&retryAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		if retryAt.Valid {
			v := retryAt.Time
			item.RetryAt = &v
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func normalizeDomain(domain string) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimPrefix(domain, "@")
	return domain
}

func (s *Store) IsDisposableEmailDomainBlocked(ctx context.Context, domain string) (bool, error) {
	domain = normalizeDomain(domain)
	if domain == "" {
		return false, nil
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM registration_disposable_email_domains
  WHERE domain=$1
    AND enabled=TRUE
)`, domain).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Store) UpsertDisposableEmailDomain(ctx context.Context, domain string, enabled bool, note string, updatedBy string) (RegistrationDisposableEmailDomain, error) {
	domain = normalizeDomain(domain)
	note = strings.TrimSpace(note)
	updatedBy = strings.TrimSpace(updatedBy)
	if domain == "" {
		return RegistrationDisposableEmailDomain{}, errors.New("domain 不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	var out RegistrationDisposableEmailDomain
	if err := s.db.QueryRowContext(ctx, `
INSERT INTO registration_disposable_email_domains(domain, enabled, note, updated_by)
VALUES($1,$2,$3,$4)
ON CONFLICT (domain) DO UPDATE SET
  enabled=EXCLUDED.enabled,
  note=EXCLUDED.note,
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()
RETURNING domain, enabled, note, updated_by, created_at, updated_at`,
		domain, enabled, note, updatedBy,
	).Scan(&out.Domain, &out.Enabled, &out.Note, &out.UpdatedBy, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return RegistrationDisposableEmailDomain{}, err
	}
	return out, nil
}

func (s *Store) DeleteDisposableEmailDomain(ctx context.Context, domain string) error {
	domain = normalizeDomain(domain)
	if domain == "" {
		return errors.New("domain 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM registration_disposable_email_domains WHERE domain=$1`, domain)
	if err != nil {
		return err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListDisposableEmailDomains(ctx context.Context, keyword string, limit int) ([]RegistrationDisposableEmailDomain, error) {
	keyword = normalizeDomain(keyword)
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT domain, enabled, note, updated_by, created_at, updated_at
FROM registration_disposable_email_domains
WHERE ($1='' OR domain ILIKE '%' || $1 || '%' OR note ILIKE '%' || $1 || '%')
ORDER BY enabled DESC, domain ASC
LIMIT $2`, keyword, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RegistrationDisposableEmailDomain, 0)
	for rows.Next() {
		var item RegistrationDisposableEmailDomain
		if err := rows.Scan(&item.Domain, &item.Enabled, &item.Note, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CreateRegistrationRequestTx(ctx context.Context, tx *sql.Tx, in UserAccount, password string) (RegistrationRequest, error) {
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	in.RealName = strings.TrimSpace(in.RealName)
	in.StudentID = strings.TrimSpace(in.StudentID)
	in.Advisor = strings.TrimSpace(in.Advisor)
	in.Phone = strings.TrimSpace(in.Phone)
	if in.Username == "" || in.Email == "" || in.RealName == "" || in.StudentID == "" || in.Advisor == "" || in.Phone == "" {
		return RegistrationRequest{}, errors.New("注册信息不完整")
	}
	if err := validateStrongPassword(password); err != nil {
		return RegistrationRequest{}, err
	}
	if in.ExpectedGraduationYear < 2000 || in.ExpectedGraduationYear > 2200 {
		return RegistrationRequest{}, errors.New("expected_graduation_year 不合法")
	}
	if in.ExpectedGraduationMonth < 1 || in.ExpectedGraduationMonth > 12 {
		return RegistrationRequest{}, errors.New("expected_graduation_month 不合法")
	}

	dup, err := s.registrationConflictFieldsTx(ctx, tx, in.Username, in.Email, in.StudentID, 0, 0)
	if err != nil {
		return RegistrationRequest{}, err
	}
	if len(dup) > 0 {
		return RegistrationRequest{}, fmt.Errorf("以下信息不可用：%s", strings.Join(dup, "、"))
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return RegistrationRequest{}, err
	}

	out := RegistrationRequest{}
	var reviewedBy sql.NullString
	var reviewedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
INSERT INTO registration_requests(
  username, email, password_hash, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, status
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'pending')
RETURNING request_id, username, email, password_hash, real_name, student_id, advisor,
          expected_graduation_year, expected_graduation_month, phone, status,
          reviewed_by, reviewed_at, reject_reason,
          reject_notify_mail_checked, reject_notify_mail_sent, reject_notify_mail_error,
          created_at, updated_at`,
		in.Username, in.Email, string(hash), in.RealName, in.StudentID, in.Advisor,
		in.ExpectedGraduationYear, in.ExpectedGraduationMonth, in.Phone,
	).Scan(
		&out.RequestID, &out.Username, &out.Email, &out.PasswordHash, &out.RealName, &out.StudentID, &out.Advisor,
		&out.ExpectedGraduationYear, &out.ExpectedGraduationMonth, &out.Phone, &out.Status,
		&reviewedBy, &reviewedAt, &out.RejectReason,
		&out.RejectNotifyMailChecked, &out.RejectNotifyMailSent, &out.RejectNotifyMailError,
		&out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			switch strings.TrimSpace(pqErr.Constraint) {
			case "uq_registration_requests_pending_username":
				return RegistrationRequest{}, errors.New("用户名已被待审核申请占用")
			case "uq_registration_requests_pending_email":
				return RegistrationRequest{}, errors.New("邮箱已被待审核申请占用")
			case "uq_registration_requests_pending_student_id":
				return RegistrationRequest{}, errors.New("学号已被待审核申请占用")
			}
		}
		return RegistrationRequest{}, err
	}
	if reviewedBy.Valid {
		v := reviewedBy.String
		out.ReviewedBy = &v
	}
	if reviewedAt.Valid {
		v := reviewedAt.Time
		out.ReviewedAt = &v
	}
	return out, nil
}

func (s *Store) ListRegistrationRequestsAdmin(ctx context.Context, status string, limit int) ([]RegistrationRequest, error) {
	status = strings.TrimSpace(status)
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	query := `
SELECT request_id, username, email, password_hash, real_name, student_id, advisor,
       expected_graduation_year, expected_graduation_month, phone, status,
       reviewed_by, reviewed_at, reject_reason,
       reject_notify_mail_checked, reject_notify_mail_sent, reject_notify_mail_error,
       created_at, updated_at
FROM registration_requests`
	args := make([]any, 0, 2)
	if status != "" {
		query += " WHERE status=$1"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"
	if status == "" {
		query += " LIMIT $1"
		args = append(args, limit)
	} else {
		query += " LIMIT $2"
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RegistrationRequest, 0)
	for rows.Next() {
		var r RegistrationRequest
		var reviewedBy sql.NullString
		var reviewedAt sql.NullTime
		if err := rows.Scan(
			&r.RequestID, &r.Username, &r.Email, &r.PasswordHash, &r.RealName, &r.StudentID, &r.Advisor,
			&r.ExpectedGraduationYear, &r.ExpectedGraduationMonth, &r.Phone, &r.Status,
			&reviewedBy, &reviewedAt, &r.RejectReason,
			&r.RejectNotifyMailChecked, &r.RejectNotifyMailSent, &r.RejectNotifyMailError,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if reviewedBy.Valid {
			v := reviewedBy.String
			r.ReviewedBy = &v
		}
		if reviewedAt.Valid {
			v := reviewedAt.Time
			r.ReviewedAt = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) createUserAccountWithHashTx(ctx context.Context, tx *sql.Tx, in RegistrationRequest, defaultBalance float64) error {
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	in.RealName = strings.TrimSpace(in.RealName)
	in.StudentID = strings.TrimSpace(in.StudentID)
	in.Advisor = strings.TrimSpace(in.Advisor)
	in.Phone = strings.TrimSpace(in.Phone)
	if in.Username == "" || in.Email == "" || in.RealName == "" || in.StudentID == "" || in.Advisor == "" || in.Phone == "" || strings.TrimSpace(in.PasswordHash) == "" {
		return errors.New("注册信息不完整")
	}
	var dup []string
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE username=$1)`, in.Username).Scan(&exists); err != nil {
		return err
	}
	if exists {
		dup = append(dup, "用户名")
	}
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE email=$1)`, in.Email).Scan(&exists); err != nil {
		return err
	}
	if exists {
		dup = append(dup, "邮箱")
	}
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE student_id=$1)`, in.StudentID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		dup = append(dup, "学号")
	}
	if len(dup) > 0 {
		return fmt.Errorf("以下信息已存在账号：%s", strings.Join(dup, "、"))
	}
	if in.ExpectedGraduationYear < 2000 || in.ExpectedGraduationYear > 2200 {
		return errors.New("expected_graduation_year 不合法")
	}
	if in.ExpectedGraduationMonth < 1 || in.ExpectedGraduationMonth > 12 {
		return errors.New("expected_graduation_month 不合法")
	}
	platformUID, _, err := s.allocatePlatformUIDTx(ctx, tx, time.Now(), nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO user_accounts(
  username, email, password_hash, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, platform_uid, role
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'user')`,
		in.Username, in.Email, in.PasswordHash, in.RealName, in.StudentID, in.Advisor, in.ExpectedGraduationYear, in.ExpectedGraduationMonth, in.Phone, platformUID,
	); err != nil {
		return err
	}
	initialBalance, err := s.resolveInitialGeneralBalanceTx(ctx, tx, in.Username, in.StudentID, defaultBalance)
	if err != nil {
		return err
	}
	_, err = s.EnsureUserTx(ctx, tx, in.Username, initialBalance)
	return err
}

func (s *Store) ReviewRegistrationRequestTx(
	ctx context.Context,
	tx *sql.Tx,
	requestID int,
	newStatus string,
	reviewedBy string,
	reviewedAt time.Time,
	rejectReason string,
	defaultBalance float64,
) (RegistrationRequest, error) {
	if requestID <= 0 {
		return RegistrationRequest{}, errors.New("request_id 不合法")
	}
	newStatus = strings.TrimSpace(newStatus)
	reviewedBy = strings.TrimSpace(reviewedBy)
	rejectReason = strings.TrimSpace(rejectReason)
	if newStatus != "approved" && newStatus != "rejected" {
		return RegistrationRequest{}, errors.New("status 仅支持 approved/rejected")
	}
	if newStatus == "rejected" && rejectReason == "" {
		return RegistrationRequest{}, errors.New("退回理由不能为空")
	}
	if reviewedBy == "" {
		reviewedBy = "admin"
	}
	var r RegistrationRequest
	var reviewedByPrev sql.NullString
	var reviewedAtPrev sql.NullTime
	if err := tx.QueryRowContext(ctx, `
SELECT request_id, username, email, password_hash, real_name, student_id, advisor,
       expected_graduation_year, expected_graduation_month, phone, status,
       reviewed_by, reviewed_at, reject_reason,
       reject_notify_mail_checked, reject_notify_mail_sent, reject_notify_mail_error,
       created_at, updated_at
FROM registration_requests
WHERE request_id=$1
FOR UPDATE`, requestID).Scan(
		&r.RequestID, &r.Username, &r.Email, &r.PasswordHash, &r.RealName, &r.StudentID, &r.Advisor,
		&r.ExpectedGraduationYear, &r.ExpectedGraduationMonth, &r.Phone, &r.Status,
		&reviewedByPrev, &reviewedAtPrev, &r.RejectReason,
		&r.RejectNotifyMailChecked, &r.RejectNotifyMailSent, &r.RejectNotifyMailError,
		&r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return RegistrationRequest{}, err
	}
	if r.Status != "pending" {
		return RegistrationRequest{}, errors.New("该注册申请已处理，不能重复审核")
	}
	if newStatus == "approved" {
		if err := s.createUserAccountWithHashTx(ctx, tx, r, defaultBalance); err != nil {
			return RegistrationRequest{}, err
		}
		rejectReason = ""
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE registration_requests
SET status=$2,
    reviewed_by=$3,
    reviewed_at=$4,
    reject_reason=$5,
    reject_notify_mail_checked=$6,
    reject_notify_mail_sent=$7,
    reject_notify_mail_error=$8,
    updated_at=NOW()
WHERE request_id=$1`, requestID, newStatus, reviewedBy, reviewedAt, rejectReason, false, false, ""); err != nil {
		return RegistrationRequest{}, err
	}
	r.Status = newStatus
	r.ReviewedBy = &reviewedBy
	r.ReviewedAt = &reviewedAt
	r.RejectReason = rejectReason
	r.RejectNotifyMailChecked = false
	r.RejectNotifyMailSent = false
	r.RejectNotifyMailError = ""
	r.UpdatedAt = reviewedAt
	return r, nil
}

func (s *Store) SetRegistrationRejectNotifyMailResult(ctx context.Context, requestID int, sent bool, mailError string) error {
	if requestID <= 0 {
		return errors.New("request_id 不合法")
	}
	mailError = strings.TrimSpace(mailError)
	res, err := s.db.ExecContext(ctx, `
UPDATE registration_requests
SET reject_notify_mail_checked=TRUE,
    reject_notify_mail_sent=$2,
    reject_notify_mail_error=$3,
    updated_at=NOW()
WHERE request_id=$1
  AND status='rejected'`, requestID, sent, mailError)
	if err != nil {
		return err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) VerifyUserPassword(ctx context.Context, username string, password string) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, errors.New("username 不能为空")
	}
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM user_accounts WHERE username=$1`, username).Scan(&hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return false, nil
	}
	return true, nil
}

// LoginHintByUsername 返回登录阻断原因：
// - pending_review: 该账号处于注册审核中
// - pending_email_verification: 该账号尚未完成邮箱验证
// - blacklisted_account: 该账号已被加入平台黑名单
func (s *Store) LoginHintByUsername(ctx context.Context, username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", nil
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM registration_requests WHERE status='pending' AND username=$1
)`, username).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return "pending_review", nil
	}
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM registration_email_verifications
  WHERE consumed_at IS NULL AND expire_at > NOW() AND username=$1
)`, username).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return "pending_email_verification", nil
	}
	exists, err := s.IsUserManuallyBlocked(ctx, username)
	if err != nil {
		return "", err
	}
	if exists {
		return "blacklisted_account", nil
	}
	return "", nil
}

func (s *Store) IsUserManuallyBlocked(ctx context.Context, username string) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM ssh_list_sources
  WHERE list_type='blacklist'
    AND source_type='platform'
    AND source_platform_username=$1
)`, username).Scan(&exists)
	if err == nil {
		return exists, nil
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if !(strings.Contains(msg, "relation") && strings.Contains(msg, "ssh_list_sources") && strings.Contains(msg, "does not exist")) {
		return false, err
	}
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM users u
  WHERE u.username=$1
    AND u.status='blocked'
    AND EXISTS (
      SELECT 1
      FROM ssh_blacklist sb
      WHERE (sb.node_id='*' AND sb.local_username=$1)
         OR (sb.node_id='*' AND EXISTS (
              SELECT 1
              FROM user_node_accounts una
              WHERE una.billing_username=$1
                AND una.local_username=sb.local_username
            ))
         OR EXISTS (
              SELECT 1
              FROM user_node_accounts una
              WHERE una.billing_username=$1
                AND una.node_id=sb.node_id
                AND una.local_username=sb.local_username
            )
    )
)`, username).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Store) GetUserAccountByUsername(ctx context.Context, username string) (UserAccount, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return UserAccount{}, errors.New("username 不能为空")
	}
	var out UserAccount
	var platformUID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT username, email, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, platform_uid, role, COALESCE(two_factor_enabled, FALSE), last_login_at, created_at, updated_at
FROM user_accounts
WHERE username=$1`, username).Scan(
		&out.Username, &out.Email, &out.RealName, &out.StudentID, &out.Advisor, &out.ExpectedGraduationYear, &out.ExpectedGraduationMonth,
		&out.Phone, &platformUID, &out.Role, &out.TwoFactorEnabled, &out.LastLoginAt, &out.CreatedAt, &out.UpdatedAt,
	)
	if platformUID.Valid && platformUID.Int64 > 0 {
		v := int(platformUID.Int64)
		out.PlatformUID = &v
	}
	return out, err
}

func (s *Store) SetUserManualStatusTx(ctx context.Context, tx *sql.Tx, username string, status string) (User, error) {
	username = strings.TrimSpace(username)
	status = strings.TrimSpace(status)
	if username == "" {
		return User{}, errors.New("username 不能为空")
	}
	if status != "normal" && status != "blocked" {
		return User{}, errors.New("status 仅支持 normal/blocked")
	}
	u, err := s.EnsureUserTx(ctx, tx, username, 0)
	if err != nil {
		return User{}, err
	}
	var blockedAt *time.Time
	if status == "blocked" {
		now := time.Now()
		blockedAt = &now
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE users
SET status=$2, blocked_at=$3
WHERE username=$1`, username, status, blockedAt); err != nil {
		return User{}, err
	}
	u.Status = status
	u.BlockedAt = blockedAt
	return u, nil
}

func (s *Store) SetUserManualStatus(ctx context.Context, username string, status string) (User, error) {
	var out User
	if err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var txErr error
		out, txErr = s.SetUserManualStatusTx(ctx, tx, username, status)
		return txErr
	}); err != nil {
		return User{}, err
	}
	return out, nil
}

func (s *Store) ArchiveUserAccountTx(ctx context.Context, tx *sql.Tx, username string, deletedBy string, reason string) (DeletedUserAccount, error) {
	username = strings.TrimSpace(username)
	deletedBy = strings.TrimSpace(deletedBy)
	reason = strings.TrimSpace(reason)
	if username == "" {
		return DeletedUserAccount{}, errors.New("username 不能为空")
	}
	if deletedBy == "" {
		deletedBy = "admin"
	}
	if reason == "" {
		reason = "管理员删除"
	}
	type rowData struct {
		Username                string
		Email                   string
		StudentID               string
		PasswordHash            string
		RealName                string
		Advisor                 string
		ExpectedGraduationYear  int
		ExpectedGraduationMonth int
		Phone                   string
		PlatformUID             sql.NullInt64
		Role                    string
		LastLoginAt             sql.NullTime
		AccountCreatedAt        sql.NullTime
		AccountUpdatedAt        sql.NullTime
		Balance                 sql.NullFloat64
		CarryoverBalance        sql.NullFloat64
		UserStatus              sql.NullString
		BlockedAt               sql.NullTime
	}
	var r rowData
	err := tx.QueryRowContext(ctx, `
SELECT username, email, student_id, password_hash, real_name, advisor,
       expected_graduation_year, expected_graduation_month, phone, platform_uid, role,
       last_login_at, created_at, updated_at
FROM user_accounts
WHERE username=$1
FOR UPDATE`, username).Scan(
		&r.Username, &r.Email, &r.StudentID, &r.PasswordHash, &r.RealName, &r.Advisor,
		&r.ExpectedGraduationYear, &r.ExpectedGraduationMonth, &r.Phone, &r.PlatformUID, &r.Role,
		&r.LastLoginAt, &r.AccountCreatedAt, &r.AccountUpdatedAt,
	)
	if err != nil {
		return DeletedUserAccount{}, err
	}
	// users 可能不存在；单独查询并加锁（若存在）避免 outer join + FOR UPDATE 限制。
	var balance float64
	var userStatus string
	err = tx.QueryRowContext(ctx, `
SELECT balance, carryover_balance, status, blocked_at
FROM users
WHERE username=$1
FOR UPDATE`, username).Scan(&balance, &r.CarryoverBalance, &userStatus, &r.BlockedAt)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return DeletedUserAccount{}, err
		}
	} else {
		r.Balance = sql.NullFloat64{Float64: balance, Valid: true}
		r.CarryoverBalance = sql.NullFloat64{Float64: r.CarryoverBalance.Float64, Valid: true}
		r.UserStatus = sql.NullString{String: strings.TrimSpace(userStatus), Valid: true}
	}

	// 归档删除前先快照其节点账号映射，便于后续恢复账号时自动恢复映射关系。
	nodeAccounts := make([]UserNodeAccount, 0)
	if rows, qErr := tx.QueryContext(ctx, `
SELECT node_id, local_username, billing_username, created_at, updated_at
FROM user_node_accounts
WHERE billing_username=$1
ORDER BY node_id, local_username`, username); qErr != nil {
		return DeletedUserAccount{}, qErr
	} else {
		defer rows.Close()
		for rows.Next() {
			var a UserNodeAccount
			if err := rows.Scan(&a.NodeID, &a.LocalUsername, &a.BillingUsername, &a.CreatedAt, &a.UpdatedAt); err != nil {
				return DeletedUserAccount{}, err
			}
			nodeAccounts = append(nodeAccounts, a)
		}
		if err := rows.Err(); err != nil {
			return DeletedUserAccount{}, err
		}
	}
	nodeAccountsJSON := "[]"
	if raw, mErr := json.Marshal(nodeAccounts); mErr == nil {
		nodeAccountsJSON = string(raw)
	}

	var out DeletedUserAccount
	var archivedPlatformUID sql.NullInt64
	var restoredAt sql.NullTime
	var restoredBy sql.NullString
	err = tx.QueryRowContext(ctx, `
INSERT INTO deleted_user_accounts(
  username, email, student_id, password_hash, real_name, advisor, expected_graduation_year, expected_graduation_month,
  phone, platform_uid, role, last_login_at, account_created_at, account_updated_at, balance, carryover_balance, user_status, blocked_at, deleted_by, delete_reason, node_accounts_json
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21::jsonb)
RETURNING deleted_id, username, email, student_id, real_name, advisor, expected_graduation_year, expected_graduation_month,
          phone, platform_uid, role, balance, carryover_balance, user_status, deleted_at, deleted_by, delete_reason, restored_at, restored_by`,
		r.Username, r.Email, r.StudentID, r.PasswordHash, r.RealName, r.Advisor, r.ExpectedGraduationYear, r.ExpectedGraduationMonth,
		r.Phone, r.PlatformUID, r.Role, r.LastLoginAt, r.AccountCreatedAt, r.AccountUpdatedAt,
		func() float64 {
			if r.Balance.Valid {
				return r.Balance.Float64
			}
			return 0
		}(),
		func() float64 {
			if r.CarryoverBalance.Valid {
				return r.CarryoverBalance.Float64
			}
			return 0
		}(),
		func() string {
			if r.UserStatus.Valid && strings.TrimSpace(r.UserStatus.String) != "" {
				return strings.TrimSpace(r.UserStatus.String)
			}
			return "normal"
		}(),
		r.BlockedAt, deletedBy, reason, nodeAccountsJSON,
	).Scan(
		&out.DeletedID, &out.Username, &out.Email, &out.StudentID, &out.RealName, &out.Advisor, &out.ExpectedGraduationYear, &out.ExpectedGraduationMonth,
		&out.Phone, &archivedPlatformUID, &out.Role, &out.Balance, &out.CarryoverBalance, &out.UserStatus, &out.DeletedAt, &out.DeletedBy, &out.DeleteReason, &restoredAt, &restoredBy,
	)
	if err != nil && (isColumnMissingErr(err, "node_accounts_json") || isColumnMissingErr(err, "platform_uid")) {
		// 兼容旧库：尚未升级 node_accounts_json 字段时，降级为旧写法。
		err = tx.QueryRowContext(ctx, `
INSERT INTO deleted_user_accounts(
  username, email, student_id, password_hash, real_name, advisor, expected_graduation_year, expected_graduation_month,
  phone, role, last_login_at, account_created_at, account_updated_at, balance, user_status, blocked_at, deleted_by, delete_reason
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
RETURNING deleted_id, username, email, student_id, real_name, advisor, expected_graduation_year, expected_graduation_month,
          phone, role, balance, user_status, deleted_at, deleted_by, delete_reason, restored_at, restored_by`,
			r.Username, r.Email, r.StudentID, r.PasswordHash, r.RealName, r.Advisor, r.ExpectedGraduationYear, r.ExpectedGraduationMonth,
			r.Phone, r.Role, r.LastLoginAt, r.AccountCreatedAt, r.AccountUpdatedAt,
			func() float64 {
				if r.Balance.Valid {
					return r.Balance.Float64
				}
				return 0
			}(),
			func() string {
				if r.UserStatus.Valid && strings.TrimSpace(r.UserStatus.String) != "" {
					return strings.TrimSpace(r.UserStatus.String)
				}
				return "normal"
			}(),
			r.BlockedAt, deletedBy, reason,
		).Scan(
			&out.DeletedID, &out.Username, &out.Email, &out.StudentID, &out.RealName, &out.Advisor, &out.ExpectedGraduationYear, &out.ExpectedGraduationMonth,
			&out.Phone, &out.Role, &out.Balance, &out.UserStatus, &out.DeletedAt, &out.DeletedBy, &out.DeleteReason, &restoredAt, &restoredBy,
		)
	}
	if err != nil {
		return DeletedUserAccount{}, err
	}
	if archivedPlatformUID.Valid && archivedPlatformUID.Int64 > 0 {
		v := int(archivedPlatformUID.Int64)
		out.PlatformUID = &v
	}
	decorateDeletedUserAccountUIDRelease(&out, time.Now())

	// 删除顺序必须“先子后父”：
	// 某些已部署实例可能存在历史外键（例如 users.username -> user_accounts.username），
	// 若先删 user_accounts 会触发外键拦截。这里统一先清理所有可能引用该平台账号的业务表。
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_records WHERE username=$1`, username); err != nil {
		return DeletedUserAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM recharge_records WHERE username=$1`, username); err != nil {
		return DeletedUserAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_node_accounts WHERE billing_username=$1`, username); err != nil {
		return DeletedUserAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_requests WHERE billing_username=$1`, username); err != nil {
		return DeletedUserAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM profile_change_requests
WHERE billing_username=$1 OR old_username=$1 OR new_username=$1`, username); err != nil {
		return DeletedUserAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM special_monthly_points_rules WHERE username=$1`, username); err != nil {
		return DeletedUserAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE username=$1`, username); err != nil {
		return DeletedUserAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_accounts WHERE username=$1`, username); err != nil {
		return DeletedUserAccount{}, err
	}
	return out, nil
}

func (s *Store) conflictFieldsForRestoreTx(ctx context.Context, tx *sql.Tx, username string, email string, studentID string) ([]string, error) {
	dup := make([]string, 0, 3)
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE username=$1)`, username).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		dup = append(dup, "用户名")
	}
	exists = false
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE email=$1)`, email).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		dup = append(dup, "邮箱")
	}
	exists = false
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE student_id=$1)`, studentID).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		dup = append(dup, "学号")
	}
	var reqID int
	if err := tx.QueryRowContext(ctx, `SELECT request_id FROM registration_requests WHERE status='pending' AND username=$1 LIMIT 1`, username).Scan(&reqID); err == nil {
		dup = append(dup, "用户名")
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT request_id FROM registration_requests WHERE status='pending' AND email=$1 LIMIT 1`, email).Scan(&reqID); err == nil {
		dup = append(dup, "邮箱")
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT request_id FROM registration_requests WHERE status='pending' AND student_id=$1 LIMIT 1`, studentID).Scan(&reqID); err == nil {
		dup = append(dup, "学号")
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `
SELECT verify_id
FROM registration_email_verifications
WHERE consumed_at IS NULL AND expire_at > NOW() AND username=$1
LIMIT 1`, username).Scan(&reqID); err == nil {
		dup = append(dup, "用户名")
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `
SELECT verify_id
FROM registration_email_verifications
WHERE consumed_at IS NULL AND expire_at > NOW() AND email=$1
LIMIT 1`, email).Scan(&reqID); err == nil {
		dup = append(dup, "邮箱")
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `
SELECT verify_id
FROM registration_email_verifications
WHERE consumed_at IS NULL AND expire_at > NOW() AND student_id=$1
LIMIT 1`, studentID).Scan(&reqID); err == nil {
		dup = append(dup, "学号")
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(dup))
	for _, x := range dup {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out, nil
}

func (s *Store) RestoreDeletedUserAccountTx(ctx context.Context, tx *sql.Tx, deletedID int64, restoredBy string) (DeletedUserAccount, error) {
	if deletedID <= 0 {
		return DeletedUserAccount{}, errors.New("deleted_id 不合法")
	}
	restoredBy = strings.TrimSpace(restoredBy)
	if restoredBy == "" {
		restoredBy = "admin"
	}
	type deletedRow struct {
		DeletedUserAccount
		PlatformUIDRaw   sql.NullInt64
		PasswordHash     string
		LastLoginAt      sql.NullTime
		AccountCreatedAt sql.NullTime
		AccountUpdatedAt sql.NullTime
		BlockedAt        sql.NullTime
		NodeAccountsJSON string
	}
	var r deletedRow
	var restoredAt sql.NullTime
	var restoredByPrev sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT deleted_id, username, email, student_id, real_name, advisor, expected_graduation_year, expected_graduation_month,
       phone, platform_uid, role, balance, carryover_balance, user_status, deleted_at, deleted_by, delete_reason, restored_at, restored_by,
       password_hash, last_login_at, account_created_at, account_updated_at, blocked_at, COALESCE(node_accounts_json::text, '[]')
FROM deleted_user_accounts
WHERE deleted_id=$1
FOR UPDATE`, deletedID).Scan(
		&r.DeletedID, &r.Username, &r.Email, &r.StudentID, &r.RealName, &r.Advisor, &r.ExpectedGraduationYear, &r.ExpectedGraduationMonth,
		&r.Phone, &r.PlatformUIDRaw, &r.Role, &r.Balance, &r.CarryoverBalance, &r.UserStatus, &r.DeletedAt, &r.DeletedBy, &r.DeleteReason, &restoredAt, &restoredByPrev,
		&r.PasswordHash, &r.LastLoginAt, &r.AccountCreatedAt, &r.AccountUpdatedAt, &r.BlockedAt, &r.NodeAccountsJSON,
	)
	if err != nil && (isColumnMissingErr(err, "node_accounts_json") || isColumnMissingErr(err, "platform_uid")) {
		err = tx.QueryRowContext(ctx, `
SELECT deleted_id, username, email, student_id, real_name, advisor, expected_graduation_year, expected_graduation_month,
       phone, role, balance, carryover_balance, user_status, deleted_at, deleted_by, delete_reason, restored_at, restored_by,
       password_hash, last_login_at, account_created_at, account_updated_at, blocked_at
FROM deleted_user_accounts
WHERE deleted_id=$1
FOR UPDATE`, deletedID).Scan(
			&r.DeletedID, &r.Username, &r.Email, &r.StudentID, &r.RealName, &r.Advisor, &r.ExpectedGraduationYear, &r.ExpectedGraduationMonth,
			&r.Phone, &r.Role, &r.Balance, &r.CarryoverBalance, &r.UserStatus, &r.DeletedAt, &r.DeletedBy, &r.DeleteReason, &restoredAt, &restoredByPrev,
			&r.PasswordHash, &r.LastLoginAt, &r.AccountCreatedAt, &r.AccountUpdatedAt, &r.BlockedAt,
		)
		r.NodeAccountsJSON = "[]"
	}
	if err != nil {
		return DeletedUserAccount{}, err
	}
	if r.PlatformUIDRaw.Valid && r.PlatformUIDRaw.Int64 > 0 {
		v := int(r.PlatformUIDRaw.Int64)
		r.PlatformUID = &v
	}
	if restoredAt.Valid {
		return DeletedUserAccount{}, errors.New("该删除记录已恢复，不能重复恢复")
	}
	dup, err := s.conflictFieldsForRestoreTx(ctx, tx, r.Username, strings.ToLower(r.Email), r.StudentID)
	if err != nil {
		return DeletedUserAccount{}, err
	}
	if len(dup) > 0 {
		return DeletedUserAccount{}, fmt.Errorf("恢复失败，以下字段与现有/待审核冲突：%s", strings.Join(dup, "、"))
	}
	platformUID := 0
	if r.PlatformUID != nil && *r.PlatformUID > 0 {
		candidate := *r.PlatformUID
		if _, sensitive := platformUIDSensitiveValues[candidate]; !sensitive {
			var occupied bool
			if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM user_accounts WHERE platform_uid=$1
  UNION ALL
  SELECT 1 FROM power_users WHERE platform_uid=$1
  UNION ALL
  SELECT 1 FROM admin_accounts WHERE platform_uid=$1
)`, candidate).Scan(&occupied); err != nil {
				return DeletedUserAccount{}, err
			}
			if !occupied {
				platformUID = candidate
			}
		}
	}
	if platformUID == 0 {
		allocatedUID, _, allocErr := s.allocatePlatformUIDTx(ctx, tx, time.Now(), nil)
		if allocErr != nil {
			return DeletedUserAccount{}, allocErr
		}
		platformUID = allocatedUID
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO user_accounts(
  username, email, password_hash, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, platform_uid, role, last_login_at
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		r.Username, strings.ToLower(r.Email), r.PasswordHash, r.RealName, r.StudentID, r.Advisor, r.ExpectedGraduationYear, r.ExpectedGraduationMonth, r.Phone, platformUID, r.Role, r.LastLoginAt,
	)
	if err != nil {
		return DeletedUserAccount{}, err
	}
	restoredUID := platformUID
	r.PlatformUID = &restoredUID
	status := strings.TrimSpace(r.UserStatus)
	if status == "" {
		status = "normal"
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO users(username, balance, carryover_balance, status, blocked_at)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT (username) DO UPDATE SET balance=EXCLUDED.balance, carryover_balance=EXCLUDED.carryover_balance, status=EXCLUDED.status, blocked_at=EXCLUDED.blocked_at`,
		r.Username, r.Balance, r.CarryoverBalance, status, r.BlockedAt,
	)
	if err != nil {
		return DeletedUserAccount{}, err
	}

	// 恢复删除前快照的节点账号映射：冲突时（已被他人占用）跳过该条，不影响账号恢复主流程。
	if strings.TrimSpace(r.NodeAccountsJSON) != "" {
		var snapshot []UserNodeAccount
		if err := json.Unmarshal([]byte(r.NodeAccountsJSON), &snapshot); err == nil {
			seen := map[string]struct{}{}
			for _, a := range snapshot {
				nodeID := strings.TrimSpace(a.NodeID)
				localUsername := strings.TrimSpace(a.LocalUsername)
				if nodeID == "" || localUsername == "" {
					continue
				}
				key := nodeID + "|" + localUsername
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				existingBilling, found, err := s.ResolveBillingUsernameTx(ctx, tx, nodeID, localUsername)
				if err != nil {
					return DeletedUserAccount{}, err
				}
				if found && strings.TrimSpace(existingBilling) != "" && strings.TrimSpace(existingBilling) != r.Username {
					// 已被其他平台账号占用，跳过该映射。
					continue
				}
				if err := s.UpsertUserNodeAccountTx(ctx, tx, nodeID, localUsername, r.Username); err != nil {
					return DeletedUserAccount{}, err
				}
			}
		}
	}
	now := time.Now()
	if _, err := tx.ExecContext(ctx, `
UPDATE deleted_user_accounts
SET restored_at=$2, restored_by=$3
WHERE deleted_id=$1`, deletedID, now, restoredBy); err != nil {
		return DeletedUserAccount{}, err
	}
	out := r.DeletedUserAccount
	out.RestoredAt = &now
	out.RestoredBy = &restoredBy
	decorateDeletedUserAccountUIDRelease(&out, now)
	return out, nil
}

func (s *Store) ListDeletedUserAccounts(ctx context.Context, includeRestored bool, limit int) ([]DeletedUserAccount, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	query := `
SELECT deleted_id, username, email, student_id, real_name, advisor, expected_graduation_year, expected_graduation_month,
       phone, platform_uid, role, balance, carryover_balance, user_status, deleted_at, deleted_by, delete_reason, restored_at, restored_by
FROM deleted_user_accounts`
	args := []any{}
	if !includeRestored {
		query += ` WHERE restored_at IS NULL`
	}
	query += ` ORDER BY deleted_at DESC LIMIT $1`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DeletedUserAccount, 0)
	now := time.Now()
	for rows.Next() {
		var r DeletedUserAccount
		var platformUID sql.NullInt64
		var restoredAt sql.NullTime
		var restoredBy sql.NullString
		if err := rows.Scan(
			&r.DeletedID, &r.Username, &r.Email, &r.StudentID, &r.RealName, &r.Advisor, &r.ExpectedGraduationYear, &r.ExpectedGraduationMonth,
			&r.Phone, &platformUID, &r.Role, &r.Balance, &r.CarryoverBalance, &r.UserStatus, &r.DeletedAt, &r.DeletedBy, &r.DeleteReason, &restoredAt, &restoredBy,
		); err != nil {
			return nil, err
		}
		if platformUID.Valid && platformUID.Int64 > 0 {
			v := int(platformUID.Int64)
			r.PlatformUID = &v
		}
		if restoredAt.Valid {
			v := restoredAt.Time
			r.RestoredAt = &v
		}
		if restoredBy.Valid {
			v := restoredBy.String
			r.RestoredBy = &v
		}
		decorateDeletedUserAccountUIDRelease(&r, now)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) FindPlatformUsersByIdentity(ctx context.Context, username string, email string, studentID string, limit int) ([]UserAccount, []DeletedUserAccount, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(strings.ToLower(email))
	studentID = strings.TrimSpace(studentID)
	if username == "" && email == "" && studentID == "" {
		return nil, nil, errors.New("username/email/student_id 不能全为空")
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	buildWhere := func(startAt int) (string, []any) {
		args := make([]any, 0, 3)
		conds := make([]string, 0, 3)
		idx := startAt
		if username != "" {
			conds = append(conds, fmt.Sprintf("username=$%d", idx))
			args = append(args, username)
			idx++
		}
		if email != "" {
			conds = append(conds, fmt.Sprintf("email=$%d", idx))
			args = append(args, email)
			idx++
		}
		if studentID != "" {
			conds = append(conds, fmt.Sprintf("student_id=$%d", idx))
			args = append(args, studentID)
			idx++
		}
		return strings.Join(conds, " OR "), args
	}

	whereActive, activeArgs := buildWhere(1)
	activeArgs = append(activeArgs, limit)
	activeRows, err := s.db.QueryContext(ctx, `
SELECT username, email, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, platform_uid, role, last_login_at, created_at, updated_at
FROM user_accounts
WHERE `+whereActive+`
ORDER BY username
LIMIT $`+strconv.Itoa(len(activeArgs)), activeArgs...)
	if err != nil {
		return nil, nil, err
	}
	defer activeRows.Close()
	active := make([]UserAccount, 0)
	for activeRows.Next() {
		var x UserAccount
		var platformUID sql.NullInt64
		if err := activeRows.Scan(&x.Username, &x.Email, &x.RealName, &x.StudentID, &x.Advisor, &x.ExpectedGraduationYear, &x.ExpectedGraduationMonth, &x.Phone, &platformUID, &x.Role, &x.LastLoginAt, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, nil, err
		}
		if platformUID.Valid && platformUID.Int64 > 0 {
			v := int(platformUID.Int64)
			x.PlatformUID = &v
		}
		active = append(active, x)
	}
	if err := activeRows.Err(); err != nil {
		return nil, nil, err
	}

	whereDeleted, deletedArgs := buildWhere(1)
	deletedArgs = append(deletedArgs, limit)
	archRows, err := s.db.QueryContext(ctx, `
SELECT deleted_id, username, email, student_id, real_name, advisor, expected_graduation_year, expected_graduation_month,
       phone, platform_uid, role, balance, carryover_balance, user_status, deleted_at, deleted_by, delete_reason, restored_at, restored_by
FROM deleted_user_accounts
WHERE (`+whereDeleted+`) AND restored_at IS NULL
ORDER BY deleted_at DESC
LIMIT $`+strconv.Itoa(len(deletedArgs)), deletedArgs...)
	if err != nil {
		return nil, nil, err
	}
	defer archRows.Close()
	deleted := make([]DeletedUserAccount, 0)
	now := time.Now()
	for archRows.Next() {
		var x DeletedUserAccount
		var platformUID sql.NullInt64
		var restoredAt sql.NullTime
		var restoredBy sql.NullString
		if err := archRows.Scan(
			&x.DeletedID, &x.Username, &x.Email, &x.StudentID, &x.RealName, &x.Advisor, &x.ExpectedGraduationYear, &x.ExpectedGraduationMonth,
			&x.Phone, &platformUID, &x.Role, &x.Balance, &x.CarryoverBalance, &x.UserStatus, &x.DeletedAt, &x.DeletedBy, &x.DeleteReason, &restoredAt, &restoredBy,
		); err != nil {
			return nil, nil, err
		}
		if platformUID.Valid && platformUID.Int64 > 0 {
			v := int(platformUID.Int64)
			x.PlatformUID = &v
		}
		decorateDeletedUserAccountUIDRelease(&x, now)
		deleted = append(deleted, x)
	}
	return active, deleted, archRows.Err()
}

func (s *Store) GetAdminProfile(ctx context.Context, username string) (AdminProfile, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return AdminProfile{}, errors.New("username 不能为空")
	}
	var out AdminProfile
	err := s.db.QueryRowContext(ctx, `
SELECT username, real_name, email, phone, created_at, updated_at
FROM admin_profiles
WHERE username=$1`, username).Scan(&out.Username, &out.RealName, &out.Email, &out.Phone, &out.CreatedAt, &out.UpdatedAt)
	if err == nil {
		return out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AdminProfile{}, err
	}
	// 首次访问自动初始化档案
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO admin_profiles(username, real_name, email, phone)
VALUES($1,'','','')
ON CONFLICT (username) DO NOTHING`, username); err != nil {
		return AdminProfile{}, err
	}
	err = s.db.QueryRowContext(ctx, `
SELECT username, real_name, email, phone, created_at, updated_at
FROM admin_profiles
WHERE username=$1`, username).Scan(&out.Username, &out.RealName, &out.Email, &out.Phone, &out.CreatedAt, &out.UpdatedAt)
	return out, err
}

func (s *Store) UpsertAdminProfile(ctx context.Context, username string, realName string, email string, phone string) (AdminProfile, error) {
	username = strings.TrimSpace(username)
	realName = strings.TrimSpace(realName)
	email = strings.TrimSpace(strings.ToLower(email))
	phone = strings.TrimSpace(phone)
	if username == "" {
		return AdminProfile{}, errors.New("username 不能为空")
	}
	if realName == "" {
		return AdminProfile{}, errors.New("真实姓名不能为空")
	}
	var out AdminProfile
	err := s.db.QueryRowContext(ctx, `
INSERT INTO admin_profiles(username, real_name, email, phone)
VALUES($1,$2,$3,$4)
ON CONFLICT (username) DO UPDATE SET
  real_name=EXCLUDED.real_name,
  email=EXCLUDED.email,
  phone=EXCLUDED.phone,
  updated_at=NOW()
RETURNING username, real_name, email, phone, created_at, updated_at`,
		username, realName, email, phone,
	).Scan(&out.Username, &out.RealName, &out.Email, &out.Phone, &out.CreatedAt, &out.UpdatedAt)
	return out, err
}

func (s *Store) UpdateUserPassword(ctx context.Context, username string, oldPassword string, newPassword string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if err := validateStrongPassword(newPassword); err != nil {
		return err
	}
	var oldHash string
	if err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM user_accounts WHERE username=$1`, username).Scan(&oldHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("账号不存在")
		}
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(oldHash), []byte(oldPassword)); err != nil {
		return errors.New("旧密码不正确")
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE user_accounts
SET password_hash=$2, updated_at=NOW()
WHERE username=$1`, username, string(newHash))
	if err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `
UPDATE power_users
SET password_hash=$2, updated_at=NOW()
WHERE username=$1`, username, string(newHash))
	return err
}

func (s *Store) UpdateAdminPassword(ctx context.Context, username string, oldPassword string, newPassword string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if err := validateStrongPassword(newPassword); err != nil {
		return err
	}
	var oldHash string
	if err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM admin_accounts WHERE username=$1`, username).Scan(&oldHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("账号不存在")
		}
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(oldHash), []byte(oldPassword)); err != nil {
		return errors.New("旧密码不正确")
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE admin_accounts
SET password_hash=$2, updated_at=NOW()
WHERE username=$1`, username, string(newHash))
	return err
}

func (s *Store) SetPasswordResetTokenByEmail(ctx context.Context, email string, tokenHash string, expireAt time.Time) (string, bool, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return "", false, errors.New("email 不能为空")
	}
	if tokenHash == "" {
		return "", false, errors.New("token_hash 不能为空")
	}
	var username string
	err := s.db.QueryRowContext(ctx, `
UPDATE user_accounts
SET reset_token_hash=$2, reset_token_expire_at=$3, updated_at=NOW()
WHERE email=$1
RETURNING username`, email, tokenHash, expireAt).Scan(&username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return username, true, nil
}

func (s *Store) ResetPasswordByToken(ctx context.Context, username string, tokenHash string, newPassword string, now time.Time) error {
	username = strings.TrimSpace(username)
	if username == "" || tokenHash == "" {
		return errors.New("username/token 不能为空")
	}
	if err := validateStrongPassword(newPassword); err != nil {
		return err
	}
	var dbHash sql.NullString
	var expireAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
SELECT reset_token_hash, reset_token_expire_at
FROM user_accounts
WHERE username=$1`, username).Scan(&dbHash, &expireAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("账号不存在")
		}
		return err
	}
	if !dbHash.Valid || !expireAt.Valid || strings.TrimSpace(dbHash.String) == "" {
		return errors.New("重置链接无效")
	}
	if now.After(expireAt.Time) {
		return errors.New("重置链接已过期")
	}
	if dbHash.String != tokenHash {
		return errors.New("重置链接无效")
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE user_accounts
SET password_hash=$2, reset_token_hash=NULL, reset_token_expire_at=NULL, updated_at=NOW()
WHERE username=$1`, username, string(newHash))
	if err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `
UPDATE power_users
SET password_hash=$2, updated_at=NOW()
WHERE username=$1`, username, string(newHash))
	return nil
}

func parseAppSettingTime(v string) *time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	t, err := parseTimeFlexible(v)
	if err != nil {
		return nil
	}
	return &t
}

func (s *Store) GetUsageRetentionSettings(ctx context.Context) (UsageRetentionSettings, error) {
	out := UsageRetentionSettings{
		RetentionDays: 0,
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM app_settings WHERE key = ANY($1)`, pq.Array([]string{
		appSettingUsageRetentionDays,
		appSettingUsageLastDeleteAt,
		appSettingUsageLastDeleteFrom,
		appSettingUsageLastDeleteTo,
		appSettingUsageLastDeleteCount,
		appSettingUsageLastDeleteMode,
		appSettingUsageLastDeleteBy,
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
		case appSettingUsageRetentionDays:
			if n, convErr := strconv.Atoi(value); convErr == nil && n >= 0 {
				out.RetentionDays = n
			}
		case appSettingUsageLastDeleteAt:
			out.LastDeletedAt = parseAppSettingTime(value)
		case appSettingUsageLastDeleteFrom:
			out.LastDeletedFrom = parseAppSettingTime(value)
		case appSettingUsageLastDeleteTo:
			out.LastDeletedTo = parseAppSettingTime(value)
		case appSettingUsageLastDeleteCount:
			if n, convErr := strconv.ParseInt(value, 10, 64); convErr == nil && n >= 0 {
				out.LastDeletedRecords = n
			}
		case appSettingUsageLastDeleteMode:
			out.LastDeletedMode = value
		case appSettingUsageLastDeleteBy:
			out.LastDeletedBy = value
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Store) UpsertUsageRetentionDays(ctx context.Context, retentionDays int) error {
	if retentionDays < 0 || retentionDays > 3650 {
		return errors.New("retention_days 需在 0-3650 天之间")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO app_settings(key, value, updated_at)
VALUES($1, $2, NOW())
ON CONFLICT (key) DO UPDATE
SET value=EXCLUDED.value, updated_at=NOW()`,
		appSettingUsageRetentionDays,
		strconv.Itoa(retentionDays),
	)
	return err
}

func (s *Store) RecordUsageDeleteRun(
	ctx context.Context,
	runAt time.Time,
	from *time.Time,
	to *time.Time,
	deletedRecords int64,
	mode string,
	operator string,
) error {
	if deletedRecords < 0 {
		deletedRecords = 0
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "manual"
	}
	operator = strings.TrimSpace(operator)
	if operator == "" {
		operator = "system"
	}
	items := map[string]string{
		appSettingUsageLastDeleteAt:    formatRFC3339InBeijing(runAt),
		appSettingUsageLastDeleteCount: strconv.FormatInt(deletedRecords, 10),
		appSettingUsageLastDeleteMode:  mode,
		appSettingUsageLastDeleteBy:    operator,
	}
	if from != nil && !from.IsZero() {
		items[appSettingUsageLastDeleteFrom] = formatRFC3339InBeijing(*from)
	} else {
		items[appSettingUsageLastDeleteFrom] = ""
	}
	if to != nil && !to.IsZero() {
		items[appSettingUsageLastDeleteTo] = formatRFC3339InBeijing(*to)
	} else {
		items[appSettingUsageLastDeleteTo] = ""
	}
	for key, value := range items {
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO app_settings(key, value, updated_at)
VALUES($1, $2, NOW())
ON CONFLICT (key) DO UPDATE
SET value=EXCLUDED.value, updated_at=NOW()`, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetNodeBindSecurityPolicy(ctx context.Context) (NodeBindSecurityPolicy, error) {
	out := defaultNodeBindSecurityPolicy()
	keys := []string{
		appSettingBindChallengeWindowSeconds,
		appSettingBindTrialCPUQuotaPercent,
		appSettingBindTrialMemoryLimitGB,
		appSettingBindTrialGPUBlocked,
		appSettingBindSingleActivePerBilling,
		appSettingBindFirstCooldownMinutes,
		appSettingBindRepeatCooldownMinutes,
		appSettingBindContentionFreezeMinute,
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM app_settings WHERE key = ANY($1)`, pq.Array(keys))
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
		case appSettingBindChallengeWindowSeconds:
			if n, convErr := strconv.Atoi(value); convErr == nil {
				out.ChallengeWindowSeconds = n
			}
		case appSettingBindTrialCPUQuotaPercent:
			if f, convErr := strconv.ParseFloat(value, 64); convErr == nil {
				out.TrialCPUQuotaPercent = f
			}
		case appSettingBindTrialMemoryLimitGB:
			if f, convErr := strconv.ParseFloat(value, 64); convErr == nil {
				out.TrialMemoryLimitGB = f
			}
		case appSettingBindTrialGPUBlocked:
			v := strings.ToLower(value)
			out.TrialGPUBlocked = (v == "1" || v == "true" || v == "yes" || v == "on")
		case appSettingBindSingleActivePerBilling:
			v := strings.ToLower(value)
			out.SingleActivePerBilling = (v == "1" || v == "true" || v == "yes" || v == "on")
		case appSettingBindFirstCooldownMinutes:
			if n, convErr := strconv.Atoi(value); convErr == nil {
				out.FirstFailureCooldownMinutes = n
			}
		case appSettingBindRepeatCooldownMinutes:
			if n, convErr := strconv.Atoi(value); convErr == nil {
				out.RepeatFailureCooldownMinutes = n
			}
		case appSettingBindContentionFreezeMinute:
			if n, convErr := strconv.Atoi(value); convErr == nil {
				out.ContentionFreezeMinutes = n
			}
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return normalizeNodeBindSecurityPolicy(out), nil
}

func (s *Store) UpsertNodeBindSecurityPolicy(ctx context.Context, policy NodeBindSecurityPolicy) error {
	p := normalizeNodeBindSecurityPolicy(policy)
	items := map[string]string{
		appSettingBindChallengeWindowSeconds: strconv.Itoa(p.ChallengeWindowSeconds),
		appSettingBindTrialCPUQuotaPercent:   strconv.FormatFloat(p.TrialCPUQuotaPercent, 'f', -1, 64),
		appSettingBindTrialMemoryLimitGB:     strconv.FormatFloat(p.TrialMemoryLimitGB, 'f', -1, 64),
		appSettingBindTrialGPUBlocked:        strconv.FormatBool(p.TrialGPUBlocked),
		appSettingBindSingleActivePerBilling: strconv.FormatBool(p.SingleActivePerBilling),
		appSettingBindFirstCooldownMinutes:   strconv.Itoa(p.FirstFailureCooldownMinutes),
		appSettingBindRepeatCooldownMinutes:  strconv.Itoa(p.RepeatFailureCooldownMinutes),
		appSettingBindContentionFreezeMinute: strconv.Itoa(p.ContentionFreezeMinutes),
	}
	for key, value := range items {
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO app_settings(key, value, updated_at)
VALUES($1,$2,NOW())
ON CONFLICT (key) DO UPDATE
SET value=EXCLUDED.value, updated_at=NOW()`, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetMailSettings(ctx context.Context, cfg Config) (MailSettings, error) {
	out := MailSettings{
		SMTPHost:  strings.TrimSpace(cfg.SMTPHost),
		SMTPPort:  cfg.SMTPPort,
		SMTPUser:  strings.TrimSpace(cfg.SMTPUser),
		SMTPPass:  strings.TrimSpace(cfg.SMTPPass),
		FromEmail: strings.TrimSpace(cfg.FromEmail),
		FromName:  strings.TrimSpace(cfg.FromName),
	}
	if out.SMTPHost == "" {
		out.SMTPHost = "smtp.163.com"
	}
	if out.SMTPPort == 0 {
		out.SMTPPort = 465
	}
	if out.FromEmail == "" {
		out.FromEmail = out.SMTPUser
	}
	if out.FromName == "" {
		out.FromName = "GPU Ops 团队"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM app_settings WHERE key = ANY($1)`, pq.Array([]string{
		appSettingSMTPHost, appSettingSMTPPort, appSettingSMTPUser, appSettingSMTPPass, appSettingFromEmail, appSettingFromName,
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
		switch key {
		case appSettingSMTPHost:
			out.SMTPHost = strings.TrimSpace(value)
		case appSettingSMTPPort:
			if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				out.SMTPPort = n
			}
		case appSettingSMTPUser:
			out.SMTPUser = strings.TrimSpace(value)
		case appSettingSMTPPass:
			out.SMTPPass = strings.TrimSpace(value)
		case appSettingFromEmail:
			out.FromEmail = strings.TrimSpace(value)
		case appSettingFromName:
			out.FromName = strings.TrimSpace(value)
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if out.SMTPHost == "" {
		out.SMTPHost = "smtp.163.com"
	}
	if out.SMTPPort == 0 {
		out.SMTPPort = 465
	}
	if out.FromEmail == "" {
		out.FromEmail = out.SMTPUser
	}
	if out.FromName == "" {
		out.FromName = "GPU Ops 团队"
	}
	return out, nil
}

func (s *Store) GetUserGuideline(ctx context.Context) (content string, updatedBy string, updatedAt *time.Time, err error) {
	content = strings.TrimSpace(`
## 平台用户使用准则
1. 平台账号、学号、邮箱均需真实且唯一，不得共享账号。
2. 节点使用须遵守学校与课题组安全规范，不得执行违法违规任务。
3. 请及时维护“节点账号映射”，确保计费与审计准确。
4. 收到管理员通知后请及时处理，若长期不处理可能被限制使用。
5. 每月 1 号系统会发放当月通用积分，并按“月结转上限”结转上月未用完的通用积分（该上限是结转池累计上限，不是每月新增上限）。
6. 结转积分属于通用积分池的一部分，扣费时会优先于当月通用积分被消耗。
7. 节点专属积分不会按月过期，不参与月度结转，仅在对应节点优先扣减。
8. 管理员可按规则调整通用积分、结转积分、节点专属积分。
9. 如违反准则导致账号受限或封禁，后果由本人承担。`)
	updatedBy = "system"

	var v string
	var t time.Time
	err = s.db.QueryRowContext(ctx, `
SELECT value, updated_at
FROM app_settings
WHERE key=$1`, appSettingUserGuideline).Scan(&v, &t)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return content, updatedBy, nil, nil
		}
		return "", "", nil, err
	}
	if strings.TrimSpace(v) != "" {
		content = strings.TrimSpace(v)
	}
	updatedAt = &t

	if err := s.db.QueryRowContext(ctx, `
SELECT value
FROM app_settings
WHERE key=$1`, appSettingUserGuidelineBy).Scan(&v); err == nil {
		if strings.TrimSpace(v) != "" {
			updatedBy = strings.TrimSpace(v)
		}
	}
	return content, updatedBy, updatedAt, nil
}

func (s *Store) UpsertUserGuideline(ctx context.Context, content string, updatedBy string) error {
	content = strings.TrimSpace(content)
	updatedBy = strings.TrimSpace(updatedBy)
	if content == "" {
		return errors.New("用户准则不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	items := map[string]string{
		appSettingUserGuideline:   content,
		appSettingUserGuidelineBy: updatedBy,
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

func (s *Store) UpsertMailSettings(ctx context.Context, settings MailSettings, updatePassword bool) error {
	settings.SMTPHost = strings.TrimSpace(settings.SMTPHost)
	settings.SMTPUser = strings.TrimSpace(settings.SMTPUser)
	settings.SMTPPass = strings.TrimSpace(settings.SMTPPass)
	settings.FromEmail = strings.TrimSpace(settings.FromEmail)
	settings.FromName = strings.TrimSpace(settings.FromName)
	if settings.SMTPHost == "" {
		return errors.New("SMTP 主机不能为空")
	}
	if settings.SMTPUser == "" {
		return errors.New("SMTP 用户名不能为空")
	}
	if settings.FromEmail == "" {
		settings.FromEmail = settings.SMTPUser
	}
	if settings.FromEmail == "" {
		return errors.New("发件邮箱不能为空")
	}
	if settings.FromName == "" {
		settings.FromName = "GPU Ops 团队"
	}
	if updatePassword && settings.SMTPPass == "" {
		return errors.New("SMTP 密码不能为空")
	}
	if settings.SMTPPort <= 0 || settings.SMTPPort > 65535 {
		return errors.New("smtp_port 不合法")
	}

	type kv struct {
		k string
		v string
	}
	items := []kv{
		{k: appSettingSMTPHost, v: settings.SMTPHost},
		{k: appSettingSMTPPort, v: strconv.Itoa(settings.SMTPPort)},
		{k: appSettingSMTPUser, v: settings.SMTPUser},
		{k: appSettingFromEmail, v: settings.FromEmail},
		{k: appSettingFromName, v: settings.FromName},
	}
	if updatePassword {
		items = append(items, kv{k: appSettingSMTPPass, v: settings.SMTPPass})
	}
	for _, it := range items {
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO app_settings(key, value, updated_at)
VALUES($1,$2,NOW())
ON CONFLICT (key) DO UPDATE
SET value=EXCLUDED.value, updated_at=NOW()`, it.k, it.v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListUserMailRecipients(ctx context.Context, usernames []string, limit int) ([]UserAccount, error) {
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}
	cleaned := make([]string, 0, len(usernames))
	seen := map[string]struct{}{}
	for _, u := range usernames {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		cleaned = append(cleaned, u)
	}

	args := []any{limit}
	query := `
SELECT username, email, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, role, last_login_at, created_at, updated_at
FROM user_accounts
WHERE role='user'
  AND email <> ''`
	if len(cleaned) > 0 {
		query += "\n  AND username = ANY($2)"
		args = append(args, pq.Array(cleaned))
	}
	query += "\nORDER BY username\nLIMIT $1"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]UserAccount, 0)
	for rows.Next() {
		var u UserAccount
		if err := rows.Scan(
			&u.Username,
			&u.Email,
			&u.RealName,
			&u.StudentID,
			&u.Advisor,
			&u.ExpectedGraduationYear,
			&u.ExpectedGraduationMonth,
			&u.Phone,
			&u.Role,
			&u.LastLoginAt,
			&u.CreatedAt,
			&u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) GetUserEmailByUsername(ctx context.Context, username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("username 不能为空")
	}
	var email string
	if err := s.db.QueryRowContext(ctx, `SELECT email FROM user_accounts WHERE username=$1`, username).Scan(&email); err != nil {
		return "", err
	}
	return strings.TrimSpace(email), nil
}

func (s *Store) GetMonthlyPointsConfig(ctx context.Context) (MonthlyPointsConfig, error) {
	out := MonthlyPointsConfig{
		DoctorPoints:      200,
		MasterPoints:      100,
		OtherPoints:       50,
		CarryoverLimit:    500,
		MaxOverdraftLimit: 500,
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM app_settings WHERE key = ANY($1)`, pq.Array([]string{
		appSettingMonthlyDoctorPoints, appSettingMonthlyMasterPoints, appSettingMonthlyOtherPoints, appSettingMonthlyCarryoverLimit, appSettingMonthlyMaxOverdraft,
	}))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v string
		if err := rows.Scan(&k, &v); err != nil {
			return out, err
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || f < 0 {
			continue
		}
		switch k {
		case appSettingMonthlyDoctorPoints:
			out.DoctorPoints = f
		case appSettingMonthlyMasterPoints:
			out.MasterPoints = f
		case appSettingMonthlyOtherPoints:
			out.OtherPoints = f
		case appSettingMonthlyCarryoverLimit:
			out.CarryoverLimit = f
		case appSettingMonthlyMaxOverdraft:
			out.MaxOverdraftLimit = f
		}
	}
	return out, rows.Err()
}

func (s *Store) LoadMonthlyPointsConfigTx(ctx context.Context, tx *sql.Tx) (MonthlyPointsConfig, error) {
	out := MonthlyPointsConfig{
		DoctorPoints:      200,
		MasterPoints:      100,
		OtherPoints:       50,
		CarryoverLimit:    500,
		MaxOverdraftLimit: 500,
	}
	rows, err := tx.QueryContext(ctx, `SELECT key, value FROM app_settings WHERE key = ANY($1)`, pq.Array([]string{
		appSettingMonthlyDoctorPoints, appSettingMonthlyMasterPoints, appSettingMonthlyOtherPoints, appSettingMonthlyCarryoverLimit, appSettingMonthlyMaxOverdraft,
	}))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v string
		if err := rows.Scan(&k, &v); err != nil {
			return out, err
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || f < 0 {
			continue
		}
		switch k {
		case appSettingMonthlyDoctorPoints:
			out.DoctorPoints = f
		case appSettingMonthlyMasterPoints:
			out.MasterPoints = f
		case appSettingMonthlyOtherPoints:
			out.OtherPoints = f
		case appSettingMonthlyCarryoverLimit:
			out.CarryoverLimit = f
		case appSettingMonthlyMaxOverdraft:
			out.MaxOverdraftLimit = f
		}
	}
	return out, rows.Err()
}

func (s *Store) UpsertMonthlyPointsConfig(ctx context.Context, cfg MonthlyPointsConfig) error {
	if cfg.DoctorPoints < 0 || cfg.MasterPoints < 0 || cfg.OtherPoints < 0 || cfg.CarryoverLimit < 0 || cfg.MaxOverdraftLimit < 0 {
		return errors.New("doctor_points/master_points/other_points/carryover_limit/max_overdraft_limit 不能为负数")
	}
	items := map[string]string{
		appSettingMonthlyDoctorPoints:   strconv.FormatFloat(cfg.DoctorPoints, 'f', -1, 64),
		appSettingMonthlyMasterPoints:   strconv.FormatFloat(cfg.MasterPoints, 'f', -1, 64),
		appSettingMonthlyOtherPoints:    strconv.FormatFloat(cfg.OtherPoints, 'f', -1, 64),
		appSettingMonthlyCarryoverLimit: strconv.FormatFloat(cfg.CarryoverLimit, 'f', -1, 64),
		appSettingMonthlyMaxOverdraft:   strconv.FormatFloat(cfg.MaxOverdraftLimit, 'f', -1, 64),
	}
	for k, v := range items {
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO app_settings(key, value, updated_at)
VALUES($1,$2,NOW())
ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=NOW()`, k, v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListUsageSummaryByUser(ctx context.Context, from time.Time, to time.Time, limit int, visibleNodeIDs []string) ([]UsageUserSummary, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	query := `
SELECT COALESCE(una.billing_username, ur.username) AS username,
       COUNT(1) AS usage_records,
       SUM(CASE WHEN ur.gpu_count > 0 THEN 1 ELSE 0 END) AS gpu_process_records,
       SUM(CASE WHEN ur.gpu_count = 0 THEN 1 ELSE 0 END) AS cpu_process_records,
       COALESCE(SUM(ur.cpu_percent), 0) AS total_cpu_percent,
       COALESCE(SUM(ur.memory_mb), 0) AS total_memory_mb,
       COALESCE(SUM(ur.cost), 0) AS total_cost
FROM usage_records ur
LEFT JOIN user_node_accounts una
  ON una.node_id = ur.node_id
 AND una.local_username = ur.username
WHERE ur.timestamp >= $1 AND ur.timestamp <= $2`
	args := []any{from, to, limit}
	cleaned := uniqTrim(visibleNodeIDs)
	if len(cleaned) > 0 {
		query += `
  AND ur.node_id = ANY($4)`
		args = append(args, pq.Array(cleaned))
	}
	query += `
GROUP BY COALESCE(una.billing_username, ur.username)
ORDER BY total_cost DESC
LIMIT $3`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UsageUserSummary, 0)
	for rows.Next() {
		var x UsageUserSummary
		if err := rows.Scan(&x.Username, &x.UsageRecords, &x.GPUProcessRecords, &x.CPUProcessRecords, &x.TotalCPUPercent, &x.TotalMemoryMB, &x.TotalCost); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListUsageMonthlyByUser(ctx context.Context, from time.Time, to time.Time, limit int, visibleNodeIDs []string) ([]UsageMonthlySummary, error) {
	if limit <= 0 || limit > 200000 {
		limit = 20000
	}
	query := `
WITH platform_users AS (
  SELECT username FROM user_accounts
  UNION
  SELECT username FROM admin_accounts
  UNION
  SELECT username FROM power_users
),
resolved AS (
  SELECT
    to_char(date_trunc('month', ur.timestamp), 'YYYY-MM') AS month,
    COALESCE(pu_ur.username, pu_map.username) AS username,
    ur.gpu_count,
    ur.cpu_percent,
    ur.memory_mb,
    ur.cost
  FROM usage_records ur
  LEFT JOIN user_node_accounts una
    ON una.node_id = ur.node_id
   AND una.local_username = COALESCE(NULLIF(ur.local_username, ''), ur.username)
  LEFT JOIN platform_users pu_ur ON pu_ur.username = ur.username
  LEFT JOIN platform_users pu_map ON pu_map.username = una.billing_username
  WHERE ur.timestamp >= $1
    AND ur.timestamp <= $2`
	args := []any{from, to, limit}
	cleaned := uniqTrim(visibleNodeIDs)
	if len(cleaned) > 0 {
		query += `
    AND ur.node_id = ANY($4)`
		args = append(args, pq.Array(cleaned))
	}
	query += `
    AND COALESCE(pu_ur.username, pu_map.username) IS NOT NULL
)
SELECT month,
       username,
       COUNT(1) AS usage_records,
       SUM(CASE WHEN gpu_count > 0 THEN 1 ELSE 0 END) AS gpu_process_records,
       SUM(CASE WHEN gpu_count = 0 THEN 1 ELSE 0 END) AS cpu_process_records,
       COALESCE(SUM(cpu_percent), 0) AS total_cpu_percent,
       COALESCE(SUM(memory_mb), 0) AS total_memory_mb,
       COALESCE(SUM(cost), 0) AS total_cost
FROM resolved
GROUP BY month, username
ORDER BY month DESC, total_cost DESC
LIMIT $3`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UsageMonthlySummary, 0)
	for rows.Next() {
		var x UsageMonthlySummary
		if err := rows.Scan(&x.Month, &x.Username, &x.UsageRecords, &x.GPUProcessRecords, &x.CPUProcessRecords, &x.TotalCPUPercent, &x.TotalMemoryMB, &x.TotalCost); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListAnnouncements(ctx context.Context, limit int) ([]Announcement, error) {
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT announcement_id, title, content, pinned, created_by, created_at, updated_at
FROM announcements
ORDER BY pinned DESC, created_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Announcement, 0)
	for rows.Next() {
		var a Announcement
		if err := rows.Scan(&a.AnnouncementID, &a.Title, &a.Content, &a.Pinned, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		normalizeAnnouncementTimes(&a)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) CreateAnnouncement(ctx context.Context, title string, content string, pinned bool, createdBy string) error {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	createdBy = strings.TrimSpace(createdBy)
	if title == "" || content == "" {
		return errors.New("title/content 不能为空")
	}
	if createdBy == "" {
		createdBy = "admin"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO announcements(title, content, pinned, created_by)
VALUES($1,$2,$3,$4)`, title, content, pinned, createdBy)
	return err
}

func (s *Store) DeleteAnnouncement(ctx context.Context, id int) error {
	if id <= 0 {
		return errors.New("id 不合法")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM announcements WHERE announcement_id=$1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListAdminNotes(ctx context.Context, fromDate string, toDate string, limit int) ([]AdminNote, error) {
	fromDate = strings.TrimSpace(fromDate)
	toDate = strings.TrimSpace(toDate)
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	conds := make([]string, 0, 2)
	args := make([]any, 0, 3)
	idx := 1
	if fromDate != "" {
		conds = append(conds, fmt.Sprintf("note_date >= $%d", idx))
		args = append(args, fromDate)
		idx++
	}
	if toDate != "" {
		conds = append(conds, fmt.Sprintf("note_date <= $%d", idx))
		args = append(args, toDate)
		idx++
	}
	args = append(args, limit)
	q := `
SELECT note_id, note_date, title, content, updated_by, created_at, updated_at
FROM admin_notes`
	if len(conds) > 0 {
		q += "\nWHERE " + strings.Join(conds, " AND ")
	}
	q += fmt.Sprintf("\nORDER BY note_date DESC, updated_at DESC\nLIMIT $%d", idx)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AdminNote, 0)
	for rows.Next() {
		var n AdminNote
		if err := rows.Scan(&n.NoteID, &n.NoteDate, &n.Title, &n.Content, &n.UpdatedBy, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		normalizeAdminNoteTimes(&n)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) CreateAdminNote(ctx context.Context, noteDate time.Time, title string, content string, updatedBy string) (AdminNote, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	updatedBy = strings.TrimSpace(updatedBy)
	if updatedBy == "" {
		updatedBy = "admin"
	}
	var out AdminNote
	if err := s.db.QueryRowContext(ctx, `
INSERT INTO admin_notes(note_date, title, content, updated_by)
VALUES($1,$2,$3,$4)
RETURNING note_id, note_date, title, content, updated_by, created_at, updated_at`,
		noteDate, title, content, updatedBy,
	).Scan(&out.NoteID, &out.NoteDate, &out.Title, &out.Content, &out.UpdatedBy, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return AdminNote{}, err
	}
	normalizeAdminNoteTimes(&out)
	return out, nil
}

func (s *Store) UpdateAdminNote(ctx context.Context, noteID int, noteDate time.Time, title string, content string, updatedBy string) (AdminNote, error) {
	if noteID <= 0 {
		return AdminNote{}, errors.New("note_id 不合法")
	}
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	updatedBy = strings.TrimSpace(updatedBy)
	if updatedBy == "" {
		updatedBy = "admin"
	}
	var out AdminNote
	if err := s.db.QueryRowContext(ctx, `
UPDATE admin_notes
SET note_date=$2, title=$3, content=$4, updated_by=$5, updated_at=NOW()
WHERE note_id=$1
RETURNING note_id, note_date, title, content, updated_by, created_at, updated_at`,
		noteID, noteDate, title, content, updatedBy,
	).Scan(&out.NoteID, &out.NoteDate, &out.Title, &out.Content, &out.UpdatedBy, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return AdminNote{}, err
	}
	normalizeAdminNoteTimes(&out)
	return out, nil
}

func (s *Store) DeleteAdminNote(ctx context.Context, noteID int) error {
	if noteID <= 0 {
		return errors.New("note_id 不合法")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM admin_notes WHERE note_id=$1`, noteID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListAdminUserDetails(ctx context.Context, limit int) ([]AdminUserDetail, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
WITH usage_agg AS (
  SELECT username, COUNT(1) AS usage_records, COALESCE(SUM(cost),0) AS total_cost, MAX(timestamp) AS last_usage_at
  FROM usage_records
  GROUP BY username
),
exclusive_agg AS (
  SELECT username, COALESCE(SUM(balance), 0) AS exclusive_balance
  FROM user_node_exclusive_points
  GROUP BY username
),
union_users AS (
  SELECT
    ua.username,
    ua.platform_uid,
    COALESCE(NULLIF(ua.role, ''), 'user') AS role,
    FALSE AS can_view_board,
    FALSE AS can_view_nodes,
    FALSE AS can_review_requests,
    COALESCE(ua.two_factor_enabled, FALSE) AS two_factor_enabled,
    ua.email,
    ua.student_id,
    ua.real_name,
    ua.advisor,
    ua.expected_graduation_year,
    ua.expected_graduation_month,
    ua.phone
  FROM user_accounts ua
  WHERE NOT EXISTS (SELECT 1 FROM admin_accounts aa WHERE aa.username = ua.username)
    AND NOT EXISTS (SELECT 1 FROM power_users pu WHERE pu.username = ua.username)
  UNION ALL
  SELECT
    aa.username,
    COALESCE(ua.platform_uid, aa.platform_uid) AS platform_uid,
    'admin' AS role,
    TRUE AS can_view_board,
    TRUE AS can_view_nodes,
    TRUE AS can_review_requests,
    COALESCE(aa.two_factor_enabled, FALSE) AS two_factor_enabled,
    '' AS email,
    '' AS student_id,
    '' AS real_name,
    '' AS advisor,
    0 AS expected_graduation_year,
    0 AS expected_graduation_month,
    '' AS phone
  FROM admin_accounts aa
  LEFT JOIN user_accounts ua ON ua.username = aa.username
  UNION ALL
  SELECT
    pu.username,
    COALESCE(ua.platform_uid, pu.platform_uid) AS platform_uid,
    'power_user' AS role,
    pu.can_view_board,
    pu.can_view_nodes,
    pu.can_review_requests,
    COALESCE(pu.two_factor_enabled, FALSE) AS two_factor_enabled,
    COALESCE(ua.email, '') AS email,
    COALESCE(ua.student_id, '') AS student_id,
    COALESCE(ua.real_name, '') AS real_name,
    COALESCE(ua.advisor, '') AS advisor,
    COALESCE(ua.expected_graduation_year, 0) AS expected_graduation_year,
    COALESCE(ua.expected_graduation_month, 0) AS expected_graduation_month,
    COALESCE(ua.phone, '') AS phone
  FROM power_users pu
  LEFT JOIN user_accounts ua ON ua.username = pu.username
)
SELECT uu.username, uu.platform_uid, uu.role, uu.can_view_board, uu.can_view_nodes, uu.can_review_requests, uu.two_factor_enabled,
       uu.email, uu.student_id, uu.real_name, uu.advisor, uu.expected_graduation_year, uu.expected_graduation_month, uu.phone,
       COALESCE(u.balance, 0) AS general_balance,
       COALESCE(u.carryover_balance, 0) AS carryover_balance,
       COALESCE(ea.exclusive_balance, 0) AS exclusive_balance,
       COALESCE(u.balance, 0) + COALESCE(u.carryover_balance, 0) + COALESCE(ea.exclusive_balance, 0) AS total_balance,
       COALESCE(u.status, 'normal'),
       COALESCE(x.usage_records, 0), COALESCE(x.total_cost, 0), COALESCE(x.last_usage_at, to_timestamp(0))
FROM union_users uu
LEFT JOIN users u ON u.username=uu.username
LEFT JOIN usage_agg x ON x.username=uu.username
LEFT JOIN exclusive_agg ea ON ea.username=uu.username
ORDER BY uu.username
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AdminUserDetail, 0)
	for rows.Next() {
		var d AdminUserDetail
		var uidRaw sql.NullInt64
		if err := rows.Scan(
			&d.Username, &uidRaw, &d.Role, &d.CanViewBoard, &d.CanViewNodes, &d.CanReviewRequest, &d.TwoFactorEnabled,
			&d.Email, &d.StudentID, &d.RealName, &d.Advisor, &d.ExpectedGradYear, &d.ExpectedGradMonth, &d.Phone,
			&d.Balance, &d.CarryoverBalance, &d.ExclusiveBalance, &d.TotalBalance, &d.Status, &d.UsageRecords, &d.TotalCost, &d.LastUsageAt,
		); err != nil {
			return nil, err
		}
		if uidRaw.Valid && uidRaw.Int64 > 0 {
			v := int(uidRaw.Int64)
			d.PlatformUID = &v
		}
		accounts, err := s.ListUserNodeAccountsByBilling(ctx, d.Username, 5000)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		d.NodeAccounts = accounts
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ListPointsUsers(ctx context.Context, keyword string, keywordField string, limit int) ([]PointsUser, error) {
	keyword = strings.TrimSpace(keyword)
	keywordField = strings.ToLower(strings.TrimSpace(keywordField))
	switch keywordField {
	case "username", "student_id", "real_name", "advisor", "email", "phone", "all":
	default:
		keywordField = "all"
	}
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
WITH exclusive_agg AS (
  SELECT username, COALESCE(SUM(balance), 0) AS exclusive_balance
  FROM user_node_exclusive_points
  GROUP BY username
),
union_users AS (
  SELECT
    ua.username,
    ua.platform_uid,
    COALESCE(NULLIF(ua.email, ''), '') AS email,
    COALESCE(NULLIF(ua.real_name, ''), '') AS real_name,
    COALESCE(NULLIF(ua.student_id, ''), '') AS student_id,
    COALESCE(NULLIF(ua.advisor, ''), '') AS advisor,
    COALESCE(NULLIF(ua.phone, ''), '') AS phone,
    COALESCE(ua.expected_graduation_year, 0) AS expected_graduation_year,
    COALESCE(ua.expected_graduation_month, 0) AS expected_graduation_month,
    COALESCE(NULLIF(ua.role, ''), 'user') AS role
  FROM user_accounts ua
  WHERE NOT EXISTS (SELECT 1 FROM admin_accounts aa WHERE aa.username = ua.username)
    AND NOT EXISTS (SELECT 1 FROM power_users pu WHERE pu.username = ua.username)
  UNION ALL
  SELECT
    aa.username,
    COALESCE(ua.platform_uid, aa.platform_uid) AS platform_uid,
    COALESCE(NULLIF(ua.email, ''), '') AS email,
    COALESCE(NULLIF(ua.real_name, ''), '') AS real_name,
    COALESCE(NULLIF(ua.student_id, ''), '') AS student_id,
    COALESCE(NULLIF(ua.advisor, ''), '') AS advisor,
    COALESCE(NULLIF(ua.phone, ''), '') AS phone,
    COALESCE(ua.expected_graduation_year, 0) AS expected_graduation_year,
    COALESCE(ua.expected_graduation_month, 0) AS expected_graduation_month,
    'admin' AS role
  FROM admin_accounts aa
  LEFT JOIN user_accounts ua ON ua.username = aa.username
  UNION ALL
  SELECT
    pu.username,
    COALESCE(ua.platform_uid, pu.platform_uid) AS platform_uid,
    COALESCE(NULLIF(ua.email, ''), '') AS email,
    COALESCE(NULLIF(ua.real_name, ''), '') AS real_name,
    COALESCE(NULLIF(ua.student_id, ''), '') AS student_id,
    COALESCE(NULLIF(ua.advisor, ''), '') AS advisor,
    COALESCE(NULLIF(ua.phone, ''), '') AS phone,
    COALESCE(ua.expected_graduation_year, 0) AS expected_graduation_year,
    COALESCE(ua.expected_graduation_month, 0) AS expected_graduation_month,
    'power_user' AS role
  FROM power_users pu
  LEFT JOIN user_accounts ua ON ua.username = pu.username
  WHERE NOT EXISTS (SELECT 1 FROM admin_accounts aa WHERE aa.username = pu.username)
)
SELECT
  uu.username,
  uu.platform_uid,
  uu.email,
  uu.real_name,
  uu.student_id,
  uu.advisor,
  uu.phone,
  uu.expected_graduation_year,
  uu.expected_graduation_month,
  uu.role,
  COALESCE(u.balance, 0) AS general_balance,
  COALESCE(u.carryover_balance, 0) AS carryover_balance,
  COALESCE(ea.exclusive_balance, 0) AS exclusive_balance,
  COALESCE(u.balance, 0) + COALESCE(u.carryover_balance, 0) + COALESCE(ea.exclusive_balance, 0) AS total_balance,
  COALESCE(u.status, 'normal') AS status
FROM union_users uu
LEFT JOIN users u ON u.username=uu.username
LEFT JOIN exclusive_agg ea ON ea.username=uu.username
WHERE (
  $1='' OR
  ($2='all' AND (
    uu.username ILIKE '%' || $1 || '%' OR
    uu.student_id ILIKE '%' || $1 || '%' OR
    uu.real_name ILIKE '%' || $1 || '%' OR
    uu.advisor ILIKE '%' || $1 || '%' OR
    uu.email ILIKE '%' || $1 || '%' OR
    uu.phone ILIKE '%' || $1 || '%'
  )) OR
  ($2='username' AND uu.username ILIKE '%' || $1 || '%') OR
  ($2='student_id' AND uu.student_id ILIKE '%' || $1 || '%') OR
  ($2='real_name' AND uu.real_name ILIKE '%' || $1 || '%') OR
  ($2='advisor' AND uu.advisor ILIKE '%' || $1 || '%') OR
  ($2='email' AND uu.email ILIKE '%' || $1 || '%') OR
  ($2='phone' AND uu.phone ILIKE '%' || $1 || '%')
)
ORDER BY
  CASE uu.role WHEN 'admin' THEN 0 WHEN 'power_user' THEN 1 ELSE 2 END,
  uu.username
LIMIT $3`, keyword, keywordField, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PointsUser, 0)
	for rows.Next() {
		var v PointsUser
		var uidRaw sql.NullInt64
		if err := rows.Scan(
			&v.Username,
			&uidRaw,
			&v.Email,
			&v.RealName,
			&v.StudentID,
			&v.Advisor,
			&v.Phone,
			&v.ExpectedGradYear,
			&v.ExpectedGradMonth,
			&v.Role,
			&v.GeneralBalance,
			&v.CarryoverBalance,
			&v.ExclusiveBalance,
			&v.TotalBalance,
			&v.Status,
		); err != nil {
			return nil, err
		}
		if uidRaw.Valid && uidRaw.Int64 > 0 {
			vv := int(uidRaw.Int64)
			v.PlatformUID = &vv
		}
		v.Balance = v.GeneralBalance
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ListRegularUserProfilesForPoints(ctx context.Context, limit int) ([]PointsUser, error) {
	if limit <= 0 || limit > 50000 {
		limit = 20000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  ua.username,
  ua.platform_uid,
  ua.student_id,
  COALESCE(u.balance, 0) AS balance,
  COALESCE(u.carryover_balance, 0) AS carryover_balance,
  COALESCE(u.status, 'normal') AS status
FROM user_accounts ua
LEFT JOIN users u ON u.username=ua.username
WHERE ua.role='user'
ORDER BY ua.username
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PointsUser, 0)
	for rows.Next() {
		var v PointsUser
		var uidRaw sql.NullInt64
		if err := rows.Scan(&v.Username, &uidRaw, &v.StudentID, &v.Balance, &v.CarryoverBalance, &v.Status); err != nil {
			return nil, err
		}
		if uidRaw.Valid && uidRaw.Int64 > 0 {
			vv := int(uidRaw.Int64)
			v.PlatformUID = &vv
		}
		v.GeneralBalance = v.Balance
		v.TotalBalance = v.Balance + v.CarryoverBalance
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) FilterExistingPointsUsernames(ctx context.Context, usernames []string) ([]string, error) {
	cleaned := uniqTrim(usernames)
	if len(cleaned) == 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
WITH all_usernames AS (
  SELECT username FROM user_accounts
  UNION
  SELECT username FROM admin_accounts
  UNION
  SELECT username FROM power_users
)
SELECT username
FROM all_usernames
WHERE username = ANY($1)
ORDER BY username`, pq.Array(cleaned))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, len(cleaned))
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return nil, err
		}
		username = strings.TrimSpace(username)
		if username != "" {
			out = append(out, username)
		}
	}
	return out, rows.Err()
}

func (s *Store) ListSpecialMonthlyPointsRules(ctx context.Context, limit int) ([]SpecialMonthlyPointsRule, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT username, monthly_points, enabled, updated_by, created_at, updated_at
FROM special_monthly_points_rules
ORDER BY username
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SpecialMonthlyPointsRule, 0)
	for rows.Next() {
		var v SpecialMonthlyPointsRule
		if err := rows.Scan(&v.Username, &v.MonthlyPoints, &v.Enabled, &v.UpdatedBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		normalizeSpecialMonthlyPointsRuleTimes(&v)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) UpsertSpecialMonthlyPointsRule(ctx context.Context, username string, monthlyPoints float64, enabled bool, updatedBy string) error {
	username = strings.TrimSpace(username)
	updatedBy = strings.TrimSpace(updatedBy)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if monthlyPoints < 0 {
		return errors.New("monthly_points 不能为负数")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO special_monthly_points_rules(username, monthly_points, enabled, updated_by)
VALUES($1,$2,$3,$4)
ON CONFLICT (username) DO UPDATE SET
  monthly_points=EXCLUDED.monthly_points,
  enabled=EXCLUDED.enabled,
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()`, username, monthlyPoints, enabled, updatedBy)
	return err
}

func (s *Store) DeleteSpecialMonthlyPointsRule(ctx context.Context, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM special_monthly_points_rules WHERE username=$1`, username)
	if err != nil {
		return err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) LoadSpecialMonthlyPointsMapTx(ctx context.Context, tx *sql.Tx) (map[string]float64, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT username, monthly_points
FROM special_monthly_points_rules
WHERE enabled=true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var username string
		var points float64
		if err := rows.Scan(&username, &points); err != nil {
			return nil, err
		}
		out[strings.TrimSpace(username)] = points
	}
	return out, rows.Err()
}

func (s *Store) IsMonthlyPointsResetRunExistsTx(ctx context.Context, tx *sql.Tx, monthKey string) (bool, error) {
	monthKey = strings.TrimSpace(monthKey)
	if monthKey == "" {
		return false, errors.New("month_key 不能为空")
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM monthly_points_reset_runs WHERE month_key=$1)`, monthKey).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Store) UpsertMonthlyPointsResetRunTx(ctx context.Context, tx *sql.Tx, monthKey string, runBy string, totalUsers int, changedUsers int, forced bool) error {
	monthKey = strings.TrimSpace(monthKey)
	runBy = strings.TrimSpace(runBy)
	if monthKey == "" {
		return errors.New("month_key 不能为空")
	}
	if runBy == "" {
		runBy = "system"
	}
	if totalUsers < 0 {
		totalUsers = 0
	}
	if changedUsers < 0 {
		changedUsers = 0
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO monthly_points_reset_runs(month_key, run_at, run_by, total_users, changed_users, forced)
VALUES($1, timezone('Asia/Shanghai', now()), $2, $3, $4, $5)
ON CONFLICT (month_key) DO UPDATE SET
  run_at=EXCLUDED.run_at,
  run_by=EXCLUDED.run_by,
  total_users=EXCLUDED.total_users,
  changed_users=EXCLUDED.changed_users,
  forced=EXCLUDED.forced`, monthKey, runBy, totalUsers, changedUsers, forced)
	return err
}

func (s *Store) GetMonthlyPointsResetRun(ctx context.Context, monthKey string) (MonthlyPointsResetRun, bool, error) {
	monthKey = strings.TrimSpace(monthKey)
	if monthKey == "" {
		return MonthlyPointsResetRun{}, false, errors.New("month_key 不能为空")
	}
	var out MonthlyPointsResetRun
	err := s.db.QueryRowContext(ctx, `
SELECT month_key, run_at, run_by, total_users, changed_users, forced
FROM monthly_points_reset_runs
WHERE month_key=$1`, monthKey).Scan(&out.MonthKey, &out.RunAt, &out.RunBy, &out.TotalUsers, &out.ChangedUsers, &out.Forced)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MonthlyPointsResetRun{}, false, nil
		}
		return MonthlyPointsResetRun{}, false, err
	}
	out.RunAt = asBeijingWallTime(out.RunAt)
	return out, true, nil
}

func (s *Store) GetLatestMonthlyPointsResetRun(ctx context.Context) (MonthlyPointsResetRun, bool, error) {
	var out MonthlyPointsResetRun
	err := s.db.QueryRowContext(ctx, `
SELECT month_key, run_at, run_by, total_users, changed_users, forced
FROM monthly_points_reset_runs
ORDER BY run_at DESC
LIMIT 1`).Scan(&out.MonthKey, &out.RunAt, &out.RunBy, &out.TotalUsers, &out.ChangedUsers, &out.Forced)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MonthlyPointsResetRun{}, false, nil
		}
		return MonthlyPointsResetRun{}, false, err
	}
	out.RunAt = asBeijingWallTime(out.RunAt)
	return out, true, nil
}

func (s *Store) ListGraduationDueUsers(ctx context.Context, now time.Time, limit int) ([]GraduationReminderUser, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  username,
  email,
  student_id,
  real_name,
  advisor,
  expected_graduation_year,
  expected_graduation_month,
  (
    (EXTRACT(YEAR FROM date_trunc('month', $1::timestamp))::int * 12 + EXTRACT(MONTH FROM date_trunc('month', $1::timestamp))::int)
    - (expected_graduation_year * 12 + expected_graduation_month)
  ) AS overdue_months
FROM user_accounts
WHERE role='user'
  AND (
    (EXTRACT(YEAR FROM date_trunc('month', $1::timestamp))::int * 12 + EXTRACT(MONTH FROM date_trunc('month', $1::timestamp))::int)
    - (expected_graduation_year * 12 + expected_graduation_month)
  ) >= 0
ORDER BY expected_graduation_year, expected_graduation_month, username
LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]GraduationReminderUser, 0)
	for rows.Next() {
		var v GraduationReminderUser
		if err := rows.Scan(
			&v.Username,
			&v.Email,
			&v.StudentID,
			&v.RealName,
			&v.Advisor,
			&v.ExpectedGraduationYear,
			&v.ExpectedGraduationMonth,
			&v.OverdueMonths,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUserProfileBase(ctx context.Context, username string, realName string, advisor string, expectedGradYear int, expectedGradMonth int, phone string) error {
	username = strings.TrimSpace(username)
	realName = strings.TrimSpace(realName)
	advisor = strings.TrimSpace(advisor)
	phone = strings.TrimSpace(phone)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if realName == "" || advisor == "" || phone == "" {
		return errors.New("真实姓名/导师/电话不能为空")
	}
	if expectedGradYear < 2000 || expectedGradYear > 2200 {
		return errors.New("预计毕业年份不合法")
	}
	if expectedGradMonth < 1 || expectedGradMonth > 12 {
		return errors.New("预计毕业月份不合法")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE user_accounts
SET real_name=$2, advisor=$3, expected_graduation_year=$4, expected_graduation_month=$5, phone=$6, updated_at=NOW()
WHERE username=$1`, username, realName, advisor, expectedGradYear, expectedGradMonth, phone)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CreateProfileChangeRequest(ctx context.Context, billingUsername string, newUsername string, newEmail string, newStudentID string, reason string) error {
	billingUsername = strings.TrimSpace(billingUsername)
	newUsername = strings.TrimSpace(newUsername)
	newEmail = strings.TrimSpace(strings.ToLower(newEmail))
	newStudentID = strings.TrimSpace(newStudentID)
	reason = strings.TrimSpace(reason)
	if billingUsername == "" || newUsername == "" || newEmail == "" || newStudentID == "" {
		return errors.New("用户名/邮箱/学号不能为空")
	}
	if reason == "" {
		return errors.New("请填写变更原因，管理员审核时需要")
	}
	var old UserAccount
	if err := s.db.QueryRowContext(ctx, `
SELECT username, email, real_name, student_id, advisor, expected_graduation_year, phone, role, last_login_at, created_at, updated_at
FROM user_accounts WHERE username=$1`, billingUsername).Scan(
		&old.Username, &old.Email, &old.RealName, &old.StudentID, &old.Advisor, &old.ExpectedGraduationYear, &old.Phone,
		&old.Role, &old.LastLoginAt, &old.CreatedAt, &old.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("用户不存在")
		}
		return err
	}
	var hasPending bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM profile_change_requests
  WHERE billing_username=$1 AND status='pending'
)`, billingUsername).Scan(&hasPending); err != nil {
		return err
	}
	if hasPending {
		return errors.New("你已有待审核的关键信息变更申请，请等待管理员处理")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO profile_change_requests(
  billing_username, old_username, old_email, old_student_id,
  new_username, new_email, new_student_id, reason, status
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending')`,
		billingUsername, old.Username, old.Email, old.StudentID,
		newUsername, newEmail, newStudentID, reason,
	)
	return err
}

func (s *Store) ListProfileChangeRequestsByUser(ctx context.Context, billingUsername string, limit int) ([]ProfileChangeRequest, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return nil, errors.New("billing_username 不能为空")
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT request_id, billing_username, old_username, old_email, old_student_id,
       new_username, new_email, new_student_id, reason, status, reviewed_by, reviewed_at, created_at, updated_at
FROM profile_change_requests
WHERE billing_username=$1
ORDER BY request_id DESC
LIMIT $2`, billingUsername, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ProfileChangeRequest, 0)
	for rows.Next() {
		var x ProfileChangeRequest
		if err := rows.Scan(
			&x.RequestID, &x.BillingUsername, &x.OldUsername, &x.OldEmail, &x.OldStudentID,
			&x.NewUsername, &x.NewEmail, &x.NewStudentID, &x.Reason, &x.Status,
			&x.ReviewedBy, &x.ReviewedAt, &x.CreatedAt, &x.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListProfileChangeRequestsAdmin(ctx context.Context, status string, username string, limit int) ([]ProfileChangeRequest, error) {
	status = strings.TrimSpace(status)
	username = strings.TrimSpace(username)
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	conds := make([]string, 0, 2)
	args := make([]any, 0, 3)
	if status != "" {
		conds = append(conds, "status=$"+strconv.Itoa(len(args)+1))
		args = append(args, status)
	}
	if username != "" {
		conds = append(conds, "billing_username=$"+strconv.Itoa(len(args)+1))
		args = append(args, username)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit)
	query := `
SELECT request_id, billing_username, old_username, old_email, old_student_id,
       new_username, new_email, new_student_id, reason, status, reviewed_by, reviewed_at, created_at, updated_at
FROM profile_change_requests
` + where + `
ORDER BY
  CASE status WHEN 'pending' THEN 0 ELSE 1 END,
  request_id DESC
LIMIT $` + strconv.Itoa(len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ProfileChangeRequest, 0)
	for rows.Next() {
		var x ProfileChangeRequest
		if err := rows.Scan(
			&x.RequestID, &x.BillingUsername, &x.OldUsername, &x.OldEmail, &x.OldStudentID,
			&x.NewUsername, &x.NewEmail, &x.NewStudentID, &x.Reason, &x.Status,
			&x.ReviewedBy, &x.ReviewedAt, &x.CreatedAt, &x.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ReviewProfileChangeRequestTx(ctx context.Context, tx *sql.Tx, requestID int, newStatus string, reviewedBy string, reviewedAt time.Time) (ProfileChangeRequest, error) {
	if requestID <= 0 {
		return ProfileChangeRequest{}, errors.New("request_id 不合法")
	}
	newStatus = strings.TrimSpace(newStatus)
	reviewedBy = strings.TrimSpace(reviewedBy)
	if newStatus != "approved" && newStatus != "rejected" {
		return ProfileChangeRequest{}, errors.New("status 仅支持 approved/rejected")
	}
	if reviewedBy == "" {
		reviewedBy = "admin"
	}
	var r ProfileChangeRequest
	if err := tx.QueryRowContext(ctx, `
SELECT request_id, billing_username, old_username, old_email, old_student_id,
       new_username, new_email, new_student_id, reason, status, reviewed_by, reviewed_at, created_at, updated_at
FROM profile_change_requests
WHERE request_id=$1`, requestID).Scan(
		&r.RequestID, &r.BillingUsername, &r.OldUsername, &r.OldEmail, &r.OldStudentID,
		&r.NewUsername, &r.NewEmail, &r.NewStudentID, &r.Reason, &r.Status, &r.ReviewedBy, &r.ReviewedAt, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProfileChangeRequest{}, errors.New("申请不存在")
		}
		return ProfileChangeRequest{}, err
	}
	if r.Status != "pending" {
		return ProfileChangeRequest{}, errors.New("该申请已处理，不能重复审核")
	}

	if newStatus == "approved" {
		// 1) 唯一性校验
		var exists bool
		if strings.TrimSpace(r.NewUsername) != strings.TrimSpace(r.OldUsername) {
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE username=$1)`, r.NewUsername).Scan(&exists); err != nil {
				return ProfileChangeRequest{}, err
			}
			if exists {
				return ProfileChangeRequest{}, errors.New("新用户名已存在")
			}
		}
		if strings.TrimSpace(strings.ToLower(r.NewEmail)) != strings.TrimSpace(strings.ToLower(r.OldEmail)) {
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE email=$1)`, strings.ToLower(r.NewEmail)).Scan(&exists); err != nil {
				return ProfileChangeRequest{}, err
			}
			if exists {
				return ProfileChangeRequest{}, errors.New("新邮箱已存在")
			}
		}
		if strings.TrimSpace(r.NewStudentID) != strings.TrimSpace(r.OldStudentID) {
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts WHERE student_id=$1)`, r.NewStudentID).Scan(&exists); err != nil {
				return ProfileChangeRequest{}, err
			}
			if exists {
				return ProfileChangeRequest{}, errors.New("新学号已存在")
			}
		}

		// 2) 更新 user_accounts
		res, err := tx.ExecContext(ctx, `
UPDATE user_accounts
SET username=$2, email=$3, student_id=$4, updated_at=NOW()
WHERE username=$1`, r.OldUsername, r.NewUsername, strings.ToLower(r.NewEmail), r.NewStudentID)
		if err != nil {
			return ProfileChangeRequest{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ProfileChangeRequest{}, errors.New("原账号不存在或已变更，请刷新后重试")
		}

		// 3) 若用户名变化，级联同步历史业务表
		if r.NewUsername != r.OldUsername {
			if _, err := tx.ExecContext(ctx, `UPDATE users SET username=$2 WHERE username=$1`, r.OldUsername, r.NewUsername); err != nil {
				return ProfileChangeRequest{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE usage_records SET username=$2 WHERE username=$1`, r.OldUsername, r.NewUsername); err != nil {
				return ProfileChangeRequest{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE recharge_records SET username=$2 WHERE username=$1`, r.OldUsername, r.NewUsername); err != nil {
				return ProfileChangeRequest{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE user_node_accounts SET billing_username=$2 WHERE billing_username=$1`, r.OldUsername, r.NewUsername); err != nil {
				return ProfileChangeRequest{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE user_requests SET billing_username=$2 WHERE billing_username=$1`, r.OldUsername, r.NewUsername); err != nil {
				return ProfileChangeRequest{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE profile_change_requests SET billing_username=$2 WHERE billing_username=$1`, r.OldUsername, r.NewUsername); err != nil {
				return ProfileChangeRequest{}, err
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE profile_change_requests
SET status=$2, reviewed_by=$3, reviewed_at=$4, updated_at=NOW()
WHERE request_id=$1`, requestID, newStatus, reviewedBy, reviewedAt); err != nil {
		return ProfileChangeRequest{}, err
	}
	if err := tx.QueryRowContext(ctx, `
SELECT request_id, billing_username, old_username, old_email, old_student_id,
       new_username, new_email, new_student_id, reason, status, reviewed_by, reviewed_at, created_at, updated_at
FROM profile_change_requests
WHERE request_id=$1`, requestID).Scan(
		&r.RequestID, &r.BillingUsername, &r.OldUsername, &r.OldEmail, &r.OldStudentID,
		&r.NewUsername, &r.NewEmail, &r.NewStudentID, &r.Reason, &r.Status,
		&r.ReviewedBy, &r.ReviewedAt, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return ProfileChangeRequest{}, err
	}
	return r, nil
}

func (s *Store) ListPlatformUsageSummaryByUser(ctx context.Context, from time.Time, to time.Time, limit int, visibleNodeIDs []string) ([]PlatformUsageUserSummary, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	query := `
WITH platform_users AS (
  SELECT username FROM user_accounts
  UNION
  SELECT username FROM admin_accounts
  UNION
  SELECT username FROM power_users
),
resolved AS (
  SELECT
    COALESCE(pu_ur.username, pu_map.username) AS platform_username,
    GREATEST(COALESCE(n.interval_seconds, 1), 1) AS interval_seconds,
    ur.gpu_usage,
    ur.cpu_percent,
    ur.cost
  FROM usage_records ur
  LEFT JOIN nodes n
    ON n.node_id = ur.node_id
  LEFT JOIN user_node_accounts una
    ON una.node_id = ur.node_id
   AND una.local_username = COALESCE(NULLIF(ur.local_username, ''), ur.username)
  LEFT JOIN platform_users pu_ur ON pu_ur.username = ur.username
  LEFT JOIN platform_users pu_map ON pu_map.username = una.billing_username
  WHERE ur.timestamp >= $1
    AND ur.timestamp <= $2`
	args := []any{from, to, limit}
	cleaned := uniqTrim(visibleNodeIDs)
	if len(cleaned) > 0 {
		query += `
    AND ur.node_id = ANY($4)`
		args = append(args, pq.Array(cleaned))
	}
	query += `
    AND COALESCE(pu_ur.username, pu_map.username) IS NOT NULL
),
agg AS (
SELECT platform_username,
       COUNT(1) AS usage_records,
       COALESCE(SUM(CASE WHEN cpu_percent > 0 THEN interval_seconds ELSE 0 END), 0) AS cpu_usage_seconds,
       COALESCE(SUM(CASE WHEN jsonb_array_length(gpu_usage) > 0 THEN interval_seconds ELSE 0 END), 0) AS gpu_usage_seconds,
       COALESCE(AVG(cpu_percent), 0) AS cpu_util_percent,
       COALESCE(AVG(CASE WHEN jsonb_array_length(gpu_usage) > 0 THEN 100.0 ELSE 0.0 END), 0) AS gpu_util_percent,
       COALESCE(SUM(cost), 0) AS total_cost
FROM resolved
GROUP BY platform_username
)
SELECT pu.username AS platform_username,
       COALESCE(a.usage_records, 0) AS usage_records,
       COALESCE(a.cpu_usage_seconds, 0) AS cpu_usage_seconds,
       COALESCE(a.gpu_usage_seconds, 0) AS gpu_usage_seconds,
       COALESCE(a.cpu_util_percent, 0) AS cpu_util_percent,
       COALESCE(a.gpu_util_percent, 0) AS gpu_util_percent,
       COALESCE(a.total_cost, 0) AS total_cost,
       COALESCE(u.balance, 0) AS general_balance
FROM platform_users pu
LEFT JOIN agg a ON a.platform_username = pu.username
LEFT JOIN users u ON u.username = pu.username
ORDER BY COALESCE(a.total_cost, 0) DESC, pu.username ASC
LIMIT $3`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PlatformUsageUserSummary, 0)
	for rows.Next() {
		var x PlatformUsageUserSummary
		if err := rows.Scan(
			&x.PlatformUsername, &x.UsageRecords,
			&x.CPUUsageSeconds, &x.GPUUsageSeconds,
			&x.CPUUtilPercent, &x.GPUUtilPercent,
			&x.TotalCost, &x.GeneralBalance,
		); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListPlatformUsageNodeDetails(ctx context.Context, platformUsername string, from time.Time, to time.Time, limit int, visibleNodeIDs []string) ([]PlatformUsageNodeDetail, error) {
	platformUsername = strings.TrimSpace(platformUsername)
	if platformUsername == "" {
		return nil, errors.New("platform_username 不能为空")
	}
	if limit <= 0 || limit > 20000 {
		limit = 2000
	}
	query := `
WITH platform_users AS (
  SELECT username FROM user_accounts
  UNION
  SELECT username FROM admin_accounts
  UNION
  SELECT username FROM power_users
),
resolved AS (
  SELECT
    ur.node_id,
    ur.timestamp,
    ur.cpu_percent,
    ur.memory_mb,
    ur.gpu_count,
    ur.cost,
    COALESCE(pu_ur.username, pu_map.username) AS platform_username
  FROM usage_records ur
  LEFT JOIN user_node_accounts una
    ON una.node_id = ur.node_id
   AND una.local_username = COALESCE(NULLIF(ur.local_username, ''), ur.username)
  LEFT JOIN platform_users pu_ur ON pu_ur.username = ur.username
  LEFT JOIN platform_users pu_map ON pu_map.username = una.billing_username
  WHERE ur.timestamp >= $2
    AND ur.timestamp <= $3`
	args := []any{platformUsername, from, to, limit}
	cleaned := uniqTrim(visibleNodeIDs)
	if len(cleaned) > 0 {
		query += `
    AND ur.node_id = ANY($5)`
		args = append(args, pq.Array(cleaned))
	}
	query += `
    AND COALESCE(pu_ur.username, pu_map.username) IS NOT NULL
)
SELECT r.node_id,
       COALESCE(n.cpu_model, '') AS cpu_model,
       COALESCE(n.cpu_count, 0) AS cpu_count,
       COALESCE(n.gpu_model, '') AS gpu_model,
       COALESCE(n.gpu_count, 0) AS gpu_count,
       COALESCE(n.last_seen_at, to_timestamp(0)) AS last_seen_at,
       COUNT(1) AS usage_records,
       COALESCE(SUM(r.cpu_percent), 0) AS total_cpu_percent,
       COALESCE(SUM(r.memory_mb), 0) AS total_memory_mb,
       COALESCE(SUM(CASE WHEN COALESCE(r.gpu_count, 0) > 0 THEN 0 ELSE r.cost END), 0) AS cpu_cost,
       COALESCE(SUM(CASE WHEN COALESCE(r.gpu_count, 0) > 0 THEN r.cost ELSE 0 END), 0) AS gpu_cost,
       COALESCE(SUM(r.cost), 0) AS total_cost,
       MAX(r.timestamp) AS last_usage_at
FROM resolved r
LEFT JOIN nodes n
  ON n.node_id = r.node_id
WHERE r.platform_username = $1
GROUP BY r.node_id, n.cpu_model, n.cpu_count, n.gpu_model, n.gpu_count, n.last_seen_at
ORDER BY total_cost DESC, r.node_id ASC
LIMIT $4`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PlatformUsageNodeDetail, 0)
	for rows.Next() {
		var x PlatformUsageNodeDetail
		if err := rows.Scan(
			&x.NodeID, &x.CPUModel, &x.CPUCount, &x.GPUModel, &x.GPUCount, &x.LastSeenAt,
			&x.UsageRecords, &x.TotalCPUPercent, &x.TotalMemoryMB, &x.CPUCost, &x.GPUCost, &x.TotalCost, &x.LastUsageAt,
		); err != nil {
			return nil, err
		}
		normalizePlatformUsageNodeDetailTimes(&x)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListNodeMonthlyUserCosts(ctx context.Context, nodeID string, from time.Time, to time.Time, limit int) ([]NodeMonthlyUserCost, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > 20000 {
		limit = 2000
	}
	rows, err := s.db.QueryContext(ctx, `
WITH platform_users AS (
  SELECT username FROM user_accounts
  UNION
  SELECT username FROM admin_accounts
  UNION
  SELECT username FROM power_users
),
resolved AS (
  SELECT
    ur.node_id,
    ur.timestamp,
    ur.cost,
    COALESCE(pu_ur.username, pu_map.username) AS platform_username
  FROM usage_records ur
  LEFT JOIN user_node_accounts una
    ON una.node_id = ur.node_id
   AND una.local_username = COALESCE(NULLIF(ur.local_username, ''), ur.username)
  LEFT JOIN platform_users pu_ur ON pu_ur.username = ur.username
  LEFT JOIN platform_users pu_map ON pu_map.username = una.billing_username
  WHERE ur.node_id = $1
    AND ur.timestamp >= $2
    AND ur.timestamp <= $3
    AND COALESCE(pu_ur.username, pu_map.username) IS NOT NULL
)
SELECT node_id,
       platform_username,
       COUNT(1) AS usage_records,
       COALESCE(SUM(cost), 0) AS total_cost,
       MAX(timestamp) AS last_usage_at
FROM resolved
GROUP BY node_id, platform_username
ORDER BY total_cost DESC, platform_username ASC
LIMIT $4`, nodeID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NodeMonthlyUserCost, 0)
	for rows.Next() {
		var x NodeMonthlyUserCost
		if err := rows.Scan(&x.NodeID, &x.PlatformUsername, &x.UsageRecords, &x.TotalCost, &x.LastUsageAt); err != nil {
			return nil, err
		}
		normalizeNodeMonthlyUserCostTimes(&x)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListRechargeSummary(ctx context.Context, from time.Time, to time.Time, limit int) ([]RechargeSummary, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT username,
       COUNT(1) AS recharge_count,
       COALESCE(SUM(amount), 0) AS recharge_total,
       MAX(created_at) AS last_recharge
FROM recharge_records
WHERE created_at >= $1 AND created_at <= $2
GROUP BY username
ORDER BY recharge_total DESC
LIMIT $3`, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RechargeSummary, 0)
	for rows.Next() {
		var x RechargeSummary
		if err := rows.Scan(&x.Username, &x.RechargeCount, &x.RechargeTotal, &x.LastRecharge); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) GetUserAccountCreatedAt(ctx context.Context, username string) (*time.Time, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username 不能为空")
	}
	var createdAt time.Time
	if err := s.db.QueryRowContext(ctx, `
WITH accounts AS (
  SELECT username, created_at FROM user_accounts
  UNION ALL
  SELECT username, created_at FROM admin_accounts
  UNION ALL
  SELECT username, created_at FROM power_users
)
SELECT created_at
FROM accounts
WHERE username=$1
ORDER BY created_at ASC
LIMIT 1`, username).Scan(&createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &createdAt, nil
}

func (s *Store) GetUserUsageCostSummary(ctx context.Context, username string, now time.Time) (monthUsed float64, totalUsed float64, err error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return 0, 0, errors.New("username 不能为空")
	}
	localNow := now.In(time.Local)
	monthStart := time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, time.Local)
	monthEnd := monthStart.AddDate(0, 1, 0)

	if err := s.db.QueryRowContext(ctx, `
WITH platform_accounts AS (
  SELECT username, created_at FROM user_accounts
  UNION ALL
  SELECT username, created_at FROM admin_accounts
  UNION ALL
  SELECT username, created_at FROM power_users
),
target_account AS (
  SELECT created_at
  FROM platform_accounts
  WHERE username = $1
  ORDER BY created_at ASC
  LIMIT 1
),
resolved AS (
  SELECT ur.timestamp, ur.cost, COALESCE(pu_ur.username, pu_map.username) AS platform_username
  FROM usage_records ur
  LEFT JOIN user_node_accounts una
    ON una.node_id = ur.node_id
   AND una.local_username = COALESCE(NULLIF(ur.local_username, ''), ur.username)
  LEFT JOIN platform_accounts pu_ur ON pu_ur.username = ur.username
  LEFT JOIN platform_accounts pu_map ON pu_map.username = una.billing_username
  WHERE (ur.username = $1 OR una.billing_username = $1)
    AND ur.timestamp >= COALESCE((SELECT created_at FROM target_account), NOW())
)
SELECT
  COALESCE(SUM(CASE WHEN resolved.timestamp >= $2 AND resolved.timestamp < $3 THEN resolved.cost ELSE 0 END), 0) AS month_used,
  COALESCE(SUM(resolved.cost), 0) AS total_used
FROM resolved
WHERE resolved.platform_username = $1`, username, monthStart, monthEnd).Scan(&monthUsed, &totalUsed); err != nil {
		return 0, 0, err
	}
	return monthUsed, totalUsed, nil
}

func (s *Store) ListPointsOperationRecords(ctx context.Context, username string, method string, limit int) ([]PointsOperationRecord, error) {
	username = strings.TrimSpace(username)
	method = strings.TrimSpace(method)
	if limit <= 0 || limit > 10000 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  recharge_id,
  username,
  CASE
    WHEN method IN ('points_batch_plus', 'points_batch_minus', 'points_batch_grant', 'points_batch_carry_plus', 'points_batch_carry_minus', 'points_batch_node_plus', 'points_batch_node_minus') THEN '全部用户'
    WHEN method IN ('points_batch_filtered_plus', 'points_batch_filtered_minus', 'points_batch_filtered_carry_plus', 'points_batch_filtered_carry_minus', 'points_batch_filtered_node_plus', 'points_batch_filtered_node_minus', 'points_batch_filtered_set', 'points_batch_filtered_set_carry', 'points_batch_filtered_set_node') THEN '筛选用户组'
    ELSE username
  END AS target_account,
  amount,
  method,
  COALESCE(points_scope, 'general'),
  COALESCE(node_id, ''),
  COALESCE(reason, ''),
  created_at
FROM recharge_records
WHERE ($1 = '' OR username ILIKE '%' || $1 || '%')
  AND ($2 = '' OR method = $2)
ORDER BY created_at DESC, recharge_id DESC
LIMIT $3`, username, method, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PointsOperationRecord, 0)
	for rows.Next() {
		var x PointsOperationRecord
		if err := rows.Scan(&x.RechargeID, &x.Username, &x.TargetAccount, &x.Amount, &x.Method, &x.PointsScope, &x.NodeID, &x.Reason, &x.CreatedAt); err != nil {
			return nil, err
		}
		x.CreatedAt = asBeijingWallTime(x.CreatedAt)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListUserPointsIncreaseRecords(ctx context.Context, username string, limit int) ([]PointsOperationRecord, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username 不能为空")
	}
	if limit <= 0 || limit > 10000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  recharge_id,
  username,
  amount,
  method,
  COALESCE(points_scope, 'general'),
  COALESCE(node_id, ''),
  COALESCE(reason, ''),
  created_at
FROM recharge_records
WHERE username=$1
  AND (amount > 0 OR method IN ('monthly_reset', 'monthly_carryover_reset'))
ORDER BY created_at DESC, recharge_id DESC
LIMIT $2`, username, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PointsOperationRecord, 0)
	for rows.Next() {
		var x PointsOperationRecord
		if err := rows.Scan(&x.RechargeID, &x.Username, &x.Amount, &x.Method, &x.PointsScope, &x.NodeID, &x.Reason, &x.CreatedAt); err != nil {
			return nil, err
		}
		x.TargetAccount = x.Username
		x.CreatedAt = asBeijingWallTime(x.CreatedAt)
		out = append(out, x)
	}
	return out, rows.Err()
}

func buildUsageAdminConds(
	billingUsername string,
	localUsername string,
	unregisteredOnly bool,
	addArg func(any) string,
) []string {
	conds := make([]string, 0, 5)
	billingUsername = strings.TrimSpace(billingUsername)
	localUsername = strings.TrimSpace(localUsername)
	if billingUsername != "" {
		conds = append(conds, "ur.username="+addArg(billingUsername))
	}
	if localUsername != "" {
		conds = append(conds, "ur.local_username="+addArg(localUsername))
	}
	if unregisteredOnly {
		conds = append(conds, `
NOT (
  EXISTS(SELECT 1 FROM user_accounts ua2 WHERE ua2.username = ur.username)
  OR EXISTS(SELECT 1 FROM admin_accounts aa2 WHERE aa2.username = ur.username)
  OR EXISTS(SELECT 1 FROM power_users pu2 WHERE pu2.username = ur.username)
)`)
	}
	return conds
}

func buildUsageCSVEstimateExpr() string {
	// 估算 CSV 文本体积：字符串列长度 + 数值/分隔符固定开销。
	return `
COALESCE(SUM(
  LENGTH(COALESCE(ur.node_id, '')) +
  LENGTH(COALESCE(ur.username, '')) +
  LENGTH(COALESCE(ur.local_username, '')) +
  LENGTH(COALESCE(ur.gpu_usage::text, '')) +
  96
), 0)::BIGINT`
}

func (s *Store) queryUsageRows(
	ctx context.Context,
	billingUsername string,
	localUsername string,
	unregisteredOnly bool,
	hasFrom bool,
	from time.Time,
	hasTo bool,
	to time.Time,
	limit int,
) (*sql.Rows, error) {
	if limit <= 0 || limit > 200000 {
		limit = 20000
	}

	conds := []string{}
	args := []any{}
	argN := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	conds = append(conds, buildUsageAdminConds(billingUsername, localUsername, unregisteredOnly, argN)...)
	if hasFrom {
		conds = append(conds, "ur.timestamp>="+argN(from))
	}
	if hasTo {
		conds = append(conds, "ur.timestamp<="+argN(to))
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	query := fmt.Sprintf(`
SELECT ur.node_id, ur.username, ur.timestamp, ur.cpu_percent, ur.memory_mb, ur.gpu_usage::text, ur.cost
     , ur.local_username
FROM usage_records ur
%s
ORDER BY ur.timestamp ASC
LIMIT %s
`, where, argN(limit))

	return s.db.QueryContext(ctx, query, args...)
}

type usageDateStat struct {
	Date              string
	RecordCount       int64
	EstimatedCSVBytes int64
}

func (s *Store) ListUsageDateStats(
	ctx context.Context,
	billingUsername string,
	localUsername string,
	unregisteredOnly bool,
	hasFrom bool,
	from time.Time,
	hasTo bool,
	to time.Time,
	limit int,
) ([]usageDateStat, error) {
	if limit <= 0 || limit > 4000 {
		limit = 4000
	}
	args := make([]any, 0, 8)
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	conds := buildUsageAdminConds(billingUsername, localUsername, unregisteredOnly, addArg)
	if hasFrom {
		conds = append(conds, "ur.timestamp>="+addArg(from))
	}
	if hasTo {
		conds = append(conds, "ur.timestamp<="+addArg(to))
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	query := `
SELECT TO_CHAR(ur.timestamp::date, 'YYYY-MM-DD') AS day_key,
       COUNT(1) AS record_count,
       ` + buildUsageCSVEstimateExpr() + ` AS estimated_csv_bytes
FROM usage_records ur
` + where + `
GROUP BY day_key
ORDER BY day_key DESC
LIMIT ` + addArg(limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]usageDateStat, 0)
	for rows.Next() {
		var x usageDateStat
		if err := rows.Scan(&x.Date, &x.RecordCount, &x.EstimatedCSVBytes); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) EstimateUsageRange(
	ctx context.Context,
	billingUsername string,
	localUsername string,
	unregisteredOnly bool,
	hasFrom bool,
	from time.Time,
	hasTo bool,
	to time.Time,
) (int64, int64, int64, error) {
	args := make([]any, 0, 8)
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	conds := buildUsageAdminConds(billingUsername, localUsername, unregisteredOnly, addArg)
	if hasFrom {
		conds = append(conds, "ur.timestamp>="+addArg(from))
	}
	if hasTo {
		conds = append(conds, "ur.timestamp<="+addArg(to))
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	var records int64
	var estimatedCSVBytes int64
	var estimatedDBBytes int64
	query := `
SELECT COUNT(1),
       ` + buildUsageCSVEstimateExpr() + `,
       COALESCE(SUM(pg_column_size(ur)), 0)::BIGINT
FROM usage_records ur
` + where
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&records, &estimatedCSVBytes, &estimatedDBBytes); err != nil {
		return 0, 0, 0, err
	}
	return records, estimatedCSVBytes, estimatedDBBytes, nil
}

func (s *Store) DeleteUsageRange(
	ctx context.Context,
	billingUsername string,
	localUsername string,
	unregisteredOnly bool,
	hasFrom bool,
	from time.Time,
	hasTo bool,
	to time.Time,
) (int64, error) {
	args := make([]any, 0, 8)
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	conds := buildUsageAdminConds(billingUsername, localUsername, unregisteredOnly, addArg)
	if hasFrom {
		conds = append(conds, "timestamp>="+addArg(from))
	}
	if hasTo {
		conds = append(conds, "timestamp<="+addArg(to))
	}
	if len(conds) == 0 {
		return 0, errors.New("删除范围为空，至少需要日期范围")
	}
	query := `
WITH deleted AS (
  DELETE FROM usage_records ur
  WHERE ` + strings.Join(conds, " AND ") + `
  RETURNING 1
)
SELECT COUNT(1) FROM deleted`
	var deleted int64
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&deleted); err != nil {
		return 0, err
	}
	return deleted, nil
}
