package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

const (
	appSettingSMTPHost              = "smtp_host"
	appSettingSMTPPort              = "smtp_port"
	appSettingSMTPUser              = "smtp_user"
	appSettingSMTPPass              = "smtp_pass"
	appSettingFromEmail             = "from_email"
	appSettingFromName              = "from_name"
	appSettingUserGuideline         = "user_guideline_markdown"
	appSettingUserGuidelineBy       = "user_guideline_updated_by"
	appSettingMonthlyDoctorPoints   = "monthly_points_doctor"
	appSettingMonthlyMasterPoints   = "monthly_points_master"
	appSettingMonthlyOtherPoints    = "monthly_points_other"
	appSettingMonthlyCarryoverLimit = "monthly_points_carryover_limit"
)

var (
	errRegistrationVerifyTokenInvalid = errors.New("registration_verify_token_invalid")
	errRegistrationVerifyTokenExpired = errors.New("registration_verify_token_expired")
	errRegistrationVerifyTokenUsed    = errors.New("registration_verify_token_used")
)

type Store struct {
	db *sql.DB
}

func NewStore(cfg Config) (*Store, error) {
	db, err := sql.Open("postgres", cfg.DatabaseDSN)
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
	PrevStatus string
	User       User
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
	_, err := tx.ExecContext(ctx, `
INSERT INTO recharge_records(username, amount, method, points_scope, node_id)
VALUES($1, $2, $3, $4, NULLIF($5, ''))`, username, amount, method, pointsScope, nodeID)
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
		PrevStatus: prevStatus,
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
		PrevStatus: prevStatus,
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
		PrevStatus: prevStatus,
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
		PrevStatus: prevStatus,
		User: User{
			Username:         username,
			Balance:          targetBalance,
			CarryoverBalance: carryoverBalance,
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
		PrevStatus: prevStatus,
		User: User{
			Username:         username,
			Balance:          targetGeneral,
			CarryoverBalance: targetCarryover,
			Status:           newStatus,
			BlockedAt:        newBlockedAt,
		},
	}, generalDelta, carryoverDelta, nil
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

func (s *Store) ListUserNodeAccountsByBilling(ctx context.Context, billingUsername string, limit int) ([]UserNodeAccount, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return nil, errors.New("billing_username 不能为空")
	}
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT node_id, local_username, billing_username, created_at, updated_at
FROM user_node_accounts
WHERE billing_username=$1
ORDER BY node_id, local_username
LIMIT $2`, billingUsername, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserNodeAccount, 0)
	for rows.Next() {
		var v UserNodeAccount
		if err := rows.Scan(&v.NodeID, &v.LocalUsername, &v.BillingUsername, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ListUserNodeAccounts(ctx context.Context, billingUsername string, limit int) ([]UserNodeAccount, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}
	if billingUsername == "" {
		rows, err := s.db.QueryContext(ctx, `
SELECT node_id, local_username, billing_username, created_at, updated_at
FROM user_node_accounts
ORDER BY billing_username, node_id, local_username
LIMIT $1`, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]UserNodeAccount, 0)
		for rows.Next() {
			var v UserNodeAccount
			if err := rows.Scan(&v.NodeID, &v.LocalUsername, &v.BillingUsername, &v.CreatedAt, &v.UpdatedAt); err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, rows.Err()
	}
	return s.ListUserNodeAccountsByBilling(ctx, billingUsername, limit)
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
         COALESCE(np.ssh_exclusive_enabled, FALSE) AS exclusive_enabled
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
    AND EXISTS (SELECT 1 FROM policy WHERE exclusive_enabled=false)
  UNION ALL
  SELECT local_username
  FROM node_exclusive_users
  WHERE node_id=$1
    AND EXISTS (SELECT 1 FROM policy WHERE exclusive_enabled=true)
  UNION ALL
  SELECT nlu.local_username
  FROM node_local_users nlu
  WHERE nlu.node_id=$1
    AND EXISTS (SELECT 1 FROM policy WHERE guard_enabled=false AND exclusive_enabled=false)
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

	if requestType != "bind" && requestType != "open" {
		return 0, errors.New("request_type 仅支持 bind/open")
	}
	if billingUsername == "" || nodeID == "" || localUsername == "" {
		return 0, errors.New("billing_username/node_id/local_username 不能为空")
	}
	if requestType == "open" {
		if utf8.RuneCountInString(message) < 10 {
			return 0, errors.New("请详细填写开通理由（至少 10 个字）")
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

	var id int
	err := tx.QueryRowContext(ctx, `
INSERT INTO user_requests(request_type, billing_username, node_id, local_username, message, status)
VALUES($1,$2,$3,$4,$5,'pending')
RETURNING request_id`, requestType, billingUsername, nodeID, localUsername, message).Scan(&id)
	return id, err
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
       reviewed_by, reviewed_at, created_at, updated_at
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
			&r.Message, &r.Status, &reviewedBy, &reviewedAt, &r.CreatedAt, &r.UpdatedAt,
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
       reviewed_by, reviewed_at, created_at, updated_at
FROM user_requests
ORDER BY created_at DESC
LIMIT $1`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT request_id, request_type, billing_username, node_id, local_username, message, status,
       reviewed_by, reviewed_at, created_at, updated_at
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
			&r.Message, &r.Status, &reviewedBy, &reviewedAt, &r.CreatedAt, &r.UpdatedAt,
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

	// 锁住记录，避免并发重复审批
	var r UserRequest
	var reviewedByPrev sql.NullString
	var reviewedAtPrev sql.NullTime
	if err := tx.QueryRowContext(ctx, `
SELECT request_id, request_type, billing_username, node_id, local_username, message, status,
       reviewed_by, reviewed_at, created_at, updated_at
FROM user_requests
WHERE request_id=$1
FOR UPDATE`, requestID).Scan(
		&r.RequestID, &r.RequestType, &r.BillingUsername, &r.NodeID, &r.LocalUsername,
		&r.Message, &r.Status, &reviewedByPrev, &reviewedAtPrev, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return UserRequest{}, err
	}
	if r.Status != "pending" {
		return UserRequest{}, errors.New("该申请已处理，不能重复审核")
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE user_requests
SET status=$2, reviewed_by=$3, reviewed_at=$4, updated_at=NOW()
WHERE request_id=$1`, requestID, newStatus, reviewedBy, reviewedAt); err != nil {
		return UserRequest{}, err
	}

	// bind 申请在 approved 时，写入映射表，供计费与 SSH 校验使用
	if newStatus == "approved" && r.RequestType == "bind" {
		if err := s.UpsertUserNodeAccountTx(ctx, tx, r.NodeID, r.LocalUsername, r.BillingUsername); err != nil {
			return UserRequest{}, err
		}
	}

	r.Status = newStatus
	r.ReviewedBy = &reviewedBy
	r.ReviewedAt = &reviewedAt
	r.UpdatedAt = reviewedAt
	return r, nil
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
	nodeIP = strings.TrimSpace(nodeIP)
	nodeMAC = strings.TrimSpace(nodeMAC)
	if cpuCount < 0 {
		cpuCount = 0
	}
	if gpuCount < 0 {
		gpuCount = 0
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

	_, err := tx.ExecContext(ctx, `
INSERT INTO nodes(
  node_id, last_seen_at, last_report_id, last_report_ts, interval_seconds,
  cpu_model, cpu_count, gpu_model, gpu_count, os_version, kernel_version, node_ip, node_mac, disk_total_gb, disk_used_gb, home_total_gb, home_used_gb, mnt_total_gb, mnt_used_gb,
  net_rx_bytes, net_tx_bytes, net_rx_mb_month, net_tx_mb_month, traffic_month,
  gpu_process_count, cpu_process_count, usage_records_count, ssh_active_count, cost_total, updated_at
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,NOW())
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
  cost_total=EXCLUDED.cost_total,
  updated_at=NOW()
`, nodeID, lastSeenAt, reportID, reportTS, intervalSeconds,
		cpuModel, cpuCount, gpuModel, gpuCount, osVersion, kernelVersion, nodeIP, nodeMAC,
		diskTotalGB, diskUsedGB, homeTotalGB, homeUsedGB, mntTotalGB, mntUsedGB,
		int64(netRxBytes), int64(netTxBytes), rxMBMonth, txMBMonth, month,
		gpuProcCount, cpuProcCount, usageRecordsCount, sshActiveCount, costTotal)
	return err
}

func (s *Store) ListNodes(ctx context.Context, limit int) ([]NodeStatus, error) {
	if limit <= 0 || limit > 2000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT n.node_id, n.last_seen_at, n.last_report_id, n.last_report_ts, n.interval_seconds,
       n.cpu_model, n.cpu_count, n.gpu_model, n.gpu_count, n.os_version, n.kernel_version, n.node_ip, n.node_mac, n.disk_total_gb, n.disk_used_gb, n.home_total_gb, n.home_used_gb, n.mnt_total_gb, n.mnt_used_gb,
       n.net_rx_mb_month, n.net_tx_mb_month,
       n.gpu_process_count, n.cpu_process_count, n.usage_records_count, n.ssh_active_count,
       COALESCE(np.ssh_guard_enabled, FALSE) AS ssh_guard_enabled,
       COALESCE(np.ssh_exclusive_enabled, FALSE) AS ssh_exclusive_enabled,
       COALESCE(np.points_intercept_enabled, FALSE) AS points_intercept_enabled,
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
		var nodePrice sql.NullFloat64
		var nodeModelPricesRaw []byte
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
			&n.SSHGuardEnabled,
			&n.SSHExclusiveEnabled,
			&n.PointsInterceptEnabled,
			&nodePrice,
			&nodeModelPricesRaw,
			&n.SecurityEventCount7d,
			&n.SuspiciousUserCount7d,
			&n.CostTotal,
			&n.UpdatedAt,
		); err != nil {
			return nil, err
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
	var nodePrice sql.NullFloat64
	var nodeModelPricesRaw []byte
	err := s.db.QueryRowContext(ctx, `
SELECT n.node_id, n.last_seen_at, n.last_report_id, n.last_report_ts, n.interval_seconds,
       n.cpu_model, n.cpu_count, n.gpu_model, n.gpu_count, n.os_version, n.kernel_version, n.node_ip, n.node_mac, n.disk_total_gb, n.disk_used_gb, n.home_total_gb, n.home_used_gb, n.mnt_total_gb, n.mnt_used_gb,
       n.net_rx_mb_month, n.net_tx_mb_month,
       n.gpu_process_count, n.cpu_process_count, n.usage_records_count, n.ssh_active_count,
       COALESCE(np.ssh_guard_enabled, FALSE) AS ssh_guard_enabled,
       COALESCE(np.ssh_exclusive_enabled, FALSE) AS ssh_exclusive_enabled,
       COALESCE(np.points_intercept_enabled, FALSE) AS points_intercept_enabled,
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
		&n.SSHGuardEnabled,
		&n.SSHExclusiveEnabled,
		&n.PointsInterceptEnabled,
		&nodePrice,
		&nodeModelPricesRaw,
		&n.CostTotal,
		&n.UpdatedAt,
	)
	if err != nil {
		return n, err
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
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return false, errors.New("node_id 不能为空")
	}
	var enabled bool
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(np.ssh_exclusive_enabled, FALSE)
FROM nodes n
LEFT JOIN node_policies np ON np.node_id=n.node_id
WHERE n.node_id=$1`, nodeID).Scan(&enabled)
	if err != nil {
		return false, err
	}
	return enabled, nil
}

func (s *Store) GetNodePointsInterceptPolicy(ctx context.Context, nodeID string) (bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return false, errors.New("node_id 不能为空")
	}
	var enabled bool
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(np.points_intercept_enabled, FALSE)
FROM nodes n
LEFT JOIN node_policies np ON np.node_id=n.node_id
WHERE n.node_id=$1`, nodeID).Scan(&enabled)
	if err != nil {
		return false, err
	}
	return enabled, nil
}

