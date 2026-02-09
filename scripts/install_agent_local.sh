#!/usr/bin/env bash
# 单点一键部署 node-agent（在计算节点本机执行）
# 处理事项：
# - 可选安装依赖（复用 install_deps_ubuntu2204.sh）
# - 清理代理变量并配置 Go 国内源
# - 本地编译 node-agent
# - 安装 systemd 服务并启动
# - 检查控制器健康状态与服务运行状态

set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
NODE_AGENT_DIR="${NODE_AGENT_DIR:-${PROJECT_ROOT}/node-agent}"
INSTALL_DEPS="${INSTALL_DEPS:-1}"
GO_PROXY="${GO_PROXY:-https://goproxy.cn,direct}"
GO_SUMDB="${GO_SUMDB:-sum.golang.google.cn}"
SERVICE_NAME="${SERVICE_NAME:-gpu-node-agent}"
ACTION_POLL_INTERVAL_SECONDS="${ACTION_POLL_INTERVAL_SECONDS:-1}"
SKIP_CONTROLLER_HEALTHCHECK="${SKIP_CONTROLLER_HEALTHCHECK:-0}"
ENABLE_SSH_GUARD="${ENABLE_SSH_GUARD:-1}"
SSH_GUARD_EXCLUDE_USERS="${SSH_GUARD_EXCLUDE_USERS:-root}"
SSH_GUARD_FAIL_OPEN="${SSH_GUARD_FAIL_OPEN:-0}"
SSH_GUARD_ALLOWLIST_FILE="${SSH_GUARD_ALLOWLIST_FILE:-/var/lib/gpu-cluster/registered_users.txt}"
SSH_GUARD_DENYLIST_FILE="${SSH_GUARD_DENYLIST_FILE:-/var/lib/gpu-cluster/blocked_users.txt}"
SSH_GUARD_EXEMPT_FILE="${SSH_GUARD_EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}"
SSH_GUARD_SYNC_INTERVAL="${SSH_GUARD_SYNC_INTERVAL:-3s}"
SSH_GUARD_ENFORCE_INTERVAL="${SSH_GUARD_ENFORCE_INTERVAL:-3s}"

NODE_ID="${NODE_ID:-}"
CONTROLLER_URL="${CONTROLLER_URL:-}"
AGENT_TOKEN="${AGENT_TOKEN:-}"

usage() {
  cat <<USAGE
用法：
  NODE_ID=60001 \\
  CONTROLLER_URL=http://192.0.2.10:8000 \\
  AGENT_TOKEN=<agent_token> \\
  bash scripts/install_agent_local.sh

可选环境变量：
  INSTALL_DEPS=1|0                是否先安装依赖（默认 1）
  GO_PROXY=...                     Go 模块代理（默认 https://goproxy.cn,direct）
  GO_SUMDB=...                     Go sumdb（默认 sum.golang.google.cn）
  ACTION_POLL_INTERVAL_SECONDS=1   节点动作轮询周期（秒，默认 1）
  SKIP_CONTROLLER_HEALTHCHECK=1    跳过控制器健康检查
  PROJECT_ROOT=...                 项目根目录（默认脚本自动推断）
  ENABLE_SSH_GUARD=1|0             是否安装 SSH 登录拦截（默认 1）
  SSH_GUARD_EXCLUDE_USERS=\"...\"    拦截排除用户（默认 root）
  SSH_GUARD_EXEMPT_FILE=...         豁免账号缓存文件（默认 /var/lib/gpu-cluster/exempt_users.txt）
  SSH_GUARD_FAIL_OPEN=0|1          控制器不可达时是否放行（默认 0，严格模式）
  SSH_GUARD_SYNC_INTERVAL=3s       白名单/黑名单缓存同步周期（默认 3s）
  SSH_GUARD_ENFORCE_INTERVAL=3s    在线会话巡检周期（默认 3s）
USAGE
}

if [[ -z "${NODE_ID}" || -z "${CONTROLLER_URL}" || -z "${AGENT_TOKEN}" ]]; then
  echo "缺少必需参数：NODE_ID/CONTROLLER_URL/AGENT_TOKEN" >&2
  usage
  exit 2
fi

if [[ ! -d "${NODE_AGENT_DIR}" ]]; then
  echo "node-agent 目录不存在：${NODE_AGENT_DIR}" >&2
  exit 2
