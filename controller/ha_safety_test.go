package main

import (
	"testing"
	"time"
)

func TestValidateRemoteHAHostRejectsUnsafeTargets(t *testing.T) {
	for _, host := range []string{"", "localhost", "127.0.0.1", "::1", "<standby-ip>", "replace-with-host"} {
		if err := validateRemoteHAHost(host); err == nil {
			t.Fatalf("expected %q to be rejected", host)
		}
	}
	if err := validateRemoteHAHost("192.0.2.10"); err != nil {
		t.Fatalf("documentation-only remote IP should pass validation: %v", err)
	}
}

func TestBackupFresh(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	status := backupJobStatus{
		State:         "success",
		LastSuccessAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
	}
	if !backupFresh(status, 36*time.Hour, now) {
		t.Fatal("recent successful backup should be fresh")
	}
	status.LastSuccessAt = now.Add(-48 * time.Hour).Format(time.RFC3339)
	if backupFresh(status, 36*time.Hour, now) {
		t.Fatal("stale backup must not be fresh")
	}
	status.State = "failed"
	status.LastSuccessAt = now.Format(time.RFC3339)
	if backupFresh(status, 36*time.Hour, now) {
		t.Fatal("failed backup must not be fresh")
	}
}

func TestConstantTimeTokenEqual(t *testing.T) {
	if !constantTimeTokenEqual("secret-token", "secret-token") {
		t.Fatal("equal tokens should match")
	}
	for _, got := range []string{"", "secret", "secret-token-x"} {
		if constantTimeTokenEqual(got, "secret-token") {
			t.Fatalf("unexpected token match for %q", got)
		}
	}
}
