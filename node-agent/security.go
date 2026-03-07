package main

import (
	"context"
	"net"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const (
	sshBruteSourceFailThreshold   = 10
	sshBruteDistinctUserThreshold = 5
	portScanDistinctPortThreshold = 12
	portScanConnThreshold         = 40
)

func (a *NodeAgent) collectSecuritySignals(ctx context.Context) *SecuritySignals {
	failedCount, failedUsers, failedSources, failBySource, usersBySource := collectSSHFailed5m(ctx)
	bruteSources := make([]string, 0)
	for src, cnt := range failBySource {
		if cnt >= sshBruteSourceFailThreshold || len(usersBySource[src]) >= sshBruteDistinctUserThreshold {
			bruteSources = append(bruteSources, src)
		}
	}
	sort.Strings(bruteSources)

	portScanDetected, portScanSources, portScanPorts := detectPortScan(ctx)

	hasAny := failedCount > 0 || len(bruteSources) > 0 || portScanDetected
	if !hasAny {
		return nil
	}
	return &SecuritySignals{
		SSHFailedCount5m:      failedCount,
		SSHFailedUsernames:    failedUsers,
		SSHFailedSourceIPs:    failedSources,
		SSHBruteforceDetected: len(bruteSources) > 0,
		SSHBruteforceSources:  bruteSources,
		PortScanDetected:      portScanDetected,
		PortScanSources:       portScanSources,
		PortScanTargetPorts:   portScanPorts,
	}
}

func collectSSHFailed5m(ctx context.Context) (int, []string, []string, map[string]int, map[string]map[string]struct{}) {
	lines := loadSSHJournalLines(ctx)
	if len(lines) == 0 {
		return 0, nil, nil, map[string]int{}, map[string]map[string]struct{}{}
	}
	count := 0
	userSet := map[string]struct{}{}
	sourceSet := map[string]struct{}{}
	failBySource := map[string]int{}
	usersBySource := map[string]map[string]struct{}{}

	for _, ln := range lines {
		line := strings.TrimSpace(ln)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		isFailure := strings.Contains(lower, "failed password") ||
			strings.Contains(lower, "invalid user") ||
			strings.Contains(lower, "authentication failure")
		if !isFailure {
			continue
		}
		count++

		user, src := parseSSHFailureLine(line)
		if user != "" {
			userSet[user] = struct{}{}
		}
		if src != "" {
			sourceSet[src] = struct{}{}
			failBySource[src]++
			if _, ok := usersBySource[src]; !ok {
				usersBySource[src] = map[string]struct{}{}
			}
			if user != "" {
				usersBySource[src][user] = struct{}{}
			}
		}
	}
	return count, sortedStringSet(userSet), sortedStringSet(sourceSet), failBySource, usersBySource
}

func parseSSHFailureLine(line string) (string, string) {
	line = strings.TrimSpace(line)
	lower := strings.ToLower(line)
	if line == "" {
		return "", ""
	}

	user := ""
	src := ""

	if idx := strings.Index(lower, "failed password for"); idx >= 0 {
		tail := line[idx+len("failed password for"):]
		tailLower := strings.ToLower(tail)
		if strings.HasPrefix(strings.TrimSpace(tailLower), "invalid user ") {
			parts := strings.SplitN(strings.TrimSpace(tail), " ", 3)
			if len(parts) >= 3 {
				tail = parts[2]
			}
		}
		if i := strings.Index(strings.ToLower(tail), " from "); i > 0 {
			user = normalizeUsername(tail[:i])
			if xs := strings.Fields(tail[i+len(" from "):]); len(xs) > 0 {
				src = normalizeIP(xs[0])
			}
		}
	}

	if user == "" {
		if idx := strings.Index(lower, "invalid user "); idx >= 0 {
			tail := line[idx+len("invalid user "):]
			if i := strings.Index(strings.ToLower(tail), " from "); i > 0 {
				user = normalizeUsername(tail[:i])
				if xs := strings.Fields(tail[i+len(" from "):]); len(xs) > 0 {
					src = normalizeIP(xs[0])
				}
			}
		}
	}

	if src == "" {
		if idx := strings.Index(lower, " from "); idx >= 0 {
			tail := strings.TrimSpace(line[idx+len(" from "):])
			if len(tail) > 0 {
				src = normalizeIP(strings.Fields(tail)[0])
			}
		}
	}

	if user == "" {
		if idx := strings.Index(lower, " user="); idx >= 0 {
			tail := strings.TrimSpace(line[idx+len(" user="):])
			if len(tail) > 0 {
				user = normalizeUsername(strings.Fields(tail)[0])
			}
		}
	}

	return user, src
}

func loadSSHJournalLines(ctx context.Context) []string {
	cmds := [][]string{
		{"journalctl", "--no-pager", "--since", "-5m", "-o", "cat", "_COMM=sshd"},
		{"journalctl", "--no-pager", "--since", "-5m", "-o", "cat", "SYSLOG_IDENTIFIER=sshd"},
		{"journalctl", "--no-pager", "--since", "-5m", "-o", "cat", "-u", "ssh", "-u", "sshd"},
	}
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, args := range cmds {
		if len(args) == 0 {
			continue
		}
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		raw, err := cmd.Output()
		if err != nil || len(raw) == 0 {
			continue
		}
		for _, ln := range strings.Split(string(raw), "\n") {
			line := strings.TrimSpace(ln)
			if line == "" {
				continue
			}
			if _, ok := seen[line]; ok {
				continue
			}
			seen[line] = struct{}{}
			out = append(out, line)
		}
		if len(out) > 0 {
			break
		}
	}
	return out
}

func detectPortScan(ctx context.Context) (bool, []string, []int) {
	out, err := exec.CommandContext(ctx, "ss", "-Htan").Output()
	if err != nil || len(out) == 0 {
		return false, nil, nil
	}
	suspiciousStates := map[string]struct{}{
		"SYN-RECV": {},
		"SYN-SENT": {},
	}
	portsBySource := map[string]map[int]struct{}{}
	connsBySource := map[string]int{}

	for _, ln := range strings.Split(string(out), "\n") {
		line := strings.TrimSpace(ln)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		state := strings.ToUpper(strings.TrimSpace(fields[0]))
		if _, ok := suspiciousStates[state]; !ok {
			continue
		}
		local := strings.TrimSpace(fields[len(fields)-2])
		peer := strings.TrimSpace(fields[len(fields)-1])
		srcIP, _, okPeer := splitHostPort(peer)
		_, dstPort, okLocal := splitHostPort(local)
		if !okPeer || !okLocal || dstPort <= 0 {
			continue
		}
		srcIP = normalizeIP(srcIP)
		if srcIP == "" {
			continue
		}
		if ip := net.ParseIP(srcIP); ip != nil && ip.IsLoopback() {
			continue
		}
		if _, ok := portsBySource[srcIP]; !ok {
			portsBySource[srcIP] = map[int]struct{}{}
		}
		portsBySource[srcIP][dstPort] = struct{}{}
		connsBySource[srcIP]++
	}

	suspiciousSources := make([]string, 0)
	portSet := map[int]struct{}{}
	for src, ports := range portsBySource {
		if len(ports) >= portScanDistinctPortThreshold || connsBySource[src] >= portScanConnThreshold {
			suspiciousSources = append(suspiciousSources, src)
			for p := range ports {
				portSet[p] = struct{}{}
			}
		}
	}
	if len(suspiciousSources) == 0 {
		return false, nil, nil
	}
	sort.Strings(suspiciousSources)
	ports := make([]int, 0, len(portSet))
	for p := range portSet {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	if len(ports) > 50 {
		ports = ports[:50]
	}
	return true, suspiciousSources, ports
}

func splitHostPort(endpoint string) (string, int, bool) {
	ep := strings.TrimSpace(endpoint)
	if ep == "" {
		return "", 0, false
	}
	ep = strings.Trim(ep, "[]")
	idx := strings.LastIndex(ep, ":")
	if idx <= 0 || idx >= len(ep)-1 {
		return "", 0, false
	}
	host := strings.TrimSpace(ep[:idx])
	portStr := strings.TrimSpace(ep[idx+1:])
	if host == "" || host == "*" {
		host = ""
	}
	if i := strings.Index(host, "%"); i > 0 {
		host = host[:i]
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		return "", 0, false
	}
	return host, port, true
}

func normalizeUsername(raw string) string {
	u := strings.TrimSpace(raw)
	u = strings.Trim(u, "[],:;()")
	u = strings.TrimPrefix(strings.ToLower(u), "user=")
	if u == "" || u == "invalid" || u == "unknown" {
		return ""
	}
	if strings.ContainsAny(u, " \t\n\r") {
		return ""
	}
	return u
}

func normalizeIP(raw string) string {
	ip := strings.TrimSpace(raw)
	ip = strings.Trim(ip, "[],:;()")
	if strings.HasPrefix(ip, "::ffff:") {
		ip = strings.TrimPrefix(ip, "::ffff:")
	}
	if parsed := net.ParseIP(ip); parsed != nil {
		return parsed.String()
	}
	return ""
}

func sortedStringSet(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for v := range set {
		if strings.TrimSpace(v) == "" {
			continue
		}
		out = append(out, v)
	}
	sort.Strings(out)
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}
