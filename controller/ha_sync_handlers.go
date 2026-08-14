package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var errHASyncRunning = errors.New("ha_sync_running")

func normalizeHASyncDirection(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "", "primary_to_standby", "p2s", "primary2standby":
		return "primary_to_standby"
	case "standby_to_primary", "s2p", "standby2primary":
		return "standby_to_primary"
	default:
		return ""
	}
}

func (s *Server) isHASyncRunning() bool {
	s.haSyncMu.Lock()
	defer s.haSyncMu.Unlock()
	return s.haSyncRunning
}

func (s *Server) withHASyncLock() error {
	s.haSyncMu.Lock()
	defer s.haSyncMu.Unlock()
	if s.haSyncRunning {
		return errHASyncRunning
	}
	s.haSyncRunning = true
	return nil
}

func (s *Server) releaseHASyncLock() {
	s.haSyncMu.Lock()
	s.haSyncRunning = false
	s.haSyncMu.Unlock()
}

func bool01(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func parseHASyncSteps(raw string) []HASyncStep {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]HASyncStep, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "STEP_RESULT|") {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		state := strings.TrimSpace(strings.ToLower(parts[2]))
		msg := strings.TrimSpace(parts[3])
		if name == "" {
			name = "unknown"
		}
		out = append(out, HASyncStep{
			Name:    name,
			Success: state == "ok" || state == "success" || state == "true",
			Message: msg,
		})
	}
	return out
}

func tailText(v string, maxLen int) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if maxLen <= 0 || len(v) <= maxLen {
		return v
	}
	return v[len(v)-maxLen:]
}

func (s *Server) executeHASyncRun(ctx context.Context, cfg HASyncConfig, direction string) (status string, summary string, detail []HASyncStep) {
	status = "failed"
	summary = "容灾同步执行失败"
	detail = []HASyncStep{}

	scriptPath := strings.TrimSpace(cfg.ScriptPath)
	if scriptPath == "" {
		detail = append(detail, HASyncStep{Name: "prepare", Success: false, Message: "script_path 未配置"})
		return
	}
	if _, err := os.Stat(scriptPath); err != nil {
		detail = append(detail, HASyncStep{Name: "prepare", Success: false, Message: "同步脚本不存在: " + err.Error()})
		return
	}
	direction = normalizeHASyncDirection(direction)
	if direction == "" {
		detail = append(detail, HASyncStep{Name: "prepare", Success: false, Message: "direction 不合法"})
		return
	}

	cmd := exec.CommandContext(ctx, "bash", scriptPath)
	rootDir := filepath.Clean(filepath.Join(filepath.Dir(scriptPath), ".."))
	cmd.Dir = rootDir
	cmd.Env = append(os.Environ(),
		"HA_SYNC_DIRECTION="+direction,
		"DR_NODE_ID="+strings.TrimSpace(cfg.DRNodeID),
		"DR_HOST="+strings.TrimSpace(cfg.DRHost),
		"DR_SSH_PORT="+strconv.Itoa(cfg.DRSSHPort),
		"DR_SSH_USER="+strings.TrimSpace(cfg.DRSSHUser),
		"DR_KEY_FILE="+strings.TrimSpace(cfg.DRKeyFile),
		"DR_CONTROLLER_PORT="+strconv.Itoa(cfg.DRControllerPort),
		"PRIMARY_HOST="+strings.TrimSpace(cfg.PrimaryHost),
		"PRIMARY_CONTROLLER_PORT="+strconv.Itoa(cfg.PrimaryControllerPort),
		"SYNC_WEB_DIST="+bool01(cfg.SyncWebDist),
		"SYNC_DATABASE="+bool01(cfg.SyncDatabase),
	)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	rawOut := buf.String()
	detail = parseHASyncSteps(rawOut)
	if len(detail) == 0 {
		if err != nil {
			detail = append(detail, HASyncStep{Name: "sync", Success: false, Message: tailText(rawOut, 500)})
		} else {
			detail = append(detail, HASyncStep{Name: "sync", Success: true, Message: "脚本执行成功"})
		}
	}

	okCount := 0
	for _, step := range detail {
		if step.Success {
			okCount++
		}
	}
	if err != nil {
		status = "failed"
		summary = fmt.Sprintf("容灾同步失败：%v（成功 %d/%d）", err, okCount, len(detail))
		return
	}
	status = "success"
	summary = fmt.Sprintf("容灾同步完成（成功 %d/%d）", okCount, len(detail))
	return
}

