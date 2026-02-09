#!/usr/bin/env bash
# 检查各节点 SSH 连通性并输出报告
# 依赖：bash, ssh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MAP_FILE="${MAP_FILE:-${ROOT_DIR}/my_ssh_keys/server_ssh_map.csv}"
KEY_DIR="${KEY_DIR:-${ROOT_DIR}/my_ssh_keys}"
SSH_TIMEOUT="${SSH_TIMEOUT:-8}"
OUT_FILE="${OUT_FILE:-${ROOT_DIR}/my_ssh_keys/connectivity_report.txt}"
VERBOSE="${VERBOSE:-0}"

if [[ ! -f "${MAP_FILE}" ]]; then
  echo "映射表不存在：${MAP_FILE}" >&2
  exit 2
fi

ok_count=0
fail_count=0
skip_count=0

ok_lines=()
fail_lines=()
skip_lines=()

check_one() {
  local txt_file="$1"
  local port="$2"
  local ip="$3"
  local node_id="$4"
  local user="$5"

  txt_file="$(echo "${txt_file}" | xargs)"
  port="$(echo "${port}" | xargs)"
  ip="$(echo "${ip}" | xargs)"
  node_id="$(echo "${node_id}" | xargs)"
  user="$(echo "${user}" | xargs)"

  local prefix="[${node_id}] ${user}@${ip}:${port}"

  if [[ -z "${user}" || "${user}" == "TODO" ]]; then
    skip_count=$((skip_count + 1))
    skip_lines+=("${prefix} -> SKIP: 用户名未配置")
    return 0
  fi

  local key_path="${KEY_DIR}/${txt_file}"
  if [[ ! -f "${key_path}" ]]; then
    skip_count=$((skip_count + 1))
    skip_lines+=("${prefix} -> SKIP: 私钥文件不存在 (${key_path})")
    return 0
  fi

  if grep -q "REPLACE_WITH_REAL_PRIVATE_KEY_CONTENT" "${key_path}"; then
    skip_count=$((skip_count + 1))
    skip_lines+=("${prefix} -> SKIP: 私钥还是模板")
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
      skip_count=$((skip_count + 1))
      skip_lines+=("${prefix} -> SKIP: 未找到有效 OpenSSH 私钥块")
      return 0
    fi
    chmod 600 "${key_use_path}" || true
  fi

  local ssh_cmd=(
    ssh
    -i "${key_use_path}"
    -p "${port}"
    -o BatchMode=yes
    -o StrictHostKeyChecking=no
    -o ConnectTimeout="${SSH_TIMEOUT}"
    -o PasswordAuthentication=no
    "${user}@${ip}"
    "echo ok"
  )

  if [[ "${VERBOSE}" == "1" ]]; then
    echo "TEST ${prefix}"
  fi

  local out
  if out="$(${ssh_cmd[@]} 2>&1)"; then
    if echo "${out}" | grep -q "ok"; then
      ok_count=$((ok_count + 1))
      ok_lines+=("${prefix} -> OK")
    else
      fail_count=$((fail_count + 1))
      fail_lines+=("${prefix} -> FAIL: 无预期响应")
    fi
  else
    fail_count=$((fail_count + 1))
    fail_lines+=("${prefix} -> FAIL: $(echo "${out}" | tr '\n' ' ' | sed 's/[[:space:]]\+/ /g' | cut -c1-300)")
  fi

  if [[ "${key_use_path}" != "${key_path}" ]]; then
    rm -f "${key_use_path}" || true
  fi
}

# 读 CSV：txt文件名,端口号,内部ip,节点id,txt对应的用户名
while IFS=',' read -r txt_file port ip node_id user; do
  if [[ "${txt_file}" == "txt文件名" ]]; then
    continue
  fi
  if [[ -z "${txt_file}${port}${ip}${node_id}" ]]; then
    continue
  fi
  check_one "${txt_file}" "${port}" "${ip}" "${node_id}" "${user}"
done < "${MAP_FILE}"

{
  echo "连通性检查时间: $(date '+%Y-%m-%d %H:%M:%S')"
  echo "映射文件: ${MAP_FILE}"
  echo "私钥目录: ${KEY_DIR}"
  echo "SSH 超时: ${SSH_TIMEOUT}s"
  echo
  echo "统计:"
  echo "- OK:   ${ok_count}"
  echo "- FAIL: ${fail_count}"
  echo "- SKIP: ${skip_count}"
  echo

  echo "=== 可连通机器 (OK) ==="
  if [[ ${#ok_lines[@]} -eq 0 ]]; then
    echo "(无)"
  else
    printf '%s\n' "${ok_lines[@]}"
  fi
  echo

  echo "=== 不可连通机器 (FAIL) ==="
  if [[ ${#fail_lines[@]} -eq 0 ]]; then
    echo "(无)"
  else
    printf '%s\n' "${fail_lines[@]}"
  fi
  echo

  echo "=== 跳过机器 (SKIP) ==="
  if [[ ${#skip_lines[@]} -eq 0 ]]; then
    echo "(无)"
  else
    printf '%s\n' "${skip_lines[@]}"
  fi
} | tee "${OUT_FILE}"

if [[ ${fail_count} -gt 0 ]]; then
  exit 1
fi
