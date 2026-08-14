#!/usr/bin/env bash
# 使用 restic 生成加密、去重、带保留策略的平台备份。
set -Eeuo pipefail
umask 077

BACKUP_ENV_FILE="${BACKUP_ENV_FILE:-/etc/gpu-controller/backup.env}"
if [[ -r "${BACKUP_ENV_FILE}" ]]; then
  # shellcheck disable=SC1090
  set -a
  source "${BACKUP_ENV_FILE}"
  set +a
fi

: "${RESTIC_REPOSITORY:?请在 backup.env 配置 RESTIC_REPOSITORY（必须位于独立磁盘或异机）}"
: "${RESTIC_PASSWORD_FILE:?请在 backup.env 配置 RESTIC_PASSWORD_FILE}"

BACKUP_STATUS_FILE="${BACKUP_STATUS_FILE:-/var/lib/gpu-controller/backup-status.json}"
BACKUP_STAGING_ROOT="${BACKUP_STAGING_ROOT:-/var/lib/gpu-controller/backup-staging}"
# 默认只保护平台自身。用户科研数据由存储系统独立负责；确有需要时再显式配置。
BACKUP_DATA_PATHS="${BACKUP_DATA_PATHS:-}"
CONTROLLER_CONFIG_PATH="${CONTROLLER_CONFIG_PATH:-/home/gpuops/gpu-ops/config/controller.local.yaml}"
WEB_DIST_PATH="${WEB_DIST_PATH:-/home/gpuops/gpu-ops/web/dist}"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-}"
POSTGRES_PUBLISHED_PORT="${POSTGRES_PUBLISHED_PORT:-5432}"
POSTGRES_DATABASE="${POSTGRES_DATABASE:-gpuops}"
POSTGRES_USER="${POSTGRES_USER:-gpuops}"
KEEP_DAILY="${KEEP_DAILY:-7}"
KEEP_WEEKLY="${KEEP_WEEKLY:-4}"
KEEP_MONTHLY="${KEEP_MONTHLY:-12}"

command -v docker >/dev/null 2>&1 || { echo "缺少 docker" >&2; exit 2; }
command -v restic >/dev/null 2>&1 || { echo "缺少 restic" >&2; exit 2; }
command -v jq >/dev/null 2>&1 || { echo "缺少 jq" >&2; exit 2; }
command -v flock >/dev/null 2>&1 || { echo "缺少 flock" >&2; exit 2; }

restic_run() {
  local backend_options=()
  if [[ -n "${RESTIC_SFTP_COMMAND:-}" ]]; then
    backend_options=(-o "sftp.command=${RESTIC_SFTP_COMMAND}")
  fi
  restic "${backend_options[@]}" "$@"
}

