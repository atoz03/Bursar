package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const cgroupV1MemoryUnlimited = int64(9223372036854771712)
const memoryLimitSystemdDropIn = "90-gpuops-memory-limit.conf"

func (a *NodeAgent) setUserMemoryLimit(ctx context.Context, username string, limitGB float64, reason string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if limitGB < 0 {
		return fmt.Errorf("memory_limit_gb 不能为负数（0 表示解除限制），实际=%v", limitGB)
	}

	uid, err := lookupUID(ctx, username)
	if err != nil {
		return err
	}

	if isSystemd() && hasCommand("systemctl") {
		if err := setMemoryLimitBySystemd(ctx, uid, limitGB); err == nil {
			if err := reinforceMemoryLimitByCgroup(uid, username, limitGB); err != nil {
				a.logger.Printf("systemd MemoryMax 已设置，但 cgroup 直接收口失败：user=%s uid=%d err=%v", username, uid, err)
			}
			a.logger.Printf("执行 set_memory_limit：user=%s uid=%d memory_limit_gb=%.2f (systemd)", username, uid, limitGB)
			return a.writeMemoryLimitState(username, limitGB, reason)
		} else {
			a.logger.Printf("systemd MemoryMax 设置失败，将尝试 cgroup：user=%s uid=%d err=%v", username, uid, err)
		}
	}

	if err := reinforceMemoryLimitByCgroup(uid, username, limitGB); err == nil {
		a.logger.Printf("执行 set_memory_limit：user=%s uid=%d memory_limit_gb=%.2f (cgroupv2)", username, uid, limitGB)
		return a.writeMemoryLimitState(username, limitGB, reason)
	}

	if err := setMemoryLimitByCgroupV1(username, uid, limitGB); err == nil {
		a.logger.Printf("执行 set_memory_limit：user=%s uid=%d memory_limit_gb=%.2f (cgroupv1)", username, uid, limitGB)
		return a.writeMemoryLimitState(username, limitGB, reason)
	}

	return fmt.Errorf("内存限额失败：需要 systemd MemoryMax 或 cgroup v2/v1（uid=%d）", uid)
}

func reinforceMemoryLimitByCgroup(uid int, username string, limitGB float64) error {
	if err := setMemoryLimitByCgroupV2(uid, limitGB); err != nil {
		return err
	}
	if err := moveUserProcsToCgroupV2(uid, username); err != nil {
		return fmt.Errorf("迁移用户进程到 memory cgroup 失败：%w", err)
	}
	return nil
}

func (a *NodeAgent) writeMemoryLimitState(username string, limitGB float64, reason string) error {
	homeDir := filepath.Join("/home", username)
	path := filepath.Join(homeDir, ".memory_limit")
	content := fmt.Sprintf("memory_limit_gb=%.2f\nreason=%s\n", limitGB, strings.TrimSpace(reason))
	_ = os.MkdirAll(homeDir, 0755)
	return os.WriteFile(path, []byte(content), 0644)
}

func memoryLimitBytes(limitGB float64) int64 {
	if limitGB <= 0 {
		return 0
	}
	v := int64(math.Round(limitGB * 1024 * 1024 * 1024))
	if v < 64*1024*1024 {
		v = 64 * 1024 * 1024
	}
	return v
}

func setMemoryLimitBySystemd(ctx context.Context, uid int, limitGB float64) error {
	if err := writeMemoryLimitSystemdDropIn(uid, limitGB); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload 失败：%w（out=%s）", err, strings.TrimSpace(string(out)))
	}

	props := []string{"MemoryAccounting=yes", "MemoryHigh=infinity", "MemoryMax=infinity"}
	if limitGB > 0 {
		props = []string{
			"MemoryAccounting=yes",
			fmt.Sprintf("MemoryHigh=%d", memoryLimitBytes(limitGB)),
			fmt.Sprintf("MemoryMax=%d", memoryLimitBytes(limitGB)),
		}
	}
	units := []string{
		fmt.Sprintf("user-%d.slice", uid),
		fmt.Sprintf("user@%d.service", uid),
	}
	applied := false
	var runtimeErr error
	for _, unit := range units {
		args := append([]string{"set-property", "--runtime", unit}, props...)
		cmd := exec.CommandContext(ctx, "systemctl", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			if limitGB > 0 && strings.Contains(strings.ToLower(string(out)), "unknown assignment") {
				fallbackArgs := []string{"set-property", "--runtime", unit, "MemoryAccounting=yes", fmt.Sprintf("MemoryMax=%d", memoryLimitBytes(limitGB))}
				fallbackCmd := exec.CommandContext(ctx, "systemctl", fallbackArgs...)
				fallbackOut, fallbackErr := fallbackCmd.CombinedOutput()
				if fallbackErr == nil {
					applied = true
					continue
				}
				out = fallbackOut
				err = fallbackErr
			}
			if isUnitNotLoadedError(err, out) {
				continue
			}
			if runtimeErr == nil {
				runtimeErr = fmt.Errorf("systemctl set-property MemoryMax 失败：unit=%s err=%w（out=%s）", unit, err, strings.TrimSpace(string(out)))
			}
			continue
		}
		applied = true
	}
	if runtimeErr != nil && !applied {
		return runtimeErr
	}
	// user-<uid>.slice 未激活时，drop-in 会在下次会话自动生效。
	return nil
}

