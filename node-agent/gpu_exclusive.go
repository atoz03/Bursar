package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const gpuExclusiveMarkerGroup = "gpu_exclusive_user"

type gpuExclusiveState struct {
	Enabled      bool                     `json:"enabled"`
	Assignments  []GPUExclusiveAssignment `json:"assignments"`
	TouchedUsers []string                 `json:"touched_users"`
	UpdatedAt    string                   `json:"updated_at,omitempty"`
}

func uniqTrimLocal(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func (a *NodeAgent) gpuExclusiveStatePath() string {
	base := strings.TrimSpace(a.stateDir)
	if base == "" {
		base = "/var/lib/gpu-node-agent"
	}
	return filepath.Join(base, "gpu_exclusive_state.json")
}

func normalizeGPUExclusiveAssignments(in []GPUExclusiveAssignment) []GPUExclusiveAssignment {
	perUser := map[string]map[int]struct{}{}
	for _, item := range in {
		u := strings.TrimSpace(item.Username)
		if u == "" {
			continue
		}
		if _, ok := perUser[u]; !ok {
			perUser[u] = map[int]struct{}{}
		}
		for _, idx := range item.GPUIndices {
			if idx < 0 {
				continue
			}
			perUser[u][idx] = struct{}{}
		}
	}
	users := make([]string, 0, len(perUser))
	for u := range perUser {
		users = append(users, u)
	}
	sort.Strings(users)
	out := make([]GPUExclusiveAssignment, 0, len(users))
	for _, u := range users {
		gset := perUser[u]
		idxs := make([]int, 0, len(gset))
		for idx := range gset {
			idxs = append(idxs, idx)
		}
		sort.Ints(idxs)
		out = append(out, GPUExclusiveAssignment{
			Username:   u,
			GPUIndices: idxs,
		})
	}
	return out
}

func parseNvidiaIndex(path string) (int, bool) {
	base := filepath.Base(strings.TrimSpace(path))
	if !strings.HasPrefix(base, "nvidia") {
		return 0, false
	}
	s := strings.TrimPrefix(base, "nvidia")
	if s == "" {
		return 0, false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func detectNvidiaIndexedDeviceNodes() map[int]string {
	matches, _ := filepath.Glob("/dev/nvidia*")
	out := map[int]string{}
	for _, path := range matches {
		idx, ok := parseNvidiaIndex(path)
		if !ok {
			continue
		}
		out[idx] = path
	}
	return out
}

func listDeviceIndices(devices map[int]string) []int {
	idxs := make([]int, 0, len(devices))
	for idx := range devices {
		idxs = append(idxs, idx)
	}
	sort.Ints(idxs)
	return idxs
}

func userExists(ctx context.Context, username string) bool {
	username = strings.TrimSpace(username)
	if username == "" {
		return false
	}
	return exec.CommandContext(ctx, "id", "-u", username).Run() == nil
}

func isSetfaclNoEntryError(out []byte, err error) bool {
	msg := strings.ToLower(strings.TrimSpace(string(out)))
	if err != nil {
		msg = msg + " " + strings.ToLower(err.Error())
	}
	return strings.Contains(msg, "no such attribute") ||
		strings.Contains(msg, "invalid argument")
}

func setGPUDeviceACLForUser(ctx context.Context, username string, devices map[int]string, deny map[int]struct{}) error {
	if !hasCommand("setfacl") {
		return fmt.Errorf("未检测到 setfacl 命令")
	}
	if !userExists(ctx, username) {
		return nil
	}
	var firstErr error
	for idx, path := range devices {
		if _, needDeny := deny[idx]; needDeny {
			out, err := exec.CommandContext(ctx, "setfacl", "-m", "u:"+username+":---", path).CombinedOutput()
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("setfacl deny 失败：user=%s gpu=%d node=%s err=%w out=%s", username, idx, path, err, strings.TrimSpace(string(out)))
			}
			continue
		}
		out, err := exec.CommandContext(ctx, "setfacl", "-x", "u:"+username, path).CombinedOutput()
		if err != nil && !isSetfaclNoEntryError(out, err) && firstErr == nil {
			firstErr = fmt.Errorf("setfacl clear 失败：user=%s gpu=%d node=%s err=%w out=%s", username, idx, path, err, strings.TrimSpace(string(out)))
		}
	}
	return firstErr
}

func (a *NodeAgent) writeGPUExclusiveState(st gpuExclusiveState) error {
	path := a.gpuExclusiveStatePath()
	st.Assignments = normalizeGPUExclusiveAssignments(st.Assignments)
	st.TouchedUsers = uniqTrimLocal(st.TouchedUsers)
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

func (a *NodeAgent) loadGPUExclusiveState() (gpuExclusiveState, bool, error) {
	path := a.gpuExclusiveStatePath()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return gpuExclusiveState{}, false, nil
		}
		return gpuExclusiveState{}, false, err
	}
	var st gpuExclusiveState
	if err := json.Unmarshal(b, &st); err != nil {
		return gpuExclusiveState{}, false, err
	}
	st.Assignments = normalizeGPUExclusiveAssignments(st.Assignments)
	st.TouchedUsers = uniqTrimLocal(st.TouchedUsers)
	return st, true, nil
}

func (a *NodeAgent) clearGPUExclusiveState() error {
	path := a.gpuExclusiveStatePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func denySetFromIndices(indices []int) map[int]struct{} {
	out := make(map[int]struct{}, len(indices))
	for _, idx := range indices {
		if idx >= 0 {
			out[idx] = struct{}{}
		}
	}
	return out
}

func (a *NodeAgent) setGPUExclusivePolicy(ctx context.Context, enabled bool, assignments []GPUExclusiveAssignment, reason string) error {
	assignments = normalizeGPUExclusiveAssignments(assignments)
	prevState, _, _ := a.loadGPUExclusiveState()
	devices := detectNvidiaIndexedDeviceNodes()
	allIndices := listDeviceIndices(devices)
	manualAllowMap := a.loadGPUVisibilityAllowMap()
	exemptUsers := loadSSHExemptUsers()
	overdraftBlocked := map[string]struct{}{}
	for _, u := range loadGroupMembers(gpuOverdraftBlockedGroup) {
		overdraftBlocked[strings.TrimSpace(u)] = struct{}{}
	}

	touchedUsers := uniqTrimLocal(append(loadHomeUsers(), prevState.TouchedUsers...))
	for _, item := range assignments {
		touchedUsers = append(touchedUsers, strings.TrimSpace(item.Username))
	}
	touchedUsers = uniqTrimLocal(touchedUsers)

	if !enabled || len(assignments) == 0 {
		for _, u := range touchedUsers {
			if strings.TrimSpace(u) == "" || strings.EqualFold(u, "root") {
				continue
			}
			if _, ok := exemptUsers[u]; ok {
				continue
			}
			deny := map[int]struct{}{}
			if _, blocked := overdraftBlocked[u]; blocked {
				deny = denySetFromIndices(allIndices)
			} else if allow, ok := manualAllowMap[u]; ok {
				deny = denyByAllowSet(allIndices, allow)
			}
			if err := setGPUDeviceACLForUser(ctx, u, devices, deny); err != nil {
				a.logger.Printf("清理 GPU 独享 ACL 失败：user=%s err=%v", u, err)
			}
			if err := removeUserFromGroup(ctx, u, gpuExclusiveMarkerGroup); err != nil {
				a.logger.Printf("清理独享标记组失败：user=%s err=%v", u, err)
			}
		}
		if err := a.clearGPUExclusiveState(); err != nil {
			a.logger.Printf("清理 GPU 独享状态失败：%v", err)
		}
		a.logger.Printf("执行 set_gpu_exclusive：enabled=false touched_users=%d reason=%s", len(touchedUsers), strings.TrimSpace(reason))
		return nil
	}

	userAllow := map[string]map[int]struct{}{}
	assignedUsers := map[string]struct{}{}
	unionAssigned := map[int]struct{}{}
	for _, item := range assignments {
		u := strings.TrimSpace(item.Username)
		if u == "" {
			continue
		}
		assignedUsers[u] = struct{}{}
		if _, ok := userAllow[u]; !ok {
			userAllow[u] = map[int]struct{}{}
		}
		for _, idx := range item.GPUIndices {
			if idx < 0 {
				continue
			}
			if _, ok := devices[idx]; !ok {
				continue
			}
			userAllow[u][idx] = struct{}{}
			unionAssigned[idx] = struct{}{}
		}
	}

	if err := ensureSystemGroup(ctx, gpuExclusiveMarkerGroup); err != nil {
		a.logger.Printf("确保独享标记组失败：group=%s err=%v", gpuExclusiveMarkerGroup, err)
	}

	for _, u := range touchedUsers {
		u = strings.TrimSpace(u)
		if u == "" || strings.EqualFold(u, "root") {
			continue
		}
		if _, exempted := exemptUsers[u]; exempted {
			// 豁免账号始终可见全部 GPU，不受独享规则影响。
			if err := setGPUDeviceACLForUser(ctx, u, devices, map[int]struct{}{}); err != nil {
				a.logger.Printf("豁免用户清理 GPU ACL 失败：user=%s err=%v", u, err)
			}
			_ = removeUserFromGroup(ctx, u, gpuExclusiveMarkerGroup)
			continue
		}
		if _, isAssigned := assignedUsers[u]; isAssigned {
			_ = addUserToGroup(ctx, u, gpuExclusiveMarkerGroup)
		} else {
			_ = removeUserFromGroup(ctx, u, gpuExclusiveMarkerGroup)
		}

		var deny map[int]struct{}
		if _, blocked := overdraftBlocked[u]; blocked {
			deny = denySetFromIndices(allIndices)
		} else if allow, ok := userAllow[u]; ok {
			deny = denyByAllowSet(allIndices, allow)
		} else {
			deny = map[int]struct{}{}
			for idx := range unionAssigned {
				deny[idx] = struct{}{}
			}
		}
		if manualAllow, ok := manualAllowMap[u]; ok && len(manualAllow) > 0 {
			manualDeny := denyByAllowSet(allIndices, manualAllow)
			for idx := range manualDeny {
				deny[idx] = struct{}{}
			}
		}

		if err := setGPUDeviceACLForUser(ctx, u, devices, deny); err != nil {
			a.logger.Printf("设置 GPU 独享 ACL 失败：user=%s err=%v", u, err)
		}
	}

	if err := a.writeGPUExclusiveState(gpuExclusiveState{
		Enabled:      true,
		Assignments:  assignments,
		TouchedUsers: touchedUsers,
	}); err != nil {
		return fmt.Errorf("写 GPU 独享状态失败：%w", err)
	}
	a.logger.Printf("执行 set_gpu_exclusive：enabled=true assignments=%d touched_users=%d reason=%s", len(assignments), len(touchedUsers), strings.TrimSpace(reason))
	return nil
}

func (a *NodeAgent) reconcilePersistedGPUExclusive(ctx context.Context) {
	st, ok, err := a.loadGPUExclusiveState()
	if err != nil {
		a.logger.Printf("读取 GPU 独享状态失败：%v", err)
		return
	}
	if !ok {
		return
	}
	restoreCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := a.setGPUExclusivePolicy(restoreCtx, st.Enabled, st.Assignments, "agent 启动恢复 GPU 独享策略"); err != nil {
		a.logger.Printf("恢复 GPU 独享策略失败：%v", err)
	}
}
