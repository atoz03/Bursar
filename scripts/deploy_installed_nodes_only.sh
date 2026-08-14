#!/usr/bin/env bash
# 并发分发代码到所有节点；仅对“已安装 gpu-node-agent 服务”的节点执行重装。
# 未安装节点只更新目录，不做安装动作。

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MAP_FILE="${MAP_FILE:-${ROOT_DIR}/my_ssh_keys/server_ssh_map.csv}"
KEY_DIR="${KEY_DIR:-${ROOT_DIR}/my_ssh_keys}"
SOURCE_DIR="${SOURCE_DIR:-${ROOT_DIR}}"
PROJECT_DIR_NAME="${PROJECT_DIR_NAME:-$(basename "${ROOT_DIR}")}"
TARGET_BASE="${TARGET_BASE:-/home}"
SSH_TIMEOUT="${SSH_TIMEOUT:-10}"
PARALLEL="${PARALLEL:-6}"
REPORT_FILE="${REPORT_FILE:-$(pwd)/计算节点部署情况.txt}"
NODE_IDS_RAW="${NODE_IDS:-}"
NODE_IDS_CANON=""
NODE_IDS_DISPLAY=""

CONTROLLER_URL="${CONTROLLER_URL:-}"
AGENT_TOKEN="${AGENT_TOKEN:-}"
NODE_AGENT_TOKENS_FILE="${NODE_AGENT_TOKENS_FILE:-}"
SSH_GUARD_EXCLUDE_USERS="${SSH_GUARD_EXCLUDE_USERS:-root}"
SKIP_CONTROLLER_HEALTHCHECK="${SKIP_CONTROLLER_HEALTHCHECK:-0}"
ENABLE_SSH_GUARD="${ENABLE_SSH_GUARD:-0}"
ENABLE_HOST_SECURITY="${ENABLE_HOST_SECURITY:-0}"
RESET_USER_CPU_QUOTA_ON_INSTALL="${RESET_USER_CPU_QUOTA_ON_INSTALL:-0}"
INSTALL_DEPS="${INSTALL_DEPS:-0}"
LEGACY_ENABLE_USER_SLICE_CPU_RESERVE="${ENABLE_USER_SLICE_CPU_RESERVE:-}"
LEGACY_USER_SLICE_CPU_RESERVE_PERCENT="${USER_SLICE_CPU_RESERVE_PERCENT:-}"
LEGACY_ENABLE_USER_SLICE_MEMORY_RESERVE="${ENABLE_USER_SLICE_MEMORY_RESERVE:-}"
LEGACY_USER_SLICE_MEMORY_RESERVE_GB="${USER_SLICE_MEMORY_RESERVE_GB:-}"
ENABLE_SYSTEM_CPU_RESERVE="${ENABLE_SYSTEM_CPU_RESERVE:-${LEGACY_ENABLE_USER_SLICE_CPU_RESERVE:-1}}"
ENABLE_SYSTEM_MEMORY_RESERVE="${ENABLE_SYSTEM_MEMORY_RESERVE:-${LEGACY_ENABLE_USER_SLICE_MEMORY_RESERVE:-1}}"
LEGACY_CPU_RESERVE_COMPAT_USED=0
LEGACY_MEMORY_RESERVE_COMPAT_USED=0
if [[ -n "${SYSTEM_CPU_RESERVE_PERCENT:-}" ]]; then
  SYSTEM_CPU_RESERVE_PERCENT="${SYSTEM_CPU_RESERVE_PERCENT}"
elif [[ -n "${LEGACY_USER_SLICE_CPU_RESERVE_PERCENT}" ]]; then
  LEGACY_CPU_RESERVE_COMPAT_USED=1
  if [[ "${LEGACY_USER_SLICE_CPU_RESERVE_PERCENT}" =~ ^[0-9]+$ ]] \
    && [[ "${LEGACY_USER_SLICE_CPU_RESERVE_PERCENT}" -ge 10 ]] \
    && [[ "${LEGACY_USER_SLICE_CPU_RESERVE_PERCENT}" -le 100 ]]; then
    SYSTEM_CPU_RESERVE_PERCENT="$((100 - LEGACY_USER_SLICE_CPU_RESERVE_PERCENT))"
  else
    SYSTEM_CPU_RESERVE_PERCENT="5"
  fi
