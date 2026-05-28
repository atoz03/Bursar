package main

import "testing"

func TestCalculateProcessCost_MatchAndDefault(t *testing.T) {
	pi := NewPriceIndex([]PriceRow{
		{Model: "A100", Price: 0.5},
		{Model: "RTX 3090", Price: 0.2},
	})
	proc := UserProcess{
		Username: "u",
		PID:      1,
		GPUUsage: []GPUUsage{
			{GPUModel: "NVIDIA A100-SXM4-80GB"},
			{GPUModel: "Unknown GPU"},
		},
	}

	got := CalculateProcessCost(proc, pi, 0.1)
	want := 0.6
	if got != want {
		t.Fatalf("cost=%v want=%v", got, want)
	}
}

func TestStatusAndActions_EnterBlockedOnlyNotify(t *testing.T) {
	u := User{Username: "alice", Balance: -600, Status: "blocked"}
	acts := DecideActions("limited", u, 100, 10, 500)
	foundNotify := false
	for _, a := range acts {
		if a.Type == "notify" {
			foundNotify = true
		}
		if a.Type == "kill_all_processes" || a.Type == "block_user" || a.Type == "unblock_user" {
			t.Fatalf("unexpected control action type=%s", a.Type)
		}
	}
	if !foundNotify {
		t.Fatalf("expected notify action when overdraft exceeded")
	}
}

func TestStatusAndActions_NoControlActionWhenWithinOverdraftLimit(t *testing.T) {
	u := User{Username: "bob", Balance: -20, Status: "blocked"}
	acts := DecideActions("blocked", u, 100, 10, 500)
	for _, a := range acts {
		if a.Type == "kill_all_processes" || a.Type == "block_user" || a.Type == "unblock_user" {
			t.Fatalf("unexpected control action type=%s when overdraft within limit", a.Type)
		}
	}
}

func TestCalculateProcessCostWithPolicy_PrecedenceAndFallback(t *testing.T) {
	global := NewPriceIndex([]PriceRow{
		{Model: "A100", Price: 0.5},
		{Model: "RTX 3090", Price: 0.2},
	})
	nodeModel := NewPriceIndex([]PriceRow{
		{Model: "A100", Price: 0.8},
	})
	nodeUniform := 0.4
	proc := UserProcess{
		Username: "u",
		PID:      1,
		GPUUsage: []GPUUsage{
			{GPUModel: "NVIDIA A100-SXM4-80GB"}, // 命中节点按卡型
			{GPUModel: "NVIDIA RTX 3090"},       // 回退节点统一
			{GPUModel: "Unknown GPU"},           // 回退节点统一
		},
	}
	got := CalculateProcessCostWithPolicy(proc, nodeModel, &nodeUniform, global, 0.1)
	want := 1.6 // 0.8 + 0.4 + 0.4
	if got != want {
		t.Fatalf("cost=%v want=%v", got, want)
	}
}

func TestCalculateProcessCostWithPolicy_GlobalAndDefaultFallback(t *testing.T) {
	global := NewPriceIndex([]PriceRow{
		{Model: "RTX 3090", Price: 0.2},
	})
	proc := UserProcess{
		Username: "u",
		PID:      1,
		GPUUsage: []GPUUsage{
			{GPUModel: "NVIDIA RTX 3090"}, // 回退全局型号
			{GPUModel: "Unknown GPU"},     // 回退默认单价
		},
	}
	got := CalculateProcessCostWithPolicy(proc, NewPriceIndex(nil), nil, global, 0.1)
	want := 0.3
	if got != want {
		t.Fatalf("cost=%v want=%v", got, want)
	}
}

func TestChargeableGPUUsageForBilling_DedupSameCardAcrossProcesses(t *testing.T) {
	global := NewPriceIndex([]PriceRow{
		{Model: "A100", Price: 0.5},
	})
	seen := map[string]struct{}{}
	sameCard := GPUUsage{GPUID: 0, GPUModel: "NVIDIA A100-SXM4-80GB", GPUBusID: "00000000:81:00.0"}

	total := 0.0
	for i := 0; i < 4; i++ {
		chargeable := ChargeableGPUUsageForBilling([]GPUUsage{sameCard}, seen)
		total += CalculateGPUUsageCostWithPolicy(chargeable, NewPriceIndex(nil), nil, global, 0.1)
	}

	if total != 0.5 {
		t.Fatalf("same GPU charged total=%v want=0.5", total)
	}
}

func TestChargeableGPUUsageForBilling_ChargeDistinctCards(t *testing.T) {
	global := NewPriceIndex([]PriceRow{
		{Model: "A100", Price: 0.5},
	})
	seen := map[string]struct{}{}
	processes := [][]GPUUsage{
		{{GPUID: 0, GPUModel: "NVIDIA A100-SXM4-80GB", GPUBusID: "00000000:81:00.0"}},
		{{GPUID: 1, GPUModel: "NVIDIA A100-SXM4-80GB", GPUBusID: "00000000:82:00.0"}},
		{{GPUID: 0, GPUModel: "NVIDIA A100-SXM4-80GB", GPUBusID: "00000000:81:00.0"}},
		{{GPUID: 1, GPUModel: "NVIDIA A100-SXM4-80GB", GPUBusID: "00000000:82:00.0"}},
	}

	total := 0.0
	for _, gpuUsage := range processes {
		chargeable := ChargeableGPUUsageForBilling(gpuUsage, seen)
		total += CalculateGPUUsageCostWithPolicy(chargeable, NewPriceIndex(nil), nil, global, 0.1)
	}

	if total != 1.0 {
		t.Fatalf("distinct GPUs charged total=%v want=1.0", total)
	}
}
