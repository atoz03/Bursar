package main

import (
	"testing"
)

func TestSplitCSVLine(t *testing.T) {
	parts := splitCSVLine("123, NVIDIA A100, 00000000:3B:00.0, 4096")
	if len(parts) != 4 {
		t.Fatalf("len(parts)=%d", len(parts))
	}
	if parts[0] != "123" || parts[3] != "4096" {
		t.Fatalf("unexpected parts=%v", parts)
	}
}

func TestParseGPUDeviceStatusLine(t *testing.T) {
	item, ok := parseGPUDeviceStatusLine("2, GPU-abc, NVIDIA A100-SXM4-80GB, 00000000:3B:00.0, 87, 4096, 81920, 66, 241.5, 400.0")
	if !ok {
		t.Fatal("expected valid GPU status")
	}
	if item.Index != 2 || item.UtilizationPct != 87 || item.MemoryTotalMB != 81920 || item.PowerDrawW != 241.5 {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestApplyGPUProcessCounts(t *testing.T) {
	devices := []GPUDeviceStatus{{Index: 0}, {Index: 1}}
	usage := map[int32][]GPUUsage{
		10: {{GPUID: 0}, {GPUID: 0}},
		11: {{GPUID: 0}, {GPUID: 1}},
	}
	got := applyGPUProcessCounts(devices, usage)
	if got[0].ComputeProcesses != 2 || got[1].ComputeProcesses != 1 {
		t.Fatalf("unexpected process counts: %+v", got)
	}
}