else
  SYSTEM_CPU_RESERVE_PERCENT="5"
fi
if [[ -n "${SYSTEM_MEMORY_RESERVE_GB:-}" ]]; then
  SYSTEM_MEMORY_RESERVE_GB="${SYSTEM_MEMORY_RESERVE_GB}"
elif [[ -n "${LEGACY_USER_SLICE_MEMORY_RESERVE_GB}" ]]; then
  LEGACY_MEMORY_RESERVE_COMPAT_USED=1
  SYSTEM_MEMORY_RESERVE_GB="${LEGACY_USER_SLICE_MEMORY_RESERVE_GB}"
else
  SYSTEM_MEMORY_RESERVE_GB="8"
fi

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
if [[ -z "${CONTROLLER_URL}" || ( -z "${AGENT_TOKEN}" && -z "${NODE_AGENT_TOKENS_FILE}" ) ]]; then
  echo "缺少必需参数：CONTROLLER_URL，以及 AGENT_TOKEN 或 NODE_AGENT_TOKENS_FILE" >&2
  exit 2
fi
if [[ -n "${NODE_AGENT_TOKENS_FILE}" && ! -f "${NODE_AGENT_TOKENS_FILE}" ]]; then
  echo "NODE_AGENT_TOKENS_FILE 不存在：${NODE_AGENT_TOKENS_FILE}" >&2
  exit 2
fi

init_node_filter

echo "SOURCE_DIR=${SOURCE_DIR}"
echo "MAP_FILE=${MAP_FILE}"
echo "TARGET=<${TARGET_BASE}>/<用户名>/${PROJECT_DIR_NAME}"
echo "PARALLEL=${PARALLEL}"
echo "REPORT_FILE=${REPORT_FILE}"
if [[ -n "${NODE_IDS_DISPLAY}" ]]; then
  echo "NODE_IDS=${NODE_IDS_DISPLAY}"
else
  echo "NODE_IDS=<all>"
fi
if [[ -n "${NODE_AGENT_TOKENS_FILE}" ]]; then
  echo "NODE_AGENT_TOKENS_FILE=${NODE_AGENT_TOKENS_FILE}"
fi
echo "SYSTEM_CPU_RESERVE_PERCENT=${SYSTEM_CPU_RESERVE_PERCENT} (enable=${ENABLE_SYSTEM_CPU_RESERVE})"
echo "SYSTEM_MEMORY_RESERVE_GB=${SYSTEM_MEMORY_RESERVE_GB} (enable=${ENABLE_SYSTEM_MEMORY_RESERVE})"
echo "ENABLE_SSH_GUARD=${ENABLE_SSH_GUARD}"
echo "ENABLE_HOST_SECURITY=${ENABLE_HOST_SECURITY}"
echo "RESET_USER_CPU_QUOTA_ON_INSTALL=${RESET_USER_CPU_QUOTA_ON_INSTALL}"
echo "INSTALL_DEPS=${INSTALL_DEPS}"
if [[ "${LEGACY_CPU_RESERVE_COMPAT_USED}" == "1" ]]; then
  echo "兼容旧变量：USER_SLICE_CPU_RESERVE_PERCENT=${LEGACY_USER_SLICE_CPU_RESERVE_PERCENT} -> SYSTEM_CPU_RESERVE_PERCENT=${SYSTEM_CPU_RESERVE_PERCENT}"
fi
if [[ "${LEGACY_MEMORY_RESERVE_COMPAT_USED}" == "1" ]]; then
  echo "兼容旧变量：USER_SLICE_MEMORY_RESERVE_GB=${LEGACY_USER_SLICE_MEMORY_RESERVE_GB} -> SYSTEM_MEMORY_RESERVE_GB=${SYSTEM_MEMORY_RESERVE_GB}"
fi

RESULT_DIR="$(mktemp -d /tmp/node-deploy-result.XXXXXX)"
trap 'rm -rf "${RESULT_DIR}"' EXIT

trim_text() {
  echo "$1" | xargs
}

