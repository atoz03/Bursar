package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

const cpuQuotaSystemdDropIn = "90-gpuops-cpu-quota.conf"

func (a *NodeAgent) setUserCPUQuota(ctx context.Context, username string, percent float64, reason string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if percent < 0 || percent > 100 {
		return fmt.Errorf("cpu_quota_percent 必须在 [0,100]（0 表示解除限制），实际=%v", percent)
	}

	uid, err := lookupUID(ctx, username)
	if err != nil {
		return err
	}

	// 优先使用 systemd（最符合运维习惯）
	if isSystemd() && hasCommand("systemctl") {
		if err := setCPUQuotaBySystemd(ctx, uid, percent); err == nil {
			return a.writeCPUQuotaState(username, percent, reason)
		} else {
			a.logger.Printf("systemd CPUQuota 设置失败，将尝试 cgroup v2：user=%s uid=%d err=%v", username, uid, err)
		}
	}

	// cgroup v2 兜底（直接写 cpu.max）
	if err := setCPUQuotaByCgroupV2(uid, percent); err == nil {
		// 尝试把该用户进程归入目标 cgroup（仅当目录存在且可写时）
		_ = moveUserProcsToCgroupV2(uid, username)
		return a.writeCPUQuotaState(username, percent, reason)
	}

	// cgroup v1 兜底（cpu.cfs_*）：兼容无法升级到 cgroup v2 的老系统
	if err := setCPUQuotaByCgroupV1(uid, username, percent); err == nil {
		return a.writeCPUQuotaState(username, percent, reason)
	}

	return fmt.Errorf("CPU 限流失败：需要 systemd CPUQuota 或 cgroup v2/cgroup v1（uid=%d）", uid)
}

func (a *NodeAgent) writeCPUQuotaState(username string, percent float64, reason string) error {
	homeDir := filepath.Join("/home", username)
	path := filepath.Join(homeDir, ".cpu_quota")
	content := fmt.Sprintf("cpu_quota_percent=%.2f\nreason=%s\n", percent, strings.TrimSpace(reason))
	_ = os.MkdirAll(homeDir, 0755)
	return os.WriteFile(path, []byte(content), 0644)
}

func lookupUID(ctx context.Context, username string) (int, error) {
	cmd := exec.CommandContext(ctx, "id", "-u", username)
	b, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("获取 uid 失败：%w", err)
	}
	uidStr := strings.TrimSpace(string(b))
	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		return 0, fmt.Errorf("解析 uid 失败：%w", err)
	}
	return uid, nil
}

