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
CONFIRM_DISTRIBUTE_WORKSPACE="${CONFIRM_DISTRIBUTE_WORKSPACE:-0}"
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

remote_harden_workspace_cmd() {
  local target_dir="$1"
  cat <<EOF
set -e
target='${target_dir}'
mkdir -p "\${target}"
rm -rf -- \
  "\${target}/config" \
  "\${target}/my_ssh_keys" \
  "\${target}/.codex" \
  "\${target}/使用手册" \
  "\${target}/README.md" \
  "\${target}/GPU Ops使用手册.md" \
  "\${target}/计算节点部署情况.txt" \
  "\${target}/控制节点安全部署命令.md" \
  "\${target}/计算节点首次安装交接.md" \
  "\${target}/go.work.sum" \
  "\${target}/sudo" \
  "\${target}/mkdir" \
  "\${target}/controller/controller" \
  "\${target}/node-agent/node-agent"
find "\${target}" -xdev \\( -type d -o -type f \\) -exec chmod 700 {} + 2>/dev/null || true
EOF
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

if [[ "${DRY_RUN}" != "1" && "${CONFIRM_DISTRIBUTE_WORKSPACE}" != "1" ]]; then
  cat >&2 <<'EOF'
拒绝执行：distribute_workspace.sh 会把当前工作区分发到计算节点。

安全用法：
  DRY_RUN=1 NODE_IDS="60020" bash scripts/distribute_workspace.sh

确认真的要分发时，必须显式加：
  CONFIRM_DISTRIBUTE_WORKSPACE=1 NODE_IDS="60020" bash scripts/distribute_workspace.sh

不要裸跑 bash scripts/distribute_workspace.sh。
EOF
  exit 2
fi

echo "SOURCE_DIR=${SOURCE_DIR}"
echo "MAP_FILE=${MAP_FILE}"
echo "TARGET=<${TARGET_BASE}>/<用户名>/${PROJECT_DIR_NAME}"
echo "PARALLEL=${PARALLEL}"
echo "DRY_RUN=${DRY_RUN}"
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
  local key_use_path=""
  local key_path=""

  cleanup_key() {
    if [[ -n "${key_use_path}" && -n "${key_path}" && "${key_use_path}" != "${key_path}" ]]; then
      rm -f "${key_use_path}" || true
    fi
  }
  trap cleanup_key RETURN

  user="$(echo "${user}" | xargs)"
  if [[ -z "${user}" || "${user}" == "TODO" ]]; then
    echo "[SKIP][${node_id}] 用户名未配置（${user}）"
    return 0
  fi

  key_path="${KEY_DIR}/${txt_file}"
  if [[ ! -f "${key_path}" ]]; then
    echo "[SKIP][${node_id}] 私钥文件不存在：${key_path}"
    return 0
  fi

  if grep -q "REPLACE_WITH_REAL_PRIVATE_KEY_CONTENT" "${key_path}"; then
    echo "[SKIP][${node_id}] 私钥文件仍是模板：${key_path}"
    return 0
  fi

  chmod 600 "${key_path}" || true

  key_use_path="${key_path}"
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
  local ssh_cmd=(ssh -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o UpdateHostKeys=no -o LogLevel=ERROR -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}")
  local ssh_cmd_no_stdin=(ssh -n -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o UpdateHostKeys=no -o LogLevel=ERROR -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}")

  echo "[COPY][${node_id}] ${user}@${ip}:${target_dir}"

  if [[ "${DRY_RUN}" == "1" ]]; then
    return 0
  fi

  if ! "${ssh_cmd_no_stdin[@]}" "$(remote_harden_workspace_cmd "${target_dir}")"; then
    echo "[FAIL][${node_id}] 远端目录创建失败：${user}@${ip}:${target_dir}" >&2
    return 1
  fi

  if ! tar -C "${SOURCE_DIR}" \
    --exclude='.git' \
    --exclude='.codex' \
    --exclude='config' \
    --exclude='README.md' \
    --exclude='GPU Ops使用手册.md' \
    --exclude='使用手册' \
    --exclude='计算节点部署情况.txt' \
    --exclude='控制节点安全部署命令.md' \
    --exclude='计算节点首次安装交接.md' \
    --exclude='go.work.sum' \
    --exclude='my_ssh_keys' \
    --exclude='web/node_modules' \
    --exclude='web/dist' \
    --exclude='controller/tmp' \
    --exclude='controller/controller' \
    --exclude='node-agent/node-agent' \
    --exclude='*.pdf' \
    --exclude='*.txt' \
    -cf - . \
    | "${ssh_cmd[@]}" "tar -C '${target_dir}' -xf - && $(remote_harden_workspace_cmd "${target_dir}")"; then
    echo "[FAIL][${node_id}] 工作区分发失败：${user}@${ip}:${target_dir}" >&2
    return 1
  fi

  echo "[DONE][${node_id}]"
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
declare -A pid_to_node=()
declare -A pid_to_target=()
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
  pid_to_node["$!"]="${node_id}"
  pid_to_target["$!"]="$(echo "${user}" | xargs)@${ip}:${TARGET_BASE}/$(echo "${user}" | xargs)/${PROJECT_DIR_NAME}"
done 3< "${MAP_FILE}"

fail_count=0
failed_nodes=()
for pid in "${job_pids[@]}"; do
  if ! wait "${pid}"; then
    fail_count=$((fail_count + 1))
    failed_nodes+=("${pid_to_node[${pid}]} (${pid_to_target[${pid}]})")
  fi
done

if (( fail_count > 0 )); then
  echo "全部处理完成，但有 ${fail_count} 个任务失败。" >&2
  printf '失败节点列表：\n' >&2
  printf '  - %s\n' "${failed_nodes[@]}" >&2
  exit 1
fi
echo "全部处理完成。"