resolve_postgres_container() {
  if [[ -n "${POSTGRES_CONTAINER}" ]]; then
    docker inspect "${POSTGRES_CONTAINER}" >/dev/null 2>&1 || {
      echo "指定的 PostgreSQL 容器不存在：${POSTGRES_CONTAINER}" >&2
      return 1
    }
    return 0
  fi

  local port_candidate_count=0
  local query_result=""
  mapfile -t candidates < <(docker ps --filter "publish=${POSTGRES_PUBLISHED_PORT}" --format '{{.ID}}' 2>/dev/null)
  port_candidate_count="${#candidates[@]}"
  if (( port_candidate_count == 1 )); then
    POSTGRES_CONTAINER="${candidates[0]}"
    return 0
  fi

  # 某些 Docker 版本或非标准网络模式无法通过 publish 过滤器定位容器。
  # 此时直接验证哪个运行中容器能查询目标数据库，避免依赖容器名称。
  candidates=()
  while IFS= read -r container_id; do
    [[ -n "${container_id}" ]] || continue
    query_result="$(docker exec "${container_id}" psql \
      --username="${POSTGRES_USER}" \
      --dbname="${POSTGRES_DATABASE}" \
      --tuples-only --no-align --command='SELECT 1' 2>/dev/null || true)"
    [[ "${query_result}" == "1" ]] && candidates+=("${container_id}")
  done < <(docker ps --quiet)

  if (( ${#candidates[@]} != 1 )); then
    echo "无法唯一识别 PostgreSQL 容器（端口候选 ${port_candidate_count} 个，数据库验证候选 ${#candidates[@]} 个），请设置 POSTGRES_CONTAINER" >&2
    return 1
  fi
  POSTGRES_CONTAINER="${candidates[0]}"
}

resolve_postgres_container

mkdir -p "$(dirname "${BACKUP_STATUS_FILE}")" "${BACKUP_STAGING_ROOT}/database" "${BACKUP_STAGING_ROOT}/system"
exec 9>"${BACKUP_STAGING_ROOT}/backup.lock"
flock -n 9 || { echo "已有备份任务运行" >&2; exit 3; }

started_at="$(date --iso-8601=seconds)"
last_success_at=""
last_snapshot_id=""
if [[ -r "${BACKUP_STATUS_FILE}" ]]; then
  last_success_at="$(jq -r '.last_success_at // empty' "${BACKUP_STATUS_FILE}" 2>/dev/null || true)"
  last_snapshot_id="$(jq -r '.last_snapshot_id // empty' "${BACKUP_STATUS_FILE}" 2>/dev/null || true)"
fi

write_status() {
  local state="$1"
  local message="$2"
  local finished_at="${3:-}"
  local snapshot_id="${4:-}"
  local database_bytes="${5:-0}"
  local included_paths_json="${6:-[]}"
  local tmp_status="${BACKUP_STATUS_FILE}.tmp.$$"
  jq -n \
    --arg state "${state}" \
    --arg started_at "${started_at}" \
    --arg finished_at "${finished_at}" \
    --arg snapshot_id "${snapshot_id}" \
    --arg message "${message}" \
    --arg last_success_at "${last_success_at}" \
    --arg last_snapshot_id "${last_snapshot_id}" \
    --argjson database_bytes "${database_bytes}" \
    --argjson included_paths "${included_paths_json}" \
    '{state:$state,started_at:$started_at,finished_at:$finished_at,snapshot_id:$snapshot_id,database_bytes:$database_bytes,included_paths:$included_paths,message:$message,last_success_at:$last_success_at,last_snapshot_id:$last_snapshot_id}' \
    >"${tmp_status}"
  chmod 0640 "${tmp_status}"
  chgrp "${BACKUP_STATUS_GROUP:-gpuops}" "${tmp_status}" 2>/dev/null || true
  mv -f "${tmp_status}" "${BACKUP_STATUS_FILE}"
}

on_error() {
  local code=$?
  local line="${BASH_LINENO[0]:-unknown}"
  trap - ERR
  write_status "failed" "备份失败（line=${line}, exit=${code}）" "$(date --iso-8601=seconds)"
  exit "${code}"
}
trap on_error ERR

write_status "running" "正在生成数据库归档并备份关键数据"

dump_path="${BACKUP_STAGING_ROOT}/database/gpuops.dump"
config_archive="${BACKUP_STAGING_ROOT}/system/controller-config.tar.gz"
docker exec "${POSTGRES_CONTAINER}" pg_dump \
  --username="${POSTGRES_USER}" \
  --dbname="${POSTGRES_DATABASE}" \
  --format=custom \
  --compress=6 \
  --no-owner \
  --no-privileges >"${dump_path}"
docker exec -i "${POSTGRES_CONTAINER}" pg_restore --list <"${dump_path}" >/dev/null
database_bytes="$(stat -c %s "${dump_path}")"

config_inputs=()
for path in \
  "${CONTROLLER_CONFIG_PATH}" \
  "${WEB_DIST_PATH}" \
  /usr/local/bin/gpu-controller \
  /etc/systemd/system/gpu-controller.service \
  /etc/keepalived/keepalived.conf; do
  [[ -r "${path}" ]] && config_inputs+=("${path}")
done
if (( ${#config_inputs[@]} > 0 )); then
  tar --absolute-names -czf "${config_archive}" "${config_inputs[@]}"
else
  tar -czf "${config_archive}" --files-from /dev/null
fi

backup_paths=("${BACKUP_STAGING_ROOT}")
read -r -a requested_data_paths <<<"${BACKUP_DATA_PATHS}"
for path in "${requested_data_paths[@]}"; do
  [[ -e "${path}" ]] && backup_paths+=("${path}")
done
included_paths_json="$(printf '%s\n' "${backup_paths[@]}" | jq -R . | jq -s .)"

restic_run backup \
  --tag gpu-ops \
  --tag controller \
  --one-file-system \
  "${backup_paths[@]}"

snapshot_id="$(restic_run snapshots --latest 1 --json --tag gpu-ops | jq -r '.[-1].short_id // .[-1].id // empty')"
[[ -n "${snapshot_id}" ]] || { echo "无法取得 restic snapshot id" >&2; exit 4; }

restic_run forget \
  --tag gpu-ops \
  --keep-daily "${KEEP_DAILY}" \
  --keep-weekly "${KEEP_WEEKLY}" \
  --keep-monthly "${KEEP_MONTHLY}" \
  --prune

finished_at="$(date --iso-8601=seconds)"
last_success_at="${finished_at}"
last_snapshot_id="${snapshot_id}"
write_status "success" "数据库归档已校验，关键数据已写入加密仓库" "${finished_at}" "${snapshot_id}" "${database_bytes}" "${included_paths_json}"
