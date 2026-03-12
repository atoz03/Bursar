package main

import "testing"

func TestNodeRuntimePolicyStateClearedWhenAgentSessionChanges(t *testing.T) {
	s := NewServer(Config{}, nil)

	if _, changed, firstObserved := s.noteAgentSession("60020", "session-a"); changed || !firstObserved {
		t.Fatal("first observed agent session should not be treated as a change")
	}
	if _, ok := s.nextCPUQuotaAction("60020", "xqt", 10, "manual", false); !ok {
		t.Fatal("expected initial cpu quota action")
	}
	if _, ok := s.nextMemoryLimitAction("60020", "xqt", 5, "manual", false); !ok {
		t.Fatal("expected initial memory limit action")
	}
	if _, ok := s.nextGPUAccessAction("60020", "xqt", true, "blocked", false); !ok {
		t.Fatal("expected initial gpu access action")
	}
	if _, ok := s.nextGPUVisibilityAction("60020", "xqt", []int{0}, "manual", false); !ok {
		t.Fatal("expected initial gpu visibility action")
	}

	if previous, changed, firstObserved := s.noteAgentSession("60020", "session-b"); !changed || firstObserved || previous != "session-a" {
		t.Fatalf("agent session change not detected, previous=%q changed=%v", previous, changed)
	}
	s.clearNodeRuntimePolicyState("60020")

	if _, ok := s.nextCPUQuotaAction("60020", "xqt", 10, "manual", false); !ok {
		t.Fatal("expected cpu quota to be re-enqueued after session change")
	}
	if _, ok := s.nextMemoryLimitAction("60020", "xqt", 5, "manual", false); !ok {
		t.Fatal("expected memory limit to be re-enqueued after session change")
	}
	if _, ok := s.nextGPUAccessAction("60020", "xqt", true, "blocked", false); !ok {
		t.Fatal("expected gpu access to be re-enqueued after session change")
	}
	if _, ok := s.nextGPUVisibilityAction("60020", "xqt", []int{0}, "manual", false); !ok {
		t.Fatal("expected gpu visibility to be re-enqueued after session change")
	}
}

func TestNodeRuntimePolicyStateClearedWhenFirstAgentSessionSeesLegacyCache(t *testing.T) {
	s := NewServer(Config{}, nil)

	if _, ok := s.nextCPUQuotaAction("60020", "xqt", 10, "manual", false); !ok {
		t.Fatal("expected initial cpu quota action")
	}
	if !s.hasNodeRuntimePolicyState("60020") {
		t.Fatal("expected legacy runtime cache to exist")
	}

	if previous, changed, firstObserved := s.noteAgentSession("60020", "session-a"); previous != "" || changed || !firstObserved {
		t.Fatalf("unexpected first session result previous=%q changed=%v firstObserved=%v", previous, changed, firstObserved)
	}
	s.clearNodeRuntimePolicyState("60020")

	if s.hasNodeRuntimePolicyState("60020") {
		t.Fatal("expected runtime cache to be cleared")
	}
	if _, ok := s.nextCPUQuotaAction("60020", "xqt", 10, "manual", false); !ok {
		t.Fatal("expected cpu quota to be re-enqueued after clearing legacy cache")
	}
}

func TestNodeRuntimePolicyForceSyncCanSendClearActionsAfterSessionReset(t *testing.T) {
	s := NewServer(Config{}, nil)

	if _, ok := s.nextCPUQuotaAction("60020", "xqt", 10, "manual", false); !ok {
		t.Fatal("expected initial cpu quota action")
	}
	if _, ok := s.nextGPUAccessAction("60020", "xqt", true, "blocked", false); !ok {
		t.Fatal("expected initial gpu access action")
	}
	if _, ok := s.nextGPUVisibilityAction("60020", "xqt", []int{0}, "manual", false); !ok {
		t.Fatal("expected initial gpu visibility action")
	}

	s.clearNodeRuntimePolicyState("60020")

	if _, ok := s.nextCPUQuotaAction("60020", "xqt", 0, "clear", false); ok {
		t.Fatal("unexpected non-forced cpu clear action after session reset")
	}
	if _, ok := s.nextGPUAccessAction("60020", "xqt", false, "unblock", false); ok {
		t.Fatal("unexpected non-forced gpu unblock action after session reset")
	}
	if _, ok := s.nextGPUVisibilityAction("60020", "xqt", nil, "clear", false); ok {
		t.Fatal("unexpected non-forced gpu visibility clear action after session reset")
	}

	if _, ok := s.nextCPUQuotaAction("60020", "xqt", 0, "clear", true); !ok {
		t.Fatal("expected forced cpu clear action after session reset")
	}
	if _, ok := s.nextGPUAccessAction("60020", "xqt", false, "unblock", true); !ok {
		t.Fatal("expected forced gpu unblock action after session reset")
	}
	if _, ok := s.nextGPUVisibilityAction("60020", "xqt", nil, "clear", true); !ok {
		t.Fatal("expected forced gpu visibility clear action after session reset")
	}
}
