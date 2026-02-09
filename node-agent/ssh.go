package main

import (
	"context"
	"os/exec"
	"sort"
	"strings"
)

func (a *NodeAgent) getSSHUsers(ctx context.Context) []string {
	cmd := exec.CommandContext(ctx, "who")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(string(out), "\n")
	seen := map[string]struct{}{}
	users := make([]string, 0)
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		fields := strings.Fields(ln)
		if len(fields) < 2 {
			continue
		}
		u := strings.TrimSpace(fields[0])
		if u == "" || u == "root" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		users = append(users, u)
	}
	sort.Strings(users)
	return users
}
