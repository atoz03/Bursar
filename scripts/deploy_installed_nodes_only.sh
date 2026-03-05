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

CONTROLLER_URL="${CONTROLLER_URL:-}"
AGENT_TOKEN="${AGENT_TOKEN:-}"
SSH_GUARD_EXCLUDE_USERS="${SSH_GUARD_EXCLUDE_USERS:-root}"
SKIP_CONTROLLER_HEALTHCHECK="${SKIP_CONTROLLER_HEALTHCHECK:-0}"
ENABLE_USER_SLICE_MEMORY_RESERVE="${ENABLE_USER_SLICE_MEMORY_RESERVE:-1}"
USER_SLICE_MEMORY_RESERVE_GB="${USER_SLICE_MEMORY_RESERVE_GB:-8}"
HOME_RESERVE_GB="${HOME_RESERVE_GB:-8}"
HOME_RESERVE_CHECK_PATH="${HOME_RESERVE_CHECK_PATH:-/home}"
HOME_RESERVE_EXEMPT_USERS="${HOME_RESERVE_EXEMPT_USERS:-root gpuops}"
HOME_RESERVE_ENFORCE_INTERVAL="${HOME_RESERVE_ENFORCE_INTERVAL:-30s}"

if [[ ! -f "${MAP_FILE}" ]]; then
  echo "映射表不存在：${MAP_FILE}" >&2
  exit 2
fi
if [[ ! -d "${SOURCE_DIR}" ]]; then
  echo "源目录不存在：${SOURCE_DIR}" >&2
  exit 2
fi
if [[ -z "${CONTROLLER_URL}" || -z "${AGENT_TOKEN}" ]]; then
  echo "缺少必需参数：CONTROLLER_URL/AGENT_TOKEN" >&2
  exit 2
fi

echo "SOURCE_DIR=${SOURCE_DIR}"
echo "MAP_FILE=${MAP_FILE}"
echo "TARGET=<${TARGET_BASE}>/<用户名>/${PROJECT_DIR_NAME}"
echo "PARALLEL=${PARALLEL}"
echo "REPORT_FILE=${REPORT_FILE}"
echo "USER_SLICE_MEMORY_RESERVE_GB=${USER_SLICE_MEMORY_RESERVE_GB} (enable=${ENABLE_USER_SLICE_MEMORY_RESERVE})"
echo "HOME_RESERVE_GB=${HOME_RESERVE_GB} (path=${HOME_RESERVE_CHECK_PATH})"

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

copy_workspace() {
  local key_use_path="$1"
  local port="$2"
  local user="$3"
  local ip="$4"
  local target_dir="$5"
  local ssh_cmd=(ssh -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}")
  local ssh_cmd_no_stdin=(ssh -n -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}")

  "${ssh_cmd_no_stdin[@]}" "mkdir -p '${target_dir}'"
  tar -C "${SOURCE_DIR}" \
    --exclude='.git' \
    --exclude='my_ssh_keys/*.txt' \
    --exclude='web/node_modules' \
    --exclude='web/dist' \
    --exclude='controller/tmp' \
    -cf - . \
    | "${ssh_cmd[@]}" "tar -C '${target_dir}' -xf -"
}

node_has_agent_service() {
  local key_use_path="$1"
  local port="$2"
  local user="$3"
  local ip="$4"
  ssh -n -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}" \
    "systemctl list-unit-files 2>/dev/null | grep -q '^gpu-node-agent\\.service' || [[ -f /etc/systemd/system/gpu-node-agent.service ]] || [[ -f /lib/systemd/system/gpu-node-agent.service ]]"
}

