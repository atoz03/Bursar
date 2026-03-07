package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

func loadSSHExemptUsers() map[string]struct{} {
	exempt := map[string]struct{}{
		"root": {},
	}
	path := filepath.FromSlash("/var/lib/gpu-cluster/exempt_users.txt")
	b, err := os.ReadFile(path)
	if err != nil {
		return exempt
	}
	for _, ln := range strings.Split(string(b), "\n") {
		u := strings.TrimSpace(ln)
		if u == "" {
			continue
		}
		exempt[u] = struct{}{}
	}
	return exempt
}

func (a *NodeAgent) ExecuteAction(ctx context.Context, action Action) error {
	switch action.Type {
	case "notify":
		return a.writeNotice(action.Username, action.Message)
	case "block_user":
		return a.blockUserGPUAccess(ctx, action.Username, action.Reason)
	case "unblock_user":
		return a.unblockUserGPUAccess(ctx, action.Username)
	case "set_cpu_quota":
		return a.setUserCPUQuota(ctx, action.Username, action.CPUQuotaPercent, action.Reason)
	case "set_memory_limit":
		return a.setUserMemoryLimit(ctx, action.Username, action.MemoryLimitGB, action.Reason)
	case "set_disk_quota":
		return a.setUserDiskQuota(ctx, action.Username, action.DiskQuotaMountpoint, action.DiskQuotaSoftMB, action.DiskQuotaHardMB, action.Reason)
	case "set_gpu_exclusive":
		return a.setGPUExclusivePolicy(ctx, action.GPUExclusiveEnabled, action.GPUExclusiveAssignments, action.Reason)
	case "set_gpu_visibility":
		return a.setUserGPUVisibility(ctx, action.Username, action.GPUIndices, action.Reason)
	case "kill_process":
		return a.killProcesses(ctx, action.Username, action.PIDs, action.Reason)
	case "kick_ssh_all":
		return a.kickAllSSH(ctx, action.Reason)
	case "kick_ssh_user":
		return a.kickSSHUser(ctx, action.Username, action.Reason)
	case "kill_all_processes":
		return a.killAllProcessesByUser(ctx, action.Username, action.Reason)
	case "kill_all_user_processes":
		return a.killAllUserProcesses(ctx, action.Reason)
	case "force_sync":
		a.triggerForceSync(action.Reason)
		return nil
	case "create_local_account":
		return a.createLocalAccount(ctx, action.Username, action.PublicKey, action.Reason)
	default:
		return fmt.Errorf("未知 action.type：%s", action.Type)
	}
}

var localUsernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

func (a *NodeAgent) createLocalAccount(ctx context.Context, username string, publicKey string, reason string) error {
	username = strings.TrimSpace(username)
	publicKey = strings.TrimSpace(publicKey)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if !localUsernamePattern.MatchString(username) {
		return fmt.Errorf("非法本地账号名：%s", username)
	}
	if publicKey == "" {
		return errors.New("public_key 不能为空")
	}
	if !strings.HasPrefix(publicKey, "ssh-ed25519 ") {
		return errors.New("public_key 格式不合法，仅支持 ssh-ed25519")
	}

	// 幂等：账号已存在时不重复 useradd，但仍会刷新 authorized_keys，确保密钥可更新。
	userExists := exec.CommandContext(ctx, "id", "-u", username).Run() == nil
	if !userExists {
		if out, err := exec.CommandContext(ctx, "useradd", "-m", "-s", "/bin/bash", username).CombinedOutput(); err != nil {
			// 兜底：竞态下可能已被其它流程创建。
			if checkErr := exec.CommandContext(ctx, "id", "-u", username).Run(); checkErr == nil {
				userExists = true
				log.Printf("执行 create_local_account：user=%s race-created reason=%s", username, strings.TrimSpace(reason))
			} else {
				return fmt.Errorf("useradd 失败：%w out=%s", err, strings.TrimSpace(string(out)))
			}
		}
	}

	sysUser, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("查询系统账号失败：%w", err)
	}
	uid, err := strconv.Atoi(sysUser.Uid)
	if err != nil {
		return fmt.Errorf("解析 uid 失败：%w", err)
	}
	gid, err := strconv.Atoi(sysUser.Gid)
	if err != nil {
		return fmt.Errorf("解析 gid 失败：%w", err)
	}
	homeDir := strings.TrimSpace(sysUser.HomeDir)
	if homeDir == "" {
		homeDir = filepath.Join("/home", username)
	}
	sshDir := filepath.Join(homeDir, ".ssh")
	authFile := filepath.Join(sshDir, "authorized_keys")

	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("创建 .ssh 目录失败：%w", err)
	}
	if err := os.Chown(sshDir, uid, gid); err != nil {
		return fmt.Errorf("设置 .ssh 目录属主失败：%w", err)
	}
	if err := os.Chmod(sshDir, 0700); err != nil {
		return fmt.Errorf("设置 .ssh 目录权限失败：%w", err)
	}
	if err := os.WriteFile(authFile, []byte(publicKey+"\n"), 0600); err != nil {
		return fmt.Errorf("写入 authorized_keys 失败：%w", err)
	}
	if err := os.Chown(authFile, uid, gid); err != nil {
		return fmt.Errorf("设置 authorized_keys 属主失败：%w", err)
	}
	if err := os.Chmod(authFile, 0600); err != nil {
		return fmt.Errorf("设置 authorized_keys 权限失败：%w", err)
	}

	if userExists {
		log.Printf("执行 create_local_account：user=%s already-exists key-updated home=%s reason=%s", username, homeDir, strings.TrimSpace(reason))
	} else {
		log.Printf("执行 create_local_account：user=%s created key-updated home=%s reason=%s", username, homeDir, strings.TrimSpace(reason))
	}
	a.invalidateLocalUsersAndQuotaCache()
	return nil
}