func (s *Store) GetNodePointsInterceptPolicyTx(ctx context.Context, tx *sql.Tx, nodeID string) (bool, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return false, errors.New("node_id 不能为空")
	}
	var enabled bool
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE((SELECT points_intercept_enabled FROM node_policies WHERE node_id=$1), FALSE)`, nodeID).Scan(&enabled); err != nil {
		return false, err
	}
	return enabled, nil
}

func (s *Store) GetNodePricePolicy(ctx context.Context, nodeID string) (*float64, map[string]float64, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, nil, errors.New("node_id 不能为空")
	}
	var p sql.NullFloat64
	var modelRaw []byte
	err := s.db.QueryRowContext(ctx, `
SELECT np.node_price_per_minute, COALESCE(np.node_model_price_overrides, '{}'::jsonb)
FROM nodes n
LEFT JOIN node_policies np ON np.node_id=n.node_id
WHERE n.node_id=$1`, nodeID).Scan(&p, &modelRaw)
	if err != nil {
		return nil, nil, err
	}
	overrides, err := parseNodeModelPriceOverridesRaw(modelRaw)
	if err != nil {
		return nil, nil, err
	}
	var price *float64
	if p.Valid {
		v := p.Float64
		price = &v
	}
	return price, overrides, nil
}

func (s *Store) GetNodePricePolicyTx(ctx context.Context, tx *sql.Tx, nodeID string) (*float64, map[string]float64, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, nil, errors.New("node_id 不能为空")
	}
	var p sql.NullFloat64
	var modelRaw []byte
	if err := tx.QueryRowContext(ctx, `
