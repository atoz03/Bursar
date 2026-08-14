#!/usr/bin/env bash
# 本机安装 controller systemd 服务（后台运行 + 开机自启）
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONTROLLER_DIR="${CONTROLLER_DIR:-${ROOT_DIR}/controller}"
DEFAULT_CONFIG_PATH="${ROOT_DIR}/config/controller.yaml"
if [[ -f "${ROOT_DIR}/config/controller.local.yaml" ]]; then
  DEFAULT_CONFIG_PATH="${ROOT_DIR}/config/controller.local.yaml"
fi
CONFIG_PATH="${CONFIG_PATH:-${DEFAULT_CONFIG_PATH}}"
SERVICE_NAME="${SERVICE_NAME:-gpu-controller}"
BIN_PATH="${BIN_PATH:-/usr/local/bin/gpu-controller}"
RUN_USER="${RUN_USER:-$(id -un)}"
RUN_GROUP="${RUN_GROUP:-$(id -gn)}"
BUILD_WEB="${BUILD_WEB:-0}"
ENABLE_HOST_SECURITY="${ENABLE_HOST_SECURITY:-1}"
ENABLE_SHARED_WORKSPACE_SUDOERS="${ENABLE_SHARED_WORKSPACE_SUDOERS:-1}"
SHARED_NODE_ROOT="${SHARED_NODE_ROOT:-/srv/gpu-ops/nodes}"
SHARED_CLUSTER_ROOT="${SHARED_CLUSTER_ROOT:-/srv/gpu-ops/cluster}"
SSH_FAIL2BAN_MAXRETRY="${SSH_FAIL2BAN_MAXRETRY:-20}"
SSH_FAIL2BAN_FINDTIME="${SSH_FAIL2BAN_FINDTIME:-5m}"
SSH_FAIL2BAN_BANTIME="${SSH_FAIL2BAN_BANTIME:-12h}"
SSH_FAIL2BAN_IGNOREIP="${SSH_FAIL2BAN_IGNOREIP:-}"

if [[ ! -d "${CONTROLLER_DIR}" ]]; then
  echo "未找到 controller 目录：${CONTROLLER_DIR}" >&2
  exit 2
fi
if [[ ! -f "${CONFIG_PATH}" ]]; then
  echo "未找到配置文件：${CONFIG_PATH}" >&2
  exit 2
fi

SUDO=""
if [[ "$(id -u)" -ne 0 ]]; then
  SUDO="sudo"
fi

INSTALL_BIN="$(command -v install)"
CHOWN_BIN="$(command -v chown)"
CHMOD_BIN="$(command -v chmod)"
SUDOERS_PATH="/etc/sudoers.d/${SERVICE_NAME}-shared-workspace"

echo "[1/6] 编译 controller"
TMP_BIN="$(mktemp /tmp/gpu-controller.XXXXXX)"
trap 'rm -f "${TMP_BIN}"' EXIT
(
  cd "${CONTROLLER_DIR}"
  # 共享工作区可能由其他账号持有；禁用 Go 的隐式 VCS 探测，避免
  # safe.directory 校验阻断本地发布。版本信息由应用自身维护。
  go build -buildvcs=false -o "${TMP_BIN}" .
)

if [[ "${BUILD_WEB}" == "1" ]]; then
  echo "[2/6] 构建前端 web"
  pnpm -C "${ROOT_DIR}/web" build
else
  echo "[2/6] 跳过前端构建（BUILD_WEB=${BUILD_WEB}）"
fi

echo "[3/6] 安装二进制到 ${BIN_PATH}"
${SUDO} install -m 0755 "${TMP_BIN}" "${BIN_PATH}"

if [[ "${ENABLE_SHARED_WORKSPACE_SUDOERS}" == "1" ]]; then
  echo "[4/6] 写入共享工作目录 sudoers"
  ${SUDO} tee "${SUDOERS_PATH}" >/dev/null <<EOF_SUDOERS
${RUN_USER} ALL=(root) NOPASSWD: ${INSTALL_BIN} -d -m 0755 -o * -g * ${SHARED_NODE_ROOT}/*/*, ${INSTALL_BIN} -d -m 0755 -o * -g * ${SHARED_CLUSTER_ROOT}/*, ${CHOWN_BIN} * ${SHARED_NODE_ROOT}/*/*, ${CHOWN_BIN} * ${SHARED_CLUSTER_ROOT}/*, ${CHMOD_BIN} 0755 ${SHARED_NODE_ROOT}/*/*, ${CHMOD_BIN} 0755 ${SHARED_CLUSTER_ROOT}/*
EOF_SUDOERS
  ${SUDO} chmod 440 "${SUDOERS_PATH}"
  ${SUDO} chown root:root "${SUDOERS_PATH}"
  ${SUDO} visudo -cf "${SUDOERS_PATH}" >/dev/null
else
  echo "[4/6] 跳过共享工作目录 sudoers（ENABLE_SHARED_WORKSPACE_SUDOERS=${ENABLE_SHARED_WORKSPACE_SUDOERS}）"
fi

echo "[5/6] 写入 systemd 服务 ${SERVICE_NAME}"
${SUDO} tee "/etc/systemd/system/${SERVICE_NAME}.service" >/dev/null <<EOF_SERVICE
[Unit]
Description=GPU Ops Controller
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${RUN_USER}
Group=${RUN_GROUP}
WorkingDirectory=${CONTROLLER_DIR}
ExecStart=${BIN_PATH} --config ${CONFIG_PATH}
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF_SERVICE

echo "[6/6] 重载并启用服务"
${SUDO} systemctl daemon-reload
${SUDO} systemctl enable "${SERVICE_NAME}"
${SUDO} systemctl restart "${SERVICE_NAME}"
${SUDO} systemctl --no-pager --full status "${SERVICE_NAME}" || true
${SUDO} journalctl -u "${SERVICE_NAME}" -n 40 --no-pager || true

if [[ "${ENABLE_HOST_SECURITY}" == "1" ]]; then
  echo "[7/7] 安装控制器主机安全基线（fail2ban）"
  if command -v apt-get >/dev/null 2>&1; then
    ${SUDO} apt-get update -y >/dev/null 2>&1 || true
    ${SUDO} apt-get install -y fail2ban >/dev/null 2>&1 || true
  fi
  if command -v fail2ban-client >/dev/null 2>&1; then
    ${SUDO} mkdir -p /etc/fail2ban/jail.d
    ${SUDO} tee /etc/fail2ban/jail.d/gpu-controller-ssh.local >/dev/null <<EOF_FAIL2BAN
[sshd]
enabled = true
backend = systemd
port = ssh
maxretry = ${SSH_FAIL2BAN_MAXRETRY}
findtime = ${SSH_FAIL2BAN_FINDTIME}
bantime = ${SSH_FAIL2BAN_BANTIME}
ignoreip = ${SSH_FAIL2BAN_IGNOREIP}

[recidive]
enabled = true
backend = systemd
logpath = /var/log/fail2ban.log
maxretry = 5
findtime = 1d
bantime = 7d
EOF_FAIL2BAN
    ${SUDO} systemctl enable --now fail2ban || true
    ${SUDO} systemctl restart fail2ban || true
  else
    echo "警告：未检测到 fail2ban，跳过控制器 SSH 防爆破配置"
  fi
fi

echo "完成：${SERVICE_NAME} 已后台运行并开机自启。"
