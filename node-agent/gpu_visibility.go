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
	// DenyAllUsers 保存「完全不可见」的用户。这类策略的允许集合为空，
	// 无法用 Assignments 表达（空集合会被当作「无限制」而放行），
	// 因此单独记录。
	DenyAllUsers []string `json:"deny_all_users,omitempty"`
	UpdatedAt    string   `json:"updated_at,omitempty"`
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
	st.DenyAllUsers = uniqTrimLocal(st.DenyAllUsers)
	st.UpdatedAt = formatRFC3339InBeijing(nowInBeijing())
	body, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	// 0640：策略文件会暴露哪些用户被限制了哪些 GPU，无需对普通用户可读。
	return os.WriteFile(path, body, 0640)
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
	st.DenyAllUsers = uniqTrimLocal(st.DenyAllUsers)
	return st, true, nil
}

func (a *NodeAgent) clearGPUVisibilityState() error {
	path := a.gpuVisibilityStatePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// loadGPUVisibilityAllowMap 返回每个用户的允许集合。
// 「完全不可见」的用户对应一个空的允许集合（存在键、集合为空），
// 与「没有策略」（键不存在）严格区分——调用方必须用 comma-ok 判断，
// 不能用 len(allow) > 0，否则会把「完全不可见」误判为「无限制」。
// 第二个返回值报告状态是否可信：读取或解析失败时为 false，
// 调用方应保持现有限制而不是放行。
func (a *NodeAgent) loadGPUVisibilityAllowMap() (map[string]map[int]struct{}, bool) {
	st, ok, err := a.loadGPUVisibilityState()
	if err != nil {
		// fail-closed：状态文件损坏时不能静默恢复全可见。
		a.logger.Printf("读取 GPU 可见限制状态失败，保持现有限制：%v", err)
		return map[string]map[int]struct{}{}, false
	}
	if !ok {
		return map[string]map[int]struct{}{}, true
	}
	out := make(map[string]map[int]struct{}, len(st.Assignments)+len(st.DenyAllUsers))
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
	for _, u := range st.DenyAllUsers {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		// 空允许集合 = 一张都不可见。放在最后覆盖，deny_all 优先。
		out[u] = map[int]struct{}{}
	}
	return out, true
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
	allowMap, trusted := a.loadGPUVisibilityAllowMap()
	if !trusted {
		// 状态不可信时保持现状，避免把「完全不可见」降级为「全可见」。
		return fmt.Errorf("GPU 可见限制状态不可信，跳过 user=%s 的重算", username)
	}
	allow, hasAllow := allowMap[username]
	overdraftBlocked := map[string]struct{}{}
	for _, u := range loadGroupMembers(gpuOverdraftBlockedGroup) {
		overdraftBlocked[strings.TrimSpace(u)] = struct{}{}
	}
	deny := map[int]struct{}{}
	if _, blocked := overdraftBlocked[username]; blocked {
		deny = denySetFromIndices(allIndices)
	} else if hasAllow {
		// allow 为空集合即「完全不可见」，denyByAllowSet 会拒绝全部设备。
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

// setUserGPUVisibility 更新单个用户的 GPU 可见性策略。
// denyAll=true 表示「完全不可见」；denyAll=false 且 gpuIndices 为空表示「解除限制」。
// 二者必须由调用方显式区分——历史实现只看 gpuIndices 是否为空，
// 导致「完全不可见」被当成「解除限制」并恢复全可见。
func (a *NodeAgent) setUserGPUVisibility(ctx context.Context, username string, gpuIndices []int, denyAll bool, reason string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username 不能为空")
	}
	if !localUsernamePattern.MatchString(username) {
		return fmt.Errorf("username 不合法：%q", username)
	}
	if strings.EqualFold(username, "root") {
		return nil
	}
	normalized := normalizeGPUVisibilityIndices(gpuIndices)
	if denyAll && len(normalized) > 0 {
		return fmt.Errorf("完全不可见时不能同时指定 gpu_indices：user=%s", username)
	}
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
	denyAllUsers := map[string]struct{}{}
	for _, u := range st.DenyAllUsers {
		if u = strings.TrimSpace(u); u != "" {
			denyAllUsers[u] = struct{}{}
		}
	}
	// 三种目标状态互斥，先清掉该用户的旧策略再写入新的。
	delete(perUser, username)
	delete(denyAllUsers, username)
	switch {
	case denyAll:
		denyAllUsers[username] = struct{}{}
	case len(normalized) > 0:
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
	denyList := make([]string, 0, len(denyAllUsers))
	for u := range denyAllUsers {
		denyList = append(denyList, u)
	}
	denyList = uniqTrimLocal(denyList)
	if len(next) == 0 && len(denyList) == 0 {
		if err := a.clearGPUVisibilityState(); err != nil {
			return err
		}
	} else {
		st.Assignments = next
		st.DenyAllUsers = denyList
		if err := a.writeGPUVisibilityState(st); err != nil {
			return err
		}
	}
	if err := a.reconcileGPUPoliciesForUser(ctx, username, "更新用户 GPU 可见限制："+strings.TrimSpace(reason)); err != nil {
		return err
	}
	a.logger.Printf("执行 set_gpu_visibility：user=%s deny_all=%v gpu_indices=%v reason=%s", username, denyAll, normalized, strings.TrimSpace(reason))
	return nil
}

func (a *NodeAgent) reconcilePersistedGPUVisibility(ctx context.Context) {
	st, ok, err := a.loadGPUVisibilityState()
	if err != nil {
		a.logger.Printf("读取 GPU 可见限制状态失败：%v", err)
		return
	}
	if !ok || (len(st.Assignments) == 0 && len(st.DenyAllUsers) == 0) {
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
	restoreUsers := make([]string, 0, len(st.Assignments)+len(st.DenyAllUsers))
	for _, item := range st.Assignments {
		restoreUsers = append(restoreUsers, item.Username)
	}
	// 「完全不可见」的用户不在 Assignments 里，必须一并恢复，
	// 否则 agent 重启后这些用户会重新看到全部 GPU。
	restoreUsers = append(restoreUsers, st.DenyAllUsers...)
	for _, u := range uniqTrimLocal(restoreUsers) {
		restoreCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		if err := a.applyCurrentGPUVisibilityForUser(restoreCtx, u); err != nil {
			a.logger.Printf("恢复 GPU 可见限制失败：user=%s err=%v", u, err)
		}
		cancel()
	}
}
