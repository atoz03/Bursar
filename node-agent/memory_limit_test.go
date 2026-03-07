package main

import "testing"

func TestMemoryLimitBytes(t *testing.T) {
	if got := memoryLimitBytes(0); got != 0 {
		t.Fatalf("memoryLimitBytes(0)=%d want=0", got)
	}
	if got := memoryLimitBytes(0.001); got != 64*1024*1024 {
		t.Fatalf("memoryLimitBytes(min)=%d want=%d", got, 64*1024*1024)
	}
	if got := memoryLimitBytes(1.5); got != 1610612736 {
		t.Fatalf("memoryLimitBytes(1.5)=%d want=%d", got, 1610612736)
	}
}