func isSystemd() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func setCPUQuotaBySystemd(ctx context.Context, uid int, percent float64) error {
	if err := writeCPUQuotaSystemdDropIn(uid, percent); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload 失败：%w（out=%s）", err, strings.TrimSpace(string(out)))
	}

	slice := fmt.Sprintf("user-%d.slice", uid)
	args := []string{"set-property", "--runtime", slice, "CPUAccounting=yes"}
	if percent <= 0 {
		// 解除限制：把属性清空即可恢复默认（无限制）
		args = append(args, "CPUQuota=")
	} else {
		args = append(args, fmt.Sprintf("CPUQuota=%.2f%%", percent))
	}
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if isUnitNotLoadedError(err, out) {
			// user-<uid>.slice 未激活时，drop-in 会在下次会话自动生效。
			return nil
		}
		return fmt.Errorf("systemctl set-property 失败：%w（out=%s）", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeCPUQuotaSystemdDropIn(uid int, percent float64) error {
	dropDir := filepath.Join("/etc/systemd/system", fmt.Sprintf("user-%d.slice.d", uid))
	dropFile := filepath.Join(dropDir, cpuQuotaSystemdDropIn)
	if percent <= 0 {
		if err := os.Remove(dropFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除 CPU 限速 drop-in 失败：%w", err)
		}
		_ = os.Remove(dropDir)
		return nil
	}
	if err := os.MkdirAll(dropDir, 0755); err != nil {
		return fmt.Errorf("创建 CPU 限速 drop-in 目录失败：%w", err)
	}
	content := fmt.Sprintf("[Slice]\nCPUAccounting=true\nCPUQuota=%.2f%%\n", percent)
	if err := os.WriteFile(dropFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入 CPU 限速 drop-in 失败：%w", err)
	}
	return nil
}

func setCPUQuotaByCgroupV2(uid int, percent float64) error {
	period := int64(100000) // 100ms
	value := ""
	if percent <= 0 {
		value = fmt.Sprintf("max %d", period)
	} else {
		quota := int64(float64(period) * (percent / 100.0))
		if quota < 1000 {
			quota = 1000
		}
		value = fmt.Sprintf("%d %d", quota, period)
	}

	paths := []string{
		fmt.Sprintf("/sys/fs/cgroup/user.slice/user-%d.slice/cpu.max", uid),
		fmt.Sprintf("/sys/fs/cgroup/user-%d.slice/cpu.max", uid),
	}
	for _, p := range paths {
		if err := os.WriteFile(p, []byte(value), 0644); err == nil {
			return nil
		}
	}
	return fmt.Errorf("未找到可写 cpu.max（uid=%d），请确认系统启用 cgroup v2 且 Agent 以 root 运行", uid)
}

func setCPUQuotaByCgroupV1(uid int, username string, percent float64) error {
	mount, err := findCgroupV1MountPoint("cpu")
	if err != nil {
		return err
	}

	groupDir := filepath.Join(mount, "gpuops", fmt.Sprintf("user-%d", uid))
	if err := os.MkdirAll(groupDir, 0755); err != nil {
		return fmt.Errorf("创建 cgroup v1 目录失败：%w", err)
	}

	period := int64(100000) // 100ms
	quota := int64(-1)      // -1 表示不限制
	if percent > 0 {
		quota = int64(float64(period) * (percent / 100.0))
		if quota < 1000 {
			quota = 1000
		}
	}

	if err := os.WriteFile(filepath.Join(groupDir, "cpu.cfs_period_us"), []byte(fmt.Sprintf("%d", period)), 0644); err != nil {
		return fmt.Errorf("写 cpu.cfs_period_us 失败：%w", err)
	}
	if err := os.WriteFile(filepath.Join(groupDir, "cpu.cfs_quota_us"), []byte(fmt.Sprintf("%d", quota)), 0644); err != nil {
		return fmt.Errorf("写 cpu.cfs_quota_us 失败：%w", err)
	}

	// 把用户进程移动进该 cgroup，才能生效
	return moveUserProcsToTasks(username, filepath.Join(groupDir, "tasks"))
}

func findCgroupV1MountPoint(controller string) (string, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return "", fmt.Errorf("无法读取 /proc/mounts：%w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		// 典型格式：cgroup /sys/fs/cgroup/cpu cgroup rw,nosuid,nodev,noexec,relatime,cpu 0 0
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		mountPoint := fields[1]
		fsType := fields[2]
		options := fields[3]
		if fsType != "cgroup" {
			continue
		}
		if optionHasController(options, controller) {
			return mountPoint, nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("扫描 /proc/mounts 失败：%w", err)
	}
	return "", fmt.Errorf("未找到 cgroup v1 %s 控制器挂载点", controller)
}

func optionHasController(options string, controller string) bool {
	for _, opt := range strings.Split(options, ",") {
		if opt == controller {
			return true
		}
	}
	return false
}

func moveUserProcsToTasks(username string, tasksPath string) error {
	if _, err := os.Stat(tasksPath); err != nil {
		return fmt.Errorf("tasks 文件不存在：%s", tasksPath)
	}

	procs, err := process.Processes()
	if err != nil {
		return fmt.Errorf("获取进程列表失败：%w", err)
	}

	f, err := os.OpenFile(tasksPath, os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开 tasks 失败：%w", err)
	}
	defer f.Close()

	for _, p := range procs {
		u, err := p.Username()
		if err != nil || u != username {
			continue
		}
		// 向 tasks 写入 PID 即可迁移（失败不致命）
		_, _ = f.WriteString(fmt.Sprintf("%d\n", p.Pid))
	}
	return nil
}

func moveUserProcsToCgroupV2(uid int, username string) error {
	paths := []string{
		fmt.Sprintf("/sys/fs/cgroup/user.slice/user-%d.slice/cgroup.procs", uid),
		fmt.Sprintf("/sys/fs/cgroup/user-%d.slice/cgroup.procs", uid),
	}
	var target string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			target = p
			break
		}
	}
	if target == "" {
		return nil
	}

	procs, err := process.Processes()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, p := range procs {
		u, err := p.Username()
		if err != nil || u != username {
			continue
		}
		_, _ = f.WriteString(fmt.Sprintf("%d\n", p.Pid))
	}
	return nil
}

func (a *NodeAgent) reconcilePersistedCPUQuotas(ctx context.Context) {
	entries, err := os.ReadDir("/home")
	if err != nil {
		a.logger.Printf("扫描 /home 恢复 CPU 限速失败：%v", err)
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
		quotaPercent, ok, err := loadPersistedCPUQuota(username)
		if err != nil {
			a.logger.Printf("读取持久化 CPU 限速失败：user=%s err=%v", username, err)
			continue
		}
		if !ok {
			continue
		}
		restoreCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = a.setUserCPUQuota(restoreCtx, username, quotaPercent, "agent 启动恢复持久化 CPU 限速")
		cancel()
		if err != nil {
			a.logger.Printf("恢复持久化 CPU 限速失败：user=%s cpu_quota_percent=%.2f err=%v", username, quotaPercent, err)
			continue
		}
		a.logger.Printf("恢复持久化 CPU 限速：user=%s cpu_quota_percent=%.2f", username, quotaPercent)
	}
}

func loadPersistedCPUQuota(username string) (float64, bool, error) {
	path := filepath.Join("/home", username, ".cpu_quota")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "cpu_quota_percent=") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "cpu_quota_percent="))
		if raw == "" {
			return 0, false, fmt.Errorf("cpu_quota_percent 为空")
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0, false, err
		}
		return v, true, nil
	}
	return 0, false, nil
}