SELECT node_price_per_minute, COALESCE(node_model_price_overrides, '{}'::jsonb)
FROM node_policies
WHERE node_id=$1`, nodeID).Scan(&p, &modelRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	overrides, err := parseNodeModelPriceOverridesRaw(modelRaw)
	if err != nil {
		return nil, nil, err
	}
	var price *float64
	if p.Valid {
		v := p.Float64
		price = &v
	}
	return price, overrides, nil
}

func (s *Store) GetNodePriceOverride(ctx context.Context, nodeID string) (*float64, error) {
	price, _, err := s.GetNodePricePolicy(ctx, nodeID)
	return price, err
}

func (s *Store) GetNodePriceOverrideTx(ctx context.Context, tx *sql.Tx, nodeID string) (*float64, error) {
	price, _, err := s.GetNodePricePolicyTx(ctx, tx, nodeID)
	return price, err
}

func (s *Store) UpsertNodePricePolicy(ctx context.Context, nodeID string, pricePerMinute *float64, modelOverrides map[string]float64, updatedBy string) error {
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
	normOverrides, err := normalizeNodeModelPriceOverrides(modelOverrides)
	if err != nil {
		return err
	}
	overridesJSON, err := json.Marshal(normOverrides)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO node_policies(node_id, node_price_per_minute, node_model_price_overrides, updated_by, updated_at)
VALUES($1,$2,$3::jsonb,$4,NOW())
ON CONFLICT (node_id) DO UPDATE SET
  node_price_per_minute=EXCLUDED.node_price_per_minute,
  node_model_price_overrides=EXCLUDED.node_model_price_overrides,
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()`,
		nodeID, price, string(overridesJSON), updatedBy,
	)
	return err
}

