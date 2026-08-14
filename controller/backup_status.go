package main

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type backupJobStatus struct {
	State          string   `json:"state"`
	StartedAt      string   `json:"started_at,omitempty"`
	FinishedAt     string   `json:"finished_at,omitempty"`
	SnapshotID     string   `json:"snapshot_id,omitempty"`
	DatabaseBytes  int64    `json:"database_bytes,omitempty"`
	IncludedPaths  []string `json:"included_paths,omitempty"`
	Message        string   `json:"message,omitempty"`
	LastSuccessAt  string   `json:"last_success_at,omitempty"`
	LastSnapshotID string   `json:"last_snapshot_id,omitempty"`
}

type backupCheck struct {
	Key     string `json:"key"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func readBackupJobStatus(path string) (backupJobStatus, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return backupJobStatus{State: "not_configured"}, false, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return backupJobStatus{State: "not_configured"}, false, nil
	}
	if err != nil {
		return backupJobStatus{State: "unreadable", Message: err.Error()}, true, err
	}
	var out backupJobStatus
	if err := json.Unmarshal(raw, &out); err != nil {
		return backupJobStatus{State: "invalid", Message: "状态文件格式错误"}, true, err
	}
	out.State = strings.TrimSpace(strings.ToLower(out.State))
	if out.State == "" {
		out.State = "unknown"
	}
	return out, true, nil
}

func parseBackupStatusTime(raw string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	return t, err == nil && !t.IsZero()
}

func backupFresh(status backupJobStatus, maxAge time.Duration, now time.Time) bool {
	if status.State != "success" {
		return false
	}
	raw := status.LastSuccessAt
	if strings.TrimSpace(raw) == "" {
		raw = status.FinishedAt
	}
	t, ok := parseBackupStatusTime(raw)
	return ok && !t.After(now.Add(5*time.Minute)) && now.Sub(t) <= maxAge
}

func (s *Server) handleAdminBackupStatus(c *gin.Context) {
	now := time.Now()
	backup, backupConfigured, backupErr := readBackupJobStatus(s.cfg.BackupStatusFile)
	verify, verifyConfigured, verifyErr := readBackupJobStatus(s.cfg.BackupVerifyStatusFile)
	backupOK := backupConfigured && backupErr == nil && backupFresh(backup, 36*time.Hour, now)
	verifyOK := verifyConfigured && verifyErr == nil && backupFresh(verify, 8*24*time.Hour, now)

	checks := []backupCheck{
		{Key: "backup_configured", OK: backupConfigured, Message: "尚未安装系统级备份任务"},
		{Key: "backup_fresh", OK: backupOK, Message: "最近 36 小时没有成功备份"},
		{Key: "restore_verified", OK: verifyOK, Message: "最近 8 天没有成功恢复演练"},
	}
	for i := range checks {
		if checks[i].OK {
			switch checks[i].Key {
			case "backup_configured":
				checks[i].Message = "备份任务已安装"
			case "backup_fresh":
				checks[i].Message = "最近备份有效"
			case "restore_verified":
				checks[i].Message = "最近恢复演练有效"
			}
		}
	}

	c.JSON(200, gin.H{
		"ready":        backupOK && verifyOK,
		"backup":       backup,
		"verification": verify,
		"checks":       checks,
		"checked_at":   now.Format(time.RFC3339),
	})
}
