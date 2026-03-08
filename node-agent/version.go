package main

import (
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
)

var (
	// 每次发布新版本时，只需要手工修改这里一次（示例：v1.9 -> v2.0）。
	agentVersion = "v2.0"
	agentCommit  = ""
	agentBuildAt = ""

	agentBuildInfoOnce sync.Once
	agentBuildInfoVal  agentBuildInfo

	agentVersionPattern = regexp.MustCompile(`^v\d+\.\d+$`)
)

type agentBuildInfo struct {
	Version string
	Commit  string
	BuildAt string
}

func currentAgentVersionLabel() string {
	return normalizeAgentVersionLabel(agentVersion)
}

func normalizeAgentVersionLabel(version string) string {
	v := strings.TrimSpace(version)
	if agentVersionPattern.MatchString(v) {
		return v
	}
	return "v0.0"
}

func currentAgentBuildInfo() agentBuildInfo {
	agentBuildInfoOnce.Do(func() {
		out := agentBuildInfo{
			Version: normalizeAgentVersionLabel(agentVersion),
			Commit:  strings.TrimSpace(agentCommit),
			BuildAt: strings.TrimSpace(agentBuildAt),
		}
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				switch strings.TrimSpace(s.Key) {
				case "vcs.revision":
					if out.Commit == "" {
						out.Commit = strings.TrimSpace(s.Value)
					}
				case "vcs.time":
					if out.BuildAt == "" {
						out.BuildAt = strings.TrimSpace(s.Value)
					}
				}
			}
		}
		agentBuildInfoVal = out
	})
	return agentBuildInfoVal
}
