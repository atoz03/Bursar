#!/usr/bin/env bash
# 容灾同步执行器：支持主->备、备->主双向同步（版本校验 + 二进制/前端/数据库）
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCAL_BIN="${LOCAL_BIN:-/usr/local/bin/gpu-controller}"
LOCAL_CONFIG_PATH="${LOCAL_CONFIG_PATH:-${ROOT_DIR}/config/controller.yaml}"
LOCAL_WEB_DIST="${LOCAL_WEB_DIST:-${ROOT_DIR}/web/dist/}"
REMOTE_PROJECT_DIR="${REMOTE_PROJECT_DIR:-/home/${DR_SSH_USER:-gpuops}/gpu-ops}"
REMOTE_BIN="${REMOTE_BIN:-/usr/local/bin/gpu-controller}"
REMOTE_CONFIG_PATH="${REMOTE_CONFIG_PATH:-${REMOTE_PROJECT_DIR}/config/controller.yaml}"
REMOTE_WEB_DIST="${REMOTE_WEB_DIST:-${REMOTE_PROJECT_DIR}/web/dist/}"
REMOTE_SERVICE_NAME="${REMOTE_SERVICE_NAME:-gpu-controller}"
LOCAL_SERVICE_NAME="${LOCAL_SERVICE_NAME:-gpu-controller}"

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
VERIFY_TOOL_VERSIONS="${VERIFY_TOOL_VERSIONS:-1}"
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

SSH_OPTS=(-i "${TMP_KEY}" -p "${DR_SSH_PORT}" -o StrictHostKeyChecking=no -o ConnectTimeout="${SSH_TIMEOUT}")
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
  scp -q "${SSH_OPTS[@]}" "${LOCAL_BIN}" "${REMOTE}:${tmp_remote}"
  ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "sudo -n install -m 0755 '${tmp_remote}' '${REMOTE_BIN}' && rm -f '${tmp_remote}'"
  local_sha="$(sha256sum "${LOCAL_BIN}" | awk '{print $1}')"
  remote_sha="$(ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "sha256sum '${REMOTE_BIN}' | awk '{print \\\$1}'" | xargs)"
  [[ "${local_sha}" == "${remote_sha}" ]] || die_step "sync_binary" "二进制哈希不一致 local=${local_sha} remote=${remote_sha}"
  step_result "sync_binary" "ok" "主->备二进制同步完成 sha256=${local_sha}"
}

sync_s2p_binary() {
  tmp_local="$(mktemp /tmp/gpu-controller.ha.sync.XXXXXX)"
  scp -q "${SSH_OPTS[@]}" "${REMOTE}:${REMOTE_BIN}" "${tmp_local}"
  ${SUDO_LOCAL} install -m 0755 "${tmp_local}" "${LOCAL_BIN}"
  remote_sha="$(ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "sha256sum '${REMOTE_BIN}' | awk '{print \\\$1}'" | xargs)"
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
    rsync -az --delete -e "ssh -i ${TMP_KEY} -p ${DR_SSH_PORT} -o StrictHostKeyChecking=no -o ConnectTimeout=${SSH_TIMEOUT}" "${LOCAL_WEB_DIST}" "${REMOTE}:${REMOTE_WEB_DIST}"
  else
    tar -C "${LOCAL_WEB_DIST}" -cf - . | ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "mkdir -p '${REMOTE_WEB_DIST}' && tar -C '${REMOTE_WEB_DIST}' -xf -"
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
    rsync -az --delete -e "ssh -i ${TMP_KEY} -p ${DR_SSH_PORT} -o StrictHostKeyChecking=no -o ConnectTimeout=${SSH_TIMEOUT}" "${REMOTE}:${REMOTE_WEB_DIST}" "${LOCAL_WEB_DIST}"
  else
    ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "tar -C '${REMOTE_WEB_DIST}' -cf - ." | tar -C "${LOCAL_WEB_DIST}" -xf -
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
  if [[ "${src_dsn}" == "${dst_dsn}" ]]; then
    step_result "sync_database" "ok" "主备共用同一数据库 DSN，无需额外同步"
    return 0
  fi
  pg_dump --clean --if-exists --no-owner --no-privileges "${src_dsn}" \
    | ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "psql '${dst_dsn}'"
  step_result "sync_database" "ok" "主->备数据库同步完成（独立数据库模式）"
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
  if [[ "${src_dsn}" == "${dst_dsn}" ]]; then
    step_result "sync_database" "ok" "主备共用同一数据库 DSN，无需额外同步"
    return 0
  fi
  ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "pg_dump --clean --if-exists --no-owner --no-privileges '${src_dsn}'" \
    | psql "${dst_dsn}"
  step_result "sync_database" "ok" "备->主数据库同步完成（独立数据库模式）"
}

restart_remote_service() {
  ssh -n "${SSH_OPTS[@]}" "${REMOTE}" "sudo -n systemctl restart '${REMOTE_SERVICE_NAME}' && sudo -n systemctl is-active '${REMOTE_SERVICE_NAME}'"
  step_result "restart_service" "ok" "容灾服务重启成功：${REMOTE_SERVICE_NAME}"
}

restart_local_service() {
  ${SUDO_LOCAL} systemctl restart "${LOCAL_SERVICE_NAME}"
  ${SUDO_LOCAL} systemctl is-active "${LOCAL_SERVICE_NAME}" >/dev/null
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
