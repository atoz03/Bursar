package main

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	gonet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

type cpuSample struct {
	total float64
	ts    time.Time
}

func (a *NodeAgent) CollectMetrics(ctx context.Context) (*MetricsData, error) {
	metrics := &MetricsData{
		NodeID:    a.nodeID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Users:     []UserProcess{},
	}
	metrics.CPUCount = a.numCPU
	if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
		metrics.CPUModel = strings.TrimSpace(infos[0].ModelName)
	}
	if hi, err := host.InfoWithContext(ctx); err == nil && hi != nil {
		metrics.OSVersion = strings.TrimSpace(strings.Join([]string{
			strings.TrimSpace(hi.Platform),
			strings.TrimSpace(hi.PlatformVersion),
		}, " "))
		metrics.OSVersion = strings.TrimSpace(metrics.OSVersion)
		metrics.KernelVersion = strings.TrimSpace(hi.KernelVersion)
	}
	metrics.NodeIP, metrics.NodeMAC = collectPrimaryNodeNetworkIdentity(ctx)
	metrics.SecuritySignals = a.collectSecuritySignals(ctx)

	gpuMap, err := a.getGPUUsageMap(ctx)
	if err != nil {
		return nil, err
	}
	if model, count, err := a.getGPUInventory(ctx); err == nil {
		metrics.GPUModel = model
		metrics.GPUCount = count
	}
	if du, err := disk.UsageWithContext(ctx, "/"); err == nil && du != nil {
		metrics.DiskTotalGB = float64(du.Total) / 1024.0 / 1024.0 / 1024.0
		metrics.DiskUsedGB = float64(du.Used) / 1024.0 / 1024.0 / 1024.0
	}
	if du, err := disk.UsageWithContext(ctx, "/home"); err == nil && du != nil {
		metrics.HomeTotalGB = float64(du.Total) / 1024.0 / 1024.0 / 1024.0
		metrics.HomeUsedGB = float64(du.Used) / 1024.0 / 1024.0 / 1024.0
	}
	if total, used := collectMountPrefixUsageGB(ctx, "/mnt"); total > 0 {
		metrics.MntTotalGB = total
		metrics.MntUsedGB = used
	}
	if ioStats, err := gonet.IOCountersWithContext(ctx, false); err == nil && len(ioStats) > 0 {
		metrics.NetRxBytes = ioStats[0].BytesRecv
		metrics.NetTxBytes = ioStats[0].BytesSent
	}
	metrics.SSHUsers = a.getSSHUsers(ctx)
	metrics.LocalUsers = a.collectNodeLocalUsersCached(ctx)

	// CPU 计费需要观察 CPU-only 进程，因此进行一次全量扫描；
	// 为控制开销，只上报「占用 CPU 超过阈值」或「使用 GPU」的进程。
	procs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("获取进程列表失败：%w", err)
	}

	now := time.Now()
	seen := make(map[int32]struct{}, len(procs))
	for _, proc := range procs {
		pid := proc.Pid
		if pid <= 0 {
			continue
		}
		seen[pid] = struct{}{}

		username, err := proc.Username()
		if err != nil || username == "" || username == "root" {
			continue
		}

		// 计算 CPU 百分比（自维护采样差分，避免依赖 gopsutil 的内部缓存行为）
		cpuPercent := a.computeCPUPercent(ctx, proc, now)
		gpuUsage := gpuMap[pid]

		if len(gpuUsage) == 0 && cpuPercent < a.cpuMinPercent {
			continue
		}

		memInfo, _ := proc.MemoryInfo()
		memoryMB := 0.0
		if memInfo != nil {
			memoryMB = float64(memInfo.RSS) / 1024 / 1024
		}
		cmdline, _ := proc.Cmdline()
		cmdline = strings.TrimSpace(cmdline)
		if len(cmdline) > 256 {
			cmdline = cmdline[:256]
		}

		metrics.Users = append(metrics.Users, UserProcess{
			Username:   username,
			PID:        pid,
			CPUPercent: cpuPercent,
			MemoryMB:   memoryMB,
			GPUUsage:   gpuUsage,
			Command:    cmdline,
		})
	}

	// 清理不再存在的 pid，防止内存增长
	for pid := range a.lastCPUSample {
		if _, ok := seen[pid]; !ok {
			delete(a.lastCPUSample, pid)
		}
	}

	return metrics, nil
}

