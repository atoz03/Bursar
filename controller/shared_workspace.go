package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func controllerPrivilegedCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	if os.Geteuid() == 0 {
		return exec.CommandContext(ctx, name, args...)
	}
	return exec.CommandContext(ctx, "sudo", append([]string{"-n", name}, args...)...)
}

func runControllerPrivilegedCommand(ctx context.Context, name string, args ...string) error {
	out, err := controllerPrivilegedCommandContext(ctx, name, args...).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if strings.Contains(msg, "a password is required") || strings.Contains(msg, "需要密码") {
		return fmt.Errorf("%s %v 失败：控制器运行用户缺少 sudo -n 权限 out=%s", name, args, msg)
	}
	return fmt.Errorf("%s %v 失败：%w out=%s", name, args, err, msg)
}

func ensureSharedWorkspaceDirOnController(ctx context.Context, path string, uid int, gid int) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." || path == "/" {
		return fmt.Errorf("共享目录路径非法：%s", path)
	}
	if err := runControllerPrivilegedCommand(
		ctx,
		"install",
		"-d",
		"-m", "0755",
		"-o", strconv.Itoa(uid),
		"-g", strconv.Itoa(gid),
		path,
	); err != nil {
		return err
	}
	if err := runControllerPrivilegedCommand(ctx, "chown", fmt.Sprintf("%d:%d", uid, gid), path); err != nil {
		return err
	}
	if err := runControllerPrivilegedCommand(ctx, "chmod", "0755", path); err != nil {
		return err
	}
	return nil
}

func (s *Server) ensureSharedWorkspaceDirsOnController(ctx context.Context, nodeID string, sharedWorkspaceUsername string, platformUID int) error {
	nodeID = strings.TrimSpace(nodeID)
	sharedWorkspaceUsername = strings.TrimSpace(sharedWorkspaceUsername)
	if nodeID == "" || sharedWorkspaceUsername == "" || platformUID <= 0 {
		return nil
	}

	nodeRoot := filepath.Clean(strings.TrimSpace(s.cfg.SharedNodeRoot))
	clusterRoot := filepath.Clean(strings.TrimSpace(s.cfg.SharedClusterRoot))
	nodeBase := filepath.Join(nodeRoot, nodeID)
	clusterBase := clusterRoot

	if info, err := os.Stat(nodeBase); err != nil || !info.IsDir() {
		if err != nil {
			return fmt.Errorf("节点共享根目录不可用：%s err=%w", nodeBase, err)
		}
		return fmt.Errorf("节点共享根目录不是目录：%s", nodeBase)
	}
	if info, err := os.Stat(clusterBase); err != nil || !info.IsDir() {
		if err != nil {
			return fmt.Errorf("集群共享根目录不可用：%s err=%w", clusterBase, err)
		}
		return fmt.Errorf("集群共享根目录不是目录：%s", clusterBase)
	}

	nodePath := filepath.Join(nodeBase, sharedWorkspaceUsername)
	clusterPath := filepath.Join(clusterBase, sharedWorkspaceUsername)
	if err := ensureSharedWorkspaceDirOnController(ctx, nodePath, platformUID, platformUID); err != nil {
		return fmt.Errorf("准备节点共享工作目录失败：%s err=%w", nodePath, err)
	}
	if err := ensureSharedWorkspaceDirOnController(ctx, clusterPath, platformUID, platformUID); err != nil {
		return fmt.Errorf("准备集群共享工作目录失败：%s err=%w", clusterPath, err)
	}
	log.Printf("控制器共享工作目录已就绪：node=%s billing=%s uid=%d node_path=%s cluster_path=%s", nodeID, sharedWorkspaceUsername, platformUID, nodePath, clusterPath)
	return nil
}

func (s *Server) ensureSharedWorkspaceDirsOnControllerWithTimeout(nodeID string, sharedWorkspaceUsername string, platformUID int, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.ensureSharedWorkspaceDirsOnController(ctx, nodeID, sharedWorkspaceUsername, platformUID)
}

func (s *Server) ensureHistoricalSharedWorkspaceDirs(ctx context.Context) {
	rows, err := s.store.ListUserNodeAccountsWithFilter(ctx, "", "", "", 20000)
	if err != nil {
		log.Printf("[warn] 启动补偿共享工作目录失败：读取映射列表失败 err=%v", err)
		return
	}
	okCount := 0
	for _, row := range rows {
		nodeID := strings.TrimSpace(row.NodeID)
		billing := strings.TrimSpace(row.BillingUsername)
		if nodeID == "" || billing == "" {
			continue
		}
		platformUID, uidErr := s.ensureBillingPlatformUID(ctx, billing)
		if uidErr != nil {
			log.Printf("[warn] 启动补偿共享工作目录失败：读取平台 UID 失败 node=%s billing=%s err=%v", nodeID, billing, uidErr)
			continue
		}
		if platformUID <= 0 {
			continue
		}
		if err := s.ensureSharedWorkspaceDirsOnController(ctx, nodeID, billing, platformUID); err != nil {
			log.Printf("[warn] 启动补偿共享工作目录失败：node=%s billing=%s uid=%d err=%v", nodeID, billing, platformUID, err)
			continue
		}
		okCount++
	}
	if okCount > 0 {
		log.Printf("启动补偿共享工作目录完成：已确认 %d 个映射目录", okCount)
	}
}
