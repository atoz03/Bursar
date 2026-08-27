package main

import (
	"testing"
	"time"
)

func TestDiskFullAlertStateRequiresStableRecoveryBeforeRearming(t *testing.T) {
	start := time.Date(2026, time.August, 17, 10, 0, 0, 0, time.UTC)
	hold := 30 * time.Minute
	state := diskFullAlertState{}

	var emit bool
	state, emit = advanceDiskFullAlertState(state, true, false, start, hold)
	if !emit || !state.Active {
		t.Fatal("first disk risk must emit an alert")
	}
	state, emit = advanceDiskFullAlertState(state, true, false, start.Add(time.Minute), hold)
	if emit {
		t.Fatal("continuing disk risk must not emit duplicate alerts")
	}
	state, emit = advanceDiskFullAlertState(state, false, true, start.Add(5*time.Minute), hold)
	if emit || state.BelowRearmSince == nil || !state.Active {
		t.Fatal("safe usage must start, but not immediately finish, the rearm hold")
	}
	state, emit = advanceDiskFullAlertState(state, true, false, start.Add(6*time.Minute), hold)
	if emit || state.BelowRearmSince != nil {
		t.Fatal("brief recovery must not rearm the disk risk alert")
	}
	state, emit = advanceDiskFullAlertState(state, false, true, start.Add(10*time.Minute), hold)
	if emit || state.BelowRearmSince == nil {
		t.Fatal("second safe period must start a new rearm hold")
	}
	state, emit = advanceDiskFullAlertState(state, false, true, start.Add(40*time.Minute), hold)
	if emit || state.Active {
		t.Fatal("stable recovery must clear the latched state without emitting")
	}
	state, emit = advanceDiskFullAlertState(state, true, false, start.Add(41*time.Minute), hold)
	if !emit || !state.Active {
		t.Fatal("disk risk must rearm after a stable recovery")
	}
}