fi

if [[ "$(id -u)" -ne 0 ]]; then
  SUDO="sudo"
else
  SUDO=""
fi

echo "[1/8] 基础信息"
echo "PROJECT_ROOT=${PROJECT_ROOT}"
echo "NODE_ID=${NODE_ID}"
echo "CONTROLLER_URL=${CONTROLLER_URL}"
echo "SERVICE_NAME=${SERVICE_NAME}"
echo "ENABLE_SSH_GUARD=${ENABLE_SSH_GUARD} (FAIL_OPEN=${SSH_GUARD_FAIL_OPEN})"

if [[ "${INSTALL_DEPS}" == "1" ]]; then
  echo "[2/8] 安装依赖"
  if [[ -f "${PROJECT_ROOT}/scripts/install_deps_ubuntu2204.sh" ]]; then
    bash "${PROJECT_ROOT}/scripts/install_deps_ubuntu2204.sh"
  else
    echo "未找到依赖脚本 scripts/install_deps_ubuntu2204.sh" >&2
    exit 2
  fi
else
  echo "[2/8] 跳过依赖安装"
fi

echo "[3/8] 清理代理变量"
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY ALL_PROXY all_proxy || true

echo "[4/8] 配置 Go 源"
go env -w GOPROXY="${GO_PROXY}"
go env -w GOSUMDB="${GO_SUMDB}"
go env -w GO111MODULE=on
echo "GOPROXY=$(go env GOPROXY)"
echo "GOSUMDB=$(go env GOSUMDB)"

if [[ "${SKIP_CONTROLLER_HEALTHCHECK}" != "1" ]]; then
  echo "[5/8] 控制器健康检查"
  if ! curl -fsS --max-time 3 "${CONTROLLER_URL}/healthz" >/tmp/agent_healthz.out 2>/tmp/agent_healthz.err; then
    echo "控制器健康检查失败：${CONTROLLER_URL}/healthz" >&2
    echo "错误详情：$(cat /tmp/agent_healthz.err 2>/dev/null || true)" >&2
    echo "如需跳过，请设置 SKIP_CONTROLLER_HEALTHCHECK=1" >&2
    exit 3
  fi
  echo "健康检查通过：$(cat /tmp/agent_healthz.out)"
else
  echo "[5/8] 跳过控制器健康检查"
fi

echo "[6/8] 编译 node-agent"
cd "${NODE_AGENT_DIR}"
go mod download
go build -o node-agent .

echo "[7/8] 安装 systemd 服务"
${SUDO} install -m 0755 node-agent /usr/local/bin/node-agent
${SUDO} tee "/etc/systemd/system/${SERVICE_NAME}.service" >/dev/null <<EOF_SERVICE
[Unit]
Description=GPU Cluster Node Agent
After=network.target

[Service]
Type=simple
User=root
Environment=NODE_ID=${NODE_ID}
Environment=CONTROLLER_URL=${CONTROLLER_URL}
Environment=AGENT_TOKEN=${AGENT_TOKEN}
Environment=ACTION_POLL_INTERVAL_SECONDS=${ACTION_POLL_INTERVAL_SECONDS}
ExecStart=/usr/local/bin/node-agent
Restart=always

[Install]
WantedBy=multi-user.target
EOF_SERVICE

${SUDO} systemctl daemon-reload
${SUDO} systemctl enable "${SERVICE_NAME}"
${SUDO} systemctl restart "${SERVICE_NAME}"

echo "[8/8] 服务状态"
${SUDO} systemctl --no-pager --full status "${SERVICE_NAME}" || true
${SUDO} journalctl -u "${SERVICE_NAME}" -n 40 --no-pager || true

if [[ "${ENABLE_SSH_GUARD}" == "1" ]]; then
  echo "[9/9] 安装 SSH Guard（PAM 登录拦截）"
  ${SUDO} mkdir -p /opt/gpu-cluster /etc/gpu-cluster /var/lib/gpu-cluster /etc/systemd/system

  ${SUDO} tee /etc/gpu-cluster/ssh_guard.conf >/dev/null <<EOF_GUARD_CONF