func cloneNodeLocalUsers(in []NodeLocalUser) []NodeLocalUser {
	if len(in) == 0 {
		return nil
	}
	out := make([]NodeLocalUser, len(in))
	copy(out, in)
	return out
}

func (a *NodeAgent) collectNodeLocalUsersCached(ctx context.Context) []NodeLocalUser {
	now := time.Now()

	a.cacheMu.Lock()
	cached := cloneNodeLocalUsers(a.localUsersCache)
	cachedAt := a.localUsersCachedAt
	refresh := a.localUsersRefreshInterval
	a.cacheMu.Unlock()

	if refresh <= 0 {
		refresh = 15 * time.Minute
	}
	if len(cached) > 0 && !cachedAt.IsZero() && now.Sub(cachedAt) < refresh {
		return cached
	}

	collectCtx := ctx
	var cancel context.CancelFunc
	if a.localUsersCollectTimeout > 0 {
		collectCtx, cancel = context.WithTimeout(ctx, a.localUsersCollectTimeout)
		defer cancel()
	}

	fresh := collectNodeLocalUsers(collectCtx)
	if len(fresh) == 0 && len(cached) > 0 {
		// 重采集失败时兜底返回上一次结果，避免控制面短时“清空”。
		return cached
	}

	a.cacheMu.Lock()
	a.localUsersCache = cloneNodeLocalUsers(fresh)
	a.localUsersCachedAt = now
	a.cacheMu.Unlock()
	return fresh
}

func collectPrimaryNodeNetworkIdentity(ctx context.Context) (string, string) {
	type candidate struct {
		ip    string
		mac   string
		score int
	}
	ifaces, err := gonet.InterfacesWithContext(ctx)
	if err != nil || len(ifaces) == 0 {
		return "", ""
	}
	best := candidate{}
	for _, it := range ifaces {
		flags := make(map[string]struct{}, len(it.Flags))
		for _, f := range it.Flags {
			flags[strings.ToLower(strings.TrimSpace(f))] = struct{}{}
		}
		if _, ok := flags["loopback"]; ok {
			continue
		}
		if _, ok := flags["up"]; !ok {
			continue
		}
		mac := strings.TrimSpace(it.HardwareAddr)
		if mac == "" {
			continue
		}
		for _, addr := range it.Addrs {
			raw := strings.TrimSpace(addr.Addr)
			if raw == "" {
				continue
			}
			ip := ""
			if strings.Contains(raw, "/") {
				if parsed, _, err := net.ParseCIDR(raw); err == nil && parsed != nil {
					ip = parsed.String()
				}
			} else if parsed := net.ParseIP(raw); parsed != nil {
				ip = parsed.String()
			}
			if ip == "" {
				continue
			}
			parsedIP := net.ParseIP(ip)
			if parsedIP == nil || parsedIP.IsLoopback() || parsedIP.To4() == nil {
				continue
			}
			score := 10
			if _, ok := flags["broadcast"]; ok {
				score += 2
			}
			name := strings.ToLower(strings.TrimSpace(it.Name))
			if strings.HasPrefix(name, "eth") || strings.HasPrefix(name, "en") || strings.HasPrefix(name, "bond") {
				score += 3
			}
			if score > best.score {
				best = candidate{ip: ip, mac: mac, score: score}
			}
		}
	}
	return best.ip, best.mac
}