func (s *Store) UpsertNodePriceOverride(ctx context.Context, nodeID string, pricePerMinute *float64, updatedBy string) error {
	return s.UpsertNodePricePolicy(ctx, nodeID, pricePerMinute, nil, updatedBy)
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

func (s *Store) UpsertNodePointsInterceptPolicy(ctx context.Context, nodeID string, enabled bool, updatedBy string) error {
	nodeID = strings.TrimSpace(nodeID)
	updatedBy = strings.TrimSpace(updatedBy)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO node_policies(node_id, points_intercept_enabled, updated_by, updated_at)
VALUES($1,$2,$3,NOW())
ON CONFLICT (node_id) DO UPDATE SET
  points_intercept_enabled=EXCLUDED.points_intercept_enabled,
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()`,
		nodeID, enabled, updatedBy,
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

func (s *Store) ReplaceNodeExclusiveUsers(ctx context.Context, nodeID string, enabled bool, users []string, updatedBy string) error {
	nodeID = strings.TrimSpace(nodeID)
	updatedBy = strings.TrimSpace(updatedBy)
	if nodeID == "" {
		return errors.New("node_id 不能为空")
	}
	if updatedBy == "" {
		updatedBy = "admin"
	}
	cleanUsers := uniqTrim(users)
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO node_policies(node_id, ssh_exclusive_enabled, updated_by, updated_at)
VALUES($1,$2,$3,NOW())
ON CONFLICT (node_id) DO UPDATE SET
  ssh_exclusive_enabled=EXCLUDED.ssh_exclusive_enabled,
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()`, nodeID, enabled, updatedBy); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM node_exclusive_users WHERE node_id=$1`, nodeID); err != nil {
			return err
		}
		for _, u := range cleanUsers {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO node_exclusive_users(node_id, local_username, updated_by, updated_at)
VALUES($1,$2,$3,NOW())`, nodeID, u, updatedBy); err != nil {
				return err
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
		homeUsedGB := u.HomeUsedGB
		if homeUsedGB < 0 {
			homeUsedGB = 0
		}
		hasSudo := u.HasSudo
		hasDocker := u.HasDocker
		if _, err := tx.ExecContext(ctx, `
INSERT INTO node_local_users(node_id, local_username, home_created_at, last_login_at, home_used_gb, has_sudo, has_docker, updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,NOW())`,
			nodeID, localUsername, u.HomeCreatedAt, u.LastLoginAt, homeUsedGB, hasSudo, hasDocker,
		); err != nil {
			return err
		}
	}
	return nil
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
       COALESCE(una.billing_username, '') AS platform_username,
       (una.billing_username IS NOT NULL) AS mapping_exists,
       (
         EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM admin_accounts aa WHERE aa.username=una.billing_username)
         OR EXISTS(SELECT 1 FROM power_users pu WHERE pu.username=una.billing_username)
       ) AS platform_exists,
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
		if err := rows.Scan(
			&u.NodeID,
			&u.LocalUsername,
			&u.PlatformUsername,
			&u.MappingExists,
			&u.PlatformExists,
			&u.HomeCreatedAt,
			&u.LastLoginAt,
			&u.HomeUsedGB,
			&u.HasSudo,
			&u.HasDocker,
			&u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, u)
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

