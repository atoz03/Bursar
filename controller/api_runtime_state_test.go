package main

import "testing"

func TestNodeRuntimePolicyStateClearedWhenAgentSessionChanges(t *testing.T) {
	s := NewServer(Config{}, nil)

	if _, changed, firstObserved := s.noteAgentSession("node-01", "session-a"); changed || !firstObserved {
		t.Fatal("first observed agent session should not be treated as a change")
	}
	if _, ok := s.nextCPUQuotaAction("node-01", "testuser", 10, "manual", false); !ok {
		t.Fatal("expected initial cpu quota action")
	}
	if _, ok := s.nextMemoryLimitAction("node-01", "testuser", 5, "manual", false); !ok {
		t.Fatal("expected initial memory limit action")
	}
	if _, ok := s.nextGPUAccessAction("node-01", "testuser", true, "blocked", false); !ok {
		t.Fatal("expected initial gpu access action")
	}
	if _, ok := s.nextGPUVisibilityAction("node-01", "testuser", []int{0}, false, "manual", false); !ok {
		t.Fatal("expected initial gpu visibility action")
	}

	if previous, changed, firstObserved := s.noteAgentSession("node-01", "session-b"); !changed || firstObserved || previous != "session-a" {
		t.Fatalf("agent session change not detected, previous=%q changed=%v", previous, changed)
	}
	s.clearNodeRuntimePolicyState("node-01")

	if _, ok := s.nextCPUQuotaAction("node-01", "testuser", 10, "manual", false); !ok {
		t.Fatal("expected cpu quota to be re-enqueued after session change")
	}
	if _, ok := s.nextMemoryLimitAction("node-01", "testuser", 5, "manual", false); !ok {
		t.Fatal("expected memory limit to be re-enqueued after session change")
	}
	if _, ok := s.nextGPUAccessAction("node-01", "testuser", true, "blocked", false); !ok {
		t.Fatal("expected gpu access to be re-enqueued after session change")
	}
	if _, ok := s.nextGPUVisibilityAction("node-01", "testuser", []int{0}, false, "manual", false); !ok {
		t.Fatal("expected gpu visibility to be re-enqueued after session change")
	}
}

func TestNodeRuntimePolicyStateClearedWhenFirstAgentSessionSeesLegacyCache(t *testing.T) {
	s := NewServer(Config{}, nil)

	if _, ok := s.nextCPUQuotaAction("node-01", "testuser", 10, "manual", false); !ok {
		t.Fatal("expected initial cpu quota action")
	}
	if !s.hasNodeRuntimePolicyState("node-01") {
		t.Fatal("expected legacy runtime cache to exist")
	}

	if previous, changed, firstObserved := s.noteAgentSession("node-01", "session-a"); previous != "" || changed || !firstObserved {
		t.Fatalf("unexpected first session result previous=%q changed=%v firstObserved=%v", previous, changed, firstObserved)
	}
	s.clearNodeRuntimePolicyState("node-01")

	if s.hasNodeRuntimePolicyState("node-01") {
		t.Fatal("expected runtime cache to be cleared")
	}
	if _, ok := s.nextCPUQuotaAction("node-01", "testuser", 10, "manual", false); !ok {
		t.Fatal("expected cpu quota to be re-enqueued after clearing legacy cache")
	}
}

