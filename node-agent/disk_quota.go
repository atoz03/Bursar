package main

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
)

func commandExists(name string) bool {
	_, err := exec.LookPath(strings.TrimSpace(name))
	return err == nil
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneNodeUserDiskQuotas(in []NodeUserDiskQuota) []NodeUserDiskQuota {
	if len(in) == 0 {
		return nil
	}
	out := make([]NodeUserDiskQuota, len(in))
	copy(out, in)
	return out
}

func isQuotaMountOpt(opt string) bool {
	opt = strings.TrimSpace(strings.ToLower(opt))
	if opt == "" {
		return false
	}
	parts := strings.Split(opt, ",")
	for _, p := range parts {
		token := strings.TrimSpace(p)
		if token == "" {
			continue
		}
		key := token
		if idx := strings.IndexByte(key, '='); idx >= 0 {
			key = strings.TrimSpace(key[:idx])
		}
		switch key {
		case "usrquota", "grpquota", "usrjquota", "grpjquota", "jqfmt", "uquota", "gquota", "prjquota", "pquota", "quota", "uqnoenforce", "gqnoenforce", "pqnoenforce":
			return true
		}
	}
	return false
}

func isQuotaCapableFSType(fsType string) bool {
	switch strings.TrimSpace(strings.ToLower(fsType)) {
	case "ext2", "ext3", "ext4", "xfs":
		return true
	default:
		return false
	}
}

func probeRepquotaMount(ctx context.Context, mountpoint string) bool {
	mountpoint = filepath.Clean(strings.TrimSpace(mountpoint))
	if mountpoint == "" || mountpoint == "." {
		return false
	}
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := exec.CommandContext(cmdCtx, "repquota", "-u", mountpoint).CombinedOutput()
	// 兼容不同系统语言与输出格式：命令成功即视为配额可读。
	return err == nil
}

func sortQuotaMounts(mounts []string) []string {
	uniq := map[string]struct{}{}
	out := make([]string, 0, len(mounts))
	for _, m := range mounts {
		x := filepath.Clean(strings.TrimSpace(m))
		if x == "" || x == "." {
			continue
		}
		if _, ok := uniq[x]; ok {
			continue
		}
		uniq[x] = struct{}{}
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool {
		// /home 优先，其次 /，最后按长度+字典序。
		a := out[i]
		b := out[j]
		score := func(v string) int {
			switch v {
			case "/home":
				return 0
			case "/":
				return 1
			default:
				return 2
			}
		}
		sa := score(a)
		sb := score(b)
		if sa != sb {
			return sa < sb
		}
		if len(a) != len(b) {
			return len(a) < len(b)
		}
		return a < b
	})
	return out
}

func detectQuotaMounts(ctx context.Context) []string {
	parts, err := disk.PartitionsWithContext(ctx, true)
	if err != nil {
		return nil
	}
	hasRepquota := commandExists("repquota")
	mounts := make([]string, 0, len(parts))
	for _, p := range parts {
		mp := filepath.Clean(strings.TrimSpace(p.Mountpoint))
		if mp == "" || mp == "." {
			continue
		}
		opts := p.Opts
		has := false
		for _, opt := range opts {
			if isQuotaMountOpt(opt) {
				has = true
				break
			}
		}
		if !has {
			// 兜底：部分系统挂载参数格式不统一（如 key=value / 组合参数），
			// 但 repquota 已可读取时，仍应判定为 quota 已启用。
			if !(hasRepquota && isQuotaCapableFSType(p.Fstype) && probeRepquotaMount(ctx, mp)) {
				continue
			}
		}
		mounts = append(mounts, mp)
	}
	return sortQuotaMounts(mounts)
}

func selectPreferredQuotaMount(mounts []string) string {
	x := sortQuotaMounts(mounts)
	if len(x) == 0 {
		return ""
	}
	return x[0]
}

func parseQuotaBlocksToMB(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	// repquota 数字列通常是整数；容忍尾部噪声字符。
	start := -1
	end := -1
	for i, ch := range s {
		if (ch >= '0' && ch <= '9') || (ch == '.' && start >= 0) {
			if start < 0 {
				start = i
			}
			end = i + 1
			continue
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 || end <= start {
		if s == "-" {
			return 0, true
		}
		return 0, false
	}
	v, err := strconv.ParseFloat(s[start:end], 64)
	if err != nil || v < 0 {
		return 0, false
	}
	// repquota block 列默认 1KB block，转换为 MB。
	return v / 1024.0, true
}

func isRepquotaFlagToken(token string) bool {
	t := strings.TrimSpace(token)
	if t == "" {
		return false
	}
	if len(t) > 4 {
		return false
	}
	for _, ch := range t {
		if ch != '+' && ch != '-' {
			return false
		}
	}
	return true
}

func parseRepquotaOutput(raw string, mountpoint string, allowed map[string]struct{}) []NodeUserDiskQuota {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]NodeUserDiskQuota, 0, 64)
	seen := map[string]struct{}{}
	for _, ln := range lines {
		line := strings.TrimSpace(ln)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "*** report") ||
			strings.HasPrefix(lower, "block grace") ||
			strings.HasPrefix(lower, "inode grace") ||
			strings.HasPrefix(lower, "user ") ||
			strings.HasPrefix(line, "-") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		username := strings.TrimSpace(fields[0])
		if username == "" || username == "root" {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[username]; !ok {
				continue
			}
		}
		idx := 1
		if len(fields) > 1 && isRepquotaFlagToken(fields[1]) {
			idx = 2
		}
		if len(fields) <= idx+2 {
			continue
		}
		usedMB, okUsed := parseQuotaBlocksToMB(fields[idx])
		softMB, okSoft := parseQuotaBlocksToMB(fields[idx+1])
		hardMB, okHard := parseQuotaBlocksToMB(fields[idx+2])
		if !okUsed || !okSoft || !okHard {
			continue
		}
		key := mountpoint + "::" + username
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, NodeUserDiskQuota{
			LocalUsername: username,
			Mountpoint:    mountpoint,
			UsedMB:        usedMB,
			SoftMB:        softMB,
			HardMB:        hardMB,
		})
	}
	return out
}

