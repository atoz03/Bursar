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

if [[ ! -f "${MAP_FILE}" ]]; then
  echo "映射表不存在：${MAP_FILE}" >&2
  exit 2
fi

if [[ ! -d "${SOURCE_DIR}" ]]; then
  echo "源目录不存在：${SOURCE_DIR}" >&2
  exit 2
fi

echo "SOURCE_DIR=${SOURCE_DIR}"
echo "MAP_FILE=${MAP_FILE}"
echo "TARGET=<${TARGET_BASE}>/<用户名>/${PROJECT_DIR_NAME}"

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

  echo "[COPY][${node_id}] ${user}@${ip}:${target_dir}"

  if [[ "${DRY_RUN}" == "1" ]]; then
    if [[ "${key_use_path}" != "${key_path}" ]]; then
      rm -f "${key_use_path}" || true
    fi
    return 0
  fi

  "${ssh_cmd[@]}" "mkdir -p '${target_dir}'"

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

# 读取 CSV（跳过表头）
# 列：txt文件名,端口号,内部ip,节点id,txt对应的用户名
while IFS=',' read -r txt_file port ip node_id user; do
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

  copy_one "${txt_file}" "${port}" "${ip}" "${node_id}" "${user}"
done < "${MAP_FILE}"

echo "全部处理完成。"