func TestNodeRuntimePolicyForceSyncCanSendClearActionsAfterSessionReset(t *testing.T) {
	s := NewServer(Config{}, nil)

	if _, ok := s.nextCPUQuotaAction("node-01", "testuser", 10, "manual", false); !ok {
		t.Fatal("expected initial cpu quota action")
	}
	if _, ok := s.nextGPUAccessAction("node-01", "testuser", true, "blocked", false); !ok {
		t.Fatal("expected initial gpu access action")
	}
	if _, ok := s.nextGPUVisibilityAction("node-01", "testuser", []int{0}, false, "manual", false); !ok {
		t.Fatal("expected initial gpu visibility action")
	}

	s.clearNodeRuntimePolicyState("node-01")

	if _, ok := s.nextCPUQuotaAction("node-01", "testuser", 0, "clear", false); ok {
		t.Fatal("unexpected non-forced cpu clear action after session reset")
	}
	if _, ok := s.nextGPUAccessAction("node-01", "testuser", false, "unblock", false); ok {
		t.Fatal("unexpected non-forced gpu unblock action after session reset")
	}
	if _, ok := s.nextGPUVisibilityAction("node-01", "testuser", nil, false, "clear", false); ok {
		t.Fatal("unexpected non-forced gpu visibility clear action after session reset")
	}

	if _, ok := s.nextCPUQuotaAction("node-01", "testuser", 0, "clear", true); !ok {
		t.Fatal("expected forced cpu clear action after session reset")
	}
	if _, ok := s.nextGPUAccessAction("node-01", "testuser", false, "unblock", true); !ok {
		t.Fatal("expected forced gpu unblock action after session reset")
	}
	if _, ok := s.nextGPUVisibilityAction("node-01", "testuser", nil, false, "clear", true); !ok {
		t.Fatal("expected forced gpu visibility clear action after session reset")
	}
}

// 「完全不可见」与「解除限制」都不带 gpu_indices。旧实现用空集合同时表示两者，
// 会把「完全不可见」当作「无限制」而放行，这里锁住二者必须互相区分。
func TestGPUVisibilityDenyAllIsDistinctFromClear(t *testing.T) {
	s := NewServer(Config{}, nil)

	action, ok := s.nextGPUVisibilityAction("node-01", "testuser", nil, true, "deny all", false)
	if !ok {
		t.Fatal("expected deny-all action to be enqueued")
	}
	if !action.GPUDenyAll {
		t.Fatal("deny-all action must carry GPUDenyAll=true")
	}
	// 旧版 agent 不认识 GPUDenyAll，只看 gpu_indices；必须带上不存在的哨兵编号，
	// 让旧版拒绝全部真实设备，而不是把空列表当作解除限制。
	if len(action.GPUIndices) != 1 || action.GPUIndices[0] != legacyGPUDenyAllSentinelIndex {
		t.Fatalf("deny-all action must carry only the legacy sentinel index, got %v", action.GPUIndices)
	}

	// 重复下发同一策略应被去重。
	if _, ok := s.nextGPUVisibilityAction("node-01", "testuser", nil, true, "deny all", false); ok {
		t.Fatal("unexpected duplicate deny-all action")
	}

	// 从「完全不可见」切到「解除限制」必须产生一次真实变更，
	// 否则节点会一直停留在拒绝状态。
	clearAction, ok := s.nextGPUVisibilityAction("node-01", "testuser", nil, false, "clear", false)
	if !ok {
		t.Fatal("expected clear action when switching away from deny-all")
	}
	if clearAction.GPUDenyAll {
		t.Fatal("clear action must carry GPUDenyAll=false")
	}

	// 反向切换同样必须被识别为变更。
	if _, ok := s.nextGPUVisibilityAction("node-01", "testuser", nil, true, "deny all again", false); !ok {
		t.Fatal("expected deny-all action when switching back from clear")
	}
}

func TestHasManualGPUVisibilityPolicyDetectsDenyAll(t *testing.T) {
	if hasManualGPUVisibilityPolicy(NodeUserGPUVisibility{}) {
		t.Fatal("empty policy must not be treated as a manual policy")
	}
	// 关键回归：DenyAll 策略的 GPUIndices 为空，不能被当作「无策略」。
	if !hasManualGPUVisibilityPolicy(NodeUserGPUVisibility{DenyAll: true}) {
		t.Fatal("deny-all policy must be treated as a manual policy")
	}
	if !hasManualGPUVisibilityPolicy(NodeUserGPUVisibility{GPUIndices: []int{1}}) {
		t.Fatal("allow-list policy must be treated as a manual policy")
	}
}
