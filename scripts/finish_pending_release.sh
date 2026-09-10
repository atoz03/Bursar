#!/usr/bin/env bash
# 在 Agent 已滚动更新后，完成主 Controller、迁移和 HA 备机发布。
#
# 必填环境变量：
#   DR_HOST       HA 备机地址
#   DR_SSH_USER   连接 HA 备机的 SSH 用户
#   DR_KEY_FILE   连接 HA 备机的 SSH 私钥
#   PRIMARY_HOST  备机回连主控时使用的主控地址
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EXPECTED_COMMIT="${EXPECTED_COMMIT:-$(git -C "${ROOT_DIR}" rev-parse --short=12 HEAD)}"
CONTROLLER_URL="${CONTROLLER_URL:-http://127.0.0.1:8080}"
DEFAULT_CONFIG_PATH="${ROOT_DIR}/config/controller.yaml"
if [[ -f "${ROOT_DIR}/config/controller.local.yaml" ]]; then
  DEFAULT_CONFIG_PATH="${ROOT_DIR}/config/controller.local.yaml"
fi
CONFIG_PATH="${CONFIG_PATH:-${DEFAULT_CONFIG_PATH}}"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-gpuops-postgres}"
POSTGRES_USER="${POSTGRES_USER:-gpuops}"
POSTGRES_DATABASE="${POSTGRES_DATABASE:-gpuops}"
DR_NODE_ID="${DR_NODE_ID:-standby-1}"
DR_HOST="${DR_HOST:-}"
DR_SSH_PORT="${DR_SSH_PORT:-22}"
DR_SSH_USER="${DR_SSH_USER:-}"
DR_KEY_FILE="${DR_KEY_FILE:-}"
DR_RUN_USER="${DR_RUN_USER:-$(id -un)}"
DR_CONTROLLER_PORT="${DR_CONTROLLER_PORT:-8080}"
PRIMARY_HOST="${PRIMARY_HOST:-}"
PRIMARY_CONTROLLER_PORT="${PRIMARY_CONTROLLER_PORT:-8080}"
REMOTE_CONFIG_PATH="${REMOTE_CONFIG_PATH:-}"

die() {
  echo "错误：$*" >&2
  exit 1
}

wait_for_url() {
  local url="$1"
  local attempts="${2:-30}"
  local i
  for ((i = 1; i <= attempts; i++)); do
    if curl -fsS --max-time 5 "${url}" >/dev/null; then
      return 0
    fi
    sleep 2
  done
  return 1
}

: "${DR_HOST:?请配置 DR_HOST}"
: "${DR_SSH_USER:?请配置 DR_SSH_USER}"
: "${DR_KEY_FILE:?请配置 DR_KEY_FILE}"
: "${PRIMARY_HOST:?请配置 PRIMARY_HOST}"

for cmd in git go curl jq sudo systemctl docker; do
  command -v "${cmd}" >/dev/null 2>&1 || die "缺少命令：${cmd}"
done
[[ -f "${CONFIG_PATH}" ]] || die "控制器配置不存在：${CONFIG_PATH}"
[[ -f "${DR_KEY_FILE}" ]] || die "容灾节点私钥不存在：${DR_KEY_FILE}"
[[ "$(git -C "${ROOT_DIR}" rev-parse --short=12 HEAD)" == "${EXPECTED_COMMIT}" ]] \
  || die "当前 commit 与待发布 commit 不一致"
[[ -z "$(git -C "${ROOT_DIR}" status --porcelain --untracked-files=no)" ]] \
  || die "存在未提交的受跟踪文件改动，拒绝构建生产 Controller"
[[ -f "${ROOT_DIR}/web/dist/index.html" ]] || die "前端产物不存在，请先执行 pnpm -C web build"
LATEST_MIGRATION="$(find "${ROOT_DIR}/database/migrations" -maxdepth 1 -type f -name '*.sql' -printf '%f\n' | sort | tail -n 1)"
[[ -n "${LATEST_MIGRATION}" ]] || die "未找到数据库迁移文件"
[[ "${LATEST_MIGRATION}" =~ ^[0-9A-Za-z._-]+$ ]] || die "迁移文件名不合法：${LATEST_MIGRATION}"

echo "[1/7] 获取一次 sudo 授权"
sudo -v

