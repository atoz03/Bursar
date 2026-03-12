#!/usr/bin/env bash
# 批量分发当前仓库到各节点 /home/<用户名>/<目录名>
# 依赖：bash, ssh, tar

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MAP_FILE="${MAP_FILE:-${ROOT_DIR}/my_ssh_keys/server_ssh_map.csv}"
KEY_DIR="${KEY_DIR:-${ROOT_DIR}/my_ssh_keys}"
SOURCE_DIR="${SOURCE_DIR:-${ROOT_DIR}}"
PROJECT_DIR_NAME="${PROJECT_DIR_NAME:-$(basename "${ROOT_DIR}")}"
TARGET_BASE="${TARGET_BASE:-/home}"
SSH_TIMEOUT="${SSH_TIMEOUT:-10}"
DRY_RUN="${DRY_RUN:-0}"
PARALLEL="${PARALLEL:-6}"
NODE_IDS_RAW="${NODE_IDS:-}"
NODE_IDS_CANON=""
NODE_IDS_DISPLAY=""

init_node_filter() {
  local raw="${NODE_IDS_RAW}"
  raw="${raw//$'\n'/ }"
  raw="${raw//$'\t'/ }"
  raw="${raw//,/ }"
  for node in ${raw}; do
    [[ -z "${node}" ]] && continue
    if [[ "${NODE_IDS_CANON}" != *",${node},"* ]]; then
      NODE_IDS_CANON+=",${node},"
      if [[ -n "${NODE_IDS_DISPLAY}" ]]; then
        NODE_IDS_DISPLAY+=" "
      fi
      NODE_IDS_DISPLAY+="${node}"
    fi
  done
}

should_process_node() {
  local node_id="$1"
  [[ -z "${NODE_IDS_CANON}" || "${NODE_IDS_CANON}" == *",${node_id},"* ]]
}

if [[ ! -f "${MAP_FILE}" ]]; then
  echo "映射表不存在：${MAP_FILE}" >&2
  exit 2
fi

if [[ ! -d "${SOURCE_DIR}" ]]; then
  echo "源目录不存在：${SOURCE_DIR}" >&2
  exit 2
fi

init_node_filter

echo "SOURCE_DIR=${SOURCE_DIR}"
echo "MAP_FILE=${MAP_FILE}"
echo "TARGET=<${TARGET_BASE}>/<用户名>/${PROJECT_DIR_NAME}"
echo "PARALLEL=${PARALLEL}"
if [[ -n "${NODE_IDS_DISPLAY}" ]]; then
  echo "NODE_IDS=${NODE_IDS_DISPLAY}"
else
  echo "NODE_IDS=<all>"
fi

copy_one() {
  local txt_file="$1"
  local port="$2"
  local ip="$3"
  local node_id="$4"
  local user="$5"

  user="$(echo "${user}" | xargs)"
  if [[ -z "${user}" || "${user}" == "TODO" ]]; then
    echo "[SKIP][${node_id}] 用户名未配置（${user}）"
    return 0
  fi

  local key_path="${KEY_DIR}/${txt_file}"
  if [[ ! -f "${key_path}" ]]; then
    echo "[SKIP][${node_id}] 私钥文件不存在：${key_path}"
    return 0
  fi

  if grep -q "REPLACE_WITH_REAL_PRIVATE_KEY_CONTENT" "${key_path}"; then
    echo "[SKIP][${node_id}] 私钥文件仍是模板：${key_path}"
    return 0
  fi

  chmod 600 "${key_path}" || true

  local key_use_path="${key_path}"
  if ! head -n 1 "${key_path}" | grep -q "BEGIN OPENSSH PRIVATE KEY"; then
    key_use_path="$(mktemp)"
    awk '
      /-----BEGIN OPENSSH PRIVATE KEY-----/ {in_key=1}
      in_key {print}
      /-----END OPENSSH PRIVATE KEY-----/ {if (in_key) exit}
    ' "${key_path}" > "${key_use_path}"
    if ! grep -q "BEGIN OPENSSH PRIVATE KEY" "${key_use_path}" || ! grep -q "END OPENSSH PRIVATE KEY" "${key_use_path}"; then
      rm -f "${key_use_path}"
      echo "[SKIP][${node_id}] 未找到有效 OpenSSH 私钥块"
      return 0
    fi
    chmod 600 "${key_use_path}" || true
  fi

  local target_dir="${TARGET_BASE}/${user}/${PROJECT_DIR_NAME}"
  local ssh_cmd=(ssh -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}")
  local ssh_cmd_no_stdin=(ssh -n -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}")

  echo "[COPY][${node_id}] ${user}@${ip}:${target_dir}"

  if [[ "${DRY_RUN}" == "1" ]]; then
    if [[ "${key_use_path}" != "${key_path}" ]]; then
      rm -f "${key_use_path}" || true
    fi
    return 0
  fi

  "${ssh_cmd_no_stdin[@]}" "mkdir -p '${target_dir}'"

  tar -C "${SOURCE_DIR}" \
    --exclude='.git' \
    --exclude='my_ssh_keys/*.txt' \
    --exclude='web/node_modules' \
    --exclude='web/dist' \
    --exclude='controller/tmp' \
    -cf - . \
    | "${ssh_cmd[@]}" "tar -C '${target_dir}' -xf -"

  echo "[DONE][${node_id}]"

  if [[ "${key_use_path}" != "${key_path}" ]]; then
    rm -f "${key_use_path}" || true
  fi
}

wait_for_slot() {
  local limit="$1"
  while true; do
    local running
    running="$(jobs -rp | wc -l | tr -d ' ')"
    if [[ -z "${running}" ]]; then
      running=0
    fi
    if (( running < limit )); then
      break
    fi
    sleep 0.1
  done
}

# 读取 CSV（跳过表头）
# 列：txt文件名,端口号,内部ip,节点id,txt对应的用户名
job_pids=()
while IFS=',' read -r txt_file port ip node_id user <&3; do
  if [[ "${txt_file}" == "txt文件名" ]]; then
    continue
  fi
  txt_file="$(echo "${txt_file}" | xargs)"
  port="$(echo "${port}" | xargs)"
  ip="$(echo "${ip}" | xargs)"
  node_id="$(echo "${node_id}" | xargs)"
  user="$(echo "${user}" | xargs)"

  if [[ -z "${txt_file}" || -z "${port}" || -z "${ip}" || -z "${node_id}" ]]; then
    echo "[SKIP] 行字段不完整：${txt_file},${port},${ip},${node_id},${user}"
    continue
  fi
  if ! should_process_node "${node_id}"; then
    continue
  fi

  wait_for_slot "${PARALLEL}"
  (
    copy_one "${txt_file}" "${port}" "${ip}" "${node_id}" "${user}"
  ) &
  job_pids+=("$!")
done 3< "${MAP_FILE}"

fail_count=0
for pid in "${job_pids[@]}"; do
  if ! wait "${pid}"; then
    fail_count=$((fail_count + 1))
  fi
done

if (( fail_count > 0 )); then
  echo "全部处理完成，但有 ${fail_count} 个任务失败。" >&2
  exit 1
fi
echo "全部处理完成。"
