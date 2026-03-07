package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type gpuVisibilityState struct {
	Assignments []GPUExclusiveAssignment `json:"assignments"`
	UpdatedAt   string                   `json:"updated_at,omitempty"`
}

func normalizeGPUVisibilityIndices(indices []int) []int {
	set := make(map[int]struct{}, len(indices))
	for _, idx := range indices {
		if idx >= 0 {
			set[idx] = struct{}{}
		}
	}
	out := make([]int, 0, len(set))
	for idx := range set {
		out = append(out, idx)
	}
	sort.Ints(out)
	return out
}

func denyByAllowSet(allIndices []int, allow map[int]struct{}) map[int]struct{} {
	deny := make(map[int]struct{})
	for _, idx := range allIndices {
		if _, ok := allow[idx]; !ok {
			deny[idx] = struct{}{}
		}
	}
	return deny
}

func (a *NodeAgent) gpuVisibilityStatePath() string {
	base := strings.TrimSpace(a.stateDir)
	if base == "" {
		base = "/var/lib/gpu-node-agent"
	}
	return filepath.Join(base, "gpu_visibility_state.json")
}

func (a *NodeAgent) writeGPUVisibilityState(st gpuVisibilityState) error {
	path := a.gpuVisibilityStatePath()
	st.Assignments = normalizeGPUExclusiveAssignments(st.Assignments)
	st.UpdatedAt = formatRFC3339InBeijing(nowInBeijing())
	body, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0644)
}

func (a *NodeAgent) loadGPUVisibilityState() (gpuVisibilityState, bool, error) {
	path := a.gpuVisibilityStatePath()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return gpuVisibilityState{}, false, nil
		}
		return gpuVisibilityState{}, false, err
	}
	var st gpuVisibilityState
	if err := json.Unmarshal(b, &st); err != nil {
		return gpuVisibilityState{}, false, err
	}
	st.Assignments = normalizeGPUExclusiveAssignments(st.Assignments)
	return st, true, nil
}

func (a *NodeAgent) clearGPUVisibilityState() error {
	path := a.gpuVisibilityStatePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (a *NodeAgent) loadGPUVisibilityAllowMap() map[string]map[int]struct{} {
	st, ok, err := a.loadGPUVisibilityState()
	if err != nil || !ok {
		return map[string]map[int]struct{}{}
	}
	out := make(map[string]map[int]struct{}, len(st.Assignments))
	for _, item := range st.Assignments {
		u := strings.TrimSpace(item.Username)
		if u == "" {
			continue
		}
		allow := make(map[int]struct{}, len(item.GPUIndices))
		for _, idx := range item.GPUIndices {
			if idx >= 0 {
				allow[idx] = struct{}{}
			}
		}
		out[u] = allow
	}
	return out
}

func (a *NodeAgent) applyCurrentGPUVisibilityForUser(ctx context.Context, username string) error {
	username = strings.TrimSpace(username)
	if username == "" || strings.EqualFold(username, "root") {
		return nil
	}
	devices := detectNvidiaIndexedDeviceNodes()
	allIndices := listDeviceIndices(devices)
	if len(allIndices) == 0 {
		return nil
	}
	allowMap := a.loadGPUVisibilityAllowMap()
	allow, hasAllow := allowMap[username]
	overdraftBlocked := map[string]struct{}{}
	for _, u := range loadGroupMembers(gpuOverdraftBlockedGroup) {
		overdraftBlocked[strings.TrimSpace(u)] = struct{}{}
	}
	deny := map[int]struct{}{}
	if _, blocked := overdraftBlocked[username]; blocked {
		deny = denySetFromIndices(allIndices)
	} else if hasAllow {
		deny = denyByAllowSet(allIndices, allow)
	}
	return setGPUDeviceACLForUser(ctx, username, devices, deny)
}

func (a *NodeAgent) reconcileGPUPoliciesForUser(ctx context.Context, username string, reason string) error {
	if st, ok, err := a.loadGPUExclusiveState(); err == nil && ok && st.Enabled {
		return a.setGPUExclusivePolicy(ctx, st.Enabled, st.Assignments, reason)
	}
	return a.applyCurrentGPUVisibilityForUser(ctx, username)
}

func (a *NodeAgent) setUserGPUVisibility(ctx context.Context, username string, gpuIndices []int, reason string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username 不能为空")
	}
	if strings.EqualFold(username, "root") {
		return nil
	}
	normalized := normalizeGPUVisibilityIndices(gpuIndices)
	st, ok, err := a.loadGPUVisibilityState()
	if err != nil {
		return err
	}
	if !ok {
		st = gpuVisibilityState{}
	}
	perUser := map[string][]int{}
	for _, item := range st.Assignments {
		u := strings.TrimSpace(item.Username)
		if u == "" {
			continue
		}
		perUser[u] = normalizeGPUVisibilityIndices(item.GPUIndices)
	}
	if len(normalized) == 0 {
		delete(perUser, username)
	} else {
		perUser[username] = normalized
	}
	next := make([]GPUExclusiveAssignment, 0, len(perUser))
	for u, idxs := range perUser {
		next = append(next, GPUExclusiveAssignment{
			Username:   u,
			GPUIndices: idxs,
		})
	}
	next = normalizeGPUExclusiveAssignments(next)
	if len(next) == 0 {
		if err := a.clearGPUVisibilityState(); err != nil {
			return err
		}
	} else {
		st.Assignments = next
		if err := a.writeGPUVisibilityState(st); err != nil {
			return err
		}
	}
	if err := a.reconcileGPUPoliciesForUser(ctx, username, "更新用户 GPU 可见限制："+strings.TrimSpace(reason)); err != nil {
		return err
	}
	a.logger.Printf("执行 set_gpu_visibility：user=%s gpu_indices=%v reason=%s", username, normalized, strings.TrimSpace(reason))
	return nil
}

func (a *NodeAgent) reconcilePersistedGPUVisibility(ctx context.Context) {
	st, ok, err := a.loadGPUVisibilityState()
	if err != nil {
		a.logger.Printf("读取 GPU 可见限制状态失败：%v", err)
		return
	}
	if !ok || len(st.Assignments) == 0 {
		return
	}
	if ex, exOK, exErr := a.loadGPUExclusiveState(); exErr == nil && exOK && ex.Enabled {
		restoreCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := a.setGPUExclusivePolicy(restoreCtx, ex.Enabled, ex.Assignments, "agent 启动恢复 GPU 可见限制并合并独享策略"); err != nil {
			a.logger.Printf("恢复 GPU 可见限制（独享合并）失败：%v", err)
		}
		return
	}
	for _, item := range st.Assignments {
		u := strings.TrimSpace(item.Username)
		if u == "" {
			continue
		}
		restoreCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		if err := a.applyCurrentGPUVisibilityForUser(restoreCtx, u); err != nil {
			a.logger.Printf("恢复 GPU 可见限制失败：user=%s err=%v", u, err)
		}
		cancel()
	}
}