func collectNodeLocalUsers(ctx context.Context) []NodeLocalUser {
	if ctx.Err() != nil {
		return nil
	}
	users := loadHomeUsers()
	if len(users) == 0 {
		return nil
	}
	priv := collectPrivilegeMap(users)
	out := make([]NodeLocalUser, 0, len(users))
	for _, username := range users {
		if ctx.Err() != nil {
			break
		}
		homeDir := filepath.Join("/home", username)
		var createdAt *string
		if ts, ok := detectHomeCreatedAt(homeDir); ok {
			v := ts.UTC().Format(time.RFC3339)
			createdAt = &v
		}
		var lastLoginAt *string
		loginCtx, cancelLogin := context.WithTimeout(ctx, 1200*time.Millisecond)
		if ts, ok := detectLastLoginAt(loginCtx, username); ok {
			v := ts.UTC().Format(time.RFC3339)
			lastLoginAt = &v
		}
		cancelLogin()
		duCtx, cancelDU := context.WithTimeout(ctx, 1200*time.Millisecond)
		homeUsedGB := detectHomeUsedGB(duCtx, homeDir)
		cancelDU()
		out = append(out, NodeLocalUser{
			LocalUsername: username,
			HomeCreatedAt: createdAt,
			LastLoginAt:   lastLoginAt,
			HomeUsedGB:    homeUsedGB,
			HasSudo:       priv.hasSudo(username),
			HasDocker:     priv.hasDocker(username),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LocalUsername < out[j].LocalUsername
	})
	return out
}

type privilegeMap struct {
	sudoUsers   map[string]bool
	dockerUsers map[string]bool
}

func (p privilegeMap) hasSudo(username string) bool {
	return p.sudoUsers[strings.TrimSpace(username)]
}

func (p privilegeMap) hasDocker(username string) bool {
	return p.dockerUsers[strings.TrimSpace(username)]
}

func collectPrivilegeMap(users []string) privilegeMap {
	sudoUsers := map[string]bool{}
	dockerUsers := map[string]bool{}

	markGroupMembers := func(groupName string, target map[string]bool) {
		for _, u := range loadGroupMembers(groupName) {
			target[u] = true
		}
	}

	// Linux 常见 sudo 组名：sudo / wheel / admin（部分发行版）。
	markGroupMembers("sudo", sudoUsers)
	markGroupMembers("wheel", sudoUsers)
	markGroupMembers("admin", sudoUsers)
	markGroupMembers("docker", dockerUsers)

	// 若在 sudoers/sudoers.d 中明确授权，也视为有 sudo 权限。
	explicitSudo := loadExplicitSudoUsers(users)
	for u := range explicitSudo {
		sudoUsers[u] = true
	}

	return privilegeMap{
		sudoUsers:   sudoUsers,
		dockerUsers: dockerUsers,
	}
}

func loadGroupMembers(groupName string) []string {
	raw, err := os.ReadFile("/etc/group")
	if err != nil {
		return nil
	}
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return nil
	}
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, ln := range strings.Split(string(raw), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		parts := strings.Split(ln, ":")
		if len(parts) < 4 {
			continue
		}
		if strings.TrimSpace(parts[0]) != groupName {
			continue
		}
		members := strings.Split(parts[3], ",")
		for _, m := range members {
			u := strings.TrimSpace(m)
			if u == "" {
				continue
			}
			if _, ok := seen[u]; ok {
				continue
			}
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}
	return out
}

func loadExplicitSudoUsers(users []string) map[string]bool {
	out := map[string]bool{}
	candidates := []string{"/etc/sudoers"}
	if files, err := filepath.Glob("/etc/sudoers.d/*"); err == nil {
		candidates = append(candidates, files...)
	}
	for _, f := range candidates {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		text := string(raw)
		for _, u := range users {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			if sudoersHasUserRule(text, u) {
				out[u] = true
			}
		}
	}
	return out
}

func sudoersHasUserRule(content string, username string) bool {
	quotedUser := regexp.QuoteMeta(strings.TrimSpace(username))
	if quotedUser == "" {
		return false
	}
	re := regexp.MustCompile(`(?m)^[ \t]*` + quotedUser + `[ \t]+ALL\s*=`)
	return re.FindStringIndex(content) != nil
}

func loadHomeUsers() []string {
	raw, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil
	}
	out := make([]string, 0, 128)
	seen := map[string]struct{}{}
	for _, ln := range strings.Split(string(raw), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		parts := strings.Split(ln, ":")
		if len(parts) < 7 {
			continue
		}
		username := strings.TrimSpace(parts[0])
		uidText := strings.TrimSpace(parts[2])
		homeDir := strings.TrimSpace(parts[5])
		shell := strings.TrimSpace(parts[6])
		if username == "" || homeDir == "" {
			continue
		}
		if !strings.HasPrefix(homeDir, "/home/") {
			continue
		}
		uid, err := strconv.Atoi(uidText)
		if err != nil || uid < 1000 {
			continue
		}
		if strings.Contains(shell, "nologin") || strings.Contains(shell, "false") {
			continue
		}
		if _, ok := seen[username]; ok {
			continue
		}
		seen[username] = struct{}{}
		out = append(out, username)
	}
	sort.Strings(out)
	return out
}

