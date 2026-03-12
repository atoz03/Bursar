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
			got := normalizeAgentVersionBase(tc.input)
			if got != tc.wantOut {
				t.Fatalf("normalizeAgentVersionBase(%q)=%q, want %q", tc.input, got, tc.wantOut)
			}
		})
	}
}

func TestFormatAgentVersionLabel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		info agentBuildInfo
		want string
	}{
		{
			name: "base only",
			info: agentBuildInfo{Version: "v2.6"},
			want: "v2.6",
		},
		{
			name: "commit and build time",
			info: agentBuildInfo{Version: "v2.6", Commit: "abcdef1234567890", BuildAt: "2026-03-10T09:08:07Z"},
			want: "v2.6+abcdef123456.20260310T090807Z",
		},
		{
			name: "dirty flag",
			info: agentBuildInfo{Version: "v2.6", Commit: "abcdef123456", VCSModified: "true"},
			want: "v2.6+abcdef123456.dirty",
		},
		{
			name: "invalid version fallback",
			info: agentBuildInfo{Version: "dev", BuildAt: "20260310T090807Z"},
			want: "v0.0+20260310T090807Z",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := formatAgentVersionLabel(tc.info)
			if got != tc.want {
				t.Fatalf("formatAgentVersionLabel(%+v)=%q, want %q", tc.info, got, tc.want)
			}
		})
	}
}
