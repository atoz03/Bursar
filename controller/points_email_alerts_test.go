package main

import (
	"context"
	"testing"
)

func TestAdvancePointsBalanceEmailAlertState_CrossingThreshold(t *testing.T) {
	tests := []struct {
		name        string
		prev        pointsBalanceEmailAlertState
		prevBalance float64
		nextBalance float64
		wantArmed   bool
		wantAlert   bool
	}{
		{
			name:        "above to threshold",
			prev:        pointsBalanceEmailAlertState{Armed: true},
			prevBalance: 101,
			nextBalance: 100,
			wantArmed:   false,
			wantAlert:   true,
		},
		{
			name:        "threshold to below",
			prev:        pointsBalanceEmailAlertState{Armed: true},
			prevBalance: 100,
			nextBalance: 99,
			wantArmed:   false,
			wantAlert:   true,
		},
		{
			name:        "below stays below",
			prev:        pointsBalanceEmailAlertState{Armed: true},
			prevBalance: 99,
			nextBalance: 98,
			wantArmed:   true,
			wantAlert:   false,
		},
		{
			name:        "recovery rearms",
			prev:        pointsBalanceEmailAlertState{Armed: false},
			prevBalance: 99,
			nextBalance: 101,
			wantArmed:   true,
			wantAlert:   false,
		},
		{
			name:        "already alerted does not repeat",
			prev:        pointsBalanceEmailAlertState{Armed: false},
			prevBalance: 101,
			nextBalance: 99,
			wantArmed:   false,
			wantAlert:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, alerted := advancePointsBalanceEmailAlertState(
				tt.prev,
				tt.prevBalance,
				tt.nextBalance,
				100,
			)
			if got.Armed != tt.wantArmed || alerted != tt.wantAlert {
				t.Fatalf("state=%+v alerted=%v, want armed=%v alerted=%v", got, alerted, tt.wantArmed, tt.wantAlert)
			}
		})
	}
}

func TestAdvancePointsBalanceEmailAlertState_Disabled(t *testing.T) {
	got, alerted := advancePointsBalanceEmailAlertState(
		pointsBalanceEmailAlertState{Armed: true},
		101,
		50,
		0,
	)
	if !got.Armed || alerted {
		t.Fatalf("disabled threshold should not alert or change state: state=%+v alerted=%v", got, alerted)
	}
}

func TestParsePointsWarningEmailThreshold(t *testing.T) {
	if got, err := parsePointsWarningEmailThreshold("100.5"); err != nil || got != 100.5 {
		t.Fatalf("threshold=%v err=%v", got, err)
	}
	for _, raw := range []string{"", "-1", "NaN", "+Inf"} {
		if _, err := parsePointsWarningEmailThreshold(raw); err == nil {
			t.Fatalf("expected invalid threshold for %q", raw)
		}
	}
}

func TestPointsBalanceEmailAlertsAreNotDeliveredByHAStandby(t *testing.T) {
	standby := NewServer(Config{HAEnabled: true, HARole: "standby"}, nil)
	if err := standby.deliverPendingPointsBalanceEmailAlerts(context.Background()); err != nil {
		t.Fatalf("standby must skip delivery without touching the database, got %v", err)
	}
	primary := NewServer(Config{HAEnabled: true, HARole: "primary"}, nil)
	if err := primary.deliverPendingPointsBalanceEmailAlerts(context.Background()); err == nil {
		t.Fatal("primary must attempt delivery (and fail here without a database)")
	}
}
