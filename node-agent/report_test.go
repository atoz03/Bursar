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
