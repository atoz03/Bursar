package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	if action.DelaySeconds > 0 {
		timer := time.NewTimer(time.Duration(action.DelaySeconds) * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
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
		return a.createLocalAccount(ctx, action.Username, action.PublicKey, action.SharedWorkspaceUsername, action.TargetUID, action.TargetPrimaryGID, action.Reason)
	default:
		return fmt.Errorf("未知 action.type：%s", action.Type)
	}
}

var localUsernamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,31}$`)

const defaultNoLoginShell = "/usr/sbin/nologin"

func normalizeOwnershipRepairRoots(homeDir string, mounts []string) []string {
	seen := make(map[string]struct{}, 2)
	out := make([]string, 0, 2)
	add := func(raw string) {
		path := filepath.Clean(strings.TrimSpace(raw))
		if path == "" || path == "." {
			return
		}
		if _, err := os.Lstat(path); err != nil {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	add(homeDir)
	add("/mnt")
	return out
}

func collectOwnershipRepairRoots(homeDir string) []string {
	return normalizeOwnershipRepairRoots(homeDir, nil)
}

const mntOwnershipScanMaxDepth = 3

func relativePathDepth(root string, path string) int {
	root = filepath.Clean(strings.TrimSpace(root))
	path = filepath.Clean(strings.TrimSpace(path))
	if root == "" || path == "" {
		return 0
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return 0
	}
	rel = filepath.Clean(strings.TrimSpace(rel))
	if rel == "." || rel == "" {
		return 0
	}
	depth := 0
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		depth++
	}
	return depth
}

func lstatOwnership(path string) (int, int, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return 0, 0, fmt.Errorf("读取路径属主失败：%s", path)
	}
	return int(st.Uid), int(st.Gid), nil
}

func chownTree(ctx context.Context, root string, newUID int, newGID int) error {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return nil
	}
	if _, err := os.Lstat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	out, err := exec.CommandContext(ctx, "chown", "-hR", fmt.Sprintf("%d:%d", newUID, newGID), root).CombinedOutput()
	if err != nil {
		return fmt.Errorf("递归修复属主失败：root=%s err=%w out=%s", root, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func repairOwnershipUnderHome(ctx context.Context, homeDir string, newUID int, newGID int) (int, int, error) {
	homeDir = filepath.Clean(strings.TrimSpace(homeDir))
	if homeDir == "" || homeDir == "." {
		return 0, 0, nil
	}
	if err := chownTree(ctx, homeDir, newUID, newGID); err != nil {
		return 0, 1, err
	}
	return 1, 0, nil
}

func collectLegacyOwnershipTargets(root string, oldUID int, oldGID int) ([]string, []string, int, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return nil, nil, 0, nil
	}
	if _, err := os.Lstat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, 0, nil
		}
		return nil, nil, 1, err
	}
	treeRoots := make([]string, 0, 16)
	singlePaths := make([]string, 0, 16)
	failed := 0
	var firstErr error
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			failed++
			if firstErr == nil {
				firstErr = walkErr
			}
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st == nil {
			failed++
			if firstErr == nil {
				firstErr = fmt.Errorf("读取路径属主失败：%s", path)
			}
			return nil
		}
		depth := relativePathDepth(root, path)
		if d != nil && d.IsDir() && depth >= mntOwnershipScanMaxDepth {
			// /mnt 只扫描到 /mnt/disk1/zhangsan/zhah 这一层；更深层交给命中的上层目录 chown -R。
			if int(st.Uid) == oldUID || int(st.Gid) == oldGID {
				if path != root {
					treeRoots = append(treeRoots, path)
				} else {
					singlePaths = append(singlePaths, path)
				}
			}
			return filepath.SkipDir
		}
		// 修复目标严格按旧 UID/GID 命中，不按用户名推断路径。
		if int(st.Uid) != oldUID && int(st.Gid) != oldGID {
			return nil
		}
		// 命中目录时直接把该目录作为 repair root，后续由 chown -R 处理整棵子树。
		if d != nil && d.IsDir() && path != root {
			treeRoots = append(treeRoots, path)
			return filepath.SkipDir
		}
		singlePaths = append(singlePaths, path)
		return nil
	})
	if walkErr != nil && firstErr == nil {
		firstErr = walkErr
	}
	return treeRoots, singlePaths, failed, firstErr
}

func chownPaths(paths []string, newUID int, newGID int) (int, int, error) {
	changed := 0
	failed := 0
	var firstErr error
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." {
			continue
		}
		if err := os.Lchown(path, newUID, newGID); err != nil {
			failed++
			if firstErr == nil {
				firstErr = fmt.Errorf("修复路径属主失败：path=%s err=%w", path, err)
			}
			continue
		}
		changed++
	}
	return changed, failed, firstErr
}

func chownTrees(ctx context.Context, roots []string, newUID int, newGID int) (int, int, error) {
	changed := 0
	failed := 0
	var firstErr error
	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" || root == "." || root == "/" {
			continue
		}
		if err := chownTree(ctx, root, newUID, newGID); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		changed++
	}
	return changed, failed, firstErr
}

func repairOwnershipUnderMnt(ctx context.Context, oldUID int, oldGID int, newUID int, newGID int) (int, int, error) {
	root := "/mnt"
	if _, err := os.Lstat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	treeRoots, singlePaths, scanFailed, scanErr := collectLegacyOwnershipTargets(root, oldUID, oldGID)
	treeChanged, treeFailed, treeErr := chownTrees(ctx, treeRoots, newUID, newGID)
	pathChanged, pathFailed, pathErr := chownPaths(singlePaths, newUID, newGID)
	changed := treeChanged + pathChanged
	if scanErr != nil {
		return changed, scanFailed + treeFailed + pathFailed, scanErr
	}
	if treeErr != nil {
		return changed, scanFailed + treeFailed + pathFailed, treeErr
	}
	if pathErr != nil {
		return changed, scanFailed + treeFailed + pathFailed, pathErr
	}
	return changed, scanFailed + treeFailed + pathFailed, nil
}

func repairOwnershipAfterIdentityChange(ctx context.Context, homeDir string, oldUID int, oldGID int, newUID int, newGID int) (int, int, error) {
	roots := collectOwnershipRepairRoots(homeDir)
	totalChanged := 0
	totalFailed := 0
	var firstErr error
	for _, root := range roots {
		var (
			changed int
			failed  int
			err     error
		)
		switch root {
		case filepath.Clean(strings.TrimSpace(homeDir)):
			changed, failed, err = repairOwnershipUnderHome(ctx, root, newUID, newGID)
		case "/mnt":
			changed, failed, err = repairOwnershipUnderMnt(ctx, oldUID, oldGID, newUID, newGID)
		default:
			continue
		}
		totalChanged += changed
		totalFailed += failed
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return totalChanged, totalFailed, firstErr
}

func normalizeTargetIdentity(uid int, gid int) (int, int) {
	if uid <= 0 && gid <= 0 {
		return 0, 0
	}
	if uid <= 0 {
		uid = gid
	}
	return uid, uid
}

func lookupPasswdByUID(ctx context.Context, uid int) (string, bool) {
	if uid <= 0 {
		return "", false
	}
	out, err := exec.CommandContext(ctx, "getent", "passwd", strconv.Itoa(uid)).Output()
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return "", false
	}
	parts := strings.Split(line, ":")
	if len(parts) == 0 {
		return "", false
	}
	name := strings.TrimSpace(parts[0])
	if name == "" {
		return "", false
	}
	return name, true
}

func chooseNoLoginShell() string {
	candidates := []string{
		defaultNoLoginShell,
		"/usr/sbin/nologin",
		"/sbin/nologin",
		"/usr/bin/false",
		"/bin/false",
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return defaultNoLoginShell
}

func lookupPasswdEntry(ctx context.Context, username string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "getent", "passwd", strings.TrimSpace(username)).Output()
	if err != nil {
		return nil, err
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return nil, fmt.Errorf("账号不存在：%s", username)
	}
	parts := strings.Split(line, ":")
	if len(parts) < 7 {
		return nil, fmt.Errorf("passwd 记录格式异常：%s", username)
	}
	return parts, nil
}

func lookupUserShell(ctx context.Context, username string) (string, error) {
	parts, err := lookupPasswdEntry(ctx, username)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(parts[6]), nil
}

type groupEntry struct {
	Name string
	GID  int
}

func lookupGroupByName(ctx context.Context, name string) (groupEntry, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return groupEntry{}, false, nil
	}
	out, err := exec.CommandContext(ctx, "getent", "group", name).Output()
	if err != nil {
		return groupEntry{}, false, nil
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return groupEntry{}, false, nil
	}
	parts := strings.Split(line, ":")
	if len(parts) < 3 {
		return groupEntry{}, false, fmt.Errorf("group 记录格式异常：%s", name)
	}
	gid, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil {
		return groupEntry{}, false, fmt.Errorf("解析 group gid 失败：%w", err)
	}
	return groupEntry{Name: strings.TrimSpace(parts[0]), GID: gid}, true, nil
}

func lookupGroupByGID(ctx context.Context, gid int) (groupEntry, bool, error) {
	if gid <= 0 {
		return groupEntry{}, false, nil
	}
	out, err := exec.CommandContext(ctx, "getent", "group", strconv.Itoa(gid)).Output()
	if err != nil {
		return groupEntry{}, false, nil
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return groupEntry{}, false, nil
	}
	parts := strings.Split(line, ":")
	if len(parts) < 3 {
		return groupEntry{}, false, fmt.Errorf("group 记录格式异常：gid=%d", gid)
	}
	actualGID, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil {
		return groupEntry{}, false, fmt.Errorf("解析 group gid 失败：%w", err)
	}
	return groupEntry{Name: strings.TrimSpace(parts[0]), GID: actualGID}, true, nil
}

func isReservedIdentityGroup(name string) bool {
	switch strings.TrimSpace(name) {
	case gpuOverdraftBlockedGroup:
		return true
	default:
		return false
	}
}

func withTemporarilyDisabledLogin(ctx context.Context, username string, fn func() error) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username 不能为空")
	}
	currentShell, err := lookupUserShell(ctx, username)
	if err != nil {
		return err
	}
	currentShell = strings.TrimSpace(currentShell)
	restoreShell := currentShell
	noLoginShell := chooseNoLoginShell()
	shellChanged := false
	if currentShell != "" && !strings.Contains(currentShell, "nologin") && !strings.Contains(currentShell, "false") {
		if out, err := exec.CommandContext(ctx, "usermod", "-s", noLoginShell, username).CombinedOutput(); err != nil {
			return fmt.Errorf("临时禁用登录失败：%w out=%s", err, strings.TrimSpace(string(out)))
		}
		shellChanged = true
	}
	defer func() {
		if !shellChanged || restoreShell == "" {
			return
		}
		if out, err := exec.CommandContext(context.Background(), "usermod", "-s", restoreShell, username).CombinedOutput(); err != nil {
			log.Printf("恢复登录 shell 失败：user=%s shell=%s err=%v out=%s", username, restoreShell, err, strings.TrimSpace(string(out)))
		}
	}()
	return fn()
}

func ensurePrimaryGroupByGID(ctx context.Context, preferredName string, gid int) error {
	if gid <= 0 {
		return nil
	}
	preferredName = strings.TrimSpace(preferredName)
	if preferredName == "" {
		preferredName = fmt.Sprintf("gpuops_uid_%d", gid)
	}
	groupByName, hasByName, err := lookupGroupByName(ctx, preferredName)
	if err != nil {
		return err
	}
	groupByGID, hasByGID, err := lookupGroupByGID(ctx, gid)
	if err != nil {
		return err
	}
	if hasByGID {
		if hasByName && groupByName.Name == groupByGID.Name && groupByName.GID == gid {
			return nil
		}
		if groupByGID.Name == preferredName {
			return nil
		}
		reservedTip := ""
		if isReservedIdentityGroup(groupByGID.Name) {
			reservedTip = "（GPU 限制策略保留组）"
		}
		return fmt.Errorf("目标主组 gid=%d 已被组 %s 占用%s，无法保证 uid/gid 一致", gid, groupByGID.Name, reservedTip)
	}
	if hasByName {
		modOut, modErr := exec.CommandContext(ctx, "groupmod", "-g", strconv.Itoa(gid), preferredName).CombinedOutput()
		if modErr != nil {
			return fmt.Errorf("调整用户私有组 gid 失败：group=%s old_gid=%d target_gid=%d err=%w out=%s", preferredName, groupByName.GID, gid, modErr, strings.TrimSpace(string(modOut)))
		}
		return nil
	}
	out, err := exec.CommandContext(ctx, "groupadd", "-g", strconv.Itoa(gid), preferredName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("创建主组失败：%w out=%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *NodeAgent) alignLocalAccountIdentity(ctx context.Context, username string, targetUID int, targetGID int, reason string) error {
	targetUID, targetGID = normalizeTargetIdentity(targetUID, targetGID)
	if targetUID <= 0 || targetGID <= 0 {
		return nil
	}
	sysUser, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("查询系统账号失败：%w", err)
	}
	currentUID, err := strconv.Atoi(sysUser.Uid)
	if err != nil {
		return fmt.Errorf("解析当前 uid 失败：%w", err)
	}
	currentGID, err := strconv.Atoi(sysUser.Gid)
	if err != nil {
		return fmt.Errorf("解析当前 gid 失败：%w", err)
	}
	if currentUID == targetUID && currentGID == targetGID {
		return nil
	}
	if owner, found := lookupPasswdByUID(ctx, targetUID); found && owner != username {
		return fmt.Errorf("目标 uid=%d 已被账号 %s 占用", targetUID, owner)
	}
	if err := ensurePrimaryGroupByGID(ctx, username, targetGID); err != nil {
		return err
	}
	homeDir := strings.TrimSpace(sysUser.HomeDir)
	if homeDir == "" {
		homeDir = filepath.Join("/home", username)
	}
	return withTemporarilyDisabledLogin(ctx, username, func() error {
		if err := a.kickSSHUser(ctx, username, "UID/GID 迁移前临时禁止登录"); err != nil {
			log.Printf("执行 create_local_account：user=%s kick-ssh-warning err=%v", username, err)
		}
		if err := a.killAllProcessesByUser(ctx, username, "UID/GID 迁移前清理活跃进程"); err != nil {
			log.Printf("执行 create_local_account：user=%s kill-process-warning err=%v", username, err)
		}
		if currentUID != targetUID {
			if out, err := exec.CommandContext(ctx, "usermod", "-u", strconv.Itoa(targetUID), username).CombinedOutput(); err != nil {
				return fmt.Errorf("调整 uid 失败：%w out=%s", err, strings.TrimSpace(string(out)))
			}
		}
		if currentGID != targetGID {
			if out, err := exec.CommandContext(ctx, "usermod", "-g", strconv.Itoa(targetGID), username).CombinedOutput(); err != nil {
				return fmt.Errorf("调整主组 gid 失败：%w out=%s", err, strings.TrimSpace(string(out)))
			}
		}
		verifyGIDOut, verifyErr := exec.CommandContext(ctx, "id", "-g", username).Output()
		if verifyErr != nil {
			return fmt.Errorf("校验主组 gid 失败：%w", verifyErr)
		}
		if actualGID, convErr := strconv.Atoi(strings.TrimSpace(string(verifyGIDOut))); convErr != nil || actualGID != targetGID {
			return fmt.Errorf("主组 gid 未对齐：want=%d got=%s", targetGID, strings.TrimSpace(string(verifyGIDOut)))
		}
		changed, failed, repairErr := repairOwnershipAfterIdentityChange(ctx, homeDir, currentUID, currentGID, targetUID, targetGID)
		if repairErr != nil {
			log.Printf("执行 create_local_account：user=%s ownership-repair-warning err=%v", username, repairErr)
		}
		if failed > 0 {
			log.Printf("执行 create_local_account：user=%s ownership-repair-partial-failure failed_paths=%d", username, failed)
		}
		log.Printf("执行 create_local_account：user=%s identity-aligned uid=%d gid=%d repaired_paths=%d failed_paths=%d reason=%s", username, targetUID, targetGID, changed, failed, strings.TrimSpace(reason))
		return nil
	})
}

func privilegedCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	if os.Geteuid() == 0 {
		return exec.CommandContext(ctx, name, args...)
	}
	return exec.CommandContext(ctx, "sudo", append([]string{"-n", name}, args...)...)
}

func ensureOwnedDir(ctx context.Context, path string, uid int, gid int) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." || path == "/" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		out, cmdErr := privilegedCommandContext(
			ctx,
			"install",
			"-d",
			"-m", "0755",
			"-o", strconv.Itoa(uid),
			"-g", strconv.Itoa(gid),
			path,
		).CombinedOutput()
		if cmdErr != nil {
			return fmt.Errorf("创建目录失败：path=%s err=%w out=%s", path, cmdErr, strings.TrimSpace(string(out)))
		}
		info = nil
	}
	if info != nil && !info.IsDir() {
		return fmt.Errorf("路径不是目录：%s", path)
	}
	chownOut, chownErr := privilegedCommandContext(ctx, "chown", fmt.Sprintf("%d:%d", uid, gid), path).CombinedOutput()
	if chownErr != nil {
		return fmt.Errorf("设置目录属主失败：path=%s err=%w out=%s", path, chownErr, strings.TrimSpace(string(chownOut)))
	}
	chmodOut, chmodErr := privilegedCommandContext(ctx, "chmod", "0755", path).CombinedOutput()
	if chmodErr != nil {
		return fmt.Errorf("设置目录权限失败：path=%s err=%w out=%s", path, chmodErr, strings.TrimSpace(string(chmodOut)))
	}
	return nil
}

func ensureSharedWorkspaceDirs(ctx context.Context, sharedWorkspaceUsername string, uid int, gid int) error {
	sharedWorkspaceUsername = strings.TrimSpace(sharedWorkspaceUsername)
	if sharedWorkspaceUsername == "" {
		return nil
	}
	if err := ensureOwnedDir(ctx, "/shared/node", 0, 0); err != nil {
		return fmt.Errorf("准备 /shared/node 根目录失败：%w", err)
	}
	if err := ensureOwnedDir(ctx, "/shared/cluster", 0, 0); err != nil {
		return fmt.Errorf("准备 /shared/cluster 根目录失败：%w", err)
	}
	if err := ensureOwnedDir(ctx, filepath.Join("/shared/node", sharedWorkspaceUsername), uid, gid); err != nil {
		return fmt.Errorf("创建 /shared/node 工作目录失败：%w", err)
	}
	clusterPath := filepath.Join("/shared/cluster", sharedWorkspaceUsername)
	if _, err := os.Stat(clusterPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("检查 /shared/cluster 工作目录失败：%w", err)
	}
	if err := ensureOwnedDir(ctx, clusterPath, uid, gid); err != nil {
		return fmt.Errorf("创建 /shared/cluster 工作目录失败：%w", err)
	}
	return nil
}

func (a *NodeAgent) createLocalAccount(ctx context.Context, username string, publicKey string, sharedWorkspaceUsername string, targetUID int, targetPrimaryGID int, reason string) (err error) {
	username = strings.TrimSpace(username)
	publicKey = strings.TrimSpace(publicKey)
	sharedWorkspaceUsername = strings.TrimSpace(sharedWorkspaceUsername)
	if username == "" {
		return errors.New("username 不能为空")
	}
	defer func() {
		a.invalidateLocalUsersAndQuotaCache()
		a.triggerForceSync("create_local_account 结束后主动刷新节点本地用户快照：" + username)
		if err != nil {
			log.Printf("执行 create_local_account：user=%s failed err=%v", username, err)
		}
	}()
	if !localUsernamePattern.MatchString(username) {
		return fmt.Errorf("非法本地账号名：%s", username)
	}
	targetUID, targetPrimaryGID = normalizeTargetIdentity(targetUID, targetPrimaryGID)
	if publicKey == "" && targetUID <= 0 {
		return errors.New("public_key 为空时，必须提供 target_uid/target_primary_gid")
	}
	if publicKey != "" && !strings.HasPrefix(publicKey, "ssh-ed25519 ") {
		return errors.New("public_key 格式不合法，仅支持 ssh-ed25519")
	}

	// 幂等：账号已存在时不重复 useradd，但仍会刷新 authorized_keys，确保密钥可更新。
	userExists := exec.CommandContext(ctx, "id", "-u", username).Run() == nil
	if !userExists && publicKey == "" {
		return errors.New("账号不存在时必须提供 public_key")
	}
	if !userExists {
		args := []string{"-m", "-s", "/bin/bash"}
		if targetUID > 0 {
			if owner, found := lookupPasswdByUID(ctx, targetUID); found && owner != username {
				return fmt.Errorf("目标 uid=%d 已被账号 %s 占用", targetUID, owner)
			}
			args = append(args, "-u", strconv.Itoa(targetUID))
		}
		if targetPrimaryGID > 0 {
			if err := ensurePrimaryGroupByGID(ctx, username, targetPrimaryGID); err != nil {
				return err
			}
			args = append(args, "-g", strconv.Itoa(targetPrimaryGID))
		}
		args = append(args, username)
		if out, err := exec.CommandContext(ctx, "useradd", args...).CombinedOutput(); err != nil {
			// 兜底：竞态下可能已被其它流程创建。
			if checkErr := exec.CommandContext(ctx, "id", "-u", username).Run(); checkErr == nil {
				userExists = true
				log.Printf("执行 create_local_account：user=%s race-created reason=%s", username, strings.TrimSpace(reason))
			} else {
				return fmt.Errorf("useradd 失败：%w out=%s", err, strings.TrimSpace(string(out)))
			}
		}
	}
	if targetUID > 0 {
		if err := a.alignLocalAccountIdentity(ctx, username, targetUID, targetPrimaryGID, reason); err != nil {
			return err
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

	if publicKey != "" {
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
	} else {
		// 对齐 UID/GID 场景下，不改写现有密钥，仅修正 .ssh 目录属主。
		if st, err := os.Stat(sshDir); err == nil && st.IsDir() {
			_ = os.Chown(sshDir, uid, gid)
		}
		if _, err := os.Stat(authFile); err == nil {
			_ = os.Chown(authFile, uid, gid)
		}
	}

	if sharedWorkspaceUsername != "" {
		log.Printf("执行 create_local_account：user=%s ensure-shared-workspace shared_workspace=%s uid=%d gid=%d", username, sharedWorkspaceUsername, uid, gid)
	}
	if err := ensureSharedWorkspaceDirs(ctx, sharedWorkspaceUsername, uid, gid); err != nil {
		log.Printf("执行 create_local_account：user=%s shared-workspace-warning shared_workspace=%s uid=%d gid=%d err=%v", username, sharedWorkspaceUsername, uid, gid, err)
	} else if sharedWorkspaceUsername != "" {
		log.Printf("执行 create_local_account：user=%s shared-workspace-ready shared_workspace=%s uid=%d gid=%d", username, sharedWorkspaceUsername, uid, gid)
	}

	if userExists {
		log.Printf(
			"执行 create_local_account：user=%s already-exists uid=%d gid=%d key-updated=%v shared_workspace=%s home=%s reason=%s",
			username,
			uid,
			gid,
			publicKey != "",
			sharedWorkspaceUsername,
			homeDir,
			strings.TrimSpace(reason),
		)
	} else {
		log.Printf(
			"执行 create_local_account：user=%s created uid=%d gid=%d key-updated=%v shared_workspace=%s home=%s reason=%s",
			username,
			uid,
			gid,
			publicKey != "",
			sharedWorkspaceUsername,
			homeDir,
			strings.TrimSpace(reason),
		)
	}
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
