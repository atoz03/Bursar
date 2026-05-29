package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

var (
	controllerVersion = "v3.1"
	controllerCommit  = ""
	controllerBuildAt = ""
	controllerStartAt = time.Now()

	buildInfoOnce   sync.Once
	cachedBuildInfo controllerBuildInfo
)

type controllerBuildInfo struct {
	Version      string
	Commit       string
	BuildAt      string
	VCSModified  string
	BinarySHA256 string
	StartedAt    time.Time
}

func initControllerBuildInfo() {
	buildInfoOnce.Do(func() {
		out := controllerBuildInfo{
			Version:   strings.TrimSpace(controllerVersion),
			Commit:    strings.TrimSpace(controllerCommit),
			BuildAt:   strings.TrimSpace(controllerBuildAt),
			StartedAt: controllerStartAt,
		}
		if bi, ok := debug.ReadBuildInfo(); ok {
			if out.Version == "" || out.Version == "dev" {
				ver := strings.TrimSpace(bi.Main.Version)
				if ver != "" && ver != "(devel)" {
					out.Version = ver
				}
			}
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
					out.VCSModified = strings.TrimSpace(s.Value)
				}
			}
		}
		if out.Version == "" {
			out.Version = "dev"
		}
		out.BinarySHA256 = localExecutableSHA256()
		cachedBuildInfo = out
	})
}

func currentControllerBuildInfo() controllerBuildInfo {
	initControllerBuildInfo()
	return cachedBuildInfo
}

func printControllerVersion() {
	info := currentControllerBuildInfo()
	fmt.Printf(
		"gpu-controller version=%s commit=%s build_at=%s vcs_modified=%s sha256=%s started_at=%s\n",
		emptyAs(info.Version, "-"),
		shortText(info.Commit, 12),
		emptyAs(info.BuildAt, "-"),
		emptyAs(info.VCSModified, "-"),
		shortText(info.BinarySHA256, 16),
		info.StartedAt.Format(time.RFC3339),
	)
}

func localExecutableSHA256() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	f, err := os.Open(exe)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func shortText(v string, n int) string {
	v = strings.TrimSpace(v)
	if n <= 0 || len(v) <= n {
		return v
	}
	return v[:n]
}

func emptyAs(v string, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
