#!/usr/bin/env bash
# HA 同步的受限 root helper。仅允许处理固定格式的临时文件和固定服务/数据库。
set -Eeuo pipefail
umask 077

ENV_FILE="${HA_APPLY_ENV_FILE:-/etc/gpu-controller/ha-apply.env}"
if [[ -r "${ENV_FILE}" ]]; then
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
fi

HA_OPERATOR="${HA_OPERATOR:-gpuops}"
CONTROLLER_BIN="${CONTROLLER_BIN:-/usr/local/bin/gpu-controller}"
CONTROLLER_SERVICE="${CONTROLLER_SERVICE:-gpu-controller}"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-gpuops-postgres-standby}"
POSTGRES_USER="${POSTGRES_USER:-gpuops}"
POSTGRES_DATABASE="${POSTGRES_DATABASE:-gpuops}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "此 helper 只能通过 sudo 运行" >&2
  exit 2
fi

require_operator_file() {
  local path="$1"
  local pattern="$2"
  [[ "${path}" =~ ${pattern} ]] || { echo "拒绝未授权路径：${path}" >&2; return 1; }
  [[ -f "${path}" && ! -L "${path}" ]] || { echo "文件不存在或类型不安全：${path}" >&2; return 1; }
  [[ "$(stat -c %U "${path}")" == "${HA_OPERATOR}" ]] || { echo "文件所有者必须是 ${HA_OPERATOR}" >&2; return 1; }
}

resolve_postgres_container() {
  docker inspect "${POSTGRES_CONTAINER}" >/dev/null 2>&1 || {
    echo "standby PostgreSQL 容器不存在：${POSTGRES_CONTAINER}" >&2
    return 1
  }
}

command_name="${1:-}"
case "${command_name}" in
  install-controller)
    source_file="${2:-}"
    require_operator_file "${source_file}" '^/tmp/gpu-controller\.ha\.[0-9]+\.bin$'
    install -m 0755 -o root -g root "${source_file}" "${CONTROLLER_BIN}"
    rm -f "${source_file}"
    ;;
  restart-controller)
    systemctl restart "${CONTROLLER_SERVICE}"
    systemctl is-active --quiet "${CONTROLLER_SERVICE}"
    ;;
  restore-database)
    dump_file="${2:-}"
    require_operator_file "${dump_file}" '^/tmp/gpuops-ha\.[0-9]+\.dump$'
    resolve_postgres_container
    systemctl stop "${CONTROLLER_SERVICE}" >/dev/null 2>&1 || true
    set +e
    docker exec -i "${POSTGRES_CONTAINER}" pg_restore \
        --username="${POSTGRES_USER}" --dbname="${POSTGRES_DATABASE}" \
        --clean --if-exists --no-owner --no-privileges --exit-on-error \
        --single-transaction <"${dump_file}"
    restore_rc=$?
    set -e
    systemctl start "${CONTROLLER_SERVICE}" >/dev/null 2>&1 || true
    (( restore_rc == 0 )) || exit "${restore_rc}"
    rm -f "${dump_file}"
    ;;
  dump-database)
    dump_file="${2:-}"
    [[ "${dump_file}" =~ ^/tmp/gpuops-ha-recovery\.[0-9]+\.dump$ ]] || {
      echo "拒绝未授权路径：${dump_file}" >&2
      exit 2
    }
    [[ ! -e "${dump_file}" ]] || { echo "目标文件已存在：${dump_file}" >&2; exit 2; }
    resolve_postgres_container
    docker exec "${POSTGRES_CONTAINER}" pg_dump \
      --username="${POSTGRES_USER}" --dbname="${POSTGRES_DATABASE}" \
      --format=custom --compress=6 --no-owner --no-privileges >"${dump_file}"
    docker exec -i "${POSTGRES_CONTAINER}" pg_restore --list <"${dump_file}" >/dev/null
    chown "${HA_OPERATOR}:${HA_OPERATOR}" "${dump_file}"
    chmod 0600 "${dump_file}"
    ;;
  *)
    echo "仅支持：install-controller、restart-controller、restore-database、dump-database" >&2
    exit 2
    ;;
esac
