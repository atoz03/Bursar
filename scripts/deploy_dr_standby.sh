#!/usr/bin/env bash
# 从 primary 分发 60009 standby，并在远端交互执行一次 sudo 引导。
set -Eeuo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PRIMARY_HOST="${PRIMARY_HOST:-192.0.2.10}"
PRIMARY_CONTROLLER_PORT="${PRIMARY_CONTROLLER_PORT:-60039}"
DR_HOST="${DR_HOST:-192.0.2.10}"
DR_SSH_USER="${DR_SSH_USER:-gpuops}"
DR_SSH_PORT="${DR_SSH_PORT:-22}"
DR_CONTROLLER_PORT="${DR_CONTROLLER_PORT:-60039}"
DR_NODE_ID="${DR_NODE_ID:-60009}"
DR_KEY_FILE="${DR_KEY_FILE:-${ROOT_DIR}/my_ssh_keys/node_60009.txt}"
REMOTE_PROJECT_DIR="${REMOTE_PROJECT_DIR:-/home/${DR_SSH_USER}/gpu-ops}"
LOCAL_CONFIG_PATH="${LOCAL_CONFIG_PATH:-${ROOT_DIR}/config/controller.local.yaml}"
LOCAL_BIN="${LOCAL_BIN:-/usr/local/bin/gpu-controller}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:18.1}"
TRANSFER_POSTGRES_IMAGE="${TRANSFER_POSTGRES_IMAGE:-1}"

for required in "${DR_KEY_FILE}" "${LOCAL_CONFIG_PATH}" "${LOCAL_BIN}"; do
  [[ -r "${required}" ]] || { echo "缺少部署文件：${required}" >&2; exit 2; }
done
[[ "${DR_HOST}" != "${PRIMARY_HOST}" ]] || { echo "容灾主机不能等于 primary" >&2; exit 2; }

tmp_key="$(mktemp /tmp/gpuops-dr-key.XXXXXX)"
image_archive=""
cleanup() {
  rm -f "${tmp_key}"
  [[ -z "${image_archive}" ]] || rm -f "${image_archive}"
}
trap cleanup EXIT
awk '/-----BEGIN OPENSSH PRIVATE KEY-----/{copy=1} copy{print} /-----END OPENSSH PRIVATE KEY-----/{if(copy) exit}' "${DR_KEY_FILE}" >"${tmp_key}"
chmod 0600 "${tmp_key}"
ssh-keygen -y -f "${tmp_key}" >/dev/null
ssh_opts=(-i "${tmp_key}" -p "${DR_SSH_PORT}" -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10)
scp_opts=(-i "${tmp_key}" -P "${DR_SSH_PORT}" -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10)
remote="${DR_SSH_USER}@${DR_HOST}"

echo "[1/4] 分发当前代码与前端产物"
ssh "${ssh_opts[@]}" "${remote}" "install -d -m 0700 '${REMOTE_PROJECT_DIR}' '${REMOTE_PROJECT_DIR}/.deploy' '${REMOTE_PROJECT_DIR}/config'"
tar -C "${ROOT_DIR}" \
  --exclude='.git' --exclude='.netcatty-paste-images' --exclude='my_ssh_keys' \
  --exclude='config' --exclude='web/node_modules' --exclude='controller/controller' \
  --exclude='node-agent/node-agent' --exclude='.codex' \
  -cf - . | ssh "${ssh_opts[@]}" "${remote}" "tar -C '${REMOTE_PROJECT_DIR}' -xf -"
scp -q "${scp_opts[@]}" "${LOCAL_CONFIG_PATH}" "${remote}:${REMOTE_PROJECT_DIR}/.deploy/controller.yaml"
scp -q "${scp_opts[@]}" "${LOCAL_BIN}" "${remote}:${REMOTE_PROJECT_DIR}/.deploy/gpu-controller"

if [[ "${TRANSFER_POSTGRES_IMAGE}" == "1" ]]; then
  echo "[1b/4] 从 primary 导出 PostgreSQL 镜像，避免容灾节点依赖 Docker Hub"
  image_archive="$(mktemp /tmp/postgres-18.1.XXXXXX.tar.gz)"
  sudo docker image inspect "${POSTGRES_IMAGE}" >/dev/null
  sudo docker save "${POSTGRES_IMAGE}" | gzip -1 >"${image_archive}"
  scp -q "${scp_opts[@]}" "${image_archive}" "${remote}:${REMOTE_PROJECT_DIR}/.deploy/postgres-image.tar.gz"
fi

echo "[2/4] 在 60009 安装独立 PostgreSQL 与 standby 控制器"
ssh -tt "${ssh_opts[@]}" "${remote}" \
  "sudo env PROJECT_DIR='${REMOTE_PROJECT_DIR}' PRIMARY_HOST='${PRIMARY_HOST}' PRIMARY_CONTROLLER_PORT='${PRIMARY_CONTROLLER_PORT}' DR_CONTROLLER_PORT='${DR_CONTROLLER_PORT}' DR_HA_NODE='controller-${DR_NODE_ID}' bash '${REMOTE_PROJECT_DIR}/scripts/bootstrap_dr_standby_local.sh'"

echo "[3/4] 验证 standby readyz"
curl -fsS --max-time 8 "http://${DR_HOST}:${DR_CONTROLLER_PORT}/readyz"
echo

echo "[4/4] standby 已就绪；下一步需重启 primary 并执行首次受控同步"
echo "DR_NODE_ID=${DR_NODE_ID} DR_HOST=${DR_HOST} DR_CONTROLLER_PORT=${DR_CONTROLLER_PORT}"
