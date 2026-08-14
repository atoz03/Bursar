#!/bin/bash
# 批量部署 node-agent（示例脚本）
#
# 特性：
# - 支持 Ubuntu 22.04 自动安装运行依赖（curl/jq 等）
# - 支持可选安装 Go（某些节点后续需要本地调试/编译时）
# - 支持非 root SSH 用户（要求该用户可 sudo）
# - 保留 SSH Guard（登记/白名单登录校验）部署逻辑

set -euo pipefail

AGENT_BIN="${AGENT_BIN:-./node-agent/node-agent}"
CONTROLLER_URL="${CONTROLLER_URL:-http://controller:8080}"
AGENT_TOKEN="${AGENT_TOKEN:-}"
NODES="${NODES:-}"
ACTION_POLL_INTERVAL_SECONDS="${ACTION_POLL_INTERVAL_SECONDS:-2}"

# SSH 连接
SSH_USER="${SSH_USER:-root}"
SSH_OPTS="${SSH_OPTS:--o StrictHostKeyChecking=no}"

# 依赖安装
INSTALL_PREREQS="${INSTALL_PREREQS:-1}"   # 1: 安装运行依赖
INSTALL_GO="${INSTALL_GO:-0}"             # 1: 额外安装 Go
GO_VERSION="${GO_VERSION:-1.26.6}"
SET_GO_PROXY="${SET_GO_PROXY:-1}"
REMOTE_GOPROXY="${REMOTE_GOPROXY:-https://goproxy.cn,direct}"
REMOTE_GOSUMDB="${REMOTE_GOSUMDB:-sum.golang.google.cn}"

ENABLE_SSH_GUARD="${ENABLE_SSH_GUARD:-0}"
SSH_GUARD_EXCLUDE_USERS="${SSH_GUARD_EXCLUDE_USERS:-root}"
SSH_GUARD_FAIL_OPEN="${SSH_GUARD_FAIL_OPEN:-1}"
SSH_GUARD_SYNC_INTERVAL="${SSH_GUARD_SYNC_INTERVAL:-10s}"
SSH_GUARD_ENFORCE_INTERVAL="${SSH_GUARD_ENFORCE_INTERVAL:-10s}"

if [[ -z "${NODES}" ]]; then
  echo "请设置环境变量 NODES，例如：" >&2
  echo "  - 旧格式（不推荐）：NODES=\"node01 node02\"" >&2
  echo "  - 推荐格式（节点编号:IP/主机名）：NODES=\"node-01:192.0.2.11 node-02:192.0.2.12\"" >&2
  exit 2
fi
if [[ -z "${AGENT_TOKEN}" ]]; then
  echo "请设置环境变量 AGENT_TOKEN（用于 X-Agent-Token）" >&2
  exit 2
fi
if [[ ! -f "${AGENT_BIN}" ]]; then
  echo "未找到 Agent 二进制：${AGENT_BIN}" >&2
  echo "提示：请先在控制器侧编译，例如：cd node-agent && go build -o node-agent ." >&2
  exit 2
fi

if [[ "${SSH_USER}" == "root" ]]; then
  REMOTE_SUDO=""
else
  REMOTE_SUDO="sudo"
fi

install_prereqs() {
  local target="$1"
  if [[ "${INSTALL_PREREQS}" != "1" ]]; then
    return 0
  fi

  echo "==> [${target}] 安装运行依赖"
  ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc '
set -euo pipefail
if ! command -v apt-get >/dev/null 2>&1; then
  echo "apt-get 不存在，当前脚本仅支持 Ubuntu/Debian" >&2
  exit 2
fi
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends ca-certificates curl jq procps pciutils
'"

  if [[ "${INSTALL_GO}" == "1" ]]; then
    echo "==> [${target}] 检查/安装 Go ${GO_VERSION}"
    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc '
set -euo pipefail
if command -v go >/dev/null 2>&1; then
  exit 0
fi
cd /tmp
rm -f go.tgz
curl -fL "https://mirrors.tuna.tsinghua.edu.cn/golang/go${GO_VERSION}.linux-amd64.tar.gz" -o go.tgz \
|| curl -fL "https://mirrors.aliyun.com/golang/go${GO_VERSION}.linux-amd64.tar.gz" -o go.tgz \
|| curl -fL "https://mirrors.cloud.tencent.com/golang/go${GO_VERSION}.linux-amd64.tar.gz" -o go.tgz
rm -rf /usr/local/go
tar -C /usr/local -xzf /tmp/go.tgz
ln -sf /usr/local/go/bin/go /usr/local/bin/go
go version
'"
  fi

  if [[ "${SET_GO_PROXY}" == "1" ]]; then
    echo "==> [${target}] 配置 Go 模块源（GOPROXY/GOSUMDB）"
    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc '
set -euo pipefail
if command -v go >/dev/null 2>&1; then
  go env -w GOPROXY=\"${REMOTE_GOPROXY}\"
  go env -w GOSUMDB=\"${REMOTE_GOSUMDB}\"
  go env -w GO111MODULE=on
  echo \"GOPROXY=$(go env GOPROXY)\"
  echo \"GOSUMDB=$(go env GOSUMDB)\"
fi
'"
  fi
}

