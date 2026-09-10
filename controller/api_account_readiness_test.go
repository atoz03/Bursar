package main

import (
	"strings"
	"testing"
	"time"
)

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

func TestClassifyUserNodeAccountIdentityReadyUsesExactTarget(t *testing.T) {
	now := time.Date(2026, 9, 10, 20, 0, 0, 0, beijingLocation)
	row := UserNodeAccount{
		NodeID:          "60010",
		LocalUsername:   "alice2",
		BillingUsername: "alice20260001",
		IdentityAligned: true,
	}
	classifyUserNodeAccountIdentity(&row, true, now)
	if row.IdentityState != "ready" || row.IdentityInitializing {
		t.Fatalf("aligned exact identity should be ready: %+v", row)
	}
}

func TestClassifyUserNodeAccountIdentityMissingRenamedTargetFailsAfterFreshSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 10, 20, 0, 0, 0, beijingLocation)
	mappingAt := now.Add(-time.Minute)
	snapshotAt := now.Add(-30 * time.Second)
	row := UserNodeAccount{
		NodeID:                "60010",
		LocalUsername:         "alice2",
		BillingUsername:       "alice20260001",
		CreatedAt:             mappingAt,
		UpdatedAt:             mappingAt,
		NodeSnapshotUpdatedAt: &snapshotAt,
	}
	classifyUserNodeAccountIdentity(&row, true, now)
	if row.IdentityState != "failed" || row.IdentityInitializing {
		t.Fatalf("missing exact renamed target after fresh snapshot should fail: %+v", row)
	}
	if !strings.Contains(row.IdentityError, "不会创建或重命名") {
		t.Fatalf("unexpected error: %q", row.IdentityError)
	}
}

func TestClassifyUserNodeAccountIdentityPendingJobStaysInitializing(t *testing.T) {
	now := time.Date(2026, 9, 10, 20, 0, 0, 0, beijingLocation)
	mappingAt := now.Add(-time.Minute)
	jobAt := now.Add(-30 * time.Second)
	row := UserNodeAccount{
		NodeID:                "60010",
		LocalUsername:         "alice2",
		BillingUsername:       "alice20260001",
		CreatedAt:             mappingAt,
		UpdatedAt:             mappingAt,
		ProvisionJobStatus:    "pending",
		ProvisionJobUpdatedAt: &jobAt,
	}
	classifyUserNodeAccountIdentity(&row, true, now)
	if row.IdentityState != "initializing" || !row.IdentityInitializing {
		t.Fatalf("pending durable action should be initializing: %+v", row)
	}
}

func TestClassifyUserNodeAccountIdentityFailedJobShowsNodeError(t *testing.T) {
	now := time.Date(2026, 9, 10, 20, 0, 0, 0, beijingLocation)
	mappingAt := now.Add(-5 * time.Minute)
	jobAt := now.Add(-time.Minute)
	row := UserNodeAccount{
		NodeID:                "60010",
		LocalUsername:         "alice2",
		BillingUsername:       "alice20260001",
		CreatedAt:             mappingAt,
		UpdatedAt:             mappingAt,
		ProvisionJobStatus:    "failed",
		ProvisionJobUpdatedAt: &jobAt,
		ProvisionLastError:    "目标 uid=20054 已被账号 alice 占用",
	}
	classifyUserNodeAccountIdentity(&row, true, now)
	if row.IdentityState != "failed" || row.IdentityError != row.ProvisionLastError {
		t.Fatalf("failed durable action should surface its error: %+v", row)
	}
}

func TestClassifyUserNodeAccountIdentityPendingUnclaimedEventuallyFails(t *testing.T) {
	now := time.Date(2026, 9, 10, 20, 0, 0, 0, beijingLocation)
	mappingAt := now.Add(-5 * time.Minute)
	jobAt := now.Add(-3 * time.Minute)
	snapshotAt := now.Add(-time.Minute)
	row := UserNodeAccount{
		NodeID:                "60010",
		LocalUsername:         "alice2",
		BillingUsername:       "alice20260001",
		CreatedAt:             mappingAt,
		UpdatedAt:             mappingAt,
		NodeSnapshotUpdatedAt: &snapshotAt,
		ProvisionJobStatus:    "pending",
		ProvisionJobUpdatedAt: &jobAt,
	}
	classifyUserNodeAccountIdentity(&row, true, now)
	if row.IdentityState != "failed" || !strings.Contains(row.IdentityError, "未领取") {
		t.Fatalf("online node not claiming a task should become an actionable failure: %+v", row)
	}
}

func TestClassifyUserNodeAccountIdentitySucceededWaitsForFreshSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 10, 20, 0, 0, 0, beijingLocation)
	mappingAt := now.Add(-time.Minute)
	completedAt := now.Add(-30 * time.Second)
	oldSnapshotAt := now.Add(-40 * time.Second)
	row := UserNodeAccount{
		NodeID:                "60010",
		LocalUsername:         "alice2",
		BillingUsername:       "alice20260001",
		CreatedAt:             mappingAt,
		UpdatedAt:             mappingAt,
		NodeSnapshotUpdatedAt: &oldSnapshotAt,
		ProvisionJobStatus:    "succeeded",
		ProvisionJobUpdatedAt: &completedAt,
		ProvisionCompletedAt:  &completedAt,
	}
	classifyUserNodeAccountIdentity(&row, true, now)
	if row.IdentityState != "initializing" || !row.IdentityInitializing {
		t.Fatalf("successful callback should allow a snapshot propagation window: %+v", row)
	}
}