wait_for_slot() {
  local limit="$1"
  while true; do
    local running
    running="$(jobs -rp | wc -l | tr -d ' ')"
    [[ -z "${running}" ]] && running=0
    if (( running < limit )); then
      break
    fi
    sleep 0.1
  done
}

write_result() {
  local node_id="$1"
  local status="$2"
  local start_ts="$3"
  local end_ts="$4"
  local ip="$5"
  local user="$6"
  local detail="$7"
  local out_file="$8"
  {
    printf 'node_id=%s\n' "${node_id}"
    printf 'status=%s\n' "${status}"
    printf 'ip=%s\n' "${ip}"
    printf 'user=%s\n' "${user}"
    printf 'start=%s\n' "${start_ts}"
    printf 'end=%s\n' "${end_ts}"
    printf 'detail=%s\n' "${detail}"
  } > "${out_file}"
}

agent_token_for_node() {
  local node_id="$1"
  local tok=""
  if [[ -n "${NODE_AGENT_TOKENS_FILE}" ]]; then
    tok="$(awk -F'=' -v id="${node_id}" '
      /^[[:space:]]*#/ { next }
      NF >= 2 {
        k=$1
        sub(/^[[:space:]]+/, "", k)
        sub(/[[:space:]]+$/, "", k)
        if (k == id) {
          $1=""
          sub(/^=/, "", $0)
          sub(/^[[:space:]]+/, "", $0)
          sub(/[[:space:]]+$/, "", $0)
          gsub(/^"|"$/, "", $0)
          print $0
          exit
        }
      }
    ' "${NODE_AGENT_TOKENS_FILE}")"
  fi
  if [[ -z "${tok}" ]]; then
    tok="${AGENT_TOKEN}"
  fi
  printf '%s' "${tok}"
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

copy_workspace() {
  local key_use_path="$1"
  local port="$2"
  local user="$3"
  local ip="$4"
  local target_dir="$5"
  local ssh_cmd=(ssh -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o UpdateHostKeys=no -o LogLevel=ERROR -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}")
  local ssh_cmd_no_stdin=(ssh -n -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o UpdateHostKeys=no -o LogLevel=ERROR -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}")

  "${ssh_cmd_no_stdin[@]}" "$(remote_harden_workspace_cmd "${target_dir}")"
  tar -C "${SOURCE_DIR}" \
    --exclude='.git' \
    --exclude='.codex' \
    --exclude='.netcatty-paste-images' \
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
    | "${ssh_cmd[@]}" "tar -C '${target_dir}' -xf - && $(remote_harden_workspace_cmd "${target_dir}")"
}

node_has_agent_service() {
  local key_use_path="$1"
  local port="$2"
  local user="$3"
  local ip="$4"
  ssh -n -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o UpdateHostKeys=no -o LogLevel=ERROR -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}" \
    "systemctl list-unit-files 2>/dev/null | grep -q '^gpu-node-agent\\.service' || [[ -f /etc/systemd/system/gpu-node-agent.service ]] || [[ -f /lib/systemd/system/gpu-node-agent.service ]]"
}

