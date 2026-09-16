package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"
)

const appSettingPointsWarningEmailThreshold = "points_warning_email_threshold"

func normalizePointsWarningEmailThreshold(threshold float64) float64 {
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 {
		return 0
	}
	return threshold
}

func parsePointsWarningEmailThreshold(raw string) (float64, error) {
	threshold, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 {
		return 0, errors.New("积分预警邮件阈值必须是大于等于 0 的有限数字")
	}
	return threshold, nil
}

func (s *Store) GetPointsWarningEmailThreshold(ctx context.Context, cfg Config) (float64, error) {
	fallback := normalizePointsWarningEmailThreshold(cfg.WarningThreshold)
	if s == nil || s.db == nil {
		return fallback, errors.New("database unavailable")
	}
	var raw string
	err := s.db.QueryRowContext(ctx, `
SELECT value
FROM app_settings
WHERE key=$1`, appSettingPointsWarningEmailThreshold).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return fallback, err
	}
	threshold, err := parsePointsWarningEmailThreshold(raw)
	if err != nil {
		return fallback, nil
	}
	return threshold, nil
}

func (s *Store) loadPointsWarningEmailThresholdTx(ctx context.Context, tx *sql.Tx, cfg Config) (float64, error) {
	fallback := normalizePointsWarningEmailThreshold(cfg.WarningThreshold)
	var raw string
	err := tx.QueryRowContext(ctx, `
SELECT value
FROM app_settings
WHERE key=$1`, appSettingPointsWarningEmailThreshold).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return fallback, err
	}
	threshold, err := parsePointsWarningEmailThreshold(raw)
	if err != nil {
		return fallback, nil
	}
	return threshold, nil
}

type pointsBalanceEmailAlertState struct {
	Armed bool
}

// advancePointsBalanceEmailAlertState 只在余额从阈值以上跨到阈值及以下时触发。
// 余额重新高于阈值后再次布防，避免每次节点上报都重复发送邮件。
func advancePointsBalanceEmailAlertState(
	prev pointsBalanceEmailAlertState,
	prevBalance float64,
	nextBalance float64,
	threshold float64,
) (pointsBalanceEmailAlertState, bool) {
	next := prev
	if threshold <= 0 {
		return next, false
	}
	if nextBalance > threshold {
		next.Armed = true
		return next, false
	}
	crossedDownward := prevBalance > threshold && nextBalance <= threshold
	if prevBalance == threshold && nextBalance < threshold {
		crossedDownward = true
	}
	if crossedDownward && prev.Armed {
		next.Armed = false
		return next, true
	}
	return next, false
}

// queuePointsBalanceEmailAlertTx 在积分余额变更事务内更新布防状态并创建待发送邮件。
// 余额口径与积分状态一致：通用积分 + 结转积分，不包含节点专属积分。
func (s *Store) queuePointsBalanceEmailAlertTx(
	ctx context.Context,
	tx *sql.Tx,
	username string,
	prevBalance float64,
	nextBalance float64,
	now time.Time,
	cfg Config,
) error {
	if s == nil || tx == nil {
		return nil
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}

	threshold, err := s.loadPointsWarningEmailThresholdTx(ctx, tx, cfg)
	if err != nil {
		return err
	}
	if threshold <= 0 {
		return nil
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO points_balance_email_alert_states(username, armed, updated_at)
VALUES($1, TRUE, $2)
ON CONFLICT (username) DO NOTHING`, username, now); err != nil {
		return err
	}

	var prev pointsBalanceEmailAlertState
	if err := tx.QueryRowContext(ctx, `
SELECT armed
FROM points_balance_email_alert_states
WHERE username=$1
FOR UPDATE`, username).Scan(&prev.Armed); err != nil {
		return err
	}
	next, shouldQueue := advancePointsBalanceEmailAlertState(prev, prevBalance, nextBalance, threshold)
	if shouldQueue {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO points_balance_email_alerts(username, threshold, balance, triggered_at, next_attempt_at)
VALUES($1, $2, $3, $4, $4)`, username, threshold, nextBalance, now); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `
UPDATE points_balance_email_alert_states
SET armed=$2, updated_at=$3
WHERE username=$1`, username, next.Armed, now)
	return err
}

type pointsBalanceEmailAlert struct {
	AlertID       int64
	Username      string
	Threshold     float64
	Balance       float64
	TriggeredAt   time.Time
	Status        string
	Attempts      int
	NextAttemptAt time.Time
	LockedAt      *time.Time
	SentAt        *time.Time
	LastError     string
}

