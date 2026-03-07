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