CONTROLLER_URL="${CONTROLLER_URL}"
NODE_ID="${NODE_ID}"
EXCLUDE_USERS="${SSH_GUARD_EXCLUDE_USERS}"
FAIL_OPEN="${SSH_GUARD_FAIL_OPEN}"
ALLOWLIST_FILE="${SSH_GUARD_ALLOWLIST_FILE}"
DENYLIST_FILE="${SSH_GUARD_DENYLIST_FILE}"
EXEMPT_FILE="${SSH_GUARD_EXEMPT_FILE}"
EOF_GUARD_CONF

  ${SUDO} tee /opt/gpu-cluster/sync_registered_users.sh >/dev/null <<'EOF_SYNC'
#!/bin/bash
set -euo pipefail

CONF="/etc/gpu-cluster/ssh_guard.conf"
if [[ -f "${CONF}" ]]; then
  # shellcheck disable=SC1090
  source "${CONF}"
fi

CONTROLLER_URL="${CONTROLLER_URL:-}"
NODE_ID="${NODE_ID:-}"
ALLOWLIST_FILE="${ALLOWLIST_FILE:-/var/lib/gpu-cluster/registered_users.txt}"
DENYLIST_FILE="${DENYLIST_FILE:-/var/lib/gpu-cluster/blocked_users.txt}"
EXEMPT_FILE="${EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}"

if [[ -z "${CONTROLLER_URL}" || -z "${NODE_ID}" ]]; then
  echo "missing CONTROLLER_URL/NODE_ID" >&2
  exit 2
fi

tmp="${ALLOWLIST_FILE}.tmp"
tmp_deny="${DENYLIST_FILE}.tmp"
tmp_exempt="${EXEMPT_FILE}.tmp"
mkdir -p "$(dirname "${ALLOWLIST_FILE}")"
mkdir -p "$(dirname "${DENYLIST_FILE}")"
mkdir -p "$(dirname "${EXEMPT_FILE}")"
curl -fsS "${CONTROLLER_URL}/api/registry/nodes/${NODE_ID}/users.txt" -o "${tmp}"
mv "${tmp}" "${ALLOWLIST_FILE}"
chmod 0644 "${ALLOWLIST_FILE}"
curl -fsS "${CONTROLLER_URL}/api/registry/nodes/${NODE_ID}/blocked.txt" -o "${tmp_deny}"
mv "${tmp_deny}" "${DENYLIST_FILE}"
chmod 0644 "${DENYLIST_FILE}"
curl -fsS "${CONTROLLER_URL}/api/registry/nodes/${NODE_ID}/exempt.txt" -o "${tmp_exempt}"
mv "${tmp_exempt}" "${EXEMPT_FILE}"
chmod 0644 "${EXEMPT_FILE}"
EOF_SYNC
  ${SUDO} chmod +x /opt/gpu-cluster/sync_registered_users.sh

  ${SUDO} tee /opt/gpu-cluster/ssh_login_check.sh >/dev/null <<'EOF_CHECK'
#!/bin/bash
set -euo pipefail

CONF="/etc/gpu-cluster/ssh_guard.conf"
if [[ -f "${CONF}" ]]; then
  # shellcheck disable=SC1090
  source "${CONF}"
fi

user="${PAM_USER:-}"
if [[ -z "${user}" ]]; then
  exit 0
fi

LOG_FILE="${LOG_FILE:-/var/log/gpu-ssh-guard.log}"
log() {
  printf '%s user=%s node=%s msg=%s\n' "$(date '+%F %T')" "${user:-}" "${NODE_ID:-}" "$1" >> "${LOG_FILE}" 2>/dev/null || true
}

EXCLUDE_USERS="${EXCLUDE_USERS:-root}"
EXEMPT_FILE="${EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}"
if [[ -f "${EXEMPT_FILE}" ]] && grep -Fxq "${user}" "${EXEMPT_FILE}"; then
  exit 0
fi
for u in ${EXCLUDE_USERS}; do
  if [[ "${user}" == "${u}" ]]; then
    exit 0
  fi
done

CONTROLLER_URL="${CONTROLLER_URL:-}"
NODE_ID="${NODE_ID:-}"
FAIL_OPEN="${FAIL_OPEN:-0}"
ALLOWLIST_FILE="${ALLOWLIST_FILE:-/var/lib/gpu-cluster/registered_users.txt}"
DENYLIST_FILE="${DENYLIST_FILE:-/var/lib/gpu-cluster/blocked_users.txt}"
EXEMPT_FILE="${EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}"

