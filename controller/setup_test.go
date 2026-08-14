package main

import (
	"reflect"
	"testing"
)

func TestNormalizeEmailDomains(t *testing.T) {
	t.Parallel()

	got, err := normalizeEmailDomains([]string{"@Example.org", "students.example.org", "example.org"})
	if err != nil {
		t.Fatalf("normalizeEmailDomains() error: %v", err)
	}
	want := []string{"example.org", "students.example.org"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeEmailDomains()=%v, want %v", got, want)
	}

	if _, err := normalizeEmailDomains([]string{"https://example.org"}); err == nil {
		t.Fatal("带协议的邮箱域名应被拒绝")
	}
}

func TestNormalizeProvisionSSHHost(t *testing.T) {
	t.Parallel()

	for _, host := range []string{"gpu.example.org", "192.0.2.10"} {
		got, err := normalizeProvisionSSHHost(host)
		if err != nil {
			t.Fatalf("normalizeProvisionSSHHost(%q) error: %v", host, err)
		}
		if got != host {
			t.Fatalf("normalizeProvisionSSHHost(%q)=%q", host, got)
		}
	}

	for _, host := range []string{"gpuops@gpu.example.org", "gpu.example.org:22", "gpu.example.org/path", "https://gpu.example.org"} {
		if _, err := normalizeProvisionSSHHost(host); err == nil {
			t.Fatalf("非法 SSH 主机 %q 应被拒绝", host)
		}
	}
}

func TestIsConfiguredSecret(t *testing.T) {
	t.Parallel()

	if isConfiguredSecret("<replace-with-agent-token>", 16) {
		t.Fatal("示例 token 不应被视为已配置")
	}
	if isConfiguredSecret("short", 16) {
		t.Fatal("过短 token 不应被视为已配置")
	}
	if !isConfiguredSecret("b6f223b9e7874b1eb9c2795998395046", 16) {
		t.Fatal("强随机 token 应被视为已配置")
	}
}