func (s *Server) startHASyncRun(triggerMode string, direction string, startedBy string) (int64, error) {
	if !s.cfg.HAEnabled || !strings.EqualFold(strings.TrimSpace(s.cfg.HARole), "primary") {
		return 0, errors.New("仅已启用 HA 的 primary 控制器可以编排同步")
	}
	loadCtx, loadCancel := context.WithTimeout(context.Background(), 8*time.Second)
	cfg, cfgErr := s.store.GetHASyncConfig(loadCtx, s.cfg)
	loadCancel()
	if cfgErr != nil {
		return 0, fmt.Errorf("读取 HA 同步配置失败: %w", cfgErr)
	}
	if err := validateHASyncConfigForRun(cfg); err != nil {
		return 0, err
	}
	if err := s.withHASyncLock(); err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	runID, err := s.store.InsertHASyncRun(ctx, triggerMode, direction, startedBy)
	if err != nil {
		s.releaseHASyncLock()
		return 0, err
	}

	go func() {
		defer s.releaseHASyncLock()
		runCtx, runCancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer runCancel()
		status, summary, detail := s.executeHASyncRun(runCtx, cfg, direction)
		if err := s.store.FinishHASyncRun(context.Background(), runID, status, summary, detail); err != nil {
			log.Printf("记录 HA 同步结果失败 run_id=%d err=%v", runID, err)
		}
	}()

	return runID, nil
}

func (s *Server) StartHASyncScheduler(ctx context.Context) {
	runIfDue := func() {
		if !s.cfg.HAEnabled || !strings.EqualFold(strings.TrimSpace(s.cfg.HARole), "primary") {
			return
		}
		runCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		cfg, err := s.store.GetHASyncConfig(runCtx, s.cfg)
		if err != nil {
			log.Printf("HA 定时同步：读取配置失败: %v", err)
			return
		}
		if !cfg.Enabled {
			return
		}
		now := nowInBeijing()
		if now.Hour() < cfg.StartHour {
			return
		}

		last, exists, err := s.store.GetLatestHASyncRun(runCtx, "scheduled")
		if err != nil {
			log.Printf("HA 定时同步：读取最近任务失败: %v", err)
			return
		}
		today := now.Format("2006-01-02")
		if exists && inBeijing(last.StartedAt).Format("2006-01-02") == today {
			return
		}
		if exists {
			lastDay := time.Date(last.StartedAt.Year(), last.StartedAt.Month(), last.StartedAt.Day(), 0, 0, 0, 0, beijingLocation)
			todayDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, beijingLocation)
			days := int(todayDay.Sub(lastDay) / (24 * time.Hour))
			if days < cfg.IntervalDays {
				return
			}
		}
		if _, err := s.startHASyncRun("scheduled", "primary_to_standby", "system"); err != nil {
			if errors.Is(err, errHASyncRunning) {
				return
			}
			log.Printf("HA 定时同步启动失败: %v", err)
		}
	}

	go func() {
		runIfDue()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runIfDue()
			}
		}
	}()
}

func (s *Server) calcHASyncNextRunAt(cfg HASyncConfig, runs []HASyncRun) time.Time {
	now := nowInBeijing()
	todayTarget := time.Date(now.Year(), now.Month(), now.Day(), cfg.StartHour, 0, 0, 0, beijingLocation)
	var lastScheduled *time.Time
	for _, run := range runs {
		if strings.EqualFold(strings.TrimSpace(run.TriggerMode), "scheduled") {
			t := inBeijing(run.StartedAt)
			lastScheduled = &t
			break
		}
	}
	if lastScheduled == nil {
		if now.Before(todayTarget) {
			return todayTarget
		}
		return now
	}
	next := time.Date(lastScheduled.Year(), lastScheduled.Month(), lastScheduled.Day(), cfg.StartHour, 0, 0, 0, beijingLocation).
		AddDate(0, 0, cfg.IntervalDays)
	if now.Before(next) {
		return next
	}
	if now.Before(todayTarget) {
		return todayTarget
	}
	return now
}

