package main

import "testing"

func TestSummarizeUserNodeAccountReadiness(t *testing.T) {
	rows := []UserNodeAccount{
		{NodeID: "60001", LocalUsername: "ready", BillingUsername: "u1", IdentityAligned: true},
		{NodeID: "60002", LocalUsername: "init", BillingUsername: "u2", IdentityInitializing: true},
		{NodeID: "node-03", LocalUsername: "failed", BillingUsername: "u3"},
	}

	filteredAll, totalNotReady, totalInitializing, totalFailed := summarizeUserNodeAccountReadiness(rows, "all")
	if totalNotReady != 2 || totalInitializing != 1 || totalFailed != 1 {
		t.Fatalf("unexpected readiness summary: not_ready=%d initializing=%d failed=%d", totalNotReady, totalInitializing, totalFailed)
	}
	if len(filteredAll) != 2 {
		t.Fatalf("expected 2 not-ready rows, got %d", len(filteredAll))
	}

	filteredInitializing, _, _, _ := summarizeUserNodeAccountReadiness(rows, "initializing")
	if len(filteredInitializing) != 1 || filteredInitializing[0].LocalUsername != "init" {
		t.Fatalf("unexpected initializing rows: %+v", filteredInitializing)
	}

	filteredFailed, _, _, _ := summarizeUserNodeAccountReadiness(rows, "failed")
	if len(filteredFailed) != 1 || filteredFailed[0].LocalUsername != "failed" {
		t.Fatalf("unexpected failed rows: %+v", filteredFailed)
	}
}