func (s *Store) ListNodeSecurityEvents(ctx context.Context, nodeID string, eventType string, limit int) ([]NodeSecurityEvent, error) {
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
		out = append(out, e)
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
	if len(password) < 8 {
		return errors.New("password 至少 8 位")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO admin_accounts(username, password_hash)
VALUES($1,$2)
ON CONFLICT (username) DO NOTHING`, username, string(hash))
	return err
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
	_, _ = s.db.ExecContext(ctx, `UPDATE admin_accounts SET last_login_at=NOW(), updated_at=NOW() WHERE username=$1`, username)
	return true, nil
}

func (s *Store) CreatePowerUser(ctx context.Context, username string, password string, canViewBoard bool, canViewNodes bool, canManageNodes bool, canReview bool, createdBy string) error {
	username = strings.TrimSpace(username)
	createdBy = strings.TrimSpace(createdBy)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if len(password) < 8 {
		return errors.New("password 至少 8 位")
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
	_, err = s.db.ExecContext(ctx, `
INSERT INTO power_users(username, password_hash, can_view_board, can_view_nodes, can_manage_nodes, can_review_requests, created_by, updated_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$7)`, username, string(hash), canViewBoard, canViewNodes, canManageNodes, canReview, createdBy)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) VerifyPowerUserPassword(ctx context.Context, username string, password string) (PowerUser, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return PowerUser{}, false, errors.New("username 不能为空")
	}
	var hash string
	var out PowerUser
	err := s.db.QueryRowContext(ctx, `
SELECT password_hash, username, can_view_board, can_view_nodes, can_manage_nodes, can_review_requests, created_by, updated_by, last_login_at, created_at, updated_at
FROM power_users
WHERE username=$1`, username).Scan(
		&hash, &out.Username, &out.CanViewBoard, &out.CanViewNodes, &out.CanManageNodes, &out.CanReviewRequests, &out.CreatedBy, &out.UpdatedBy, &out.LastLoginAt, &out.CreatedAt, &out.UpdatedAt,
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
	_, _ = s.db.ExecContext(ctx, `UPDATE power_users SET last_login_at=NOW(), updated_at=NOW() WHERE username=$1`, username)
	return out, true, nil
}

func (s *Store) ListPowerUsers(ctx context.Context, limit int) ([]PowerUser, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT pu.username,
       EXISTS(SELECT 1 FROM user_accounts ua WHERE ua.username=pu.username) AS is_platform_user,
       pu.can_view_board,
       pu.can_view_nodes,
       pu.can_manage_nodes,
       pu.can_review_requests,
       pu.created_by,
       pu.updated_by,
       pu.last_login_at,
       pu.created_at,
       pu.updated_at
FROM power_users pu
ORDER BY username
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PowerUser, 0)
	for rows.Next() {
		var p PowerUser
		if err := rows.Scan(&p.Username, &p.IsPlatformUser, &p.CanViewBoard, &p.CanViewNodes, &p.CanManageNodes, &p.CanReviewRequests, &p.CreatedBy, &p.UpdatedBy, &p.LastLoginAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) PromotePlatformUserToPowerUser(ctx context.Context, username string, canViewBoard bool, canViewNodes bool, canManageNodes bool, canReview bool, updatedBy string) error {
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
		if _, err := tx.ExecContext(ctx, `
INSERT INTO power_users(username, password_hash, can_view_board, can_view_nodes, can_manage_nodes, can_review_requests, created_by, updated_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$7)
ON CONFLICT (username) DO UPDATE SET
  password_hash=EXCLUDED.password_hash,
  can_view_board=EXCLUDED.can_view_board,
  can_view_nodes=EXCLUDED.can_view_nodes,
  can_manage_nodes=EXCLUDED.can_manage_nodes,
  can_review_requests=EXCLUDED.can_review_requests,
  updated_by=EXCLUDED.updated_by,
  updated_at=NOW()`, username, pwdHash, canViewBoard, canViewNodes, canManageNodes, canReview, updatedBy); err != nil {
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

func (s *Store) UpdatePowerUserPermissions(ctx context.Context, username string, canViewBoard bool, canViewNodes bool, canManageNodes bool, canReview bool, updatedBy string) error {
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
SET can_view_board=$2, can_view_nodes=$3, can_manage_nodes=$4, can_review_requests=$5, updated_by=$6, updated_at=NOW()
WHERE username=$1`, username, canViewBoard, canViewNodes, canManageNodes, canReview, updatedBy)
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
		return "admin", uint32(permViewBoard | permViewNodes | permManageNodes | permReviewRequests), true, nil
	}
	var canViewBoard bool
	var canViewNodes bool
	var canManageNodes bool
	var canReview bool
	err := s.db.QueryRowContext(ctx, `
SELECT can_view_board, can_view_nodes, COALESCE(can_manage_nodes,false), can_review_requests
FROM power_users
WHERE username=$1`, username).Scan(&canViewBoard, &canViewNodes, &canManageNodes, &canReview)
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
	if len(password) < 8 {
		return errors.New("password 至少 8 位")
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
	if _, err := tx.ExecContext(ctx, `
INSERT INTO user_accounts(
  username, email, password_hash, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, role
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'user')`,
		in.Username, in.Email, string(hash), in.RealName, in.StudentID, in.Advisor, in.ExpectedGraduationYear, in.ExpectedGraduationMonth, in.Phone); err != nil {
		return err
	}
	_, err = s.EnsureUserTx(ctx, tx, in.Username, defaultBalance)
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
	if len(password) < 8 {
		return errors.New("密码至少 8 位")
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
          reviewed_by, reviewed_at, reject_reason, created_at, updated_at`,
		v.Username, v.Email, v.PasswordHash, v.RealName, v.StudentID, v.Advisor,
		v.ExpectedGraduationYear, v.ExpectedGraduationMonth, v.Phone,
	).Scan(
		&v.RequestID, &v.Username, &v.Email, &v.PasswordHash, &v.RealName, &v.StudentID, &v.Advisor,
		&v.ExpectedGraduationYear, &v.ExpectedGraduationMonth, &v.Phone, &v.Status,
		&reviewedBy, &reviewedAt, &v.RejectReason, &v.CreatedAt, &v.UpdatedAt,
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
	if len(password) < 8 {
		return RegistrationRequest{}, errors.New("密码至少 8 位")
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
          reviewed_by, reviewed_at, reject_reason, created_at, updated_at`,
		in.Username, in.Email, string(hash), in.RealName, in.StudentID, in.Advisor,
		in.ExpectedGraduationYear, in.ExpectedGraduationMonth, in.Phone,
	).Scan(
		&out.RequestID, &out.Username, &out.Email, &out.PasswordHash, &out.RealName, &out.StudentID, &out.Advisor,
		&out.ExpectedGraduationYear, &out.ExpectedGraduationMonth, &out.Phone, &out.Status,
		&reviewedBy, &reviewedAt, &out.RejectReason, &out.CreatedAt, &out.UpdatedAt,
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
       reviewed_by, reviewed_at, reject_reason, created_at, updated_at
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
			&reviewedBy, &reviewedAt, &r.RejectReason, &r.CreatedAt, &r.UpdatedAt,
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
	if _, err := tx.ExecContext(ctx, `
INSERT INTO user_accounts(
  username, email, password_hash, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, role
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'user')`,
		in.Username, in.Email, in.PasswordHash, in.RealName, in.StudentID, in.Advisor, in.ExpectedGraduationYear, in.ExpectedGraduationMonth, in.Phone,
	); err != nil {
		return err
	}
	_, err := s.EnsureUserTx(ctx, tx, in.Username, defaultBalance)
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
	if reviewedBy == "" {
		reviewedBy = "admin"
	}
	var r RegistrationRequest
	var reviewedByPrev sql.NullString
	var reviewedAtPrev sql.NullTime
	if err := tx.QueryRowContext(ctx, `
SELECT request_id, username, email, password_hash, real_name, student_id, advisor,
       expected_graduation_year, expected_graduation_month, phone, status,
       reviewed_by, reviewed_at, reject_reason, created_at, updated_at
FROM registration_requests
WHERE request_id=$1
FOR UPDATE`, requestID).Scan(
		&r.RequestID, &r.Username, &r.Email, &r.PasswordHash, &r.RealName, &r.StudentID, &r.Advisor,
		&r.ExpectedGraduationYear, &r.ExpectedGraduationMonth, &r.Phone, &r.Status,
		&reviewedByPrev, &reviewedAtPrev, &r.RejectReason, &r.CreatedAt, &r.UpdatedAt,
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
SET status=$2, reviewed_by=$3, reviewed_at=$4, reject_reason=$5, updated_at=NOW()
WHERE request_id=$1`, requestID, newStatus, reviewedBy, reviewedAt, rejectReason); err != nil {
		return RegistrationRequest{}, err
	}
	r.Status = newStatus
	r.ReviewedBy = &reviewedBy
	r.ReviewedAt = &reviewedAt
	r.RejectReason = rejectReason
	r.UpdatedAt = reviewedAt
	return r, nil
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
	_, _ = s.db.ExecContext(ctx, `UPDATE user_accounts SET last_login_at=NOW(), updated_at=NOW() WHERE username=$1`, username)
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
	// 平台用户被管理员拉黑时会写入全局 SSH 黑名单（node_id='*'）。
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM ssh_blacklist WHERE node_id='*' AND local_username=$1
)`, username).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return "blacklisted_account", nil
	}
	return "", nil
}

