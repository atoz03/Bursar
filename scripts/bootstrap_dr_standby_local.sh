#!/usr/bin/env bash
# 在容灾主机本机执行：创建独立 PostgreSQL、安装 standby 控制器及受限 HA helper。
set -Eeuo pipefail
umask 077

PROJECT_DIR="${PROJECT_DIR:-/home/gpuops/gpu-ops}"
RUN_USER="${RUN_USER:-gpuops}"
RUN_GROUP="${RUN_GROUP:-gpuops}"
PRIMARY_HOST="${PRIMARY_HOST:-}"
PRIMARY_CONTROLLER_PORT="${PRIMARY_CONTROLLER_PORT:-60039}"
DR_CONTROLLER_PORT="${DR_CONTROLLER_PORT:-60039}"
DR_HA_NODE="${DR_HA_NODE:-controller-60009}"
DOCKER_DATA_ROOT="${DOCKER_DATA_ROOT:-/var/lib/gpu-ops/gpu-ops-docker}"
POSTGRES_DATA_ROOT="${POSTGRES_DATA_ROOT:-/var/lib/gpu-ops/gpu-ops-postgres}"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-gpuops-postgres-standby}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:18.1}"
POSTGRES_IMAGE_ARCHIVE="${POSTGRES_IMAGE_ARCHIVE:-${PROJECT_DIR}/.deploy/postgres-image.tar.gz}"
CONFIG_SOURCE="${CONFIG_SOURCE:-${PROJECT_DIR}/.deploy/controller.yaml}"
CONFIG_PATH="${CONFIG_PATH:-${PROJECT_DIR}/config/controller.yaml}"
CONTROLLER_SOURCE="${CONTROLLER_SOURCE:-${PROJECT_DIR}/.deploy/gpu-controller}"

[[ "$(id -u)" -eq 0 ]] || { echo "请在 60009 上使用 sudo 运行" >&2; exit 2; }
[[ "${PRIMARY_HOST}" =~ ^[0-9a-fA-F:.]+$ ]] || { echo "PRIMARY_HOST 格式不安全" >&2; exit 2; }
[[ -f "${CONFIG_SOURCE}" ]] || { echo "缺少待部署配置：${CONFIG_SOURCE}" >&2; exit 2; }
[[ -f "${CONTROLLER_SOURCE}" ]] || { echo "缺少待部署控制器：${CONTROLLER_SOURCE}" >&2; exit 2; }
[[ -d /var/lib/gpu-ops ]] || { echo "缺少独立数据盘 /var/lib/gpu-ops" >&2; exit 2; }

echo "[1/8] 安装 Docker 与基础工具"
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y docker.io jq curl

echo "[2/8] 将 Docker 数据固定到独立 NVMe"
install -d -m 0710 -o root -g docker "${DOCKER_DATA_ROOT}" /etc/docker
daemon_tmp="$(mktemp /tmp/docker-daemon.XXXXXX.json)"
trap 'rm -f "${daemon_tmp}" "${seed_dump:-}"' EXIT
if [[ -s /etc/docker/daemon.json ]]; then
  jq --arg root "${DOCKER_DATA_ROOT}" '. + {"data-root":$root}' /etc/docker/daemon.json >"${daemon_tmp}"
else
  jq -n --arg root "${DOCKER_DATA_ROOT}" '{"data-root":$root}' >"${daemon_tmp}"
fi
install -m 0644 -o root -g root "${daemon_tmp}" /etc/docker/daemon.json
systemctl enable docker.service docker.socket
systemctl restart docker.service
actual_docker_root="$(docker info --format '{{.DockerRootDir}}')"
[[ "${actual_docker_root}" == "${DOCKER_DATA_ROOT}" ]] || {
  echo "Docker 数据目录不符合预期：${actual_docker_root}" >&2
  exit 3
}

