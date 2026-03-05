package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// setUserGPUAccessByCgroup 通过 cgroup devices 控制用户 GPU 设备访问。
// allow=false 时拒绝访问 /dev/nvidia* 对应主设备号；allow=true 时恢复访问。
// 当前优先使用 cgroup v1 devices（兼容性最好且规则可显式 allow/deny）。
func setUserGPUAccessByCgroup(ctx context.Context, username string, allow bool) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username 不能为空")
	}
	uid, err := lookupUID(ctx, username)
	if err != nil {
		return err
	}

	rules, err := detectNvidiaDeviceRules()
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return fmt.Errorf("未检测到 /dev/nvidia* 设备")
	}

	mount, err := findCgroupV1MountPoint("devices")
	if err != nil {
		return fmt.Errorf("未找到 cgroup devices 控制器：%w", err)
	}
	groupDir := filepath.Join(mount, "gpuops-gpu", fmt.Sprintf("user-%d", uid))
	if err := os.MkdirAll(groupDir, 0755); err != nil {
		return fmt.Errorf("创建 GPU cgroup 目录失败：%w", err)
	}

	controlFile := filepath.Join(groupDir, "devices.deny")
	if allow {
		controlFile = filepath.Join(groupDir, "devices.allow")
	}
	f, err := os.OpenFile(controlFile, os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开 %s 失败：%w", controlFile, err)
	}
	for _, rule := range rules {
		if _, werr := f.WriteString(rule + "\n"); werr != nil {
			_ = f.Close()
			return fmt.Errorf("写入 %s 失败：%w", controlFile, werr)
		}
	}
	_ = f.Close()

	// 将该用户当前进程迁入目标 cgroup，使限制即时生效。
	tasksPath := filepath.Join(groupDir, "tasks")
	if err := moveUserProcsToTasks(username, tasksPath); err != nil {
		return fmt.Errorf("迁移用户进程到 GPU cgroup 失败：%w", err)
	}
	return nil
}

func detectNvidiaDeviceRules() ([]string, error) {
	patterns := []string{
		"/dev/nvidia*",
		"/dev/nvidia-caps/*",
	}
	majorSet := make(map[uint32]struct{})

	for _, p := range patterns {
		matches, _ := filepath.Glob(p)
		for _, path := range matches {
			fi, err := os.Stat(path)
			if err != nil {
				continue
			}
			if fi.Mode()&os.ModeCharDevice == 0 {
				continue
			}
			st, ok := fi.Sys().(*syscall.Stat_t)
			if !ok {
				continue
			}
			major := uint32(unix.Major(uint64(st.Rdev)))
			if major == 0 {
				continue
			}
			majorSet[major] = struct{}{}
		}
	}

	if len(majorSet) == 0 {
		return nil, fmt.Errorf("未发现 NVIDIA 字符设备")
	}

	majors := make([]int, 0, len(majorSet))
	for m := range majorSet {
		majors = append(majors, int(m))
	}
	sort.Ints(majors)

	rules := make([]string, 0, len(majors))
	for _, m := range majors {
		rules = append(rules, "c "+strconv.Itoa(m)+":* rwm")
	}
	return rules, nil
}

func detectNvidiaDeviceNodes() []string {
	patterns := []string{
		"/dev/nvidia*",
		"/dev/nvidia-caps/*",
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, 16)
	for _, p := range patterns {
		matches, _ := filepath.Glob(p)
		for _, path := range matches {
			if _, ok := seen[path]; ok {
				continue
			}
			fi, err := os.Stat(path)
			if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
				continue
			}
			seen[path] = struct{}{}
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// setUserGPUAccessByACL 通过 POSIX ACL 拒绝/恢复用户访问 /dev/nvidia*。
// 该方式对“新启动进程”也有效，可作为 cgroup 失败时的兜底。
func setUserGPUAccessByACL(ctx context.Context, username string, allow bool) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username 不能为空")
	}
	if !hasCommand("setfacl") {
		return fmt.Errorf("未检测到 setfacl 命令")
	}
	nodes := detectNvidiaDeviceNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("未检测到 /dev/nvidia* 设备")
	}
	var firstErr error
	for _, node := range nodes {
		var cmd *exec.Cmd
		if allow {
			cmd = exec.CommandContext(ctx, "setfacl", "-x", "u:"+username, node)
		} else {
			cmd = exec.CommandContext(ctx, "setfacl", "-m", "u:"+username+":---", node)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("setfacl 失败 node=%s err=%w out=%s", node, err, strings.TrimSpace(string(out)))
			}
		}
	}
	return firstErr
}

// hasNvidiaDeviceNodes 仅用于诊断日志/调试。
func hasNvidiaDeviceNodes() bool {
	cmd := exec.Command("bash", "-lc", "ls /dev/nvidia* >/dev/null 2>&1")
	return cmd.Run() == nil
}