if [[ -z "${NODE_ID}" ]]; then
  exit 1
fi

resp=""
if [[ -n "${CONTROLLER_URL}" ]]; then
  # 优先实时校验：保证“白名单删除/黑名单新增后立即生效”
  resp="$(curl -fsS --max-time 2 "${CONTROLLER_URL}/api/registry/resolve?node_id=${NODE_ID}&local_username=${user}" 2>/dev/null || true)"
  if echo "${resp}" | grep -q '"registered":true'; then
    log "registry_allow_realtime"
    exit 0
  fi
  if [[ -n "${resp}" ]]; then
    log "registry_deny_realtime"
    exit 1
  fi
fi

# 控制器不可达时才回退本地缓存（保证控制节点故障时仍可按缓存运行）
if [[ -f "${EXEMPT_FILE}" ]]; then
  if grep -Fxq "${user}" "${EXEMPT_FILE}"; then
    log "exempt_allow_fallback"
    exit 0
  fi
fi
if [[ -f "${DENYLIST_FILE}" ]]; then
  if grep -Fxq "${user}" "${DENYLIST_FILE}"; then
    log "denylist_deny_fallback"
    exit 1
  fi
fi
if [[ -f "${ALLOWLIST_FILE}" ]]; then
  if grep -Fxq "${user}" "${ALLOWLIST_FILE}"; then
    log "allowlist_allow_fallback"
    exit 0
  fi
  log "allowlist_deny_fallback"
  exit 1
fi

if [[ "${FAIL_OPEN}" == "1" ]]; then
  log "fail_open_allow"
  exit 0
fi
log "fail_close_deny"
exit 1
EOF_CHECK
  ${SUDO} chmod +x /opt/gpu-cluster/ssh_login_check.sh
  ${SUDO} touch /var/log/gpu-ssh-guard.log || true
  ${SUDO} chmod 0644 /var/log/gpu-ssh-guard.log || true

  ${SUDO} tee /opt/gpu-cluster/enforce_ssh_sessions.sh >/dev/null <<'EOF_ENFORCE'
#!/bin/bash
set -euo pipefail

CONF="/etc/gpu-cluster/ssh_guard.conf"
if [[ -f "${CONF}" ]]; then
  # shellcheck disable=SC1090
  source "${CONF}"
fi

CONTROLLER_URL="${CONTROLLER_URL:-}"
NODE_ID="${NODE_ID:-}"
FAIL_OPEN="${FAIL_OPEN:-0}"
EXCLUDE_USERS="${EXCLUDE_USERS:-root}"
ALLOWLIST_FILE="${ALLOWLIST_FILE:-/var/lib/gpu-cluster/registered_users.txt}"
DENYLIST_FILE="${DENYLIST_FILE:-/var/lib/gpu-cluster/blocked_users.txt}"
EXEMPT_FILE="${EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}"

if [[ -z "${NODE_ID}" ]]; then
  exit 0
fi

is_excluded() {
  local u="$1"
  if [[ -f "${EXEMPT_FILE}" ]] && grep -Fxq "${u}" "${EXEMPT_FILE}"; then
    return 0
  fi
  for x in ${EXCLUDE_USERS}; do
    if [[ "${u}" == "${x}" ]]; then
      return 0
    fi
  done
  return 1
}

check_allowed() {
  local u="$1"
  if [[ -n "${CONTROLLER_URL}" ]]; then
    local resp
    resp="$(curl -fsS --max-time 2 "${CONTROLLER_URL}/api/registry/resolve?node_id=${NODE_ID}&local_username=${u}" 2>/dev/null || true)"
    if [[ -n "${resp}" ]]; then
      if echo "${resp}" | grep -q '"registered":true'; then
        return 0
      fi
      return 1
    fi
  fi
  if [[ -f "${EXEMPT_FILE}" ]] && grep -Fxq "${u}" "${EXEMPT_FILE}"; then
    return 0
  fi
  if [[ -f "${DENYLIST_FILE}" ]] && grep -Fxq "${u}" "${DENYLIST_FILE}"; then
    return 1
  fi
  if [[ -f "${ALLOWLIST_FILE}" ]] && grep -Fxq "${u}" "${ALLOWLIST_FILE}"; then
    return 0
  fi
  if [[ "${FAIL_OPEN}" == "1" ]]; then
    return 0
  fi
  return 1
}

