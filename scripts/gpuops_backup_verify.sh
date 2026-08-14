#!/usr/bin/env bash
# 将最新数据库备份恢复到一次性 PostgreSQL 容器，验证备份确实可恢复。
set -Eeuo pipefail
umask 077

BACKUP_ENV_FILE="${BACKUP_ENV_FILE:-/etc/gpu-controller/backup.env}"
if [[ -r "${BACKUP_ENV_FILE}" ]]; then
  # shellcheck disable=SC1090
  set -a
  source "${BACKUP_ENV_FILE}"
  set +a
fi

: "${RESTIC_REPOSITORY:?请配置 RESTIC_REPOSITORY}"
: "${RESTIC_PASSWORD_FILE:?请配置 RESTIC_PASSWORD_FILE}"

BACKUP_VERIFY_STATUS_FILE="${BACKUP_VERIFY_STATUS_FILE:-/var/lib/gpu-controller/backup-verify-status.json}"
BACKUP_STAGING_ROOT="${BACKUP_STAGING_ROOT:-/var/lib/gpu-controller/backup-staging}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:18.1}"
VERIFY_MEMORY_LIMIT="${VERIFY_MEMORY_LIMIT:-4g}"

command -v docker >/dev/null 2>&1 || { echo "缺少 docker" >&2; exit 2; }
command -v restic >/dev/null 2>&1 || { echo "缺少 restic" >&2; exit 2; }
command -v jq >/dev/null 2>&1 || { echo "缺少 jq" >&2; exit 2; }

mkdir -p "$(dirname "${BACKUP_VERIFY_STATUS_FILE}")"
started_at="$(date --iso-8601=seconds)"
snapshot_id="$(restic snapshots --latest 1 --json --tag gpu-ops | jq -r '.[-1].short_id // .[-1].id // empty')"
[[ -n "${snapshot_id}" ]] || { echo "没有可验证的备份快照" >&2; exit 3; }
container="gpuops-restore-verify-$(date +%s)-$$"
password="$(openssl rand -hex 24)"
last_success_at=""
last_snapshot_id=""
if [[ -r "${BACKUP_VERIFY_STATUS_FILE}" ]]; then
  last_success_at="$(jq -r '.last_success_at // empty' "${BACKUP_VERIFY_STATUS_FILE}" 2>/dev/null || true)"
  last_snapshot_id="$(jq -r '.last_snapshot_id // empty' "${BACKUP_VERIFY_STATUS_FILE}" 2>/dev/null || true)"
fi

write_status() {
  local state="$1"
  local message="$2"
  local finished_at="${3:-}"
  local tmp_status="${BACKUP_VERIFY_STATUS_FILE}.tmp.$$"
  jq -n --arg state "${state}" --arg started_at "${started_at}" --arg finished_at "${finished_at}" \
    --arg snapshot_id "${snapshot_id}" --arg message "${message}" --arg previous_success "${last_success_at}" --arg previous_snapshot "${last_snapshot_id}" \
    '{state:$state,started_at:$started_at,finished_at:$finished_at,snapshot_id:$snapshot_id,last_success_at:(if $state=="success" then $finished_at else $previous_success end),last_snapshot_id:(if $state=="success" then $snapshot_id else $previous_snapshot end),message:$message}' \
    >"${tmp_status}"
  chmod 0640 "${tmp_status}"
  chgrp "${BACKUP_STATUS_GROUP:-gpuops}" "${tmp_status}" 2>/dev/null || true
  mv -f "${tmp_status}" "${BACKUP_VERIFY_STATUS_FILE}"
}

cleanup() {
  if [[ "${container}" == gpuops-restore-verify-* ]]; then
    docker rm -f "${container}" >/dev/null 2>&1 || true
  fi
}
on_error() {
  local code=$?
  trap - ERR
  cleanup
  write_status "failed" "恢复演练失败（exit=${code}）" "$(date --iso-8601=seconds)"
  exit "${code}"
}
trap cleanup EXIT
trap on_error ERR

write_status "running" "正在一次性数据库中执行完整恢复"
docker run -d --name "${container}" --memory "${VERIFY_MEMORY_LIMIT}" --cpus 2 \
  -e POSTGRES_PASSWORD="${password}" "${POSTGRES_IMAGE}" >/dev/null

ready=0
for _ in $(seq 1 60); do
  if docker exec "${container}" pg_isready -U postgres >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done
[[ "${ready}" == "1" ]] || { echo "演练数据库启动超时" >&2; exit 4; }

dump_repo_path="${BACKUP_STAGING_ROOT}/database/gpuops.dump"
restic dump "${snapshot_id}" "${dump_repo_path}" \
  | docker exec -i "${container}" pg_restore \
      --username=postgres \
      --dbname=postgres \
      --exit-on-error \
      --no-owner \
      --no-privileges

core_counts="$(docker exec "${container}" psql -U postgres -d postgres -At -F, -c \
  "SELECT (SELECT COUNT(*) FROM users),(SELECT COUNT(*) FROM user_accounts),(SELECT COUNT(*) FROM usage_records),(SELECT COUNT(*) FROM nodes);")"
[[ -n "${core_counts}" ]] || { echo "核心表校验失败" >&2; exit 5; }

finished_at="$(date --iso-8601=seconds)"
write_status "success" "隔离恢复成功，核心表计数：${core_counts}" "${finished_at}"
cleanup
