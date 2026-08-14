#!/usr/bin/env bash
# 容灾同步执行器：支持主->备、备->主双向同步（版本校验 + 二进制/前端/数据库）
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCAL_BIN="${LOCAL_BIN:-/usr/local/bin/gpu-controller}"
DEFAULT_LOCAL_CONFIG_PATH="${ROOT_DIR}/config/controller.yaml"
if [[ -f "${ROOT_DIR}/config/controller.local.yaml" ]]; then
  DEFAULT_LOCAL_CONFIG_PATH="${ROOT_DIR}/config/controller.local.yaml"
fi
LOCAL_CONFIG_PATH="${LOCAL_CONFIG_PATH:-${DEFAULT_LOCAL_CONFIG_PATH}}"
LOCAL_WEB_DIST="${LOCAL_WEB_DIST:-${ROOT_DIR}/web/dist/}"
REMOTE_PROJECT_DIR="${REMOTE_PROJECT_DIR:-/home/${DR_SSH_USER:-gpuops}/gpu-ops}"
REMOTE_BIN="${REMOTE_BIN:-/usr/local/bin/gpu-controller}"
REMOTE_CONFIG_PATH="${REMOTE_CONFIG_PATH:-${REMOTE_PROJECT_DIR}/config/controller.yaml}"
REMOTE_WEB_DIST="${REMOTE_WEB_DIST:-${REMOTE_PROJECT_DIR}/web/dist/}"
REMOTE_SERVICE_NAME="${REMOTE_SERVICE_NAME:-gpu-controller}"
LOCAL_SERVICE_NAME="${LOCAL_SERVICE_NAME:-gpu-controller}"
REMOTE_APPLY_HELPER="${REMOTE_APPLY_HELPER:-/usr/local/sbin/gpuops-ha-apply}"
LOCAL_POSTGRES_CONTAINER="${LOCAL_POSTGRES_CONTAINER:-}"
POSTGRES_DATABASE="${POSTGRES_DATABASE:-gpuops}"
POSTGRES_USER="${POSTGRES_USER:-gpuops}"

HA_SYNC_DIRECTION="${HA_SYNC_DIRECTION:-primary_to_standby}"
DR_NODE_ID="${DR_NODE_ID:-60019}"
DR_HOST="${DR_HOST:-192.0.2.10}"
DR_SSH_PORT="${DR_SSH_PORT:-22}"
DR_SSH_USER="${DR_SSH_USER:-gpuops}"
DR_KEY_FILE="${DR_KEY_FILE:-${ROOT_DIR}/my_ssh_keys/node_60019.txt}"
DR_CONTROLLER_PORT="${DR_CONTROLLER_PORT:-60019}"
PRIMARY_HOST="${PRIMARY_HOST:-127.0.0.1}"
PRIMARY_CONTROLLER_PORT="${PRIMARY_CONTROLLER_PORT:-60039}"
SYNC_WEB_DIST="${SYNC_WEB_DIST:-1}"
SYNC_DATABASE="${SYNC_DATABASE:-1}"
VERIFY_TOOL_VERSIONS="${VERIFY_TOOL_VERSIONS:-0}"
ALLOW_VERSION_MISMATCH="${ALLOW_VERSION_MISMATCH:-0}"
SSH_TIMEOUT="${SSH_TIMEOUT:-10}"

sanitize_line() {
  echo "$1" | tr '\n' ' ' | sed 's/[[:space:]]\+/ /g; s/^ *//; s/ *$//'
}

step_result() {
  local step="$1"
  local state="$2"
  local msg="$3"
  echo "STEP_RESULT|${step}|${state}|$(sanitize_line "${msg}")"
}

die_step() {
  local step="$1"
  local msg="$2"
  step_result "${step}" "fail" "${msg}"
  exit 1
}

extract_key_file() {
  local src="$1"
  local out="$2"
  if [[ ! -f "${src}" ]]; then
    return 1
  fi
  if head -n 1 "${src}" | grep -q "BEGIN OPENSSH PRIVATE KEY"; then
    cp "${src}" "${out}"
  else
    awk '
      /-----BEGIN OPENSSH PRIVATE KEY-----/ {in_key=1}
      in_key {print}
      /-----END OPENSSH PRIVATE KEY-----/ {if (in_key) exit}
    ' "${src}" > "${out}"
  fi
  if ! grep -q "BEGIN OPENSSH PRIVATE KEY" "${out}" || ! grep -q "END OPENSSH PRIVATE KEY" "${out}"; then
    return 1
  fi
  chmod 600 "${out}" || true
  return 0
}