func (s *Store) GetUserAccountByUsername(ctx context.Context, username string) (UserAccount, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return UserAccount{}, errors.New("username 不能为空")
	}
	var out UserAccount
	err := s.db.QueryRowContext(ctx, `
SELECT username, email, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, role, last_login_at, created_at, updated_at
FROM user_accounts
WHERE username=$1`, username).Scan(
		&out.Username, &out.Email, &out.RealName, &out.StudentID, &out.Advisor, &out.ExpectedGraduationYear, &out.ExpectedGraduationMonth,
		&out.Phone, &out.Role, &out.LastLoginAt, &out.CreatedAt, &out.UpdatedAt,
	)
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
       expected_graduation_year, expected_graduation_month, phone, role,
       last_login_at, created_at, updated_at
FROM user_accounts
WHERE username=$1
FOR UPDATE`, username).Scan(
		&r.Username, &r.Email, &r.StudentID, &r.PasswordHash, &r.RealName, &r.Advisor,
		&r.ExpectedGraduationYear, &r.ExpectedGraduationMonth, &r.Phone, &r.Role,
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
	var restoredAt sql.NullTime
	var restoredBy sql.NullString
	err = tx.QueryRowContext(ctx, `
INSERT INTO deleted_user_accounts(
  username, email, student_id, password_hash, real_name, advisor, expected_graduation_year, expected_graduation_month,
  phone, role, last_login_at, account_created_at, account_updated_at, balance, carryover_balance, user_status, blocked_at, deleted_by, delete_reason, node_accounts_json
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20::jsonb)
RETURNING deleted_id, username, email, student_id, real_name, advisor, expected_graduation_year, expected_graduation_month,
          phone, role, balance, carryover_balance, user_status, deleted_at, deleted_by, delete_reason, restored_at, restored_by`,
		r.Username, r.Email, r.StudentID, r.PasswordHash, r.RealName, r.Advisor, r.ExpectedGraduationYear, r.ExpectedGraduationMonth,
		r.Phone, r.Role, r.LastLoginAt, r.AccountCreatedAt, r.AccountUpdatedAt,
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
		&out.Phone, &out.Role, &out.Balance, &out.CarryoverBalance, &out.UserStatus, &out.DeletedAt, &out.DeletedBy, &out.DeleteReason, &restoredAt, &restoredBy,
	)
	if err != nil && isColumnMissingErr(err, "node_accounts_json") {
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
       phone, role, balance, carryover_balance, user_status, deleted_at, deleted_by, delete_reason, restored_at, restored_by,
       password_hash, last_login_at, account_created_at, account_updated_at, blocked_at, COALESCE(node_accounts_json::text, '[]')
FROM deleted_user_accounts
WHERE deleted_id=$1
FOR UPDATE`, deletedID).Scan(
		&r.DeletedID, &r.Username, &r.Email, &r.StudentID, &r.RealName, &r.Advisor, &r.ExpectedGraduationYear, &r.ExpectedGraduationMonth,
		&r.Phone, &r.Role, &r.Balance, &r.CarryoverBalance, &r.UserStatus, &r.DeletedAt, &r.DeletedBy, &r.DeleteReason, &restoredAt, &restoredByPrev,
		&r.PasswordHash, &r.LastLoginAt, &r.AccountCreatedAt, &r.AccountUpdatedAt, &r.BlockedAt, &r.NodeAccountsJSON,
	)
	if err != nil && isColumnMissingErr(err, "node_accounts_json") {
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
	_, err = tx.ExecContext(ctx, `
INSERT INTO user_accounts(
  username, email, password_hash, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, role, last_login_at
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		r.Username, strings.ToLower(r.Email), r.PasswordHash, r.RealName, r.StudentID, r.Advisor, r.ExpectedGraduationYear, r.ExpectedGraduationMonth, r.Phone, r.Role, r.LastLoginAt,
	)
	if err != nil {
		return DeletedUserAccount{}, err
	}
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
	return out, nil
}

func (s *Store) ListDeletedUserAccounts(ctx context.Context, includeRestored bool, limit int) ([]DeletedUserAccount, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	query := `
SELECT deleted_id, username, email, student_id, real_name, advisor, expected_graduation_year, expected_graduation_month,
       phone, role, balance, carryover_balance, user_status, deleted_at, deleted_by, delete_reason, restored_at, restored_by
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
	for rows.Next() {
		var r DeletedUserAccount
		var restoredAt sql.NullTime
		var restoredBy sql.NullString
		if err := rows.Scan(
			&r.DeletedID, &r.Username, &r.Email, &r.StudentID, &r.RealName, &r.Advisor, &r.ExpectedGraduationYear, &r.ExpectedGraduationMonth,
			&r.Phone, &r.Role, &r.Balance, &r.CarryoverBalance, &r.UserStatus, &r.DeletedAt, &r.DeletedBy, &r.DeleteReason, &restoredAt, &restoredBy,
		); err != nil {
			return nil, err
		}
		if restoredAt.Valid {
			v := restoredAt.Time
			r.RestoredAt = &v
		}
		if restoredBy.Valid {
			v := restoredBy.String
			r.RestoredBy = &v
		}
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
SELECT username, email, real_name, student_id, advisor, expected_graduation_year, expected_graduation_month, phone, role, last_login_at, created_at, updated_at
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
		if err := activeRows.Scan(&x.Username, &x.Email, &x.RealName, &x.StudentID, &x.Advisor, &x.ExpectedGraduationYear, &x.ExpectedGraduationMonth, &x.Phone, &x.Role, &x.LastLoginAt, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, nil, err
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
       phone, role, balance, carryover_balance, user_status, deleted_at, deleted_by, delete_reason, restored_at, restored_by
FROM deleted_user_accounts
WHERE (`+whereDeleted+`) AND restored_at IS NULL
ORDER BY deleted_at DESC
LIMIT $`+strconv.Itoa(len(deletedArgs)), deletedArgs...)
	if err != nil {
		return nil, nil, err
	}
	defer archRows.Close()
	deleted := make([]DeletedUserAccount, 0)
	for archRows.Next() {
		var x DeletedUserAccount
		var restoredAt sql.NullTime
		var restoredBy sql.NullString
		if err := archRows.Scan(
			&x.DeletedID, &x.Username, &x.Email, &x.StudentID, &x.RealName, &x.Advisor, &x.ExpectedGraduationYear, &x.ExpectedGraduationMonth,
			&x.Phone, &x.Role, &x.Balance, &x.CarryoverBalance, &x.UserStatus, &x.DeletedAt, &x.DeletedBy, &x.DeleteReason, &restoredAt, &restoredBy,
		); err != nil {
			return nil, nil, err
		}
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
	if len(newPassword) < 8 {
		return errors.New("新密码至少 8 位")
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
	if len(newPassword) < 8 {
		return errors.New("新密码至少 8 位")
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
	if len(newPassword) < 8 {
		return errors.New("新密码至少 8 位")
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
		DoctorPoints:   200,
		MasterPoints:   100,
		OtherPoints:    50,
		CarryoverLimit: 500,
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM app_settings WHERE key = ANY($1)`, pq.Array([]string{
		appSettingMonthlyDoctorPoints, appSettingMonthlyMasterPoints, appSettingMonthlyOtherPoints, appSettingMonthlyCarryoverLimit,
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
		}
	}
	return out, rows.Err()
}

func (s *Store) LoadMonthlyPointsConfigTx(ctx context.Context, tx *sql.Tx) (MonthlyPointsConfig, error) {
	out := MonthlyPointsConfig{
		DoctorPoints:   200,
		MasterPoints:   100,
		OtherPoints:    50,
		CarryoverLimit: 500,
	}
	rows, err := tx.QueryContext(ctx, `SELECT key, value FROM app_settings WHERE key = ANY($1)`, pq.Array([]string{
		appSettingMonthlyDoctorPoints, appSettingMonthlyMasterPoints, appSettingMonthlyOtherPoints, appSettingMonthlyCarryoverLimit,
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
		}
	}
	return out, rows.Err()
}

func (s *Store) UpsertMonthlyPointsConfig(ctx context.Context, cfg MonthlyPointsConfig) error {
	if cfg.DoctorPoints < 0 || cfg.MasterPoints < 0 || cfg.OtherPoints < 0 || cfg.CarryoverLimit < 0 {
		return errors.New("doctor_points/master_points/other_points/carryover_limit 不能为负数")
	}
	items := map[string]string{
		appSettingMonthlyDoctorPoints:   strconv.FormatFloat(cfg.DoctorPoints, 'f', -1, 64),
		appSettingMonthlyMasterPoints:   strconv.FormatFloat(cfg.MasterPoints, 'f', -1, 64),
		appSettingMonthlyOtherPoints:    strconv.FormatFloat(cfg.OtherPoints, 'f', -1, 64),
		appSettingMonthlyCarryoverLimit: strconv.FormatFloat(cfg.CarryoverLimit, 'f', -1, 64),
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
    COALESCE(NULLIF(ua.role, ''), 'user') AS role,
    FALSE AS can_view_board,
    FALSE AS can_view_nodes,
    FALSE AS can_review_requests,
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
    'admin' AS role,
    TRUE AS can_view_board,
    TRUE AS can_view_nodes,
    TRUE AS can_review_requests,
    '' AS email,
    '' AS student_id,
    '' AS real_name,
    '' AS advisor,
    0 AS expected_graduation_year,
    0 AS expected_graduation_month,
    '' AS phone
  FROM admin_accounts aa
  UNION ALL
  SELECT
    pu.username,
    'power_user' AS role,
    pu.can_view_board,
    pu.can_view_nodes,
    pu.can_review_requests,
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
SELECT uu.username, uu.role, uu.can_view_board, uu.can_view_nodes, uu.can_review_requests,
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
		if err := rows.Scan(
			&d.Username, &d.Role, &d.CanViewBoard, &d.CanViewNodes, &d.CanReviewRequest,
			&d.Email, &d.StudentID, &d.RealName, &d.Advisor, &d.ExpectedGradYear, &d.ExpectedGradMonth, &d.Phone,
			&d.Balance, &d.CarryoverBalance, &d.ExclusiveBalance, &d.TotalBalance, &d.Status, &d.UsageRecords, &d.TotalCost, &d.LastUsageAt,
		); err != nil {
			return nil, err
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

func (s *Store) ListPointsUsers(ctx context.Context, keyword string, limit int) ([]PointsUser, error) {
	keyword = strings.TrimSpace(keyword)
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
    COALESCE(NULLIF(ua.student_id, ''), '') AS student_id,
    COALESCE(NULLIF(ua.role, ''), 'user') AS role
  FROM user_accounts ua
  WHERE NOT EXISTS (SELECT 1 FROM admin_accounts aa WHERE aa.username = ua.username)
    AND NOT EXISTS (SELECT 1 FROM power_users pu WHERE pu.username = ua.username)
  UNION ALL
  SELECT
    aa.username,
    COALESCE(NULLIF(ua.student_id, ''), '') AS student_id,
    'admin' AS role
  FROM admin_accounts aa
  LEFT JOIN user_accounts ua ON ua.username = aa.username
  UNION ALL
  SELECT
    pu.username,
    COALESCE(NULLIF(ua.student_id, ''), '') AS student_id,
    'power_user' AS role
  FROM power_users pu
  LEFT JOIN user_accounts ua ON ua.username = pu.username
  WHERE NOT EXISTS (SELECT 1 FROM admin_accounts aa WHERE aa.username = pu.username)
)
SELECT
  uu.username,
  uu.student_id,
  uu.role,
  COALESCE(u.balance, 0) AS general_balance,
  COALESCE(u.carryover_balance, 0) AS carryover_balance,
  COALESCE(ea.exclusive_balance, 0) AS exclusive_balance,
  COALESCE(u.balance, 0) + COALESCE(u.carryover_balance, 0) + COALESCE(ea.exclusive_balance, 0) AS total_balance,
  COALESCE(u.status, 'normal') AS status
FROM union_users uu
LEFT JOIN users u ON u.username=uu.username
LEFT JOIN exclusive_agg ea ON ea.username=uu.username
WHERE ($1='' OR uu.username ILIKE '%' || $1 || '%' OR uu.student_id ILIKE '%' || $1 || '%')
ORDER BY
  CASE uu.role WHEN 'admin' THEN 0 WHEN 'power_user' THEN 1 ELSE 2 END,
  uu.username
LIMIT $2`, keyword, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PointsUser, 0)
	for rows.Next() {
		var v PointsUser
		if err := rows.Scan(&v.Username, &v.StudentID, &v.Role, &v.GeneralBalance, &v.CarryoverBalance, &v.ExclusiveBalance, &v.TotalBalance, &v.Status); err != nil {
			return nil, err
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
		if err := rows.Scan(&v.Username, &v.StudentID, &v.Balance, &v.CarryoverBalance, &v.Status); err != nil {
			return nil, err
		}
		v.GeneralBalance = v.Balance
		v.TotalBalance = v.Balance + v.CarryoverBalance
		out = append(out, v)
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
VALUES($1, NOW(), $2, $3, $4, $5)
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
    ur.gpu_usage,
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
),
agg AS (
SELECT platform_username,
       COUNT(1) AS usage_records,
       COALESCE(SUM(CASE WHEN jsonb_array_length(gpu_usage) > 0 THEN 1 ELSE 0 END), 0) AS gpu_records,
       COALESCE(SUM(CASE WHEN jsonb_array_length(gpu_usage) = 0 THEN 1 ELSE 0 END), 0) AS cpu_records,
       COALESCE(SUM(cpu_percent), 0) AS total_cpu_percent,
       COALESCE(SUM(memory_mb), 0) AS total_memory_mb,
       COALESCE(SUM(cost), 0) AS total_cost
FROM resolved
GROUP BY platform_username
)
SELECT pu.username AS platform_username,
       COALESCE(a.usage_records, 0) AS usage_records,
       COALESCE(a.gpu_records, 0) AS gpu_records,
       COALESCE(a.cpu_records, 0) AS cpu_records,
       COALESCE(a.total_cpu_percent, 0) AS total_cpu_percent,
       COALESCE(a.total_memory_mb, 0) AS total_memory_mb,
       COALESCE(a.total_cost, 0) AS total_cost
FROM platform_users pu
LEFT JOIN agg a ON a.platform_username = pu.username
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
			&x.PlatformUsername, &x.UsageRecords, &x.GPUProcessCount, &x.CPUProcessCount,
			&x.TotalCPUPercent, &x.TotalMemoryMB, &x.TotalCost,
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
			&x.UsageRecords, &x.TotalCPUPercent, &x.TotalMemoryMB, &x.TotalCost, &x.LastUsageAt,
		); err != nil {
			return nil, err
		}
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
    WHEN method IN ('points_batch_plus', 'points_batch_minus', 'points_batch_grant') THEN '全部用户'
    ELSE username
  END AS target_account,
  amount,
  method,
  COALESCE(points_scope, 'general'),
  COALESCE(node_id, ''),
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
		if err := rows.Scan(&x.RechargeID, &x.Username, &x.TargetAccount, &x.Amount, &x.Method, &x.PointsScope, &x.NodeID, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) queryUsageRows(
	ctx context.Context,
	username string,
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

	if strings.TrimSpace(username) != "" {
		conds = append(conds, "username="+argN(username))
	}
	if hasFrom {
		conds = append(conds, "timestamp>="+argN(from))
	}
	if hasTo {
		conds = append(conds, "timestamp<="+argN(to))
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	query := fmt.Sprintf(`
SELECT node_id, username, timestamp, cpu_percent, memory_mb, gpu_usage::text, cost
     , local_username
FROM usage_records
%s
ORDER BY timestamp ASC
LIMIT %s
`, where, argN(limit))

	return s.db.QueryContext(ctx, query, args...)
}
