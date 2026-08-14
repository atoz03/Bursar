package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestDetectPortScanIgnoresOutboundSynSent(t *testing.T) {
	var lines []string
	publicResolvers := []string{"1.0.0.1", "1.1.1.1", "8.8.4.4", "8.8.8.8"}
	for i := 0; i < 64; i++ {
		peer := publicResolvers[i%len(publicResolvers)]
		lines = append(lines, fmt.Sprintf(
			"SYN-SENT 0 1 192.0.2.10:%d %s:443",
			33000+i*2,
			peer,
		))
	}
	detected, sources, ports := detectPortScanFromSSOutput(strings.Join(lines, "\n"))
	if detected || len(sources) != 0 || len(ports) != 0 {
		t.Fatalf("outbound SYN-SENT must not trigger scan: detected=%v sources=%v ports=%v", detected, sources, ports)
	}
}

func TestDetectPortScanDetectsInboundDistinctPorts(t *testing.T) {
	var lines []string
	for i := 0; i < portScanDistinctPortThreshold; i++ {
		lines = append(lines, fmt.Sprintf(
			"SYN-RECV 0 0 192.0.2.10:%d 203.0.113.10:%d",
			2200+i,
			42000+i,
		))
	}
	detected, sources, ports := detectPortScanFromSSOutput(strings.Join(lines, "\n"))
	if !detected {
		t.Fatal("expected inbound distinct-port scan to be detected")
	}
	if len(sources) != 1 || sources[0] != "203.0.113.10" {
		t.Fatalf("unexpected sources: %v", sources)
	}
	if len(ports) != portScanDistinctPortThreshold || ports[0] != 2200 {
		t.Fatalf("unexpected ports: %v", ports)
	}
}

func TestDetectPortScanDetectsInboundConnectionFlood(t *testing.T) {
	var lines []string
	for i := 0; i < portScanConnThreshold; i++ {
		lines = append(lines, fmt.Sprintf(
			"SYN-RECV 0 0 192.0.2.10:22 198.51.100.20:%d",
			43000+i,
		))
	}
	detected, sources, ports := detectPortScanFromSSOutput(strings.Join(lines, "\n"))
	if !detected || len(sources) != 1 || sources[0] != "198.51.100.20" {
		t.Fatalf("expected inbound connection flood: detected=%v sources=%v", detected, sources)
	}
	if len(ports) != 1 || ports[0] != 22 {
		t.Fatalf("unexpected target ports: %v", ports)
	}
}