read_yaml_scalar() {
  local file="$1"
  local key="$2"
  if [[ ! -f "${file}" ]]; then
    return 0
  fi
  local line
  line="$(grep -E "^[[:space:]]*${key}:[[:space:]]*" "${file}" | head -n1 || true)"
  line="$(echo "${line}" | sed -E 's/^[^:]+:[[:space:]]*//; s/[[:space:]]+#.*$//')"
  line="$(echo "${line}" | sed -E 's/^"(.*)"$/\1/; s/^\x27(.*)\x27$/\1/')"
  echo "$(echo "${line}" | xargs)"
}

tool_go_ver() { go version 2>/dev/null | awk '{print $3}' || true; }
tool_node_ver() { node -v 2>/dev/null || true; }
tool_pnpm_ver() { pnpm -v 2>/dev/null || true; }
tool_docker_ver() { docker --version 2>/dev/null | awk '{print $3}' | tr -d ',' || true; }
tool_psql_ver() { psql --version 2>/dev/null | awk '{print $3}' || true; }

require_cmd() {
  local missing=0
  for cmd in "$@"; do
    if ! command -v "${cmd}" >/dev/null 2>&1; then
      echo "${cmd}"
      missing=1
    fi
  done
  if [[ "${missing}" == "1" ]]; then
    return 0
  fi
  return 1
}

TMP_KEY="$(mktemp /tmp/ha-sync-key.XXXXXX)"
trap 'rm -f "${TMP_KEY}"' EXIT
if ! extract_key_file "${DR_KEY_FILE}" "${TMP_KEY}"; then
  die_step "prepare_key" "无法解析容灾节点私钥文件：${DR_KEY_FILE}"
fi
step_result "prepare_key" "ok" "已加载容灾节点私钥"

if require_cmd ssh scp sha256sum awk sed grep >/tmp/ha-sync-missing-cmd.txt 2>/dev/null; then
  missing_cmds="$(cat /tmp/ha-sync-missing-cmd.txt | tr '\n' ' ')"
  rm -f /tmp/ha-sync-missing-cmd.txt
  die_step "precheck_cmd" "缺少命令：${missing_cmds}"
fi
rm -f /tmp/ha-sync-missing-cmd.txt
step_result "precheck_cmd" "ok" "本机命令检查通过"

local_ips=" $(hostname -I 2>/dev/null || true) 127.0.0.1 ::1 "
resolved_dr_ips="$(getent ahosts "${DR_HOST}" 2>/dev/null | awk '{print $1}' | sort -u | xargs || true)"
for resolved_ip in ${resolved_dr_ips}; do
  if [[ "${local_ips}" == *" ${resolved_ip} "* ]]; then
    die_step "validate_target" "容灾节点 ${DR_HOST} 解析到本机 ${resolved_ip}，已拒绝自同步"
  fi
done
step_result "validate_target" "ok" "容灾目标为独立主机：${DR_HOST}"

KNOWN_HOSTS_FILE="${HA_KNOWN_HOSTS_FILE:-${ROOT_DIR}/config/ha_known_hosts}"
mkdir -p "$(dirname "${KNOWN_HOSTS_FILE}")"
touch "${KNOWN_HOSTS_FILE}"
chmod 600 "${KNOWN_HOSTS_FILE}" || true
SSH_OPTS=(-i "${TMP_KEY}" -p "${DR_SSH_PORT}" -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="${KNOWN_HOSTS_FILE}" -o ConnectTimeout="${SSH_TIMEOUT}")
SCP_OPTS=(-i "${TMP_KEY}" -P "${DR_SSH_PORT}" -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="${KNOWN_HOSTS_FILE}" -o ConnectTimeout="${SSH_TIMEOUT}")
REMOTE="${DR_SSH_USER}@${DR_HOST}"

if ! ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "echo ok" >/dev/null 2>&1; then
  die_step "connectivity" "SSH 连接失败：${REMOTE}:${DR_SSH_PORT}"
