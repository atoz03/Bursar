package main

import (
	"context"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

func (a *NodeAgent) getSSHUsers(ctx context.Context) []string {
	seen := map[string]struct{}{}
	users := make([]string, 0)

	addUser := func(raw string) {
		u := strings.TrimSpace(raw)
		if u == "" || u == "root" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		users = append(users, u)
	}

	// 通道1：who（覆盖大多数标准 SSH 会话）
	if out, err := exec.CommandContext(ctx, "who").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, ln := range lines {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			fields := strings.Fields(ln)
			if len(fields) < 2 {
				continue
			}
			addUser(fields[0])
		}
	}

	// 通道2：w -h（某些系统 who 为空时，w 仍可拿到会话）
	if out, err := exec.CommandContext(ctx, "w", "-h").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, ln := range lines {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			fields := strings.Fields(ln)
			if len(fields) == 0 {
				continue
			}
			addUser(fields[0])
		}
	}

	// 通道3：sshd 进程兜底（避免 who/w 不更新导致漏报）
	// 优先使用进程属主，其次再从命令行解析用户名。
	if out, err := exec.CommandContext(ctx, "ps", "-eo", "user=,args=").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, ln := range lines {
			ln = strings.TrimSpace(ln)
			if ln == "" || !strings.Contains(ln, "sshd:") {
				continue
			}
			fields := strings.Fields(ln)
			if len(fields) >= 2 {
				owner := strings.TrimSpace(fields[0])
				// root 往往是 sshd master/priv 进程；普通会话通常是对应用户。
				if owner != "" && owner != "root" {
					addUser(owner)
					continue
				}
			}
		}
	}

	// 通道4：命令行模式匹配（补充 owner= root 的会话）
	// 典型格式：sshd: gpuops@pts/2 / sshd: gpuops [priv]
	re := regexp.MustCompile(`sshd:\s*([a-zA-Z0-9._-]+)(?:@|\s+\[)`)
	if out, err := exec.CommandContext(ctx, "ps", "-eo", "args=").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, ln := range lines {
			ln = strings.TrimSpace(ln)
			if ln == "" || !strings.Contains(ln, "sshd:") {
				continue
			}
			m := re.FindStringSubmatch(ln)
			if len(m) >= 2 {
				addUser(m[1])
			}
		}
	}

	// 通道5：users（某些发行版上 who/w 输出不完整时，这个命令更稳定）
	if out, err := exec.CommandContext(ctx, "users").Output(); err == nil {
		for _, u := range strings.Fields(string(out)) {
			addUser(u)
		}
	}

	// 通道6：TTY 兜底（交互式 shell 通常绑定 pts/*，可补齐部分漏报）
	// 这里不过滤 comm，避免用户切换 shell/program 后漏掉在线会话。
	if out, err := exec.CommandContext(ctx, "ps", "-eo", "user=,tty=").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, ln := range lines {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			fields := strings.Fields(ln)
			if len(fields) < 2 {
				continue
			}
			user := strings.TrimSpace(fields[0])
			tty := strings.TrimSpace(fields[1])
			if user == "" || tty == "" || tty == "?" {
				continue
			}
			ttyLower := strings.ToLower(tty)
			if strings.HasPrefix(ttyLower, "pts/") || strings.HasPrefix(ttyLower, "tty") {
				addUser(user)
			}
		}
	}

	sort.Strings(users)
	return users
}