for item in ${NODES}; do
  node_id="${item}"
  host="${item}"
  if [[ "${item}" == *:* ]]; then
    node_id="${item%%:*}"
    host="${item#*:}"
  fi
  target="${SSH_USER}@${host}"

  echo "==> 部署到 ${target}（NODE_ID=${node_id}）"

  install_prereqs "${target}"

  scp ${SSH_OPTS} "${AGENT_BIN}" "${target}:/tmp/node-agent"
  ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} mv /tmp/node-agent /usr/local/bin/node-agent"
  ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} chmod +x /usr/local/bin/node-agent"

  ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} mkdir -p /etc/systemd/system"
  ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'cat > /etc/systemd/system/gpu-node-agent.service <<SVC
[Unit]
Description=GPU Cluster Node Agent
After=network.target

[Service]
Type=simple
User=root
Environment=NODE_ID=${node_id}
Environment=CONTROLLER_URL=${CONTROLLER_URL}
Environment=AGENT_TOKEN=${AGENT_TOKEN}
Environment=ACTION_POLL_INTERVAL_SECONDS=${ACTION_POLL_INTERVAL_SECONDS}
ExecStart=/usr/local/bin/node-agent
Restart=always

[Install]
WantedBy=multi-user.target
SVC'"

  ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} systemctl daemon-reload && ${REMOTE_SUDO} systemctl enable gpu-node-agent && ${REMOTE_SUDO} systemctl restart gpu-node-agent"
  ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'mkdir -p /etc/gpu-cluster
cat > /etc/gpu-cluster/public.env <<CONF
CONTROLLER_URL=\"${CONTROLLER_URL}\"
NODE_ID=\"${node_id}\"
CONF
chmod 0644 /etc/gpu-cluster/public.env
chown root:root /etc/gpu-cluster/public.env 2>/dev/null || true'"
  ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'cat > /usr/local/bin/gpuops-claim <<\"EOF2\"
#!/bin/bash
set -euo pipefail