fi
step_result "connectivity" "ok" "SSH 连接容灾节点成功"

# 版本一致性校验
if [[ "${VERIFY_TOOL_VERSIONS}" == "1" ]]; then
  local_go="$(tool_go_ver)"; [[ -z "${local_go}" ]] && local_go="missing"
  local_node="$(tool_node_ver)"; [[ -z "${local_node}" ]] && local_node="missing"
  local_pnpm="$(tool_pnpm_ver)"; [[ -z "${local_pnpm}" ]] && local_pnpm="missing"
  local_docker="$(tool_docker_ver)"; [[ -z "${local_docker}" ]] && local_docker="missing"
  local_psql="$(tool_psql_ver)"; [[ -z "${local_psql}" ]] && local_psql="missing"

  remote_go="$(ssh -n "${SSH_OPTS[@]}" "${REMOTE}" 'go version 2>/dev/null | awk "{print \$3}" || true' | xargs)"; [[ -z "${remote_go}" ]] && remote_go="missing"
  remote_node="$(ssh -n "${SSH_OPTS[@]}" "${REMOTE}" 'node -v 2>/dev/null || true' | xargs)"; [[ -z "${remote_node}" ]] && remote_node="missing"
  remote_pnpm="$(ssh -n "${SSH_OPTS[@]}" "${REMOTE}" 'pnpm -v 2>/dev/null || true' | xargs)"; [[ -z "${remote_pnpm}" ]] && remote_pnpm="missing"
  remote_docker="$(ssh -n "${SSH_OPTS[@]}" "${REMOTE}" 'docker --version 2>/dev/null | awk "{print \$3}" | tr -d "," || true' | xargs)"; [[ -z "${remote_docker}" ]] && remote_docker="missing"
  remote_psql="$(ssh -n "${SSH_OPTS[@]}" "${REMOTE}" 'psql --version 2>/dev/null | awk "{print \$3}" || true' | xargs)"; [[ -z "${remote_psql}" ]] && remote_psql="missing"

  mismatch=""
  [[ "${local_go}" != "${remote_go}" ]] && mismatch+="go(local=${local_go},remote=${remote_go}) "
  [[ "${local_node}" != "${remote_node}" ]] && mismatch+="node(local=${local_node},remote=${remote_node}) "
  [[ "${local_pnpm}" != "${remote_pnpm}" ]] && mismatch+="pnpm(local=${local_pnpm},remote=${remote_pnpm}) "
  [[ "${local_docker}" != "${remote_docker}" ]] && mismatch+="docker(local=${local_docker},remote=${remote_docker}) "
  [[ "${local_psql}" != "${remote_psql}" ]] && mismatch+="psql(local=${local_psql},remote=${remote_psql}) "

  if [[ -n "${mismatch}" ]]; then
    if [[ "${ALLOW_VERSION_MISMATCH}" == "1" ]]; then
      step_result "check_versions" "ok" "检测到版本不一致，但已按 ALLOW_VERSION_MISMATCH=1 放行：${mismatch}"
    else
      die_step "check_versions" "主备工具版本不一致：${mismatch}"
    fi
  else
    step_result "check_versions" "ok" "主备工具版本一致（go/node/pnpm/docker/psql）"
  fi
else
  step_result "check_versions" "ok" "已跳过版本校验（VERIFY_TOOL_VERSIONS=${VERIFY_TOOL_VERSIONS}）"
fi

SUDO_LOCAL=""
if [[ "$(id -u)" -ne 0 ]]; then
  SUDO_LOCAL="sudo"
fi

sync_p2s_binary() {
  [[ -f "${LOCAL_BIN}" ]] || die_step "sync_binary" "本机二进制不存在：${LOCAL_BIN}"
  tmp_remote="/tmp/gpu-controller.ha.$$.bin"
  if ! scp -q "${SCP_OPTS[@]}" "${LOCAL_BIN}" "${REMOTE}:${tmp_remote}"; then
    die_step "sync_binary" "上传控制器二进制失败"
  fi
  if ! ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "sudo -n '${REMOTE_APPLY_HELPER}' install-controller '${tmp_remote}'"; then
    die_step "sync_binary" "安装容灾控制器二进制失败，请检查远端 HA helper"
  fi
  local_sha="$(sha256sum "${LOCAL_BIN}" | awk '{print $1}')"
  remote_sha="$(ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "sha256sum '${REMOTE_BIN}'" | awk '{print $1}' | xargs)"
  [[ "${local_sha}" == "${remote_sha}" ]] || die_step "sync_binary" "二进制哈希不一致 local=${local_sha} remote=${remote_sha}"
  step_result "sync_binary" "ok" "主->备二进制同步完成 sha256=${local_sha}"
}