echo "[3/8] 解析控制器数据库配置"
database_dsn="$(sed -n 's/^database_dsn:[[:space:]]*"\(.*\)"[[:space:]]*$/\1/p' "${CONFIG_SOURCE}")"
if [[ ! "${database_dsn}" =~ ^postgres(ql)?://([^:]+):([^@]+)@[^/]+/([^?]+) ]]; then
  echo "仅支持标准 PostgreSQL URL 格式的 database_dsn" >&2
  exit 3
fi
postgres_user="${BASH_REMATCH[2]}"
postgres_password="${BASH_REMATCH[3]}"
postgres_database="${BASH_REMATCH[4]}"
for value in "${postgres_user}" "${postgres_password}" "${postgres_database}"; do
  [[ "${value}" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "数据库账号配置包含不支持的字符" >&2; exit 3; }
done

echo "[4/8] 启动独立 standby PostgreSQL"
install -d -m 0700 -o 999 -g 999 "${POSTGRES_DATA_ROOT}"
if ! docker image inspect "${POSTGRES_IMAGE}" >/dev/null 2>&1 && [[ -r "${POSTGRES_IMAGE_ARCHIVE}" ]]; then
  echo "从 primary 导入 PostgreSQL 镜像"
  gzip -dc "${POSTGRES_IMAGE_ARCHIVE}" | docker load
fi
docker image inspect "${POSTGRES_IMAGE}" >/dev/null 2>&1 || {
  echo "缺少 PostgreSQL 镜像 ${POSTGRES_IMAGE}，且本地归档不可用" >&2
  exit 4
}
rm -f "${POSTGRES_IMAGE_ARCHIVE}"
if docker inspect "${POSTGRES_CONTAINER}" >/dev/null 2>&1; then
  docker start "${POSTGRES_CONTAINER}" >/dev/null || true
else
  docker run -d \
    --name "${POSTGRES_CONTAINER}" \
    --restart unless-stopped \
    -e "POSTGRES_USER=${postgres_user}" \
    -e "POSTGRES_PASSWORD=${postgres_password}" \
    -e "POSTGRES_DB=${postgres_database}" \
    -p 127.0.0.1:5432:5432 \
    -v "${POSTGRES_DATA_ROOT}:/var/lib/postgresql" \
    "${POSTGRES_IMAGE}" >/dev/null
fi
database_ready=0
for _ in $(seq 1 90); do
  if docker exec "${POSTGRES_CONTAINER}" pg_isready --username="${postgres_user}" --dbname="${postgres_database}" >/dev/null 2>&1; then
    database_ready=1
    break
  fi
  sleep 1
done
[[ "${database_ready}" == "1" ]] || { echo "standby PostgreSQL 启动超时" >&2; exit 4; }

echo "[5/8] 从 primary 生成并恢复一致性种子"
systemctl stop gpu-controller.service >/dev/null 2>&1 || true
seed_dump="/var/lib/gpu-ops/gpuops-standby-seed.$$.dump"
docker exec -e "PGPASSWORD=${postgres_password}" "${POSTGRES_CONTAINER}" pg_dump \
  --host="${PRIMARY_HOST}" --port=5432 \
  --username="${postgres_user}" --dbname="${postgres_database}" \
  --format=custom --compress=6 --no-owner --no-privileges >"${seed_dump}"
docker exec -i "${POSTGRES_CONTAINER}" pg_restore --list <"${seed_dump}" >/dev/null
docker exec -i "${POSTGRES_CONTAINER}" pg_restore \
  --username="${postgres_user}" --dbname="${postgres_database}" \
  --clean --if-exists --no-owner --no-privileges --exit-on-error \
  --single-transaction <"${seed_dump}"
rm -f "${seed_dump}"
seed_dump=""

echo "[6/8] 安装 standby 配置与控制器"
install -d -m 0700 -o "${RUN_USER}" -g "${RUN_GROUP}" "${PROJECT_DIR}/config"
install -m 0600 -o "${RUN_USER}" -g "${RUN_GROUP}" "${CONFIG_SOURCE}" "${CONFIG_PATH}"
set_yaml() {
  local key="$1" value="$2"
  if grep -Eq "^[[:space:]]*${key}:" "${CONFIG_PATH}"; then
    sed -i -E "s#^[[:space:]]*${key}:.*#${key}: \"${value}\"#" "${CONFIG_PATH}"
  else
    printf '%s: "%s"\n' "${key}" "${value}" >>"${CONFIG_PATH}"
  fi
}
set_yaml_bool() {
  local key="$1" value="$2"
  if grep -Eq "^[[:space:]]*${key}:" "${CONFIG_PATH}"; then
    sed -i -E "s#^[[:space:]]*${key}:.*#${key}: ${value}#" "${CONFIG_PATH}"
  else
    printf '%s: %s\n' "${key}" "${value}" >>"${CONFIG_PATH}"
  fi
}
set_yaml listen_addr "0.0.0.0:${DR_CONTROLLER_PORT}"
set_yaml database_dsn "postgres://${postgres_user}:${postgres_password}@127.0.0.1:5432/${postgres_database}?sslmode=disable"
set_yaml_bool ha_enabled true
set_yaml ha_node "${DR_HA_NODE}"
set_yaml ha_role "standby"
set_yaml ha_peer_url "http://${PRIMARY_HOST}:${PRIMARY_CONTROLLER_PORT}"
install -m 0755 -o root -g root "${CONTROLLER_SOURCE}" /usr/local/bin/gpu-controller

cat >/etc/systemd/system/gpu-controller.service <<EOF_SERVICE
[Unit]
Description=GPU Ops Standby Controller
After=network-online.target docker.service
Wants=network-online.target
Requires=docker.service

[Service]
Type=simple
User=${RUN_USER}
Group=${RUN_GROUP}
WorkingDirectory=${PROJECT_DIR}/controller
ExecStart=/usr/local/bin/gpu-controller --config ${CONFIG_PATH}
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF_SERVICE

echo "[7/8] 安装受限 HA helper"
install -d -m 0750 -o root -g "${RUN_GROUP}" /etc/gpu-controller
install -m 0755 -o root -g root "${PROJECT_DIR}/scripts/gpuops_ha_apply.sh" /usr/local/sbin/gpuops-ha-apply
cat >/etc/gpu-controller/ha-apply.env <<EOF_ENV
HA_OPERATOR=${RUN_USER}
CONTROLLER_BIN=/usr/local/bin/gpu-controller
CONTROLLER_SERVICE=gpu-controller
POSTGRES_CONTAINER=${POSTGRES_CONTAINER}
POSTGRES_USER=${postgres_user}
POSTGRES_DATABASE=${postgres_database}
EOF_ENV
chmod 0600 /etc/gpu-controller/ha-apply.env
cat >/etc/sudoers.d/gpuops-ha-apply <<EOF_SUDOERS
${RUN_USER} ALL=(root) NOPASSWD: /usr/local/sbin/gpuops-ha-apply *
EOF_SUDOERS
chmod 0440 /etc/sudoers.d/gpuops-ha-apply
visudo -cf /etc/sudoers.d/gpuops-ha-apply >/dev/null

echo "[8/8] 启动并验证 standby"
systemctl disable --now gpu-node-agent.service >/dev/null 2>&1 || true
systemctl daemon-reload
systemctl enable gpu-controller.service
systemctl restart gpu-controller.service
for _ in $(seq 1 60); do
  if curl -fsS --max-time 2 "http://127.0.0.1:${DR_CONTROLLER_PORT}/readyz" | grep -q '"ok":true'; then
    echo
    systemctl --no-pager --full status gpu-controller.service || true
    echo "standby bootstrap 完成"
    exit 0
  fi
  sleep 1
done
echo "standby 控制器未在 60 秒内就绪" >&2
journalctl -u gpu-controller.service -n 80 --no-pager >&2 || true
exit 5