func (s *Store) claimNextPointsBalanceEmailAlert(ctx context.Context, now time.Time) (pointsBalanceEmailAlert, bool, error) {
	if s == nil || s.db == nil {
		return pointsBalanceEmailAlert{}, false, errors.New("database unavailable")
	}
	if now.IsZero() {
		now = time.Now()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pointsBalanceEmailAlert{}, false, err
	}
	rollback := func() {
		_ = tx.Rollback()
	}

	var alert pointsBalanceEmailAlert
	var lockedAt sql.NullTime
	var sentAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
SELECT alert_id, username, threshold, balance, triggered_at, status, attempts,
       next_attempt_at, locked_at, sent_at, last_error
FROM points_balance_email_alerts
WHERE (
        status='pending' AND next_attempt_at <= $1
      )
   OR (
        status='sending' AND locked_at IS NOT NULL AND locked_at <= $1 - INTERVAL '10 minutes'
      )
ORDER BY triggered_at ASC, alert_id ASC
LIMIT 1
FOR UPDATE SKIP LOCKED`, now).Scan(
		&alert.AlertID,
		&alert.Username,
		&alert.Threshold,
		&alert.Balance,
		&alert.TriggeredAt,
		&alert.Status,
		&alert.Attempts,
		&alert.NextAttemptAt,
		&lockedAt,
		&sentAt,
		&alert.LastError,
	)
	if errors.Is(err, sql.ErrNoRows) {
		rollback()
		return pointsBalanceEmailAlert{}, false, nil
	}
	if err != nil {
		rollback()
		return pointsBalanceEmailAlert{}, false, err
	}
	if lockedAt.Valid {
		v := lockedAt.Time
		alert.LockedAt = &v
	}
	if sentAt.Valid {
		v := sentAt.Time
		alert.SentAt = &v
	}

	alert.Attempts++
	if _, err := tx.ExecContext(ctx, `
UPDATE points_balance_email_alerts
SET status='sending', attempts=$2, locked_at=$3, updated_at=$3
WHERE alert_id=$1`, alert.AlertID, alert.Attempts, now); err != nil {
		rollback()
		return pointsBalanceEmailAlert{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return pointsBalanceEmailAlert{}, false, err
	}
	alert.Status = "sending"
	alert.LockedAt = &now
	return alert, true, nil
}

func (s *Store) markPointsBalanceEmailAlertSent(ctx context.Context, alertID int64, sentAt time.Time) error {
	if sentAt.IsZero() {
		sentAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE points_balance_email_alerts
SET status='sent', sent_at=$2, locked_at=NULL, last_error='', updated_at=$2
WHERE alert_id=$1 AND status='sending'`, alertID, sentAt)
	return err
}

func (s *Store) markPointsBalanceEmailAlertFailed(ctx context.Context, alertID int64, nextAttemptAt time.Time, alertError string) error {
	if nextAttemptAt.IsZero() {
		nextAttemptAt = time.Now().Add(5 * time.Minute)
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE points_balance_email_alerts
SET status='pending', next_attempt_at=$2, locked_at=NULL, last_error=$3, updated_at=NOW()
WHERE alert_id=$1 AND status='sending'`, alertID, nextAttemptAt, strings.TrimSpace(alertError))
	return err
}

func pointsBalanceEmailAlertRetryDelay(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return time.Minute
	case attempts <= 2:
		return 5 * time.Minute
	case attempts <= 4:
		return 15 * time.Minute
	default:
		return 30 * time.Minute
	}
}

// isHAStandbyController 表示本控制器按配置处于 HA 备机角色。
func (s *Server) isHAStandbyController() bool {
	return s.cfg.HAEnabled && strings.EqualFold(strings.TrimSpace(s.cfg.HARole), "standby")
}

func (s *Server) deliverPendingPointsBalanceEmailAlerts(ctx context.Context) error {
	if s != nil && s.isHAStandbyController() {
		// 备机数据库是主控的完整副本，包含尚未发送或发送中的预警；由备机投递会重复发信。
		// 接管时按运维流程把 ha_role 改为 primary 并重启，积压的预警届时再发送。
		return nil
	}
	if s == nil || s.store == nil {
		return errors.New("database unavailable")
	}
	settings, err := s.store.GetMailSettings(ctx, s.cfg)
	if err != nil {
		return err
	}
	if settings.PointsWarningEmailThreshold <= 0 {
		return nil
	}

	for i := 0; i < 100; i++ {
		alert, ok, err := s.store.claimNextPointsBalanceEmailAlert(ctx, time.Now())
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}

		email, err := s.store.GetUserEmailByUsername(ctx, alert.Username)
		if err == nil {
			email = strings.TrimSpace(email)
			if email == "" {
				err = errors.New("该用户未配置邮箱")
			}
		} else if errors.Is(err, sql.ErrNoRows) {
			err = errors.New("未找到该用户邮箱")
		}
		if err == nil {
			platformName := s.platformName(ctx)
			subject := platformName + " 积分余额预警"
			body := fmt.Sprintf(
				"你好 %s，\n\n你的可用积分已降至 %.2f，达到预警阈值 %.2f。\n可用积分按通用积分与结转积分合计计算。请及时联系管理员补充积分，避免后续触发限速或资源使用受限。\n\n%s 团队",
				alert.Username,
				alert.Balance,
				alert.Threshold,
				platformName,
			)
			err = sendPlainTextMail(settings, email, subject, body)
		}
		if err == nil {
			if markErr := s.store.markPointsBalanceEmailAlertSent(ctx, alert.AlertID, time.Now()); markErr != nil {
				return markErr
			}
			continue
		}

		log.Printf("积分余额预警邮件发送失败：username=%s alert_id=%d attempt=%d err=%v", alert.Username, alert.AlertID, alert.Attempts, err)
		retryAt := time.Now().Add(pointsBalanceEmailAlertRetryDelay(alert.Attempts))
		if markErr := s.store.markPointsBalanceEmailAlertFailed(ctx, alert.AlertID, retryAt, err.Error()); markErr != nil {
			return markErr
		}
	}
	return nil
}

func (s *Server) StartPointsBalanceEmailAlertScheduler(ctx context.Context) {
	go func() {
		run := func() {
			jobCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			if err := s.deliverPendingPointsBalanceEmailAlerts(jobCtx); err != nil {
				log.Printf("积分余额预警邮件任务异常：%v", err)
			}
		}
		run()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