run_install_agent() {
  local key_use_path="$1"
  local port="$2"
  local user="$3"
  local ip="$4"
  local target_dir="$5"
  local node_id="$6"
  local node_agent_token
  node_agent_token="$(agent_token_for_node "${node_id}")"
  if [[ -z "${node_agent_token}" ]]; then
    echo "节点 ${node_id} 缺少 agent token" >&2
    return 2
  fi
  local esc_exclude
  esc_exclude="$(printf "%s" "${SSH_GUARD_EXCLUDE_USERS}" | sed "s/'/'\"'\"'/g")"
  local esc_controller_url
  esc_controller_url="$(printf "%s" "${CONTROLLER_URL}" | sed "s/'/'\"'\"'/g")"
  local esc_agent_token
  esc_agent_token="$(printf "%s" "${node_agent_token}" | sed "s/'/'\"'\"'/g")"
  local esc_skip_health
  esc_skip_health="$(printf "%s" "${SKIP_CONTROLLER_HEALTHCHECK}" | sed "s/'/'\"'\"'/g")"
  local esc_enable_ssh_guard
  esc_enable_ssh_guard="$(printf "%s" "${ENABLE_SSH_GUARD}" | sed "s/'/'\"'\"'/g")"
  local esc_enable_host_security
  esc_enable_host_security="$(printf "%s" "${ENABLE_HOST_SECURITY}" | sed "s/'/'\"'\"'/g")"
  local esc_reset_user_cpu_quota
  esc_reset_user_cpu_quota="$(printf "%s" "${RESET_USER_CPU_QUOTA_ON_INSTALL}" | sed "s/'/'\"'\"'/g")"
  local esc_install_deps
  esc_install_deps="$(printf "%s" "${INSTALL_DEPS}" | sed "s/'/'\"'\"'/g")"
  local esc_enable_cpu_reserve
  esc_enable_cpu_reserve="$(printf "%s" "${ENABLE_SYSTEM_CPU_RESERVE}" | sed "s/'/'\"'\"'/g")"
  local esc_cpu_reserve_pct
  esc_cpu_reserve_pct="$(printf "%s" "${SYSTEM_CPU_RESERVE_PERCENT}" | sed "s/'/'\"'\"'/g")"
  local esc_enable_mem_reserve
  esc_enable_mem_reserve="$(printf "%s" "${ENABLE_SYSTEM_MEMORY_RESERVE}" | sed "s/'/'\"'\"'/g")"
  local esc_mem_reserve_gb
  esc_mem_reserve_gb="$(printf "%s" "${SYSTEM_MEMORY_RESERVE_GB}" | sed "s/'/'\"'\"'/g")"
  ssh -n -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o UpdateHostKeys=no -o LogLevel=ERROR -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}" \
    "cd '${target_dir}' && sudo -n /bin/bash '${target_dir}/scripts/install_agent_local.sh' 'CONTROLLER_URL=${esc_controller_url}' 'AGENT_TOKEN=${esc_agent_token}' 'SSH_GUARD_EXCLUDE_USERS=${esc_exclude}' 'SKIP_CONTROLLER_HEALTHCHECK=${esc_skip_health}' 'ENABLE_SSH_GUARD=${esc_enable_ssh_guard}' 'ENABLE_HOST_SECURITY=${esc_enable_host_security}' 'RESET_USER_CPU_QUOTA_ON_INSTALL=${esc_reset_user_cpu_quota}' 'INSTALL_DEPS=${esc_install_deps}' 'ENABLE_SYSTEM_CPU_RESERVE=${esc_enable_cpu_reserve}' 'SYSTEM_CPU_RESERVE_PERCENT=${esc_cpu_reserve_pct}' 'ENABLE_SYSTEM_MEMORY_RESERVE=${esc_enable_mem_reserve}' 'SYSTEM_MEMORY_RESERVE_GB=${esc_mem_reserve_gb}'"
}

agent_service_ready() {
  local key_use_path="$1"
  local port="$2"
  local user="$3"
  local ip="$4"
  ssh -n -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o UpdateHostKeys=no -o LogLevel=ERROR -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}" \
    "systemctl is-enabled gpu-node-agent >/dev/null 2>&1 && systemctl is-active gpu-node-agent >/dev/null 2>&1"
}

can_run_sudo_nopass() {
  local key_use_path="$1"
  local port="$2"
  local user="$3"
  local ip="$4"
  local target_dir="$5"
  ssh -n -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o UpdateHostKeys=no -o LogLevel=ERROR -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}" \
    "sudo -n -l /bin/bash '${target_dir}/scripts/install_agent_local.sh' >/dev/null 2>&1"
}

