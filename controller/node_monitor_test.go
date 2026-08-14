package main

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestNormalizeMonitorGPUDevices(t *testing.T) {
	got := normalizeMonitorGPUDevices([]GPUDeviceStatus{
		{Index: 1, Name: " A100 ", UtilizationPct: 120, MemoryUsedMB: 12, MemoryTotalMB: 10},
		{Index: 0, Name: " L40 ", UtilizationPct: math.NaN()},
		{Index: 1, Name: "duplicate"},
		{Index: -1, Name: "invalid"},
	})
	if len(got) != 2 {
		t.Fatalf("len=%d want=2", len(got))
	}
	if got[0].Index != 0 || got[1].Index != 1 {
		t.Fatalf("devices not sorted: %+v", got)
	}
	if got[1].Name != "A100" || got[1].UtilizationPct != 100 || got[1].MemoryUsedMB != 10 {
		t.Fatalf("unexpected normalized device: %+v", got[1])
	}
}

func TestNodeMonitorViewJSONFlattensNodeStatus(t *testing.T) {
	ts := time.Date(2026, 8, 14, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	payload, err := json.Marshal(NodeMonitorView{
		NodeStatus:              NodeStatus{NodeID: "60001"},
		MonitorReportTS:         &ts,
		MonitorMetricsAvailable: true,
		GPUDevices:              []GPUDeviceStatus{{Index: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["node_id"] != "60001" || decoded["monitor_metrics_available"] != true {
		t.Fatalf("unexpected payload: %s", payload)
	}
}