func (a *NodeAgent) killAllProcessesByUser(ctx context.Context, username string, reason string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if username == "root" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "pkill", "-KILL", "-u", username)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// pkill 返回 1 表示没有匹配进程，不视为失败。
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			log.Printf("执行 kill_all_processes：user=%s no-process reason=%s", username, strings.TrimSpace(reason))
			return nil
		}
		return fmt.Errorf("pkill 失败：%w out=%s", err, strings.TrimSpace(string(out)))
	}
	log.Printf("执行 kill_all_processes：user=%s reason=%s", username, strings.TrimSpace(reason))
	return nil
}

func (a *NodeAgent) killAllUserProcesses(ctx context.Context, reason string) error {
	exempt := loadSSHExemptUsers()
	users := loadHomeUsers()
	killedUsers := 0
	for _, username := range users {
		if _, ok := exempt[username]; ok {
			continue
		}
		if err := a.killAllProcessesByUser(ctx, username, reason); err == nil {
			killedUsers++
		}
	}
	log.Printf("执行 kill_all_user_processes：affected_users=%d reason=%s", killedUsers, strings.TrimSpace(reason))
	return nil
}

func (a *NodeAgent) kickAllSSH(ctx context.Context, reason string) error {
	out, err := exec.CommandContext(ctx, "who").Output()
	if err != nil {
		return err
	}
	lines := strings.Split(string(out), "\n")
	exclude := loadSSHExemptUsers()
	ttys := make(map[string]struct{})
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		fields := strings.Fields(ln)
		if len(fields) < 2 {
			continue
		}
		user := strings.TrimSpace(fields[0])
		tty := strings.TrimSpace(fields[1])
		if user == "" || tty == "" {
			continue
		}
		if _, ok := exclude[user]; ok {
			continue
		}
		ttys[tty] = struct{}{}
	}
	count := 0
	for tty := range ttys {
		if err := exec.CommandContext(ctx, "pkill", "-KILL", "-t", tty).Run(); err == nil {
			count++
		}
	}
	log.Printf("执行 kick_ssh_all：killed_tty=%d reason=%s", count, strings.TrimSpace(reason))
	return nil
}

func (a *NodeAgent) kickSSHUser(ctx context.Context, username string, reason string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	out, err := exec.CommandContext(ctx, "who").Output()
	if err != nil {
		return err
	}
	lines := strings.Split(string(out), "\n")
	exclude := loadSSHExemptUsers()
	if _, ok := exclude[username]; ok {
		return nil
	}
	ttys := make(map[string]struct{})
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		fields := strings.Fields(ln)
		if len(fields) < 2 {
			continue
		}
		user := strings.TrimSpace(fields[0])
		tty := strings.TrimSpace(fields[1])
		if user == "" || tty == "" || user != username {
			continue
		}
		ttys[tty] = struct{}{}
	}
	count := 0
	for tty := range ttys {
		if err := exec.CommandContext(ctx, "pkill", "-KILL", "-t", tty).Run(); err == nil {
			count++
		}
	}
	// 兜底：部分场景 who 可能取不到 tty，直接清理该用户的 sshd 会话进程。
	pattern := "^sshd: " + regexp.QuoteMeta(username) + "@"
	_ = exec.CommandContext(ctx, "pkill", "-KILL", "-f", pattern).Run()
	log.Printf("执行 kick_ssh_user：user=%s killed_tty=%d reason=%s", username, count, strings.TrimSpace(reason))
	return nil
}

func (a *NodeAgent) writeNotice(username string, message string) error {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(message) == "" {
		return nil
	}
	homeDir := filepath.Join("/home", username)
	noticeFile := filepath.Join(homeDir, ".gpu_notice")
	content := fmt.Sprintf("%s\n%s\n", formatRFC3339InBeijing(nowInBeijing()), message)
	return os.WriteFile(noticeFile, []byte(content), 0644)
}

const gpuOverdraftBlockedGroup = "gpu_overdraft_blocked"

