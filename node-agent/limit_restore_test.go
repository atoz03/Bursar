package main

import "testing"

func TestWriteCPUQuotaSystemdDropInTracksChanges(t *testing.T) {
	oldDir := systemdSystemDir
	systemdSystemDir = t.TempDir()
	defer func() {
		systemdSystemDir = oldDir
	}()

	changed, err := writeCPUQuotaSystemdDropIn(20000, 25)
	if err != nil {
		t.Fatalf("write cpu drop-in failed: %v", err)
	}
	if !changed {
		t.Fatal("expected first cpu drop-in write to be marked changed")
	}

	changed, err = writeCPUQuotaSystemdDropIn(20000, 25)
	if err != nil {
		t.Fatalf("rewrite cpu drop-in failed: %v", err)
	}
	if changed {
		t.Fatal("expected identical cpu drop-in write to be skipped")
	}

	changed, err = writeCPUQuotaSystemdDropIn(20000, 0)
	if err != nil {
		t.Fatalf("remove cpu drop-in failed: %v", err)
	}
	if !changed {
		t.Fatal("expected cpu drop-in removal to be marked changed")
	}

	changed, err = writeCPUQuotaSystemdDropIn(20000, 0)
	if err != nil {
		t.Fatalf("repeat cpu drop-in removal failed: %v", err)
	}
	if changed {
		t.Fatal("expected repeated cpu drop-in removal to be skipped")
	}
}

func TestWriteMemoryLimitSystemdDropInTracksChanges(t *testing.T) {
	oldDir := systemdSystemDir
	systemdSystemDir = t.TempDir()
	defer func() {
		systemdSystemDir = oldDir
	}()

	changed, err := writeMemoryLimitSystemdDropIn(20000, 8)
	if err != nil {
		t.Fatalf("write memory drop-in failed: %v", err)
	}
	if !changed {
		t.Fatal("expected first memory drop-in write to be marked changed")
	}

	changed, err = writeMemoryLimitSystemdDropIn(20000, 8)
	if err != nil {
		t.Fatalf("rewrite memory drop-in failed: %v", err)
	}
	if changed {
		t.Fatal("expected identical memory drop-in write to be skipped")
	}

	changed, err = writeMemoryLimitSystemdDropIn(20000, 0)
	if err != nil {
		t.Fatalf("remove memory drop-in failed: %v", err)
	}
	if !changed {
		t.Fatal("expected memory drop-in removal to be marked changed")
	}

	changed, err = writeMemoryLimitSystemdDropIn(20000, 0)
	if err != nil {
		t.Fatalf("repeat memory drop-in removal failed: %v", err)
	}
	if changed {
		t.Fatal("expected repeated memory drop-in removal to be skipped")
	}
}
