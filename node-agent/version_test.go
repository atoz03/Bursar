package main

import "testing"

func TestNormalizeAgentVersionLabel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantOut string
	}{
		{name: "valid", input: "v1.3", wantOut: "v1.3"},
		{name: "trim valid", input: "  v12.0  ", wantOut: "v12.0"},
		{name: "invalid empty", input: "", wantOut: "v0.0"},
		{name: "invalid dev", input: "dev", wantOut: "v0.0"},
		{name: "invalid sha", input: "sha-f1a96673916c", wantOut: "v0.0"},
		{name: "invalid patch", input: "v1.3.1", wantOut: "v0.0"},
		{name: "invalid missing minor", input: "v1", wantOut: "v0.0"},
		{name: "invalid no prefix", input: "1.3", wantOut: "v0.0"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeAgentVersionLabel(tc.input)
			if got != tc.wantOut {
				t.Fatalf("normalizeAgentVersionLabel(%q)=%q, want %q", tc.input, got, tc.wantOut)
			}
		})
	}
}
