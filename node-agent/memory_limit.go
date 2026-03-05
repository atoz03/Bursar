package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const cgroupV1MemoryUnlimited = int64(9223372036854771712)

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
			return a.writeMemoryLimitState(username, limitGB, reason)
		} else {
			a.logger.Printf("systemd MemoryMax 设置失败，将尝试 cgroup：user=%s uid=%d err=%v", username, uid, err)
		}
	}

	if err := setMemoryLimitByCgroupV2(uid, limitGB); err == nil {
		_ = moveUserProcsToCgroupV2(uid, username)
		return a.writeMemoryLimitState(username, limitGB, reason)
	}

	if err := setMemoryLimitByCgroupV1(username, uid, limitGB); err == nil {
		return a.writeMemoryLimitState(username, limitGB, reason)
	}

	return fmt.Errorf("内存限额失败：需要 systemd MemoryMax 或 cgroup v2/v1（uid=%d）", uid)
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
	slice := fmt.Sprintf("user-%d.slice", uid)
	var cmd *exec.Cmd
	if limitGB <= 0 {
		cmd = exec.CommandContext(ctx, "systemctl", "set-property", "--runtime", slice, "MemoryMax=")
	} else {
		cmd = exec.CommandContext(ctx, "systemctl", "set-property", "--runtime", slice, fmt.Sprintf("MemoryMax=%d", memoryLimitBytes(limitGB)))
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl set-property MemoryMax 失败：%w（out=%s）", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func setMemoryLimitByCgroupV2(uid int, limitGB float64) error {
	value := "max"
	if limitGB > 0 {
		value = fmt.Sprintf("%d", memoryLimitBytes(limitGB))
	}
	paths := []string{
		fmt.Sprintf("/sys/fs/cgroup/user.slice/user-%d.slice/memory.max", uid),
		fmt.Sprintf("/sys/fs/cgroup/user-%d.slice/memory.max", uid),
	}
	for _, p := range paths {
		if err := os.WriteFile(p, []byte(value), 0644); err == nil {
			return nil
		}
	}
	return fmt.Errorf("未找到可写 memory.max（uid=%d）", uid)
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