process_one() {
  local txt_file="$1"
  local port="$2"
  local ip="$3"
  local node_id="$4"
  local user="$5"
  local result_file="$6"
  local start_ts end_ts
  start_ts="$(date '+%F %T')"

  user="$(trim_text "${user}")"
  if [[ -z "${user}" || "${user}" == "TODO" ]]; then
    end_ts="$(date '+%F %T')"
    write_result "${node_id}" "SKIPPED" "${start_ts}" "${end_ts}" "${ip}" "${user}" "用户名未配置" "${result_file}"
    return 0
  fi

  local key_path="${KEY_DIR}/${txt_file}"
  if [[ ! -f "${key_path}" ]]; then
    end_ts="$(date '+%F %T')"
    write_result "${node_id}" "SKIPPED" "${start_ts}" "${end_ts}" "${ip}" "${user}" "私钥不存在: ${key_path}" "${result_file}"
    return 0
  fi
  if grep -q "REPLACE_WITH_REAL_PRIVATE_KEY_CONTENT" "${key_path}"; then
    end_ts="$(date '+%F %T')"
    write_result "${node_id}" "SKIPPED" "${start_ts}" "${end_ts}" "${ip}" "${user}" "私钥仍是模板" "${result_file}"
    return 0
  fi

  # 不修改共享目录中的原始密钥权限；始终抽取到仅当前进程可读的临时文件，
  # 同时兼容“纯私钥文件”和带说明文字的密钥文件。
  local key_use_path
  key_use_path="$(mktemp)"
  awk '
    /-----BEGIN OPENSSH PRIVATE KEY-----/ {in_key=1}
    in_key {print}
    /-----END OPENSSH PRIVATE KEY-----/ {if (in_key) exit}
  ' "${key_path}" > "${key_use_path}"
  if ! grep -q "BEGIN OPENSSH PRIVATE KEY" "${key_use_path}" || ! grep -q "END OPENSSH PRIVATE KEY" "${key_use_path}"; then
    rm -f "${key_use_path}"
    end_ts="$(date '+%F %T')"
    write_result "${node_id}" "SKIPPED" "${start_ts}" "${end_ts}" "${ip}" "${user}" "未找到有效私钥块" "${result_file}"
    return 0
  fi
  chmod 600 "${key_use_path}" || true

  local target_dir="${TARGET_BASE}/${user}/${PROJECT_DIR_NAME}"
  local install_log
  install_log="$(mktemp)"
  if ! copy_workspace "${key_use_path}" "${port}" "${user}" "${ip}" "${target_dir}"; then
    rm -f "${install_log}" || true
    rm -f "${key_use_path}" || true
    end_ts="$(date '+%F %T')"
    write_result "${node_id}" "FAILED" "${start_ts}" "${end_ts}" "${ip}" "${user}" "分发失败" "${result_file}"
    return 0
  fi

  if node_has_agent_service "${key_use_path}" "${port}" "${user}" "${ip}"; then
    if can_run_sudo_nopass "${key_use_path}" "${port}" "${user}" "${ip}" "${target_dir}"; then
      if run_install_agent "${key_use_path}" "${port}" "${user}" "${ip}" "${target_dir}" "${node_id}" >"${install_log}" 2>&1; then
        if agent_service_ready "${key_use_path}" "${port}" "${user}" "${ip}"; then
          end_ts="$(date '+%F %T')"
          write_result "${node_id}" "DEPLOYED" "${start_ts}" "${end_ts}" "${ip}" "${user}" "已安装服务，执行 install_agent_local.sh 成功，已自动 enable --now gpu-node-agent" "${result_file}"
        else
          local status_tail
          status_tail="$(tail -n 6 "${install_log}" | tr '\n' ';' | sed 's/[[:space:]]\+/ /g; s/;$/ /')"
          [[ -z "${status_tail}" ]] && status_tail="无输出（可手动到节点执行 systemctl status gpu-node-agent 查看）"
          end_ts="$(date '+%F %T')"
          write_result "${node_id}" "FAILED" "${start_ts}" "${end_ts}" "${ip}" "${user}" "install_agent_local.sh 执行成功，但 gpu-node-agent 未处于 enabled+active；末尾日志: ${status_tail}" "${result_file}"
        fi
      else
        local fail_tail
        fail_tail="$(tail -n 3 "${install_log}" | tr '\n' ';' | sed 's/[[:space:]]\+/ /g; s/;$/ /')"
        [[ -z "${fail_tail}" ]] && fail_tail="无输出（可手动到节点执行 install_agent_local.sh 查看）"
        end_ts="$(date '+%F %T')"
        write_result "${node_id}" "FAILED" "${start_ts}" "${end_ts}" "${ip}" "${user}" "已安装服务，sudo -n 可用，但 install_agent_local.sh 执行失败；末尾日志: ${fail_tail}" "${result_file}"
      fi
    else
      end_ts="$(date '+%F %T')"
      write_result "${node_id}" "UPDATED_ONLY" "${start_ts}" "${end_ts}" "${ip}" "${user}" "已安装服务，但 sudoers 未放行 install_agent_local.sh（或路径不匹配），仅更新目录" "${result_file}"
    fi
  else
    end_ts="$(date '+%F %T')"
    write_result "${node_id}" "UPDATED_ONLY" "${start_ts}" "${end_ts}" "${ip}" "${user}" "未安装服务，仅更新代码目录" "${result_file}"
  fi

  rm -f "${install_log}" || true
  rm -f "${key_use_path}" || true
  return 0
}

