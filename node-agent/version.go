package main

import (
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

var (
	// 每次发布新版本时，只需要手工修改这里一次（示例：v2.0 -> v2.6）。
	agentVersion     = "v2.6"
	agentCommit      = ""
	agentBuildAt     = ""
	agentVCSModified = ""

	agentBuildInfoOnce sync.Once
	agentBuildInfoVal  agentBuildInfo

	agentVersionPattern     = regexp.MustCompile(`^v\d+\.\d+$`)
	agentBuildMetaSanitizer = regexp.MustCompile(`[^0-9A-Za-z-]+`)
)

type agentBuildInfo struct {
	Version     string
	Commit      string
	BuildAt     string
	VCSModified string
}

func currentAgentVersionLabel() string {
	return formatAgentVersionLabel(currentAgentBuildInfo())
}

func normalizeAgentVersionBase(version string) string {
	v := strings.TrimSpace(version)
	if agentVersionPattern.MatchString(v) {
		return v
	}
	return "v0.0"
}

func shortAgentBuildCommit(commit string) string {
	commit = sanitizeAgentBuildMetadata(commit)
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

func sanitizeAgentBuildMetadata(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = agentBuildMetaSanitizer.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-.")
	return value
}

func compactAgentBuildAt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts.UTC().Format("20060102T150405Z")
	}
	return sanitizeAgentBuildMetadata(value)
}

func formatAgentVersionLabel(info agentBuildInfo) string {
	base := normalizeAgentVersionBase(info.Version)
	meta := make([]string, 0, 3)
	if commit := shortAgentBuildCommit(info.Commit); commit != "" {
		meta = append(meta, commit)
	}
	if buildAt := compactAgentBuildAt(info.BuildAt); buildAt != "" {
		meta = append(meta, buildAt)
	}
	if strings.EqualFold(strings.TrimSpace(info.VCSModified), "true") {
		meta = append(meta, "dirty")
	}
	if len(meta) == 0 {
		return base
	}
	return base + "+" + strings.Join(meta, ".")
}

func currentAgentBuildInfo() agentBuildInfo {
	agentBuildInfoOnce.Do(func() {
		out := agentBuildInfo{
			Version:     normalizeAgentVersionBase(agentVersion),
			Commit:      strings.TrimSpace(agentCommit),
			BuildAt:     strings.TrimSpace(agentBuildAt),
			VCSModified: strings.TrimSpace(agentVCSModified),
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
				case "vcs.modified":
					if out.VCSModified == "" {
						out.VCSModified = strings.TrimSpace(s.Value)
					}
				}
			}
		}
		agentBuildInfoVal = out
	})
	return agentBuildInfoVal
}