func (a *NodeAgent) collectDiskQuotaSnapshotCached(ctx context.Context, users []NodeLocalUser) (bool, []string, []NodeUserDiskQuota) {
	now := time.Now()
	refresh := a.diskQuotaRefreshInterval
	if refresh <= 0 {
		refresh = 2 * time.Minute
	}

	a.cacheMu.Lock()
	cachedInstalled := a.diskQuotaInstalledCache
	cachedMounts := cloneStringSlice(a.diskQuotaMountsCache)
	cachedQuotas := cloneNodeUserDiskQuotas(a.userDiskQuotasCache)
	cachedAt := a.diskQuotaCachedAt
	a.cacheMu.Unlock()

	if !cachedAt.IsZero() && now.Sub(cachedAt) < refresh {
		return cachedInstalled, cachedMounts, cachedQuotas
	}

	installed := commandExists("setquota")
	mounts := detectQuotaMounts(ctx)

	allowed := map[string]struct{}{}
	for _, u := range users {
		name := strings.TrimSpace(u.LocalUsername)
		if name == "" {
			continue
		}
		allowed[name] = struct{}{}
	}

	quotas := make([]NodeUserDiskQuota, 0)
	if installed && len(mounts) > 0 && commandExists("repquota") {
		for _, mountpoint := range mounts {
			if ctx.Err() != nil {
				break
			}
			cmdCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
			out, err := exec.CommandContext(cmdCtx, "repquota", "-u", mountpoint).CombinedOutput()
			cancel()
			if err != nil {
				continue
			}
			quotas = append(quotas, parseRepquotaOutput(string(out), mountpoint, allowed)...)
		}
		sort.Slice(quotas, func(i, j int) bool {
			if quotas[i].Mountpoint == quotas[j].Mountpoint {
				return quotas[i].LocalUsername < quotas[j].LocalUsername
			}
			return quotas[i].Mountpoint < quotas[j].Mountpoint
		})
	}

	a.cacheMu.Lock()
	a.diskQuotaInstalledCache = installed
	a.diskQuotaMountsCache = cloneStringSlice(mounts)
	a.userDiskQuotasCache = cloneNodeUserDiskQuotas(quotas)
	a.diskQuotaCachedAt = now
	a.cacheMu.Unlock()
	return installed, mounts, quotas
}

func (a *NodeAgent) setUserDiskQuota(ctx context.Context, username string, mountpoint string, softMB float64, hardMB float64, reason string) error {
	username = strings.TrimSpace(username)
	mountpoint = filepath.Clean(strings.TrimSpace(mountpoint))
	if username == "" {
		return fmt.Errorf("username 不能为空")
	}
	if !localUsernamePattern.MatchString(username) {
		return fmt.Errorf("非法本地账号名：%s", username)
	}
	if softMB < 0 || hardMB < 0 {
		return fmt.Errorf("disk_quota_soft_mb / disk_quota_hard_mb 必须为非负数")
	}
	if softMB > 0 && hardMB > 0 && hardMB < softMB {
		return fmt.Errorf("hard 不能小于 soft（soft=%.2fMB hard=%.2fMB）", softMB, hardMB)
	}
	if !commandExists("setquota") {
		return fmt.Errorf("节点未安装 setquota，无法设置磁盘配额")
	}

	if mountpoint == "" || mountpoint == "." {
		installed, mounts, _ := a.collectDiskQuotaSnapshotCached(ctx, nil)
		if !installed {
			return fmt.Errorf("节点未安装 quota 工具")
		}
		mountpoint = selectPreferredQuotaMount(mounts)
	}
	if mountpoint == "" || mountpoint == "." {
		return fmt.Errorf("未检测到已启用 quota 的分区，请先在节点启用 usrquota/uquota")
	}

	if out, err := exec.CommandContext(ctx, "id", "-u", username).CombinedOutput(); err != nil {
		return fmt.Errorf("节点账号不存在：%s (out=%s)", username, strings.TrimSpace(string(out)))
	}

	softKB := int64(math.Round(softMB * 1024.0))
	hardKB := int64(math.Round(hardMB * 1024.0))
	if softKB < 0 {
		softKB = 0
	}
	if hardKB < 0 {
		hardKB = 0
	}

	cmd := exec.CommandContext(
		ctx,
		"setquota",
		"-u",
		username,
		strconv.FormatInt(softKB, 10),
		strconv.FormatInt(hardKB, 10),
		"0",
		"0",
		mountpoint,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("setquota 失败：%w out=%s", err, strings.TrimSpace(string(out)))
	}
	a.invalidateLocalUsersAndQuotaCache()
	a.logger.Printf(
		"执行 set_disk_quota：user=%s mount=%s soft=%.2fMB hard=%.2fMB reason=%s",
		username,
		mountpoint,
		softMB,
		hardMB,
		strings.TrimSpace(reason),
	)
	return nil
}
