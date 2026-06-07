#!/usr/bin/env bash
# 生成每个计算节点的专属 agent token（本地文件，勿提交）。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MAP_FILE="${MAP_FILE:-${ROOT_DIR}/my_ssh_keys/server_ssh_map.csv}"
OUT_FILE="${OUT_FILE:-${ROOT_DIR}/config/agent_node_tokens.local.env}"
YAML_FILE="${YAML_FILE:-${ROOT_DIR}/config/agent_node_tokens.local.yaml}"

if [[ ! -f "${MAP_FILE}" ]]; then
  echo "映射表不存在：${MAP_FILE}" >&2
  exit 2
fi

tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

{
  echo "# node_id=agent_token"
  echo "# generated_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  while IFS=',' read -r txt_file port ip node_id user; do
    if [[ "${txt_file}" == "txt文件名" ]]; then
      continue
    fi
    node_id="$(echo "${node_id}" | xargs)"
    [[ -z "${node_id}" ]] && continue
    printf '%s=%s\n' "${node_id}" "$(openssl rand -hex 32)"
  done < "${MAP_FILE}"
} > "${tmp}"

install -m 0600 "${tmp}" "${OUT_FILE}"
awk -F'=' '
  BEGIN { print "agent_node_tokens:" }
  /^[[:space:]]*#/ { next }
  NF >= 2 {
    printf "  \"%s\": \"%s\"\n", $1, $2
  }
' "${OUT_FILE}" > "${tmp}"
install -m 0600 "${tmp}" "${YAML_FILE}"

echo "已生成：${OUT_FILE}"
echo "已生成：${YAML_FILE}"
echo "请把 ${YAML_FILE} 的 agent_node_tokens 片段合并到 config/controller.local.yaml。"
