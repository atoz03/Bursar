package main

import "testing"

func TestNextMemoryLimitAction_DedupAndRelease(t *testing.T) {
	s := NewServer(Config{}, nil)

	action, ok := s.nextMemoryLimitAction("n1", "alice", 12, "overdraft", false)
	if !ok {
		t.Fatal("expected first memory action")
	}
	if action.Type != "set_memory_limit" || action.MemoryLimitGB != 12 {
		t.Fatalf("unexpected action: %#v", action)
	}

	if _, ok := s.nextMemoryLimitAction("n1", "alice", 12, "overdraft", false); ok {
		t.Fatal("same memory limit should not enqueue twice")
	}

	release, ok := s.nextMemoryLimitAction("n1", "alice", 0, "recovered", false)
	if !ok {
		t.Fatal("expected release action")
	}
	if release.MemoryLimitGB != 0 {
		t.Fatalf("release action memory_limit_gb=%v want=0", release.MemoryLimitGB)
	}
}