func (s *Server) handleAdminHASyncConfigGet(c *gin.Context) {
	cfg, err := s.store.GetHASyncConfig(c.Request.Context(), s.cfg)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	runs, err := s.store.ListHASyncRuns(c.Request.Context(), parseLimit(c.Query("limit"), 20, 100))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	var lastRun *HASyncRun
	if len(runs) > 0 {
		tmp := runs[0]
		lastRun = &tmp
	}
	nextRunAt := s.calcHASyncNextRunAt(cfg, runs)
	c.JSON(200, gin.H{
		"config":      cfg,
		"runs":        runs,
		"running":     s.isHASyncRunning(),
		"last_run":    lastRun,
		"next_run_at": formatRFC3339InBeijing(nextRunAt),
	})
}

func (s *Server) handleAdminHASyncConfigSet(c *gin.Context) {
	var req HASyncConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.UpsertHASyncConfig(c.Request.Context(), req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cfg, err := s.store.GetHASyncConfig(c.Request.Context(), s.cfg)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "config": cfg})
}

type adminHASyncNowReq struct {
	Direction   string `json:"direction"`
	TriggerMode string `json:"trigger_mode"`
}

func (s *Server) handleAdminHASyncNow(c *gin.Context) {
	var req adminHASyncNowReq
	_ = c.ShouldBindJSON(&req)
	direction := normalizeHASyncDirection(req.Direction)
	if direction == "" {
		c.JSON(400, gin.H{"error": "direction 仅支持 primary_to_standby / standby_to_primary"})
		return
	}
	triggerMode := strings.TrimSpace(strings.ToLower(req.TriggerMode))
	if triggerMode == "" {
		triggerMode = "manual"
	}
	operator := s.currentOperator(c)
	runID, err := s.startHASyncRun(triggerMode, direction, operator)
	if err != nil {
		if errors.Is(err, errHASyncRunning) {
			c.JSON(409, gin.H{"error": "已有 HA 同步任务在运行，请稍后再试"})
			return
		}
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "run_id": runID, "message": "HA 同步任务已启动"})
}

func (s *Server) handleAdminHASyncRuns(c *gin.Context) {
	runs, err := s.store.ListHASyncRuns(c.Request.Context(), parseLimit(c.Query("limit"), 50, 200))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"runs": runs, "running": s.isHASyncRunning()})
}

func runLocalSystemctl(ctx context.Context, service string) (HASyncStep, error) {
	service = strings.TrimSpace(service)
	if service == "" {
		return HASyncStep{Name: "start_service", Success: false, Message: "service 不能为空"}, errors.New("service 不能为空")
	}
	cmd := exec.CommandContext(ctx, "systemctl", "start", service)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(out.String())
		if msg == "" {
			msg = err.Error()
		}
		return HASyncStep{Name: "start_" + service, Success: false, Message: msg}, err
	}
	return HASyncStep{Name: "start_" + service, Success: true, Message: "已启动 " + service}, nil
}

func (s *Server) handleAdminHAFailoverActivate(c *gin.Context) {
	if !s.cfg.HAEnabled || !strings.EqualFold(strings.TrimSpace(s.cfg.HARole), "standby") {
		c.JSON(409, gin.H{"error": "仅 standby 控制器允许执行接管，primary 上已禁止该操作"})
		return
	}
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Confirm != "ACTIVATE_STANDBY" {
		c.JSON(400, gin.H{"error": "需要明确确认 ACTIVATE_STANDBY"})
		return
	}
	cfg, err := s.store.GetHASyncConfig(c.Request.Context(), s.cfg)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if !cfg.AutoFailover {
		c.JSON(409, gin.H{"error": "接管策略未启用"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	step, err := runLocalSystemctl(ctx, "keepalived")
	steps := []HASyncStep{step}
	if err != nil {
		c.JSON(500, gin.H{"error": "启动 Keepalived 失败，未执行接管", "steps": steps})
		return
	}
	c.JSON(200, gin.H{"ok": true, "message": "Standby 接管服务已启动", "steps": steps})
}
