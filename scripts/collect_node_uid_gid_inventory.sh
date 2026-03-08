#!/usr/bin/env bash
# 采集所有节点当前已使用的 UID/GID（含主组 gid），并生成本地安全清单文件。

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MAP_FILE="${MAP_FILE:-${ROOT_DIR}/my_ssh_keys/server_ssh_map.csv}"
KEY_DIR="${KEY_DIR:-${ROOT_DIR}/my_ssh_keys}"
SSH_TIMEOUT="${SSH_TIMEOUT:-8}"
OUT_DETAIL="${OUT_DETAIL:-${ROOT_DIR}/my_ssh_keys/node_uid_gid_inventory.tsv}"
OUT_IDS="${OUT_IDS:-${ROOT_DIR}/my_ssh_keys/node_uid_gid_used_ids.txt}"
VERBOSE="${VERBOSE:-0}"

if [[ ! -f "${MAP_FILE}" ]]; then
  echo "映射表不存在：${MAP_FILE}" >&2
  exit 2
fi

tmp_detail="$(mktemp)"
tmp_ids="$(mktemp)"
tmp_fail="$(mktemp)"
trap 'rm -f "${tmp_detail}" "${tmp_ids}" "${tmp_fail}"' EXIT

ok_count=0
fail_count=0
skip_count=0

append_fail() {
  local msg="$1"
  echo "${msg}" >> "${tmp_fail}"
  fail_count=$((fail_count + 1))
}

prepare_key_for_ssh() {
  local key_path="$1"
  local prepared="${key_path}"
  if ! head -n 1 "${key_path}" | grep -q "BEGIN OPENSSH PRIVATE KEY"; then
    prepared="$(mktemp)"
    awk '
      /-----BEGIN OPENSSH PRIVATE KEY-----/ {in_key=1}
      in_key {print}
      /-----END OPENSSH PRIVATE KEY-----/ {if (in_key) exit}
    ' "${key_path}" > "${prepared}"
    if ! grep -q "BEGIN OPENSSH PRIVATE KEY" "${prepared}" || ! grep -q "END OPENSSH PRIVATE KEY" "${prepared}"; then
      rm -f "${prepared}"
      echo ""
      return 1
    fi
  fi
  chmod 600 "${prepared}" || true
  echo "${prepared}"
  return 0
}

collect_one() {
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
    return 0
  fi

  local key_path="${KEY_DIR}/${txt_file}"
  if [[ ! -f "${key_path}" ]]; then
    skip_count=$((skip_count + 1))
    return 0
  fi
  chmod 600 "${key_path}" || true

  local key_use_path
  if ! key_use_path="$(prepare_key_for_ssh "${key_path}")"; then
    skip_count=$((skip_count + 1))
    return 0
  fi

  local remote_cmd
  remote_cmd=$'getent passwd | awk -F: \'NF>=7 { printf "%s:%s:%s:%s:%s\\n", $1, $3, $4, $6, $7 }\''
  if [[ "${VERBOSE}" == "1" ]]; then
    echo "COLLECT ${prefix}"
  fi
  local out
  if ! out="$(ssh -n -i "${key_use_path}" -p "${port}" -o BatchMode=yes -o StrictHostKeyChecking=no -o ConnectTimeout="${SSH_TIMEOUT}" -o PasswordAuthentication=no "${user}@${ip}" "${remote_cmd}" 2>&1)"; then
    append_fail "${prefix} -> ${out}"
    if [[ "${key_use_path}" != "${key_path}" ]]; then
      rm -f "${key_use_path}" || true
    fi
    return 1
  fi

  local line
  while IFS= read -r line; do
    line="$(echo "${line}" | tr -d '\r')"
    [[ -z "${line}" ]] && continue
    IFS=':' read -r name uid gid home shell <<< "${line}"
    uid="$(echo "${uid}" | xargs)"
    gid="$(echo "${gid}" | xargs)"
    [[ -z "${uid}" || -z "${gid}" ]] && continue
    if [[ ! "${uid}" =~ ^[0-9]+$ ]] || [[ ! "${gid}" =~ ^[0-9]+$ ]]; then
      continue
    fi
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "${node_id}" "${ip}" "${name}" "${uid}" "${gid}" "${home}" "${shell}" >> "${tmp_detail}"
    printf '%s\n' "${uid}" >> "${tmp_ids}"
    printf '%s\n' "${gid}" >> "${tmp_ids}"
  done <<< "${out}"

  ok_count=$((ok_count + 1))
  if [[ "${key_use_path}" != "${key_path}" ]]; then
    rm -f "${key_use_path}" || true
  fi
  return 0
}

while IFS=',' read -r txt_file port ip node_id user; do
  if [[ "${txt_file}" == "txt文件名" ]]; then
    continue
  fi
  if [[ -z "${txt_file}${port}${ip}${node_id}" ]]; then
    continue
  fi
  collect_one "${txt_file}" "${port}" "${ip}" "${node_id}" "${user}" || true
done < "${MAP_FILE}"

generated_at="$(date '+%Y-%m-%d %H:%M:%S')"
mkdir -p "$(dirname "${OUT_DETAIL}")"
mkdir -p "$(dirname "${OUT_IDS}")"

{
  echo "# generated_at: ${generated_at}"
  echo "# source_map: ${MAP_FILE}"
  echo "# columns: node_id internal_ip local_username uid primary_gid home shell"
  echo "# summary: ok=${ok_count} fail=${fail_count} skip=${skip_count}"
  echo
  echo -e "node_id\tinternal_ip\tlocal_username\tuid\tprimary_gid\thome\tshell"
  if [[ -s "${tmp_detail}" ]]; then
    sort -u "${tmp_detail}"
  fi
} > "${OUT_DETAIL}"

{
  echo "# generated_at: ${generated_at}"
  echo "# caution: do not allocate platform_uid from IDs listed below"
  if [[ -s "${tmp_ids}" ]]; then
    sort -n -u "${tmp_ids}" | awk 'NF>0 {print $1}'
  fi
} > "${OUT_IDS}"

chmod 600 "${OUT_DETAIL}" "${OUT_IDS}"

echo "UID/GID 采集完成："
echo "- 详情文件: ${OUT_DETAIL}"
echo "- 保留ID文件: ${OUT_IDS}"
echo "- 统计: ok=${ok_count} fail=${fail_count} skip=${skip_count}"
if [[ -s "${tmp_fail}" ]]; then
  echo
  echo "失败节点："
  cat "${tmp_fail}"
fi

if [[ "${fail_count}" -gt 0 ]]; then
  exit 1
fi