func ensureSystemGroup(ctx context.Context, groupName string) error {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return errors.New("group_name 不能为空")
	}
	if err := exec.CommandContext(ctx, "getent", "group", groupName).Run(); err == nil {
		return nil
	}
	out, err := exec.CommandContext(ctx, "groupadd", "--system", groupName).CombinedOutput()
	if err != nil {
		// 幂等：并发场景下可能已被其它进程创建。
		if checkErr := exec.CommandContext(ctx, "getent", "group", groupName).Run(); checkErr == nil {
			return nil
		}
		return fmt.Errorf("创建系统组失败：%w out=%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func addUserToGroup(ctx context.Context, username string, groupName string) error {
	out, err := exec.CommandContext(ctx, "gpasswd", "-a", username, groupName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("加入用户组失败：%w out=%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func removeUserFromGroup(ctx context.Context, username string, groupName string) error {
	out, err := exec.CommandContext(ctx, "gpasswd", "-d", username, groupName).CombinedOutput()
	if err != nil {
		// 用户本就不在组内时不视为失败。
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 3 {
			return nil
		}
		return fmt.Errorf("移除用户组失败：%w out=%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *NodeAgent) blockUserGPUAccess(ctx context.Context, username string, reason string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	homeDir := filepath.Join("/home", username)
	flagFile := filepath.Join(homeDir, ".gpu_blocked")
	if strings.TrimSpace(reason) == "" {
		reason = "余额不足，限制新 GPU 任务"
	}
	if err := os.WriteFile(flagFile, []byte(reason+"\n"), 0644); err != nil {
		return err
	}
	if err := ensureSystemGroup(ctx, gpuOverdraftBlockedGroup); err != nil {
		log.Printf("确保欠费 GPU 限制组失败：group=%s err=%v", gpuOverdraftBlockedGroup, err)
	} else if err := addUserToGroup(ctx, username, gpuOverdraftBlockedGroup); err != nil {
		log.Printf("加入欠费 GPU 限制组失败：user=%s group=%s err=%v", username, gpuOverdraftBlockedGroup, err)
	}
	cgroupErr := setUserGPUAccessByCgroup(ctx, username, false)
	aclErr := setUserGPUAccessByACL(ctx, username, false)
	if cgroupErr != nil {
		log.Printf("设置 GPU cgroup 限制失败：user=%s err=%v", username, cgroupErr)
	}
	if aclErr != nil {
		log.Printf("设置 GPU ACL 限制失败：user=%s err=%v", username, aclErr)
	}
	if cgroupErr != nil && aclErr != nil {
		return fmt.Errorf("GPU 限制下发失败：cgroup=%v acl=%v", cgroupErr, aclErr)
	}
	if err := a.reconcileGPUPoliciesForUser(ctx, username, "欠费 GPU 限制变更后重算策略（block）"); err != nil {
		return err
	}
	return nil
}

func (a *NodeAgent) unblockUserGPUAccess(ctx context.Context, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	flagFile := filepath.Join("/home", username, ".gpu_blocked")
	if err := os.Remove(flagFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := removeUserFromGroup(ctx, username, gpuOverdraftBlockedGroup); err != nil {
		log.Printf("移除欠费 GPU 限制组失败：user=%s group=%s err=%v", username, gpuOverdraftBlockedGroup, err)
	}
	cgroupErr := setUserGPUAccessByCgroup(ctx, username, true)
	aclErr := setUserGPUAccessByACL(ctx, username, true)
	if cgroupErr != nil {
		log.Printf("解除 GPU cgroup 限制失败：user=%s err=%v", username, cgroupErr)
	}
	if aclErr != nil {
		log.Printf("解除 GPU ACL 限制失败：user=%s err=%v", username, aclErr)
	}
	if cgroupErr != nil && aclErr != nil {
		return fmt.Errorf("解除 GPU 限制失败：cgroup=%v acl=%v", cgroupErr, aclErr)
	}
	if err := a.reconcileGPUPoliciesForUser(ctx, username, "欠费 GPU 限制变更后重算策略（unblock）"); err != nil {
		return err
	}
	return nil
}

func (a *NodeAgent) killProcesses(ctx context.Context, username string, pids []int32, reason string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	if len(pids) == 0 {
		return nil
	}

	log.Printf("执行 kill_process：user=%s pids=%v reason=%s", username, pids, strings.TrimSpace(reason))

	for _, pid := range pids {
		proc, err := process.NewProcess(pid)
		if err != nil {
			continue
		}
		procUser, err := proc.Username()
		if err != nil || procUser != username {
			continue
		}

		_ = syscall.Kill(int(pid), syscall.SIGTERM)
	}

	// 给进程一点退出时间，避免直接 SIGKILL 造成数据损坏
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
	}

	for _, pid := range pids {
		proc, err := process.NewProcess(pid)
		if err != nil {
			continue
		}
		procUser, err := proc.Username()
		if err != nil || procUser != username {
			continue
		}
		if err := syscall.Kill(int(pid), syscall.SIGKILL); err != nil {
			// 可能已经退出，不视为硬错误
			log.Printf("SIGKILL 失败 pid=%d err=%v", pid, err)
		}
	}

	return nil
}
