#!/bin/bash
# 用户侧 Bash Hook：在“疑似会启动 GPU 任务”的场景下提示/阻止。
#
# 设计原则：
# - 尽量不影响用户日常 SSH/CPU 工作流
# - 控制器不可达时尽量不误伤：仅在本地存在 .gpu_blocked 标记时阻止
#
# 使用方式（由管理员部署到所有用户 .bashrc）：
#   source /opt/gpu-cluster/check_quota.sh

set -euo pipefail

CONTROLLER_URL="${CONTROLLER_URL:-http://controller:8080}"
GPUOPS_CURL_TIMEOUT="${GPUOPS_CURL_TIMEOUT:-2}"
# 余额查询接口需要鉴权。节点上通过只读文件下发一个查询用 Agent token；
# 该文件对普通用户可读，因此只应放置 token，不要放置 admin_token。
GPUOPS_QUERY_TOKEN_FILE="${GPUOPS_QUERY_TOKEN_FILE:-/etc/gpu-ops/query-token}"

_gpuops_username() {
  whoami
}

_gpuops_block_flag_path() {
  local user="$1"
  echo "/home/${user}/.gpu_blocked"
}

_gpuops_query_token() {
  if [[ -n "${GPUOPS_QUERY_TOKEN:-}" ]]; then
    echo "${GPUOPS_QUERY_TOKEN}"
    return 0
  fi
  if [[ -r "${GPUOPS_QUERY_TOKEN_FILE}" ]]; then
    head -n 1 "${GPUOPS_QUERY_TOKEN_FILE}" 2>/dev/null | tr -d '[:space:]'
    return 0
  fi
  return 1
}

_gpuops_fetch_balance_json() {
  local user="$1"
  local token=""
  token="$(_gpuops_query_token)" || return 1
  [[ -n "${token}" ]] || return 1
  curl -fsS --max-time "${GPUOPS_CURL_TIMEOUT}" \
    -H "X-Agent-Token: ${token}" \
    "${CONTROLLER_URL}/api/users/${user}/balance" 2>/dev/null || return 1
}

_gpuops_json_get() {
  local key="$1"
  if command -v jq >/dev/null 2>&1; then
    jq -r "${key}"
  else
    python3 -c "import sys,json;print(json.load(sys.stdin)${key})" 2>/dev/null || return 1
  fi
}

gpuops_check_balance() {
  local user="$(_gpuops_username)"
  local flag="$(_gpuops_block_flag_path "${user}")"

  local json=""
  if json="$(_gpuops_fetch_balance_json "${user}")"; then
    local status balance
    status="$(echo "${json}" | _gpuops_json_get "['status']")" || status=""
    balance="$(echo "${json}" | _gpuops_json_get "['balance']")" || balance=""

    if [[ "${status}" == "limited" ]]; then
      echo "❌ 余额不足（当前余额：${balance} 元），无法启动新的 GPU 任务"
      return 1
    fi
    if [[ "${status}" == "blocked" ]]; then
      echo "❌ 已欠费（当前余额：${balance} 元），GPU 任务将被终止/限制"
      return 1
    fi
    if [[ "${status}" == "warning" ]]; then
      echo "⚠️  余额预警：当前余额 ${balance} 元，请及时充值"
      return 0
    fi
    return 0
  fi

  # 控制器不可达：仅在本地存在阻止标记时拦截
  if [[ -f "${flag}" ]]; then
    echo "❌ 控制器不可达且检测到限制标记：${flag}"
    echo "原因：$(head -n 1 "${flag}" 2>/dev/null || true)"
    return 1
  fi

  return 0
}

_gpuops_is_probably_gpu_python() {
  # 尽量快速：对脚本文件做最小静态检测；对 -c 的代码片段检测关键词
  local args=("$@")
  if [[ "${#args[@]}" -eq 0 ]]; then
    return 1
  fi

  # 显式设置 CUDA_VISIBLE_DEVICES 通常意味着会用 GPU（但也可能仅做限制）
  if [[ -n "${CUDA_VISIBLE_DEVICES:-}" ]]; then
    return 0
  fi

  if [[ "${args[0]}" == "-c" && "${#args[@]}" -ge 2 ]]; then
    local code="${args[1]}"
    echo "${code}" | grep -Eiq "torch|tensorflow|jax|cuda" && return 0
    return 1
  fi

  for a in "${args[@]}"; do
    if [[ "${a}" == *.py && -f "${a}" ]]; then
      grep -Eiq "import[[:space:]]+torch|import[[:space:]]+tensorflow|from[[:space:]]+torch|from[[:space:]]+tensorflow|jax|cuda" "${a}" && return 0
    fi
  done

  return 1
}

python() {
  if _gpuops_is_probably_gpu_python "$@"; then
    gpuops_check_balance || return 1
  fi
  command python "$@"
}

python3() {
  if _gpuops_is_probably_gpu_python "$@"; then
    gpuops_check_balance || return 1
  fi
  command python3 "$@"
}

nvidia-smi() {
  gpuops_check_balance || return 1
  command nvidia-smi "$@"
}

export -f python
export -f python3
export -f nvidia-smi
export -f gpuops_check_balance
