package main

import (
	"testing"
	"time"
)

func TestSuggestedNodePowerW(t *testing.T) {
	if got := suggestedNodePowerW(600); got != 1100 {
		t.Fatalf("suggested 600W GPU=%d, want 1100", got)
	}
	if got := suggestedNodePowerW(1200); got != 1700 {
		t.Fatalf("suggested 1200W GPU=%d, want 1700", got)
	}
	if got := suggestedNodePowerW(0); got != 0 {
		t.Fatalf("suggested 0W GPU=%d, want 0", got)
	}
}

func TestEstimatedNodePowerW(t *testing.T) {
	item := PowerRackNode{AllocatedPowerW: 1700}
	if got := estimatedNodePowerW(item, 200, 1200, 0); got != 375 {
		t.Fatalf("idle estimate=%v, want 375", got)
	}
	if got := estimatedNodePowerW(item, 1200, 1200, 100); got != 1700 {
		t.Fatalf("full estimate=%v, want 1700", got)
	}
}

func TestBuildPowerRackOverviewDetectsActualPowerIssues(t *testing.T) {
	now := nowInBeijing()
	lastSeen := now.Add(-10 * time.Second)
	racks := []PowerRack{{RackCode: "04", Name: "机柜 04", CapacityW: 6700, SlotCount: 5}}
	assignments := []PowerRackNode{
		{NodeID: "60002", RackCode: "04", SlotNumber: 5, AllocatedPowerW: 1100},
		{NodeID: "60001", RackCode: "04", SlotNumber: 4, AllocatedPowerW: 500},
	}
	nodes := []NodeStatus{
		{NodeID: "60002", LastSeenAt: lastSeen, IntervalSeconds: 15, GPUCount: 1, GPUModel: "GPU-600W"},
	}
	monitors := []NodeMonitorStatus{{
		NodeID: "60002", MonitorMetricsAvailable: true,
		GPUDevices: []GPUDeviceStatus{{Index: 0, PowerDrawW: 34, PowerLimitW: 600}},
	}}
	got := buildPowerRackOverview(racks, assignments, nodes, monitors, now)
	if len(got.Racks) != 1 || got.Racks[0].AllocatedPowerW != 1600 {
		t.Fatalf("unexpected rack totals: %+v", got.Racks)
	}
	if got.Racks[0].GPUPowerDrawW != 34 || got.Racks[0].GPUPowerLimitW != 600 {
		t.Fatalf("unexpected live power: %+v", got.Racks[0])
	}
	if got.Summary.UnreportedNodeCount != 1 || got.Summary.ConfigIssueCount != 0 {
		t.Fatalf("unexpected missing/config counts: %+v", got.Summary)
	}
	var suggested int
	for _, item := range got.Racks[0].Nodes {
		if item.NodeID == "60002" {
			suggested = item.SuggestedPowerW
		}
	}
	if suggested != 1100 {
		t.Fatalf("unexpected suggestion: %d", suggested)
	}
}

func TestBuildPowerRackOverviewMarksAllocationBelowGPULimit(t *testing.T) {
	now := nowInBeijing()
	node := NodeStatus{NodeID: "60001", LastSeenAt: now, IntervalSeconds: 15, GPUCount: 8, GPUModel: "GPU-350W"}
	monitor := NodeMonitorStatus{NodeID: "60001", MonitorMetricsAvailable: true, GPUDevices: make([]GPUDeviceStatus, 8)}
	for i := range monitor.GPUDevices {
		monitor.GPUDevices[i] = GPUDeviceStatus{Index: int32(i), PowerLimitW: 350}
	}
	got := buildPowerRackOverview(
		[]PowerRack{{RackCode: "01", Name: "机柜 01", CapacityW: 6700, SlotCount: 5}},
		[]PowerRackNode{{NodeID: "60001", RackCode: "01", SlotNumber: 5, AllocatedPowerW: 2500}},
		[]NodeStatus{node}, []NodeMonitorStatus{monitor}, now,
	)
	if got.Summary.ConfigIssueCount != 1 {
		t.Fatalf("config issue count=%d, want 1", got.Summary.ConfigIssueCount)
	}
	if got.Racks[0].Nodes[0].GPUPowerLimitW != 2800 {
		t.Fatalf("gpu limit=%v, want 2800", got.Racks[0].Nodes[0].GPUPowerLimitW)
	}
}