pids=()
while IFS=',' read -r txt_file port ip node_id user <&3; do
  if [[ "${txt_file}" == "txt文件名" ]]; then
    continue
  fi
  txt_file="$(trim_text "${txt_file}")"
  port="$(trim_text "${port}")"
  ip="$(trim_text "${ip}")"
  node_id="$(trim_text "${node_id}")"
  user="$(trim_text "${user}")"
  if [[ -z "${txt_file}" || -z "${port}" || -z "${ip}" || -z "${node_id}" ]]; then
    continue
  fi
  if ! should_process_node "${node_id}"; then
    continue
  fi

  wait_for_slot "${PARALLEL}"
  (
    process_one "${txt_file}" "${port}" "${ip}" "${node_id}" "${user}" "${RESULT_DIR}/${node_id}.result"
  ) &
  pids+=("$!")
done 3< "${MAP_FILE}"

for pid in "${pids[@]}"; do
  wait "${pid}" || true
done

report_time="$(date '+%F %T')"
{
  echo "计算节点部署情况报告"
  echo "生成时间: ${report_time}"
  echo "映射表: ${MAP_FILE}"
  echo "并发数: ${PARALLEL}"
  if [[ -n "${NODE_IDS_DISPLAY}" ]]; then
    echo "目标节点: ${NODE_IDS_DISPLAY}"
  else
    echo "目标节点: 全部"
  fi
  echo "控制器地址: ${CONTROLLER_URL}"
  echo

  total=0
  deployed=0
  updated_only=0
  skipped=0
  failed=0

  while IFS= read -r f; do
    [[ -z "${f}" ]] && continue
    total=$((total + 1))
    status="$(grep '^status=' "${f}" | cut -d'=' -f2-)"
    case "${status}" in
      DEPLOYED) deployed=$((deployed + 1)) ;;
      UPDATED_ONLY) updated_only=$((updated_only + 1)) ;;
      SKIPPED) skipped=$((skipped + 1)) ;;
      FAILED) failed=$((failed + 1)) ;;
    esac
  done < <(find "${RESULT_DIR}" -type f -name '*.result' | sort)

  echo "总节点数: ${total}"
  echo "已部署(重装成功): ${deployed}"
  echo "仅更新目录(未安装服务): ${updated_only}"
  echo "跳过: ${skipped}"
  echo "失败: ${failed}"
  echo
  echo "详细结果:"
  echo "----------------------------------------"

  while IFS= read -r f; do
    [[ -z "${f}" ]] && continue
    node_id="$(grep '^node_id=' "${f}" | cut -d'=' -f2-)"
    status="$(grep '^status=' "${f}" | cut -d'=' -f2-)"
    ip="$(grep '^ip=' "${f}" | cut -d'=' -f2-)"
    user="$(grep '^user=' "${f}" | cut -d'=' -f2-)"
    start="$(grep '^start=' "${f}" | cut -d'=' -f2-)"
    end="$(grep '^end=' "${f}" | cut -d'=' -f2-)"
    detail="$(grep '^detail=' "${f}" | cut -d'=' -f2-)"
    echo "[${status}] 节点=${node_id} ip=${ip} user=${user} 开始=${start} 结束=${end}"
    echo "  说明: ${detail}"
  done < <(find "${RESULT_DIR}" -type f -name '*.result' | sort)
} > "${REPORT_FILE}"

cat "${REPORT_FILE}"
echo
echo "报告已写入：${REPORT_FILE}"
