package main

import (
	"bufio"
	"context"
	"os/exec"
	"strings"
	"time"
)

func defaultSystemServiceUnits() []string {
	return []string{
		"gpu-node-agent.service",
		"gpu-ssh-guard-sync.timer",
		"gpu-ssh-guard-enforce.timer",
	}
}

func normalizeSystemServiceUnitList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', '\n', '\r', '\t', ' ':
			return true
		default:
			return false
		}
	})
	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		unit := strings.TrimSpace(field)
		if unit == "" {
			continue
		}
		if _, ok := seen[unit]; ok {
			continue
		}
		seen[unit] = struct{}{}
		out = append(out, unit)
	}
	return out
}

func cloneSystemServiceStatuses(in []NodeSystemServiceStatus) []NodeSystemServiceStatus {
	if len(in) == 0 {
		return nil
	}
	out := make([]NodeSystemServiceStatus, len(in))
	copy(out, in)
	return out
}

func (a *NodeAgent) collectSystemServicesCached(ctx context.Context) (string, []NodeSystemServiceStatus) {
	now := time.Now()

	a.cacheMu.Lock()
	cached := cloneSystemServiceStatuses(a.systemServicesCache)
	cachedAt := a.systemServicesCachedAt
	refresh := a.systemServicesRefreshInterval
	a.cacheMu.Unlock()

	if refresh <= 0 {
		refresh = 30 * time.Minute
	}
	if !cachedAt.IsZero() && now.Sub(cachedAt) < refresh {
		if len(cached) == 0 {
			return "", nil
		}
		return formatRFC3339InBeijing(cachedAt), cached
	}

	freshAt := now
	fresh := a.collectSystemServices(ctx)
	a.cacheMu.Lock()
	a.systemServicesCache = cloneSystemServiceStatuses(fresh)
	a.systemServicesCachedAt = freshAt
	a.cacheMu.Unlock()

	if len(fresh) == 0 {
		return "", nil
	}
	return formatRFC3339InBeijing(freshAt), fresh
}

func (a *NodeAgent) collectSystemServices(ctx context.Context) []NodeSystemServiceStatus {
	units := a.systemServiceUnits
	if len(units) == 0 {
		units = defaultSystemServiceUnits()
	}
	units = normalizeSystemServiceUnitList(strings.Join(units, " "))
	if len(units) == 0 {
		return nil
	}
	if !isSystemd() || !hasCommand("systemctl") {
		return nil
	}

	collectCtx := ctx
	var cancel context.CancelFunc
	if a.systemServicesCollectTimeout > 0 {
		collectCtx, cancel = context.WithTimeout(ctx, a.systemServicesCollectTimeout)
		defer cancel()
	}

	out := make([]NodeSystemServiceStatus, 0, len(units))
	for _, unit := range units {
		out = append(out, inspectSystemService(collectCtx, unit))
	}
	return out
}

func inspectSystemService(ctx context.Context, unit string) NodeSystemServiceStatus {
	status := NodeSystemServiceStatus{Name: strings.TrimSpace(unit)}
	if status.Name == "" {
		return status
	}
	props, err := systemctlShowProperties(ctx, status.Name)
	if err != nil {
		status.LoadState = "unknown"
		status.ActiveState = "unknown"
		status.SubState = "error"
		status.UnitFileState = "unknown"
		return status
	}

	status.LoadState = props["LoadState"]
	status.UnitFileState = props["UnitFileState"]
	status.ActiveState = props["ActiveState"]
	status.SubState = props["SubState"]
	status.Deployed = !strings.EqualFold(status.LoadState, "not-found")
	status.Healthy = strings.EqualFold(status.ActiveState, "active")
	return status
}

func systemctlShowProperties(ctx context.Context, unit string) (map[string]string, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "show", unit, "--property=LoadState,UnitFileState,ActiveState,SubState", "--no-pager")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			out = ee.Stderr
		} else {
			return nil, err
		}
	}

	props := make(map[string]string, 4)
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		props[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if len(props) == 0 && err != nil {
		return nil, err
	}
	return props, nil
}