func writeMemoryLimitSystemdDropIn(uid int, limitGB float64) error {
	dropDir := filepath.Join("/etc/systemd/system", fmt.Sprintf("user-%d.slice.d", uid))
	dropFile := filepath.Join(dropDir, memoryLimitSystemdDropIn)
	if limitGB <= 0 {
		if err := os.Remove(dropFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除内存限额 drop-in 失败：%w", err)
		}
		_ = os.Remove(dropDir)
		return nil
	}
	if err := os.MkdirAll(dropDir, 0755); err != nil {
		return fmt.Errorf("创建内存限额 drop-in 目录失败：%w", err)
	}
	content := fmt.Sprintf("[Slice]\nMemoryAccounting=true\nMemoryMax=%d\n", memoryLimitBytes(limitGB))
	if err := os.WriteFile(dropFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入内存限额 drop-in 失败：%w", err)
	}
	return nil
}

func isUnitNotLoadedError(execErr error, out []byte) bool {
	msg := strings.ToLower(strings.TrimSpace(string(out)))
	if execErr != nil {
		msg = msg + " " + strings.ToLower(execErr.Error())
	}
	return strings.Contains(msg, "not loaded") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "no such file")
}

func setMemoryLimitByCgroupV2(uid int, limitGB float64) error {
	value := "max"
	highValue := "max"
	swapValue := "max"
	oomGroupValue := "0"
	if limitGB > 0 {
		value = fmt.Sprintf("%d", memoryLimitBytes(limitGB))
		highValue = value
		swapValue = "0"
		// cgroup v2 默认是“按进程”OOM；开启 group 模式后会按 cgroup 整体回收，
		// 避免只杀掉部分进程导致后台任务残留。
		oomGroupValue = "1"
	}
	dirs := []string{
		fmt.Sprintf("/sys/fs/cgroup/user.slice/user-%d.slice", uid),
		fmt.Sprintf("/sys/fs/cgroup/user-%d.slice", uid),
	}
	var lastErr error
	for _, dir := range dirs {
		maxPath := filepath.Join(dir, "memory.max")
		if err := os.WriteFile(maxPath, []byte(value), 0644); err == nil {
			_ = writeOptionalCgroupValue(filepath.Join(dir, "memory.high"), highValue)
			_ = writeOptionalCgroupValue(filepath.Join(dir, "memory.swap.max"), swapValue)
			_ = writeOptionalCgroupValue(filepath.Join(dir, "memory.oom.group"), oomGroupValue)
			return nil
		} else {
			lastErr = fmt.Errorf("写 %s 失败：%w", maxPath, err)
		}
	}
	if lastErr != nil {
		return fmt.Errorf("未找到可写 memory.max（uid=%d）：%w", uid, lastErr)
	}
	return fmt.Errorf("未找到可写 memory.max（uid=%d）", uid)
}

func writeOptionalCgroupValue(path string, value string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(path, []byte(value), 0644)
}

func (a *NodeAgent) reconcilePersistedMemoryLimits(ctx context.Context) {
	entries, err := os.ReadDir("/home")
	if err != nil {
		a.logger.Printf("扫描 /home 恢复内存限额失败：%v", err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		username := strings.TrimSpace(entry.Name())
		if username == "" {
			continue
		}
		limitGB, ok, err := loadPersistedMemoryLimit(username)
		if err != nil {
			a.logger.Printf("读取持久化内存限额失败：user=%s err=%v", username, err)
			continue
		}
		if !ok {
			continue
		}
		restoreCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = a.setUserMemoryLimit(restoreCtx, username, limitGB, "agent 启动恢复持久化内存限额")
		cancel()
		if err != nil {
			a.logger.Printf("恢复持久化内存限额失败：user=%s memory_limit_gb=%.2f err=%v", username, limitGB, err)
			continue
		}
		a.logger.Printf("恢复持久化内存限额：user=%s memory_limit_gb=%.2f", username, limitGB)
	}
}

func loadPersistedMemoryLimit(username string) (float64, bool, error) {
	path := filepath.Join("/home", username, ".memory_limit")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "memory_limit_gb=") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "memory_limit_gb="))
		if raw == "" {
			return 0, false, fmt.Errorf("memory_limit_gb 为空")
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0, false, fmt.Errorf("解析 memory_limit_gb 失败：%w", err)
		}
		if v < 0 {
			v = 0
		}
		return v, true, nil
	}
	return 0, false, nil
}

func setMemoryLimitByCgroupV1(username string, uid int, limitGB float64) error {
	mount, err := findCgroupV1MountPoint("memory")
	if err != nil {
		return err
	}
	groupDir := filepath.Join(mount, "gpuops", fmt.Sprintf("user-%d", uid))
	if err := os.MkdirAll(groupDir, 0755); err != nil {
		return fmt.Errorf("创建 cgroup v1 memory 目录失败：%w", err)
	}
	limit := cgroupV1MemoryUnlimited
	if limitGB > 0 {
		limit = memoryLimitBytes(limitGB)
	}
	if err := os.WriteFile(filepath.Join(groupDir, "memory.limit_in_bytes"), []byte(fmt.Sprintf("%d", limit)), 0644); err != nil {
		return fmt.Errorf("写 memory.limit_in_bytes 失败：%w", err)
	}
	return moveUserProcsToTasks(username, filepath.Join(groupDir, "tasks"))
}
