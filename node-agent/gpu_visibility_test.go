package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func writeFileForTest(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0640)
}

func newGPUVisibilityTestAgent(t *testing.T) *NodeAgent {
	t.Helper()
	return &NodeAgent{
		stateDir: t.TempDir(),
		logger:   log.New(io.Discard, "", 0),
	}
}

// 核心回归：「完全不可见」必须持久化为「存在策略、允许集合为空」，
// 而不是被当作「解除限制」删除掉。旧实现只看 gpu_indices 是否为空，
// 会把完全不可见写成「无策略」，重算后用户反而看到全部 GPU。
func TestGPUVisibilityDenyAllPersistsAsEmptyAllowSet(t *testing.T) {
	a := newGPUVisibilityTestAgent(t)

	st := gpuVisibilityState{DenyAllUsers: []string{"alice"}}
	if err := a.writeGPUVisibilityState(st); err != nil {
		t.Fatalf("write state: %v", err)
	}

	allowMap, trusted := a.loadGPUVisibilityAllowMap()
	if !trusted {
		t.Fatal("freshly written state must be trusted")
	}
	allow, ok := allowMap["alice"]
	if !ok {
		t.Fatal("deny-all user must be present in the allow map (policy exists)")
	}
	if len(allow) != 0 {
		t.Fatalf("deny-all user must have an empty allow set, got %v", allow)
	}

	// 空允许集合经 denyByAllowSet 必须拒绝全部设备。
	deny := denyByAllowSet([]int{0, 1, 2, 3}, allow)
	if len(deny) != 4 {
		t.Fatalf("empty allow set must deny every gpu, got deny=%v", deny)
	}
}

// 「没有策略」与「完全不可见」必须可区分：前者键不存在，后者键存在但集合为空。
func TestGPUVisibilityNoPolicyIsDistinctFromDenyAll(t *testing.T) {
	a := newGPUVisibilityTestAgent(t)

	if err := a.writeGPUVisibilityState(gpuVisibilityState{
		Assignments:  []GPUExclusiveAssignment{{Username: "bob", GPUIndices: []int{1}}},
		DenyAllUsers: []string{"alice"},
	}); err != nil {
		t.Fatalf("write state: %v", err)
	}

	allowMap, trusted := a.loadGPUVisibilityAllowMap()
	if !trusted {
		t.Fatal("state must be trusted")
	}
	if _, ok := allowMap["carol"]; ok {
		t.Fatal("user without a policy must be absent from the allow map")
	}
	alice, ok := allowMap["alice"]
	if !ok || len(alice) != 0 {
		t.Fatalf("alice must map to an empty allow set, ok=%v allow=%v", ok, alice)
	}
	bob, ok := allowMap["bob"]
	if !ok || len(bob) != 1 {
		t.Fatalf("bob must keep his allow list, ok=%v allow=%v", ok, bob)
	}
	if _, allowed := bob[1]; !allowed {
		t.Fatalf("bob must be allowed gpu 1, got %v", bob)
	}
}

// 状态文件损坏时必须 fail-closed：不能返回「可信的空策略」，
// 否则所有受限用户都会被静默恢复为全可见。
func TestGPUVisibilityCorruptStateIsNotTrusted(t *testing.T) {
	a := newGPUVisibilityTestAgent(t)

	if err := writeFileForTest(a.gpuVisibilityStatePath(), []byte("{not json")); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	if _, trusted := a.loadGPUVisibilityAllowMap(); trusted {
		t.Fatal("corrupt state must not be reported as trusted")
	}
}

// 缺失状态文件是合法的「无任何策略」，应当可信。
func TestGPUVisibilityMissingStateIsTrusted(t *testing.T) {
	a := newGPUVisibilityTestAgent(t)

	allowMap, trusted := a.loadGPUVisibilityAllowMap()
	if !trusted {
		t.Fatal("missing state file must be trusted as 'no policy'")
	}
	if len(allowMap) != 0 {
		t.Fatalf("missing state must yield an empty map, got %v", allowMap)
	}
}

// 旧版 agent 不认识 gpu_deny_all，只按 gpu_indices 计算允许集合；控制器附带的哨兵编号
// 不对应任何真实设备，因此旧版也必须拒绝全部 GPU，而不是解除限制。
func TestLegacyDenyAllSentinelDeniesEveryGPU(t *testing.T) {
	allow := map[int]struct{}{}
	for _, idx := range normalizeGPUVisibilityIndices([]int{65535}) {
		allow[idx] = struct{}{}
	}
	if len(allow) != 1 {
		t.Fatalf("sentinel index must survive normalization, got %v", allow)
	}
	if deny := denyByAllowSet([]int{0, 1, 2, 3, 4, 5, 6, 7}, allow); len(deny) != 8 {
		t.Fatalf("sentinel allow set must deny every gpu, got deny=%v", deny)
	}
}

func TestWritePolicyStateFileReplacesAtomicallyAndTightensMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gpu_visibility_state.json")
	if err := os.WriteFile(path, []byte("{\"stale\":true}"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := writePolicyStateFile(path, []byte("{}")); err != nil {
		t.Fatalf("write: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "{}" {
		t.Fatalf("unexpected content %q err=%v", body, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0640 {
		t.Fatalf("mode=%o, want 640", mode)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v err=%v", entries, err)
	}
}