PUBLIC_CONF=\"/etc/gpu-cluster/public.env\"
if [[ -r \"${PUBLIC_CONF}\" ]]; then
  source \"${PUBLIC_CONF}\"
fi

TOKEN=\"${1:-}\"
if [[ -z \"${TOKEN}\" ]]; then
  echo \"用法：gpuops-claim <challenge_token>\" >&2
  exit 2
fi

CONTROLLER_URL=\"${CONTROLLER_URL:-}\"
NODE_ID=\"${NODE_ID:-}\"
LOCAL_USER=\"$(id -un 2>/dev/null || whoami)\"
if [[ -z \"${CONTROLLER_URL}\" || -z \"${NODE_ID}\" || -z \"${LOCAL_USER}\" ]]; then
  echo \"缺少 CONTROLLER_URL/NODE_ID/LOCAL_USER，无法提交 challenge\" >&2
  exit 2
fi

api=\"${CONTROLLER_URL%/}/api/registry/bind-claim\"
payload=\"$(printf '{\\\"token\\\":\\\"%s\\\",\\\"node_id\\\":\\\"%s\\\",\\\"local_username\\\":\\\"%s\\\"}' \"${TOKEN}\" \"${NODE_ID}\" \"${LOCAL_USER}\")\"
tmp_resp=\"$(mktemp)\"
trap 'rm -f \"${tmp_resp}\"' EXIT
http_code=\"$(curl -sS -o \"${tmp_resp}\" -w '%{http_code}' -H \"Content-Type: application/json\" --data \"${payload}\" \"${api}\")\"
resp=\"$(cat \"${tmp_resp}\")\"
if [[ ! \"${http_code}\" =~ ^2[0-9][0-9]$ ]]; then
  if [[ -n \"${resp}\" ]]; then
    echo \"${resp}\" >&2
  fi
  echo \"gpuops-claim 失败：http ${http_code}\" >&2
  exit 1
fi
echo \"${resp}\"
msg=\"$(echo \"${resp}\" | sed -n \"s/.*\\\"message\\\":\\\"\\([^\\\"]*\\)\\\".*/\\1/p\")\"
wait_s=\"$(echo \"${resp}\" | sed -n \"s/.*\\\"estimated_wait_seconds\\\":\\([0-9][0-9]*\\).*/\\1/p\")\"
if [[ -n \"${msg}\" ]]; then
  echo \"提示：${msg}\"
fi
if [[ -n \"${wait_s}\" ]]; then
  echo \"建议等待：${wait_s} 秒\"
fi
echo
EOF2'"
  ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} chmod 0755 /usr/local/bin/gpuops-claim"

  if [[ "${ENABLE_SSH_GUARD}" == "1" ]]; then
    echo "==> 安装 SSH 登录拦截（仅允许已登记用户；排除：${SSH_GUARD_EXCLUDE_USERS}）"
    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} mkdir -p /opt/gpu-cluster /etc/gpu-cluster /var/lib/gpu-cluster /etc/systemd/system"

    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'cat > /etc/gpu-cluster/ssh_guard.conf <<CONF
CONTROLLER_URL=\"${CONTROLLER_URL}\"
NODE_ID=\"${node_id}\"
AGENT_TOKEN=\"${AGENT_TOKEN}\"
EXCLUDE_USERS=\"${SSH_GUARD_EXCLUDE_USERS}\"
FAIL_OPEN=\"${SSH_GUARD_FAIL_OPEN}\"
ALLOWLIST_FILE=\"/var/lib/gpu-cluster/registered_users.txt\"
DENYLIST_FILE=\"/var/lib/gpu-cluster/blocked_users.txt\"
EXEMPT_FILE=\"/var/lib/gpu-cluster/exempt_users.txt\"
CONF'"

    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'cat > /opt/gpu-cluster/sync_registered_users.sh <<\"EOF2\"
#!/bin/bash
set -euo pipefail

CONF=\"/etc/gpu-cluster/ssh_guard.conf\"
if [[ -f \"${CONF}\" ]]; then
  source \"${CONF}\"
fi

CONTROLLER_URL=\"${CONTROLLER_URL:-}\"
NODE_ID=\"${NODE_ID:-}\"
AGENT_TOKEN=\"${AGENT_TOKEN:-}\"
ALLOWLIST_FILE=\"${ALLOWLIST_FILE:-/var/lib/gpu-cluster/registered_users.txt}\"
DENYLIST_FILE=\"${DENYLIST_FILE:-/var/lib/gpu-cluster/blocked_users.txt}\"
EXEMPT_FILE=\"${EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}\"

if [[ -z \"${CONTROLLER_URL}\" || -z \"${NODE_ID}\" ]]; then
  echo \"missing CONTROLLER_URL/NODE_ID\" >&2
  exit 2
fi

tmp=\"${ALLOWLIST_FILE}.tmp\"
tmp_deny=\"${DENYLIST_FILE}.tmp\"
tmp_exempt=\"${EXEMPT_FILE}.tmp\"
mkdir -p \"$(dirname \"${ALLOWLIST_FILE}\")\"
mkdir -p \"$(dirname \"${DENYLIST_FILE}\")\"
mkdir -p \"$(dirname \"${EXEMPT_FILE}\")\"

curl_auth=()
if [[ -n \"${AGENT_TOKEN}\" ]]; then
  curl_auth=(-H \"X-Agent-Token: ${AGENT_TOKEN}\")
fi

curl -fsS \"${curl_auth[@]}\" \"${CONTROLLER_URL}/api/registry/nodes/${NODE_ID}/users.txt\" -o \"${tmp}\"
mv \"${tmp}\" \"${ALLOWLIST_FILE}\"
chmod 0644 \"${ALLOWLIST_FILE}\"
curl -fsS \"${curl_auth[@]}\" \"${CONTROLLER_URL}/api/registry/nodes/${NODE_ID}/blocked.txt\" -o \"${tmp_deny}\"
mv \"${tmp_deny}\" \"${DENYLIST_FILE}\"
chmod 0644 \"${DENYLIST_FILE}\"
curl -fsS \"${curl_auth[@]}\" \"${CONTROLLER_URL}/api/registry/nodes/${NODE_ID}/exempt.txt\" -o \"${tmp_exempt}\"
mv \"${tmp_exempt}\" \"${EXEMPT_FILE}\"
chmod 0644 \"${EXEMPT_FILE}\"
EOF2'"
    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} chmod +x /opt/gpu-cluster/sync_registered_users.sh"

    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'cat > /opt/gpu-cluster/ssh_login_check.sh <<\"EOF2\"
#!/bin/bash
set -euo pipefail

CONF=\"/etc/gpu-cluster/ssh_guard.conf\"
if [[ -f \"${CONF}\" ]]; then
  source \"${CONF}\"
fi

user=\"${PAM_USER:-}\"
if [[ -z \"${user}\" ]]; then
  exit 0
fi

EXCLUDE_USERS=\"${EXCLUDE_USERS:-root}\"
EXEMPT_FILE=\"${EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}\"
if [[ -f \"${EXEMPT_FILE}\" ]] && grep -Fxq \"${user}\" \"${EXEMPT_FILE}\"; then
  exit 0
fi
for u in ${EXCLUDE_USERS}; do
  if [[ \"${user}\" == \"${u}\" ]]; then
    exit 0
  fi
done

CONTROLLER_URL=\"${CONTROLLER_URL:-}\"
NODE_ID=\"${NODE_ID:-}\"
AGENT_TOKEN=\"${AGENT_TOKEN:-}\"
FAIL_OPEN=\"${FAIL_OPEN:-1}\"
ALLOWLIST_FILE=\"${ALLOWLIST_FILE:-/var/lib/gpu-cluster/registered_users.txt}\"
DENYLIST_FILE=\"${DENYLIST_FILE:-/var/lib/gpu-cluster/blocked_users.txt}\"
EXEMPT_FILE=\"${EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}\"

if [[ -z \"${NODE_ID}\" ]]; then
  exit 0
fi

curl_auth=()
if [[ -n \"${AGENT_TOKEN}\" ]]; then
  curl_auth=(-H \"X-Agent-Token: ${AGENT_TOKEN}\")
fi

resp=\"\"
if [[ -n \"${CONTROLLER_URL}\" ]]; then
  # 优先实时校验：保证白名单撤销立即生效
  resp=\"$(curl -fsS --max-time 2 \"${curl_auth[@]}\" \"${CONTROLLER_URL}/api/registry/resolve?node_id=${NODE_ID}&local_username=${user}\" 2>/dev/null || true)\"
  if echo \"${resp}\" | grep -q '\"registered\":true'; then
    exit 0
  fi
  if [[ -n \"${resp}\" ]]; then
    exit 1
  fi
fi

# 控制器不可达时回退本地缓存
if [[ -f \"${EXEMPT_FILE}\" ]]; then
  if grep -Fxq \"${user}\" \"${EXEMPT_FILE}\"; then
    exit 0
  fi
fi
if [[ -f \"${DENYLIST_FILE}\" ]]; then
  if grep -Fxq \"${user}\" \"${DENYLIST_FILE}\"; then
    exit 1
  fi
fi
if [[ -f \"${ALLOWLIST_FILE}\" ]]; then
  if grep -Fxq \"${user}\" \"${ALLOWLIST_FILE}\"; then
    exit 0
  fi
  exit 1
fi

if [[ \"${FAIL_OPEN}\" == \"1\" ]]; then
  exit 0
fi
exit 1
EOF2'"
    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} chmod +x /opt/gpu-cluster/ssh_login_check.sh"
    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} touch /var/log/gpu-ssh-guard.log || true"
    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} chmod 0644 /var/log/gpu-ssh-guard.log || true"
    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'cat > /usr/local/bin/gpuops-claim <<\"EOF2\"
#!/bin/bash
set -euo pipefail

PUBLIC_CONF=\"/etc/gpu-cluster/public.env\"
if [[ -r \"${PUBLIC_CONF}\" ]]; then
  source \"${PUBLIC_CONF}\"
fi

TOKEN=\"${1:-}\"
if [[ -z \"${TOKEN}\" ]]; then
  echo \"用法：gpuops-claim <challenge_token>\" >&2
  exit 2
fi

CONTROLLER_URL=\"${CONTROLLER_URL:-}\"
NODE_ID=\"${NODE_ID:-}\"
LOCAL_USER=\"$(id -un 2>/dev/null || whoami)\"
if [[ -z \"${CONTROLLER_URL}\" || -z \"${NODE_ID}\" || -z \"${LOCAL_USER}\" ]]; then
  echo \"缺少 CONTROLLER_URL/NODE_ID/LOCAL_USER，无法提交 challenge\" >&2
  exit 2
fi

api=\"${CONTROLLER_URL%/}/api/registry/bind-claim\"
payload=\"$(printf '{\\\"token\\\":\\\"%s\\\",\\\"node_id\\\":\\\"%s\\\",\\\"local_username\\\":\\\"%s\\\"}' \"${TOKEN}\" \"${NODE_ID}\" \"${LOCAL_USER}\")\"
tmp_resp=\"$(mktemp)\"
trap 'rm -f \"${tmp_resp}\"' EXIT
http_code=\"$(curl -sS -o \"${tmp_resp}\" -w '%{http_code}' -H \"Content-Type: application/json\" --data \"${payload}\" \"${api}\")\"
resp=\"$(cat \"${tmp_resp}\")\"
if [[ ! \"${http_code}\" =~ ^2[0-9][0-9]$ ]]; then
  if [[ -n \"${resp}\" ]]; then
    echo \"${resp}\" >&2
  fi
  echo \"gpuops-claim 失败：http ${http_code}\" >&2
  exit 1
fi
echo \"${resp}\"
msg=\"$(echo \"${resp}\" | sed -n \"s/.*\\\"message\\\":\\\"\\([^\\\"]*\\)\\\".*/\\1/p\")\"
wait_s=\"$(echo \"${resp}\" | sed -n \"s/.*\\\"estimated_wait_seconds\\\":\\([0-9][0-9]*\\).*/\\1/p\")\"
if [[ -n \"${msg}\" ]]; then
  echo \"提示：${msg}\"
fi
if [[ -n \"${wait_s}\" ]]; then
  echo \"建议等待：${wait_s} 秒\"
fi
echo
EOF2'"
    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} chmod 0755 /usr/local/bin/gpuops-claim"

    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'cat > /opt/gpu-cluster/enforce_ssh_sessions.sh <<\"EOF2\"
#!/bin/bash
set -euo pipefail

CONF=\"/etc/gpu-cluster/ssh_guard.conf\"
if [[ -f \"${CONF}\" ]]; then
  source \"${CONF}\"
fi

CONTROLLER_URL=\"${CONTROLLER_URL:-}\"
NODE_ID=\"${NODE_ID:-}\"
AGENT_TOKEN=\"${AGENT_TOKEN:-}\"
FAIL_OPEN=\"${FAIL_OPEN:-1}\"
EXCLUDE_USERS=\"${EXCLUDE_USERS:-root}\"
ALLOWLIST_FILE=\"${ALLOWLIST_FILE:-/var/lib/gpu-cluster/registered_users.txt}\"
DENYLIST_FILE=\"${DENYLIST_FILE:-/var/lib/gpu-cluster/blocked_users.txt}\"
EXEMPT_FILE=\"${EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}\"

if [[ -z \"${NODE_ID}\" ]]; then
  exit 0
fi

curl_auth=()
if [[ -n \"${AGENT_TOKEN}\" ]]; then
  curl_auth=(-H \"X-Agent-Token: ${AGENT_TOKEN}\")
fi

is_excluded() {
  local u=\"$1\"
  if [[ -f \"${EXEMPT_FILE}\" ]] && grep -Fxq \"${u}\" \"${EXEMPT_FILE}\"; then
    return 0
  fi
  for x in ${EXCLUDE_USERS}; do
    if [[ \"${u}\" == \"${x}\" ]]; then
      return 0
    fi
  done
  return 1
}

check_allowed() {
  local u=\"$1\"
  if [[ -n \"${CONTROLLER_URL}\" ]]; then
    local resp
    resp=\"$(curl -fsS --max-time 2 \"${curl_auth[@]}\" \"${CONTROLLER_URL}/api/registry/resolve?node_id=${NODE_ID}&local_username=${u}\" 2>/dev/null || true)\"
    if [[ -n \"${resp}\" ]]; then
      if echo \"${resp}\" | grep -q '\"registered\":true'; then
        return 0
      fi
      return 1
    fi
  fi
  if [[ -f \"${EXEMPT_FILE}\" ]] && grep -Fxq \"${u}\" \"${EXEMPT_FILE}\"; then
    return 0
  fi
  if [[ -f \"${DENYLIST_FILE}\" ]] && grep -Fxq \"${u}\" \"${DENYLIST_FILE}\"; then
    return 1
  fi
  if [[ -f \"${ALLOWLIST_FILE}\" ]] && grep -Fxq \"${u}\" \"${ALLOWLIST_FILE}\"; then
    return 0
  fi
  if [[ \"${FAIL_OPEN}\" == \"1\" ]]; then
    return 0
  fi
  return 1
}

while read -r user tty _; do
  user=\"$(echo \"${user}\" | xargs)\"
  tty=\"$(echo \"${tty}\" | xargs)\"
  if [[ -z \"${user}\" || -z \"${tty}\" ]]; then
    continue
  fi
  if is_excluded \"${user}\"; then
    continue
  fi
  if ! check_allowed \"${user}\"; then
    pkill -KILL -t \"${tty}\" >/dev/null 2>&1 || true
    pkill -KILL -f \"^sshd: ${user}@\" >/dev/null 2>&1 || true
  fi
done < <(who)
EOF2'"
    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} chmod +x /opt/gpu-cluster/enforce_ssh_sessions.sh"

    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'cat > /etc/systemd/system/gpu-ssh-guard-sync.service <<\"EOF2\"
[Unit]
Description=GPU SSH Guard List Sync
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/opt/gpu-cluster/sync_registered_users.sh
EOF2'"

    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'cat > /etc/systemd/system/gpu-ssh-guard-sync.timer <<\"EOF2\"
[Unit]
Description=GPU SSH Guard List Sync Timer

[Timer]
OnBootSec=30
OnUnitActiveSec=${SSH_GUARD_SYNC_INTERVAL}
Unit=gpu-ssh-guard-sync.service

[Install]
WantedBy=timers.target
EOF2'"

    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'cat > /etc/systemd/system/gpu-ssh-guard-enforce.service <<\"EOF2\"
[Unit]
Description=GPU SSH Guard Session Enforcer
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/opt/gpu-cluster/enforce_ssh_sessions.sh
EOF2'"

    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'cat > /etc/systemd/system/gpu-ssh-guard-enforce.timer <<\"EOF2\"
[Unit]
Description=GPU SSH Guard Session Enforcer Timer

[Timer]
OnBootSec=40
OnUnitActiveSec=${SSH_GUARD_ENFORCE_INTERVAL}
Unit=gpu-ssh-guard-enforce.service

[Install]
WantedBy=timers.target
EOF2'"

    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} systemctl daemon-reload && ${REMOTE_SUDO} systemctl enable --now gpu-ssh-guard-sync.timer gpu-ssh-guard-enforce.timer && ${REMOTE_SUDO} systemctl start gpu-ssh-guard-sync.service gpu-ssh-guard-enforce.service || true"
    ssh ${SSH_OPTS} "${target}" "${REMOTE_SUDO} bash -lc 'if [[ -f /etc/pam.d/sshd ]] && ! grep -q \"/opt/gpu-cluster/ssh_login_check.sh\" /etc/pam.d/sshd; then echo \"account required pam_exec.so quiet /opt/gpu-cluster/ssh_login_check.sh\" >> /etc/pam.d/sshd; fi'"
  fi

done