resolve_local_postgres_container() {
  if [[ -n "${LOCAL_POSTGRES_CONTAINER}" ]]; then
    docker inspect "${LOCAL_POSTGRES_CONTAINER}" >/dev/null 2>&1 || return 1
    return 0
  fi
  local -a candidates=()
  local container_id query_result
  mapfile -t candidates < <(docker ps --filter 'publish=5432' --format '{{.ID}}' 2>/dev/null)
  if (( ${#candidates[@]} == 1 )); then
    LOCAL_POSTGRES_CONTAINER="${candidates[0]}"
    return 0
  fi
  candidates=()
  while IFS= read -r container_id; do
    [[ -n "${container_id}" ]] || continue
    query_result="$(docker exec "${container_id}" psql --username="${POSTGRES_USER}" \
      --dbname="${POSTGRES_DATABASE}" --tuples-only --no-align --command='SELECT 1' 2>/dev/null || true)"
    [[ "${query_result}" == "1" ]] && candidates+=("${container_id}")
  done < <(docker ps --quiet)
  (( ${#candidates[@]} == 1 )) || return 1
  LOCAL_POSTGRES_CONTAINER="${candidates[0]}"
}

create_local_database_dump() {
  local dsn="$1" dump_file="$2"
  if command -v pg_dump >/dev/null 2>&1 && command -v pg_restore >/dev/null 2>&1; then
    pg_dump --format=custom --compress=6 --no-owner --no-privileges --file="${dump_file}" "${dsn}"
    pg_restore --list "${dump_file}" >/dev/null
    return
  fi
  command -v docker >/dev/null 2>&1 || return 1
  resolve_local_postgres_container || return 1
  docker exec "${LOCAL_POSTGRES_CONTAINER}" pg_dump \
    --username="${POSTGRES_USER}" --dbname="${POSTGRES_DATABASE}" \
    --format=custom --compress=6 --no-owner --no-privileges >"${dump_file}"
  docker exec -i "${LOCAL_POSTGRES_CONTAINER}" pg_restore --list <"${dump_file}" >/dev/null
}

restore_local_database_dump() {
  local dsn="$1" dump_file="$2"
  if command -v pg_restore >/dev/null 2>&1; then
    pg_restore --clean --if-exists --no-owner --no-privileges --exit-on-error \
      --single-transaction --dbname="${dsn}" "${dump_file}"
    return
  fi
  command -v docker >/dev/null 2>&1 || return 1
  resolve_local_postgres_container || return 1
  docker exec -i "${LOCAL_POSTGRES_CONTAINER}" pg_restore \
    --username="${POSTGRES_USER}" --dbname="${POSTGRES_DATABASE}" \
    --clean --if-exists --no-owner --no-privileges --exit-on-error \
    --single-transaction <"${dump_file}"
}

sync_s2p_binary() {
  tmp_local="$(mktemp /tmp/gpu-controller.ha.sync.XXXXXX)"
  if ! scp -q "${SCP_OPTS[@]}" "${REMOTE}:${REMOTE_BIN}" "${tmp_local}"; then
    die_step "sync_binary" "下载容灾控制器二进制失败"
  fi
  if ! ${SUDO_LOCAL} install -m 0755 "${tmp_local}" "${LOCAL_BIN}"; then
    die_step "sync_binary" "安装本机控制器二进制失败"
  fi
  remote_sha="$(ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "sha256sum '${REMOTE_BIN}'" | awk '{print $1}' | xargs)"
  local_sha="$(sha256sum "${LOCAL_BIN}" | awk '{print $1}')"
  [[ "${local_sha}" == "${remote_sha}" ]] || die_step "sync_binary" "二进制哈希不一致 local=${local_sha} remote=${remote_sha}"
  rm -f "${tmp_local}" || true
  step_result "sync_binary" "ok" "备->主二进制同步完成 sha256=${local_sha}"
}

sync_p2s_web() {
  if [[ "${SYNC_WEB_DIST}" != "1" ]]; then
    step_result "sync_web_dist" "ok" "已跳过（SYNC_WEB_DIST=${SYNC_WEB_DIST}）"
    return 0
  fi
  if [[ ! -d "${LOCAL_WEB_DIST}" ]]; then
    die_step "sync_web_dist" "本机 web/dist 不存在：${LOCAL_WEB_DIST}"
  fi
  if command -v rsync >/dev/null 2>&1; then
    if ! rsync -az --delete -e "ssh -i ${TMP_KEY} -p ${DR_SSH_PORT} -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=${KNOWN_HOSTS_FILE} -o ConnectTimeout=${SSH_TIMEOUT}" "${LOCAL_WEB_DIST}" "${REMOTE}:${REMOTE_WEB_DIST}"; then
      die_step "sync_web_dist" "主->备 web/dist 同步失败"
    fi
  else
    if ! tar -C "${LOCAL_WEB_DIST}" -cf - . | ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "mkdir -p '${REMOTE_WEB_DIST}' && tar -C '${REMOTE_WEB_DIST}' -xf -"; then
      die_step "sync_web_dist" "主->备 web/dist 同步失败"
    fi
  fi
  step_result "sync_web_dist" "ok" "主->备 web/dist 同步完成"
}

sync_s2p_web() {
  if [[ "${SYNC_WEB_DIST}" != "1" ]]; then
    step_result "sync_web_dist" "ok" "已跳过（SYNC_WEB_DIST=${SYNC_WEB_DIST}）"
    return 0
  fi
  mkdir -p "${LOCAL_WEB_DIST}"
  if command -v rsync >/dev/null 2>&1; then
    if ! rsync -az --delete -e "ssh -i ${TMP_KEY} -p ${DR_SSH_PORT} -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=${KNOWN_HOSTS_FILE} -o ConnectTimeout=${SSH_TIMEOUT}" "${REMOTE}:${REMOTE_WEB_DIST}" "${LOCAL_WEB_DIST}"; then
      die_step "sync_web_dist" "备->主 web/dist 同步失败"
    fi
  else
    if ! ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "tar -C '${REMOTE_WEB_DIST}' -cf - ." | tar -C "${LOCAL_WEB_DIST}" -xf -; then
      die_step "sync_web_dist" "备->主 web/dist 同步失败"
    fi
  fi
  step_result "sync_web_dist" "ok" "备->主 web/dist 同步完成"
}

sync_p2s_database() {
  if [[ "${SYNC_DATABASE}" != "1" ]]; then
    step_result "sync_database" "ok" "已跳过（SYNC_DATABASE=${SYNC_DATABASE}）"
    return 0
  fi
  [[ -f "${LOCAL_CONFIG_PATH}" ]] || die_step "sync_database" "本机配置不存在：${LOCAL_CONFIG_PATH}"
  src_dsn="$(read_yaml_scalar "${LOCAL_CONFIG_PATH}" "database_dsn")"
  dst_dsn="$(ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "grep -E '^[[:space:]]*database_dsn:' '${REMOTE_CONFIG_PATH}' | head -n1" | sed -E 's/^[^:]+:[[:space:]]*//; s/[[:space:]]+#.*$//; s/^"//; s/"$//' | xargs)"
  [[ -n "${src_dsn}" ]] || die_step "sync_database" "本机 database_dsn 为空"
  [[ -n "${dst_dsn}" ]] || die_step "sync_database" "容灾 database_dsn 为空"
  # 两端 DSN 都可能写成 127.0.0.1，但分别指向各自主机上的独立数据库，不能按字符串相等跳过。
  dump_local="$(mktemp /tmp/gpuops-ha.XXXXXX.dump)"
  dump_remote="/tmp/gpuops-ha.$$.dump"
  if ! create_local_database_dump "${src_dsn}" "${dump_local}"; then
    die_step "sync_database" "生成主数据库归档失败"
  fi
  if ! scp -q "${SCP_OPTS[@]}" "${dump_local}" "${REMOTE}:${dump_remote}"; then
    die_step "sync_database" "上传数据库归档失败"
  fi
  if ! ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "sudo -n '${REMOTE_APPLY_HELPER}' restore-database '${dump_remote}'"; then
    die_step "sync_database" "容灾数据库单事务恢复失败，原数据库未被部分覆盖"
  fi
  rm -f "${dump_local}"
  step_result "sync_database" "ok" "主->备数据库归档校验及单事务恢复完成"
}

sync_s2p_database() {
  if [[ "${SYNC_DATABASE}" != "1" ]]; then
    step_result "sync_database" "ok" "已跳过（SYNC_DATABASE=${SYNC_DATABASE}）"
    return 0
  fi
  [[ -f "${LOCAL_CONFIG_PATH}" ]] || die_step "sync_database" "本机配置不存在：${LOCAL_CONFIG_PATH}"
  dst_dsn="$(read_yaml_scalar "${LOCAL_CONFIG_PATH}" "database_dsn")"
  src_dsn="$(ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "grep -E '^[[:space:]]*database_dsn:' '${REMOTE_CONFIG_PATH}' | head -n1" | sed -E 's/^[^:]+:[[:space:]]*//; s/[[:space:]]+#.*$//; s/^"//; s/"$//' | xargs)"
  [[ -n "${src_dsn}" ]] || die_step "sync_database" "容灾 database_dsn 为空"
  [[ -n "${dst_dsn}" ]] || die_step "sync_database" "本机 database_dsn 为空"
  # 两端 DSN 都可能写成 127.0.0.1，但分别指向各自主机上的独立数据库，必须执行回切同步。
  dump_local="$(mktemp /tmp/gpuops-ha-recovery.XXXXXX.dump)"
  dump_remote="/tmp/gpuops-ha-recovery.$$.dump"
  if ! ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "sudo -n '${REMOTE_APPLY_HELPER}' dump-database '${dump_remote}'"; then
    die_step "sync_database" "生成容灾数据库归档失败"
  fi
  if ! scp -q "${SCP_OPTS[@]}" "${REMOTE}:${dump_remote}" "${dump_local}"; then
    die_step "sync_database" "下载容灾数据库归档失败"
  fi
  ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "rm -f '${dump_remote}'" || true
  if ! restore_local_database_dump "${dst_dsn}" "${dump_local}"; then
    die_step "sync_database" "主数据库单事务回切失败，原数据库未被部分覆盖"
  fi
  rm -f "${dump_local}"
  step_result "sync_database" "ok" "备->主数据库归档校验及单事务恢复完成"
}

restart_remote_service() {
  if ! ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "sudo -n '${REMOTE_APPLY_HELPER}' restart-controller"; then
    die_step "restart_service" "容灾服务重启或存活检查失败：${REMOTE_SERVICE_NAME}"
  fi
  step_result "restart_service" "ok" "容灾服务重启成功：${REMOTE_SERVICE_NAME}"
}

restart_local_service() {
  if ! ${SUDO_LOCAL} systemctl restart "${LOCAL_SERVICE_NAME}"; then
    die_step "restart_service" "本机服务重启失败：${LOCAL_SERVICE_NAME}"
  fi
  if ! ${SUDO_LOCAL} systemctl is-active "${LOCAL_SERVICE_NAME}" >/dev/null; then
    die_step "restart_service" "本机服务未恢复 active：${LOCAL_SERVICE_NAME}"
  fi
  step_result "restart_service" "ok" "本机服务重启成功：${LOCAL_SERVICE_NAME}"
}

case "${HA_SYNC_DIRECTION}" in
  primary_to_standby|p2s|primary2standby)
    sync_p2s_binary
    sync_p2s_web
    sync_p2s_database
    restart_remote_service
    ;;
  standby_to_primary|s2p|standby2primary)
    sync_s2p_binary
    sync_s2p_web
    sync_s2p_database
    restart_local_service
    ;;
  *)
    die_step "direction" "不支持的 HA_SYNC_DIRECTION=${HA_SYNC_DIRECTION}"
    ;;
esac

step_result "done" "ok" "容灾同步执行完成 direction=${HA_SYNC_DIRECTION}"
