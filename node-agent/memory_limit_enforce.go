package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/shirou/gopsutil/v3/process"
)

type userMemorySnapshot struct {
	totalMB float64
	pids    []int32
}

// enforcePersistedMemoryLimits 按已下发的 .memory_limit 做“实际 RSS 累积”巡检。
// 当用户累计内存超限时，保留 SSH 会话相关进程，清理其余进程。
func (a *NodeAgent) enforcePersistedMemoryLimits(ctx context.Context) error {
	limits, err := loadPersistedPositiveMemoryLimits()
	if err != nil {
		return err
	}
	if len(limits) == 0 {
		return nil
	}

	snapshots, err := collectTrackedUserMemorySnapshots(ctx, limits)
	if err != nil {
		return err
	}

	for username, limitGB := range limits {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		snap := snapshots[username]
		limitMB := limitGB * 1024
		if snap.totalMB <= limitMB {
			continue
		}
		reason := fmt.Sprintf("累计内存 %.2fMB 超过限制 %.2fMB，保留 SSH 进程并清理其余进程", snap.totalMB, limitMB)
		killed, kept, killErr := a.killUserProcessesExceptSSH(ctx, username, snap.pids, reason)
		if killErr != nil {
			a.logger.Printf("内存超限清理失败：user=%s rss_mb=%.2f limit_mb=%.2f err=%v", username, snap.totalMB, limitMB, killErr)
			continue
		}
		if killed > 0 {
			a.logger.Printf("内存超限清理完成：user=%s rss_mb=%.2f limit_mb=%.2f killed=%d kept_ssh=%d", username, snap.totalMB, limitMB, killed, kept)
		}
	}
	return nil
}

func loadPersistedPositiveMemoryLimits() (map[string]float64, error) {
	entries, err := os.ReadDir("/home")
	if err != nil {
		return nil, fmt.Errorf("扫描 /home 读取内存限额失败：%w", err)
	}
	out := make(map[string]float64, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		username := strings.TrimSpace(entry.Name())
		if username == "" || username == "root" {
			continue
		}
		limitGB, ok, err := loadPersistedMemoryLimit(username)
		if err != nil {
			continue
		}
		if !ok || limitGB <= 0 {
			continue
		}
		out[username] = limitGB
	}
	return out, nil
}

func collectTrackedUserMemorySnapshots(ctx context.Context, tracked map[string]float64) (map[string]userMemorySnapshot, error) {
	out := make(map[string]userMemorySnapshot, len(tracked))
	procs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("获取进程列表失败：%w", err)
	}
	for _, proc := range procs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		pid := proc.Pid
		if pid <= 0 {
			continue
		}
		username, err := proc.Username()
		if err != nil {
			continue
		}
		username = strings.TrimSpace(username)
		if username == "" {
			continue
		}
		if _, ok := tracked[username]; !ok {
			continue
		}
		memInfo, err := proc.MemoryInfo()
		if err != nil || memInfo == nil {
			continue
		}
		snap := out[username]
		snap.totalMB += float64(memInfo.RSS) / 1024 / 1024
		snap.pids = append(snap.pids, pid)
		out[username] = snap
	}
	return out, nil
}

func (a *NodeAgent) killUserProcessesExceptSSH(ctx context.Context, username string, pids []int32, reason string) (int, int, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return 0, 0, errors.New("username 不能为空")
	}
	if username == "root" {
		return 0, 0, nil
	}
	killed := 0
	keptSSH := 0
	for _, pid := range pids {
		select {
		case <-ctx.Done():
			return killed, keptSSH, ctx.Err()
		default:
		}
		if pid <= 1 {
			continue
		}
		proc, err := process.NewProcess(pid)
		if err != nil {
			continue
		}
		procUser, err := proc.Username()
		if err != nil || strings.TrimSpace(procUser) != username {
			continue
		}
		name, _ := proc.Name()
		cmdline, _ := proc.Cmdline()
		if isSSHRelatedUserProcess(name, cmdline) {
			keptSSH++
			continue
		}
		if err := syscall.Kill(int(pid), syscall.SIGKILL); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				continue
			}
			return killed, keptSSH, fmt.Errorf("kill pid=%d 失败：%w", pid, err)
		}
		killed++
	}
	if killed > 0 {
		a.logger.Printf("执行 memory_over_limit_kill：user=%s killed=%d kept_ssh=%d reason=%s", username, killed, keptSSH, strings.TrimSpace(reason))
	}
	return killed, keptSSH, nil
}

func isSSHRelatedUserProcess(name string, cmdline string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	c := strings.ToLower(strings.TrimSpace(cmdline))
	if n == "sshd" || n == "ssh-agent" || n == "sftp-server" || n == "internal-sftp" {
		return true
	}
	return strings.Contains(c, "sshd:") ||
		strings.Contains(c, "/sshd") ||
		strings.Contains(c, "sftp-server")
}