func detectHomeCreatedAt(homeDir string) (time.Time, bool) {
	info, err := os.Stat(homeDir)
	if err != nil {
		return time.Time{}, false
	}
	mod := info.ModTime()
	// 使用 stat 的 birth time（若支持）优先，否则回退 mtime。
	out, err := exec.Command("stat", "-c", "%W", homeDir).Output()
	if err != nil {
		return mod, true
	}
	epochText := strings.TrimSpace(string(out))
	epoch, err := strconv.ParseInt(epochText, 10, 64)
	if err != nil || epoch <= 0 {
		return mod, true
	}
	return time.Unix(epoch, 0), true
}

func detectHomeUsedGB(ctx context.Context, homeDir string) float64 {
	if ctx.Err() != nil {
		return 0
	}
	out, err := exec.CommandContext(ctx, "du", "-s", "-B1", homeDir).Output()
	if err != nil {
		return 0
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return 0
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return 0
	}
	bytes, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || bytes < 0 {
		return 0
	}
	return bytes / 1024.0 / 1024.0 / 1024.0
}

func detectLastLoginAt(ctx context.Context, username string) (time.Time, bool) {
	if ctx.Err() != nil {
		return time.Time{}, false
	}
	// 优先使用 ISO 时间，规避本地化语言导致解析失败。
	if out, err := exec.CommandContext(ctx, "lastlog", "-u", username, "--time-format", "iso").Output(); err == nil {
		if t, ok := parseLastLoginOutput(string(out)); ok {
			return t, true
		}
	}
	// 回退老格式。
	if out, err := exec.CommandContext(ctx, "lastlog", "-u", username).Output(); err == nil {
		if t, ok := parseLastLoginOutput(string(out)); ok {
			return t, true
		}
	}
	// 最后尝试从 wtmp 获取最近一次登录。
	if out, err := exec.CommandContext(ctx, "last", "-n", "1", "-F", username).Output(); err == nil {
		if t, ok := parseLastCommandOutput(string(out)); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseLastLoginOutput(raw string) (time.Time, bool) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 2 {
		return time.Time{}, false
	}
	line := strings.TrimSpace(lines[1])
	if line == "" {
		return time.Time{}, false
	}
	lower := strings.ToLower(line)
	if strings.Contains(lower, "never logged in") || strings.Contains(lower, "never") || strings.Contains(line, "从未") {
		return time.Time{}, false
	}
	for i := 0; i < len(line); i++ {
		candidate := strings.TrimSpace(line[i:])
		if t, ok := parseFlexibleTime(candidate); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseLastCommandOutput(raw string) (time.Time, bool) {
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "wtmp begins") || strings.Contains(lower, "still logged in") {
			continue
		}
		for i := 0; i < len(line); i++ {
			candidate := strings.TrimSpace(line[i:])
			if t, ok := parseFlexibleTime(candidate); ok {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func parseFlexibleTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}

	// 先直接从文本中提取常见时间片段，避免整行含有额外字段导致解析失败。
	patterns := []string{
		`(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} [+-]\d{4})`,
		`(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`,
		`([A-Z][a-z]{2} [A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}\s+[A-Z]{3}\s+\d{4})`,
		`([A-Z][a-z]{2} [A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}\s+[+-]\d{4}\s+\d{4})`,
		`([A-Z][a-z]{2} [A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}\s+\d{4})`,
	}
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		if m := re.FindStringSubmatch(s); len(m) >= 2 {
			if t, ok := parseTimeLayouts(m[1]); ok {
				return t, true
			}
		}
	}
	return parseTimeLayouts(s)
}

func parseTimeLayouts(s string) (time.Time, bool) {
	type layoutDef struct {
		layout  string
		hasZone bool
	}
	layouts := []layoutDef{
		{layout: "2006-01-02 15:04:05 -0700", hasZone: true},
		{layout: "2006-01-02 15:04:05", hasZone: false},
		{layout: "Mon Jan 2 15:04:05 -0700 2006", hasZone: true},
		{layout: "Mon Jan 2 15:04:05 2006", hasZone: false},
		{layout: "Mon Jan 2 15:04:05 MST 2006", hasZone: true},
	}
	for _, it := range layouts {
		var (
			t   time.Time
			err error
		)
		if it.hasZone {
			t, err = time.Parse(it.layout, s)
		} else {
			// lastlog/last 常见输出不带时区，按节点本地时区解析，避免被误当 UTC 导致“未来时间”。
			t, err = time.ParseInLocation(it.layout, s, time.Local)
		}
		if err == nil {
			// 防御：若采样时钟/解析异常导致未来时间，直接判为无效，避免前端出现“明天登录”。
			if t.After(time.Now().Add(2 * time.Minute)) {
				return time.Time{}, false
			}
			return t, true
		}
	}
	return time.Time{}, false
}

func (a *NodeAgent) validateConfig() error {
	if a.nodeID == "" {
		return fmt.Errorf("NODE_ID 不能为空")
	}
	if a.controllerURL == "" {
		return fmt.Errorf("CONTROLLER_URL 不能为空")
	}
	if a.agentToken == "" {
		return fmt.Errorf("AGENT_TOKEN 不能为空")
	}
	return nil
}

func (a *NodeAgent) computeCPUPercent(ctx context.Context, proc *process.Process, now time.Time) float64 {
	t, err := proc.TimesWithContext(ctx)
	if err != nil {
		return 0
	}
	total := t.User + t.System

	last, ok := a.lastCPUSample[proc.Pid]
	a.lastCPUSample[proc.Pid] = cpuSample{total: total, ts: now}
	if !ok {
		return 0
	}

	elapsed := now.Sub(last.ts).Seconds()
	if elapsed <= 0 {
		return 0
	}
	delta := total - last.total
	if delta <= 0 {
		return 0
	}

	percent := (delta / elapsed) * 100.0
	if math.IsNaN(percent) || math.IsInf(percent, 0) {
		return 0
	}
	// 某些平台可能出现短时间抖动，做个上限保护（不严格依赖 numCPU）
	if percent < 0 {
		return 0
	}
	return percent
}

// collectMountPrefixUsageGB 统计某个挂载前缀（如 /mnt）下所有挂载点的总容量/已用容量。
// 这样当真实挂载点是 /mnt/data 时，不会误把 /mnt 目录当作根分区容量。
func collectMountPrefixUsageGB(ctx context.Context, prefix string) (float64, float64) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "/" {
		return 0, 0
	}
	parts, err := disk.PartitionsWithContext(ctx, true)
	if err != nil {
		return 0, 0
	}
	totalGB := 0.0
	usedGB := 0.0
	seen := make(map[string]struct{})
	for _, p := range parts {
		mp := filepath.Clean(strings.TrimSpace(p.Mountpoint))
		if mp == "." || mp == "/" {
			continue
		}
		if mp != prefix && !strings.HasPrefix(mp, prefix+"/") {
			continue
		}
		// 规避重复挂载点重复累加。
		key := strings.TrimSpace(p.Device) + "|" + mp
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		du, err := disk.UsageWithContext(ctx, mp)
		if err != nil || du == nil || du.Total == 0 {
			continue
		}
		totalGB += float64(du.Total) / 1024.0 / 1024.0 / 1024.0
		usedGB += float64(du.Used) / 1024.0 / 1024.0 / 1024.0
	}
	return totalGB, usedGB
}
