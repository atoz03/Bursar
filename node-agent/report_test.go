package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFlushPendingHandlesLargeLineWithoutScannerLimit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "retry later", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	agent := newReportTestAgent(t, server.URL, server.Client())
	largeMetrics := &MetricsData{
		NodeID: "node-large",
		Users: []UserProcess{
			{
				Username: "alice",
				PID:      1001,
				Command:  strings.Repeat("x", 70*1024),
			},
		},
	}
	if err := agent.appendPending(largeMetrics); err != nil {
		t.Fatalf("appendPending() error = %v", err)
	}

	if err := agent.flushPending(context.Background()); err != nil {
		t.Fatalf("flushPending() error = %v", err)
	}

	lines := readPendingLinesForTest(t, filepath.Join(agent.stateDir, "pending.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("pending lines = %d, want 1", len(lines))
	}

	var got MetricsData
	if err := json.Unmarshal(lines[0], &got); err != nil {
		t.Fatalf("unmarshal retained metrics error = %v", err)
	}
	if len(got.Users) != 1 || got.Users[0].Command != largeMetrics.Users[0].Command {
		t.Fatalf("retained large metrics mismatch")
	}
}

func TestFlushPendingRetainsOnlyLatestFailures(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "retry later", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	agent := newReportTestAgent(t, server.URL, server.Client())
	for i := 0; i < maxPendingMetricsRetained+100; i++ {
		metrics := &MetricsData{
			NodeID:   "node-trim",
			ReportID: "report-" + strconv.Itoa(i),
			Users:    []UserProcess{{Username: "alice", PID: int32(1000 + i), Command: "python job.py"}},
		}
		if err := agent.appendPending(metrics); err != nil {
			t.Fatalf("appendPending(%d) error = %v", i, err)
		}
	}

	if err := agent.flushPending(context.Background()); err != nil {
		t.Fatalf("flushPending() error = %v", err)
	}

	lines := readPendingLinesForTest(t, filepath.Join(agent.stateDir, "pending.jsonl"))
	if len(lines) != maxPendingMetricsRetained {
		t.Fatalf("pending lines = %d, want %d", len(lines), maxPendingMetricsRetained)
	}

	first := decodeMetricsForTest(t, lines[0])
	last := decodeMetricsForTest(t, lines[len(lines)-1])
	if first.ReportID != "report-100" {
		t.Fatalf("first retained report_id = %s, want report-100", first.ReportID)
	}
	if last.ReportID != "report-599" {
		t.Fatalf("last retained report_id = %s, want report-599", last.ReportID)
	}
}

func TestReportActionResult(t *testing.T) {
	t.Parallel()

	var gotMethod string
	var gotPath string
	var gotToken string
	var got struct {
		NodeID      string `json:"node_id"`
		ActionToken string `json:"action_token"`
		Success     bool   `json:"success"`
		Error       string `json:"error"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Agent-Token")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode result: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	agent := newReportTestAgent(t, server.URL, server.Client())
	agent.nodeID = "60010"
	err := agent.ReportActionResult(context.Background(), Action{
		ActionID:    81,
		ActionToken: "delivery-token",
		Type:        "create_local_account",
		Username:    "alice2",
	}, io.EOF)
	if err != nil {
		t.Fatalf("ReportActionResult() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/node/actions/81/result" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotToken != "test-token" {
		t.Fatalf("unexpected agent token: %q", gotToken)
	}
	if got.NodeID != "60010" || got.ActionToken != "delivery-token" || got.Success || got.Error != io.EOF.Error() {
		t.Fatalf("unexpected result payload: %+v", got)
	}
}

func newReportTestAgent(t *testing.T, controllerURL string, client *http.Client) *NodeAgent {
	t.Helper()

	stateDir := t.TempDir()
	if client == nil {
		client = &http.Client{}
	}
	return &NodeAgent{
		controllerURL: controllerURL,
		agentToken:    "test-token",
		stateDir:      stateDir,
		client:        client,
		logger:        log.New(io.Discard, "", 0),
	}
}

func readPendingLinesForTest(t *testing.T, path string) [][]byte {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open pending file error = %v", err)
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	out := make([][]byte, 0)
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && readErr != io.EOF {
			t.Fatalf("read pending line error = %v", readErr)
		}
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			out = append(out, append([]byte(nil), line...))
		}
		if readErr == io.EOF {
			break
		}
	}
	return out
}

func decodeMetricsForTest(t *testing.T, line []byte) MetricsData {
	t.Helper()

	var metrics MetricsData
	if err := json.Unmarshal(line, &metrics); err != nil {
		t.Fatalf("decode metrics error = %v", err)
	}
	return metrics
}