run_install_agent() {
  local key_use_path="$1"
  local port="$2"
  local user="$3"
  local ip="$4"
  local target_dir="$5"
  local esc_exclude
  esc_exclude="$(printf "%s" "${SSH_GUARD_EXCLUDE_USERS}" | sed "s/'/'\"'\"'/g")"
  local esc_skip_health
  esc_skip_health="$(printf "%s" "${SKIP_CONTROLLER_HEALTHCHECK}" | sed "s/'/'\"'\"'/g")"
  local esc_enable_mem_reserve
  esc_enable_mem_reserve="$(printf "%s" "${ENABLE_USER_SLICE_MEMORY_RESERVE}" | sed "s/'/'\"'\"'/g")"
  local esc_mem_reserve_gb
  esc_mem_reserve_gb="$(printf "%s" "${USER_SLICE_MEMORY_RESERVE_GB}" | sed "s/'/'\"'\"'/g")"
  local esc_home_reserve_gb
  esc_home_reserve_gb="$(printf "%s" "${HOME_RESERVE_GB}" | sed "s/'/'\"'\"'/g")"
  local esc_home_reserve_path
  esc_home_reserve_path="$(printf "%s" "${HOME_RESERVE_CHECK_PATH}" | sed "s/'/'\"'\"'/g")"
  local esc_home_reserve_exempt
  esc_home_reserve_exempt="$(printf "%s" "${HOME_RESERVE_EXEMPT_USERS}" | sed "s/'/'\"'\"'/g")"
  local esc_home_reserve_interval
  esc_home_reserve_interval="$(printf "%s" "${HOME_RESERVE_ENFORCE_INTERVAL}" | sed "s/'/'\"'\"'/g")"
  ssh -n -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}" \
    "cd '${target_dir}' && SSH_GUARD_EXCLUDE_USERS='${esc_exclude}' SKIP_CONTROLLER_HEALTHCHECK='${esc_skip_health}' ENABLE_USER_SLICE_MEMORY_RESERVE='${esc_enable_mem_reserve}' USER_SLICE_MEMORY_RESERVE_GB='${esc_mem_reserve_gb}' HOME_RESERVE_GB='${esc_home_reserve_gb}' HOME_RESERVE_CHECK_PATH='${esc_home_reserve_path}' HOME_RESERVE_EXEMPT_USERS='${esc_home_reserve_exempt}' HOME_RESERVE_ENFORCE_INTERVAL='${esc_home_reserve_interval}' sudo -n /bin/bash '${target_dir}/scripts/install_agent_local.sh'"
}

can_run_sudo_nopass() {
  local key_use_path="$1"
  local port="$2"
  local user="$3"
  local ip="$4"
  local target_dir="$5"
  ssh -n -i "${key_use_path}" -p "${port}" -o StrictHostKeyChecking=no -o ConnectTimeout="${SSH_TIMEOUT}" "${user}@${ip}" \
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
      end_ts="$(date '+%F %T')"
      write_result "${node_id}" "SKIPPED" "${start_ts}" "${end_ts}" "${ip}" "${user}" "未找到有效私钥块" "${result_file}"
      return 0
    fi
    chmod 600 "${key_use_path}" || true
  fi

  local target_dir="${TARGET_BASE}/${user}/${PROJECT_DIR_NAME}"
  local install_log
  install_log="$(mktemp)"
  if ! copy_workspace "${key_use_path}" "${port}" "${user}" "${ip}" "${target_dir}"; then
    rm -f "${install_log}" || true
    [[ "${key_use_path}" != "${key_path}" ]] && rm -f "${key_use_path}" || true
    end_ts="$(date '+%F %T')"
    write_result "${node_id}" "FAILED" "${start_ts}" "${end_ts}" "${ip}" "${user}" "分发失败" "${result_file}"
    return 0
  fi

  if node_has_agent_service "${key_use_path}" "${port}" "${user}" "${ip}"; then
    if can_run_sudo_nopass "${key_use_path}" "${port}" "${user}" "${ip}" "${target_dir}"; then
      if run_install_agent "${key_use_path}" "${port}" "${user}" "${ip}" "${target_dir}" >"${install_log}" 2>&1; then
        end_ts="$(date '+%F %T')"
        write_result "${node_id}" "DEPLOYED" "${start_ts}" "${end_ts}" "${ip}" "${user}" "已安装服务，执行 install_agent_local.sh 成功（sudoers 命令匹配）" "${result_file}"
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
  [[ "${key_use_path}" != "${key_path}" ]] && rm -f "${key_use_path}" || true
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
