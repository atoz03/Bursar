package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var systemdSystemDir = filepath.FromSlash("/etc/systemd/system")
var execCommandContext = exec.CommandContext

type persistedCPUQuotaRestore struct {
	username string
	uid      int
	percent  float64
}

type persistedMemoryLimitRestore struct {
	username string
	uid      int
	limitGB  float64
}

func daemonReloadSystemd(ctx context.Context) error {
	out, err := execCommandContext(ctx, "systemctl", "daemon-reload").CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl daemon-reload 失败：%w（out=%s）", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeFileIfChanged(path string, content []byte, perm os.FileMode) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, content) {
			return false, nil
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(path, content, perm); err != nil {
		return false, err
	}
	return true, nil
}

func removeFileIfExists(path string) (bool, error) {
	err := os.Remove(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func restoreCPUQuotaByCgroup(uid int, username string, percent float64) error {
	if err := setCPUQuotaByCgroupV2(uid, percent); err == nil {
		_ = moveUserProcsToCgroupV2(uid, username)
		return nil
	}
	if err := setCPUQuotaByCgroupV1(uid, username, percent); err == nil {
		return nil
	}
	return fmt.Errorf("CPU 限流失败：需要 systemd CPUQuota 或 cgroup v2/cgroup v1（uid=%d）", uid)
}

func applyMemoryLimitByCgroupFallback(uid int, username string, limitGB float64) error {
	if err := reinforceMemoryLimitByCgroup(uid, username, limitGB); err == nil {
		return nil
	}
	if err := setMemoryLimitByCgroupV1(username, uid, limitGB); err == nil {
		return nil
	}
	return fmt.Errorf("内存限额失败：需要 systemd MemoryMax 或 cgroup v2/v1（uid=%d）", uid)
}

func (a *NodeAgent) reconcilePersistedResourceLimits(ctx context.Context) {
	if !isSystemd() || !hasCommand("systemctl") {
		a.reconcilePersistedCPUQuotas(ctx)
		a.reconcilePersistedMemoryLimits(ctx)
		return
	}

	entries, err := os.ReadDir("/home")
	if err != nil {
		a.logger.Printf("扫描 /home 恢复资源限制失败：%v", err)
		return
	}

	cpuEntries := make([]persistedCPUQuotaRestore, 0, len(entries))
	memoryEntries := make([]persistedMemoryLimitRestore, 0, len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		username := strings.TrimSpace(entry.Name())
		if username == "" {
			continue
		}

		quotaPercent, hasCPU, cpuErr := loadPersistedCPUQuota(username)
		if cpuErr != nil {
			a.logger.Printf("读取持久化 CPU 限速失败：user=%s err=%v", username, cpuErr)
		}
		limitGB, hasMemory, memoryErr := loadPersistedMemoryLimit(username)
		if memoryErr != nil {
			a.logger.Printf("读取持久化内存限额失败：user=%s err=%v", username, memoryErr)
		}
		if !hasCPU && !hasMemory {
			continue
		}

		uid, err := lookupUID(ctx, username)
		if err != nil {
			if hasCPU {
				a.logger.Printf("恢复持久化 CPU 限速失败：user=%s err=%v", username, err)
			}
			if hasMemory {
				a.logger.Printf("恢复持久化内存限额失败：user=%s err=%v", username, err)
			}
			continue
		}

		if hasCPU {
			cpuEntries = append(cpuEntries, persistedCPUQuotaRestore{
				username: username,
				uid:      uid,
				percent:  quotaPercent,
			})
		}
		if hasMemory {
			memoryEntries = append(memoryEntries, persistedMemoryLimitRestore{
				username: username,
				uid:      uid,
				limitGB:  limitGB,
			})
		}
	}

	cpuWriteFallback := make(map[string]struct{}, len(cpuEntries))
	memoryWriteFallback := make(map[string]struct{}, len(memoryEntries))
	dropInChanged := false

	for _, entry := range cpuEntries {
		changed, err := writeCPUQuotaSystemdDropIn(entry.uid, entry.percent)
		if err != nil {
			a.logger.Printf("批量写入 CPU 限速 drop-in 失败：user=%s uid=%d cpu_quota_percent=%.2f err=%v", entry.username, entry.uid, entry.percent, err)
			cpuWriteFallback[entry.username] = struct{}{}
			continue
		}
		dropInChanged = dropInChanged || changed
	}

	for _, entry := range memoryEntries {
		changed, err := writeMemoryLimitSystemdDropIn(entry.uid, entry.limitGB)
		if err != nil {
			a.logger.Printf("批量写入内存限额 drop-in 失败：user=%s uid=%d memory_limit_gb=%.2f err=%v", entry.username, entry.uid, entry.limitGB, err)
			memoryWriteFallback[entry.username] = struct{}{}
			continue
		}
		dropInChanged = dropInChanged || changed
	}

	systemdRuntimeReady := true
	if dropInChanged {
		if err := daemonReloadSystemd(ctx); err != nil {
			a.logger.Printf("批量恢复持久化资源限制时 daemon-reload 失败，将改用 cgroup 兜底：%v", err)
			systemdRuntimeReady = false
		}
	}

	for _, entry := range cpuEntries {
		_, useFallback := cpuWriteFallback[entry.username]
		if !useFallback && systemdRuntimeReady {
			if err := applyCPUQuotaSystemdRuntime(ctx, entry.uid, entry.percent); err == nil {
				a.logger.Printf("恢复持久化 CPU 限速：user=%s cpu_quota_percent=%.2f", entry.username, entry.percent)
				continue
			} else {
				a.logger.Printf("批量恢复 CPU 限速 runtime 失败，将尝试 cgroup：user=%s uid=%d cpu_quota_percent=%.2f err=%v", entry.username, entry.uid, entry.percent, err)
			}
		}

		if err := restoreCPUQuotaByCgroup(entry.uid, entry.username, entry.percent); err != nil {
			a.logger.Printf("恢复持久化 CPU 限速失败：user=%s cpu_quota_percent=%.2f err=%v", entry.username, entry.percent, err)
			continue
		}
		a.logger.Printf("恢复持久化 CPU 限速：user=%s cpu_quota_percent=%.2f (cgroup)", entry.username, entry.percent)
	}

	for _, entry := range memoryEntries {
		_, useFallback := memoryWriteFallback[entry.username]
		if !useFallback && systemdRuntimeReady {
			if err := applyMemoryLimitSystemdRuntime(ctx, entry.uid, entry.limitGB); err == nil {
				if err := reinforceMemoryLimitByCgroup(entry.uid, entry.username, entry.limitGB); err != nil {
					a.logger.Printf("批量恢复内存限额：systemd MemoryMax 已设置，但 cgroup 直接收口失败：user=%s uid=%d err=%v", entry.username, entry.uid, err)
				}
				a.logger.Printf("恢复持久化内存限额：user=%s memory_limit_gb=%.2f", entry.username, entry.limitGB)
				continue
			} else {
				a.logger.Printf("批量恢复内存限额 runtime 失败，将尝试 cgroup：user=%s uid=%d memory_limit_gb=%.2f err=%v", entry.username, entry.uid, entry.limitGB, err)
			}
		}

		if err := applyMemoryLimitByCgroupFallback(entry.uid, entry.username, entry.limitGB); err != nil {
			a.logger.Printf("恢复持久化内存限额失败：user=%s memory_limit_gb=%.2f err=%v", entry.username, entry.limitGB, err)
			continue
		}
		a.logger.Printf("恢复持久化内存限额：user=%s memory_limit_gb=%.2f (cgroup)", entry.username, entry.limitGB)
	}
}