while read -r user tty _; do
  user="$(echo "${user}" | xargs)"
  tty="$(echo "${tty}" | xargs)"
  if [[ -z "${user}" || -z "${tty}" ]]; then
    continue
  fi
  if is_excluded "${user}"; then
    continue
  fi
  if ! check_allowed "${user}"; then
    pkill -KILL -t "${tty}" >/dev/null 2>&1 || true
    pkill -KILL -f "^sshd: ${user}@" >/dev/null 2>&1 || true
  fi
done < <(who)
EOF_ENFORCE
  ${SUDO} chmod +x /opt/gpu-cluster/enforce_ssh_sessions.sh

  ${SUDO} tee /etc/systemd/system/gpu-ssh-guard-sync.service >/dev/null <<'EOF_SYNC_SVC'
[Unit]
Description=GPU SSH Guard List Sync
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/opt/gpu-cluster/sync_registered_users.sh
EOF_SYNC_SVC

  ${SUDO} tee /etc/systemd/system/gpu-ssh-guard-sync.timer >/dev/null <<EOF_SYNC_TIMER
[Unit]
Description=GPU SSH Guard List Sync Timer

[Timer]
OnBootSec=30
OnUnitActiveSec=${SSH_GUARD_SYNC_INTERVAL}
Unit=gpu-ssh-guard-sync.service

[Install]
WantedBy=timers.target
EOF_SYNC_TIMER

  ${SUDO} tee /etc/systemd/system/gpu-ssh-guard-enforce.service >/dev/null <<'EOF_ENFORCE_SVC'
[Unit]
Description=GPU SSH Guard Session Enforcer
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/opt/gpu-cluster/enforce_ssh_sessions.sh
EOF_ENFORCE_SVC

  ${SUDO} tee /etc/systemd/system/gpu-ssh-guard-enforce.timer >/dev/null <<EOF_ENFORCE_TIMER
[Unit]
Description=GPU SSH Guard Session Enforcer Timer

[Timer]
OnBootSec=40
OnUnitActiveSec=${SSH_GUARD_ENFORCE_INTERVAL}
Unit=gpu-ssh-guard-enforce.service

[Install]
WantedBy=timers.target
EOF_ENFORCE_TIMER

  if [[ -f /etc/pam.d/sshd ]]; then
    # 先移除旧规则（避免重复/位置错误），再插入到 common-account 前面，防止被 sufficient 规则短路。
    ${SUDO} sed -i '\#ssh_login_check.sh#d' /etc/pam.d/sshd
    if grep -q '^@include common-account' /etc/pam.d/sshd; then
      ${SUDO} sed -i '/^@include common-account/i account requisite pam_exec.so /opt/gpu-cluster/ssh_login_check.sh' /etc/pam.d/sshd
    else
      echo "account requisite pam_exec.so /opt/gpu-cluster/ssh_login_check.sh" | ${SUDO} tee -a /etc/pam.d/sshd >/dev/null
    fi
  fi

  # 确保 sshd 启用 PAM，否则 pam_exec 不生效
  if [[ -f /etc/ssh/sshd_config ]]; then
    if grep -Eq '^[[:space:]]*UsePAM[[:space:]]+no' /etc/ssh/sshd_config; then
      ${SUDO} sed -i 's/^[[:space:]]*UsePAM[[:space:]]\\+no/UsePAM yes/g' /etc/ssh/sshd_config
    elif ! grep -Eq '^[[:space:]]*UsePAM[[:space:]]+yes' /etc/ssh/sshd_config; then
      echo 'UsePAM yes' | ${SUDO} tee -a /etc/ssh/sshd_config >/dev/null
    fi
  fi

  ${SUDO} systemctl daemon-reload
  ${SUDO} systemctl enable --now gpu-ssh-guard-sync.timer
  ${SUDO} systemctl enable --now gpu-ssh-guard-enforce.timer
  ${SUDO} systemctl start gpu-ssh-guard-sync.service || true
  ${SUDO} systemctl start gpu-ssh-guard-enforce.service || true
  ${SUDO} systemctl restart ssh || ${SUDO} systemctl restart sshd || true
fi

echo

echo "部署完成。"