echo "[2/7] 生成部署前即时备份"
sudo systemctl start gpuops-backup.service
backup_result="$(systemctl show gpuops-backup.service -p Result --value)"
backup_exit="$(systemctl show gpuops-backup.service -p ExecMainStatus --value)"
[[ "${backup_result}" == "success" && "${backup_exit}" == "0" ]] \
  || die "即时备份失败：result=${backup_result} exit=${backup_exit}"
echo "即时备份成功"

echo "[3/7] 安装并重启主 Controller"
BUILD_WEB=0 \
ENABLE_HOST_SECURITY=0 \
ENABLE_SHARED_WORKSPACE_SUDOERS=0 \
CONFIG_PATH="${CONFIG_PATH}" \
bash "${ROOT_DIR}/scripts/install_controller_local.sh"

echo "[4/7] 验证主 Controller 健康状态和版本"
wait_for_url "${CONTROLLER_URL}/healthz" || die "主 Controller healthz 未恢复"
wait_for_url "${CONTROLLER_URL}/readyz" || die "主 Controller readyz 未恢复"
installed_version="$(/usr/local/bin/gpu-controller --version)"
echo "${installed_version}"
[[ "${installed_version}" == *"commit=${EXPECTED_COMMIT}"* ]] \
  || die "已安装 Controller commit 与 ${EXPECTED_COMMIT} 不一致"

echo "[5/7] 验证最新数据库迁移 ${LATEST_MIGRATION}"
sudo docker inspect "${POSTGRES_CONTAINER}" >/dev/null 2>&1 \
  || die "PostgreSQL 容器不存在：${POSTGRES_CONTAINER}"
migration_applied="$(
  sudo docker exec "${POSTGRES_CONTAINER}" \
    psql -U "${POSTGRES_USER}" -d "${POSTGRES_DATABASE}" -At \
    -c "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename='${LATEST_MIGRATION}');"
)"
[[ "${migration_applied}" == "t" ]] || die "迁移 ${LATEST_MIGRATION} 尚未应用"

echo "[6/7] 同步 Controller、迁移、前端和数据库到 HA 备机"
sync_env=(
  HA_SYNC_DIRECTION=primary_to_standby
  DR_NODE_ID="${DR_NODE_ID}"
  DR_HOST="${DR_HOST}"
  DR_SSH_PORT="${DR_SSH_PORT}"
  DR_SSH_USER="${DR_SSH_USER}"
  DR_KEY_FILE="${DR_KEY_FILE}"
  DR_CONTROLLER_PORT="${DR_CONTROLLER_PORT}"
  PRIMARY_HOST="${PRIMARY_HOST}"
  PRIMARY_CONTROLLER_PORT="${PRIMARY_CONTROLLER_PORT}"
  LOCAL_CONFIG_PATH="${CONFIG_PATH}"
  POSTGRES_USER="${POSTGRES_USER}"
  POSTGRES_DATABASE="${POSTGRES_DATABASE}"
  SYNC_MIGRATIONS=1
  SYNC_WEB_DIST=1
  SYNC_DATABASE=1
)
if [[ -n "${REMOTE_CONFIG_PATH}" ]]; then
  sync_env+=(REMOTE_CONFIG_PATH="${REMOTE_CONFIG_PATH}")
fi
sudo -u "${DR_RUN_USER}" env "${sync_env[@]}" bash "${ROOT_DIR}/scripts/ha_sync_worker.sh"

echo "[7/7] 验证主备版本"
admin_token="$(awk '$1=="admin_token:" {v=$2; gsub(/\042/, "", v); print v; exit}' "${CONFIG_PATH}")"
[[ -n "${admin_token}" ]] || die "无法从配置读取 admin_token"
version_match=""
peer_reachable=""
for _ in $(seq 1 30); do
  ha_status="$(curl -fsS --max-time 10 -H "Authorization: Bearer ${admin_token}" "${CONTROLLER_URL}/api/admin/ha/status")"
  version_match="$(jq -r '.version_match // false' <<<"${ha_status}")"
  peer_reachable="$(jq -r '.peer.reachable // false' <<<"${ha_status}")"
  if [[ "${version_match}" == "true" && "${peer_reachable}" == "true" ]]; then
    break
  fi
  sleep 2
done
admin_token=""
[[ "${version_match}" == "true" && "${peer_reachable}" == "true" ]] \
  || die "HA 验证失败：version_match=${version_match} peer_reachable=${peer_reachable}"

echo "发布完成：commit=${EXPECTED_COMMIT}，主 Controller、迁移和 HA 备机均已验证。"
