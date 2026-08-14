#!/usr/bin/env bash
# 单点一键部署 node-agent（在计算节点本机执行）
# 处理事项：
# - 可选安装依赖（复用 install_deps_ubuntu2204.sh）
# - 清理代理变量并配置 Go 国内源
# - 本地编译 node-agent
# - 安装 systemd 服务并启动
# - 检查控制器健康状态与服务运行状态

set -euo pipefail

SHOW_USAGE=0
CLI_INVALID_ARG=""
while (($# > 0)); do
  case "$1" in
    -h|--help)
      SHOW_USAGE=1
      ;;
    *=*)
      cli_name="${1%%=*}"
      cli_value="${1#*=}"
      if [[ ! "${cli_name}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
        CLI_INVALID_ARG="$1"
        break
      fi
      printf -v "${cli_name}" '%s' "${cli_value}"
      export "${cli_name}"
      ;;
    *)
      CLI_INVALID_ARG="$1"
      break
      ;;
  esac
  shift
done

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
NODE_AGENT_DIR="${NODE_AGENT_DIR:-${PROJECT_ROOT}/node-agent}"
INSTALL_DEPS="${INSTALL_DEPS:-1}"
INSTALL_NODE_DEPS="${INSTALL_NODE_DEPS:-0}"
INSTALL_DOCKER_DEPS="${INSTALL_DOCKER_DEPS:-0}"
NODE_ID_AUTO_DETECT="${NODE_ID_AUTO_DETECT:-1}"
NODE_MAP_FILE="${NODE_MAP_FILE:-${PROJECT_ROOT}/my_ssh_keys/server_ssh_map.csv}"
NODE_MAP_USER="${NODE_MAP_USER:-}"
GO_PROXY="${GO_PROXY:-https://goproxy.cn,direct}"
GO_SUMDB="${GO_SUMDB:-sum.golang.google.cn}"
SERVICE_NAME="${SERVICE_NAME:-gpu-node-agent}"
NODE_AGENT_ENV_FILE="${NODE_AGENT_ENV_FILE:-/etc/gpu-cluster/node-agent.env}"
PUBLIC_ENV_FILE="${PUBLIC_ENV_FILE:-/etc/gpu-cluster/public.env}"
ACTION_POLL_INTERVAL_SECONDS="${ACTION_POLL_INTERVAL_SECONDS:-2}"
LOCAL_USERS_REFRESH_SECONDS="${LOCAL_USERS_REFRESH_SECONDS:-900}"
LOCAL_USERS_COLLECT_TIMEOUT_SECONDS="${LOCAL_USERS_COLLECT_TIMEOUT_SECONDS:-8}"
SYSTEM_SERVICES_REFRESH_SECONDS="${SYSTEM_SERVICES_REFRESH_SECONDS:-60}"
SYSTEM_SERVICES_COLLECT_TIMEOUT_SECONDS="${SYSTEM_SERVICES_COLLECT_TIMEOUT_SECONDS:-8}"
GPU_BUS_MAP_CACHE_SECONDS="${GPU_BUS_MAP_CACHE_SECONDS:-600}"
GPU_INVENTORY_CACHE_SECONDS="${GPU_INVENTORY_CACHE_SECONDS:-1800}"
GPU_COMMAND_TIMEOUT_SECONDS="${GPU_COMMAND_TIMEOUT_SECONDS:-4}"
SKIP_CONTROLLER_HEALTHCHECK="${SKIP_CONTROLLER_HEALTHCHECK:-0}"
SYNC_TIME_WITH_CONTROLLER="${SYNC_TIME_WITH_CONTROLLER:-1}"
ENABLE_SSH_GUARD="${ENABLE_SSH_GUARD:-1}"
SSH_GUARD_EXCLUDE_USERS="${SSH_GUARD_EXCLUDE_USERS:-root}"
SSH_GUARD_FAIL_OPEN="${SSH_GUARD_FAIL_OPEN:-0}"
SSH_GUARD_ALLOWLIST_FILE="${SSH_GUARD_ALLOWLIST_FILE:-/var/lib/gpu-cluster/registered_users.txt}"
SSH_GUARD_DENYLIST_FILE="${SSH_GUARD_DENYLIST_FILE:-/var/lib/gpu-cluster/blocked_users.txt}"
SSH_GUARD_EXEMPT_FILE="${SSH_GUARD_EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}"
SSH_GUARD_STATE_FILE="${SSH_GUARD_STATE_FILE:-/var/lib/gpu-cluster/guard_state.env}"
SSH_GUARD_REALTIME_LOOKUP="${SSH_GUARD_REALTIME_LOOKUP:-0}"
SSH_GUARD_SYNC_INTERVAL="${SSH_GUARD_SYNC_INTERVAL:-10s}"
SSH_GUARD_ENFORCE_INTERVAL="${SSH_GUARD_ENFORCE_INTERVAL:-10s}"
ENABLE_HOST_SECURITY="${ENABLE_HOST_SECURITY:-1}"
SSH_FAIL2BAN_MAXRETRY="${SSH_FAIL2BAN_MAXRETRY:-20}"
SSH_FAIL2BAN_FINDTIME="${SSH_FAIL2BAN_FINDTIME:-5m}"
SSH_FAIL2BAN_BANTIME="${SSH_FAIL2BAN_BANTIME:-12h}"
SSH_FAIL2BAN_IGNOREIP="${SSH_FAIL2BAN_IGNOREIP:-}"
ENABLE_SHARED_NFS="${ENABLE_SHARED_NFS:-0}"
NFS_SERVER="${NFS_SERVER:-}"
NFS_NODE_EXPORT_ROOT="${NFS_NODE_EXPORT_ROOT:-/srv/gpu-ops/nodes}"
NFS_CLUSTER_EXPORT="${NFS_CLUSTER_EXPORT:-/srv/gpu-ops/cluster}"
NFS_NODE_MOUNT="${NFS_NODE_MOUNT:-/shared/node}"
NFS_CLUSTER_MOUNT="${NFS_CLUSTER_MOUNT:-/shared/cluster}"
NFS_MOUNT_OPTIONS="${NFS_MOUNT_OPTIONS:-vers=4.1,rw,hard,timeo=600,retrans=2,_netdev,x-systemd.automount}"
NFS_MOUNT_TIMEOUT_SECONDS="${NFS_MOUNT_TIMEOUT_SECONDS:-20}"
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
RESET_USER_CPU_QUOTA_ON_INSTALL="${RESET_USER_CPU_QUOTA_ON_INSTALL:-1}"
CLEAR_USER_LIMITS="${CLEAR_USER_LIMITS:-}"

NODE_ID="${NODE_ID:-}"
CONTROLLER_URL="${CONTROLLER_URL:-}"
AGENT_TOKEN="${AGENT_TOKEN:-}"

load_existing_service_env() {
  local svc=""
  local env_file="${NODE_AGENT_ENV_FILE}"
  if [[ -f /etc/systemd/system/${SERVICE_NAME}.service ]]; then
    svc="/etc/systemd/system/${SERVICE_NAME}.service"
  elif [[ -f /lib/systemd/system/${SERVICE_NAME}.service ]]; then
    svc="/lib/systemd/system/${SERVICE_NAME}.service"
  fi
  if [[ -f "${env_file}" ]]; then
    if [[ -z "${NODE_ID}" ]]; then
      NODE_ID="$(awk -F'=' '/^[[:space:]]*NODE_ID=/{print $2; exit}' "${env_file}" | sed -e 's/^"//' -e 's/"$//')"
    fi
    if [[ -z "${CONTROLLER_URL}" ]]; then
      CONTROLLER_URL="$(awk -F'=' '/^[[:space:]]*CONTROLLER_URL=/{print $2; exit}' "${env_file}" | sed -e 's/^"//' -e 's/"$//')"
    fi
    if [[ -z "${AGENT_TOKEN}" ]]; then
      AGENT_TOKEN="$(awk -F'=' '/^[[:space:]]*AGENT_TOKEN=/{print $2; exit}' "${env_file}" | sed -e 's/^"//' -e 's/"$//')"
    fi
  fi
  if [[ -z "${svc}" ]]; then
    return 0
  fi

  if [[ -z "${NODE_ID}" ]]; then
    NODE_ID="$(awk -F'=' '/^[[:space:]]*Environment=NODE_ID=/{print $3; exit}' "${svc}" | sed -e 's/^"//' -e 's/"$//')"
  fi
  if [[ -z "${CONTROLLER_URL}" ]]; then
    CONTROLLER_URL="$(awk -F'=' '/^[[:space:]]*Environment=CONTROLLER_URL=/{print $3; exit}' "${svc}" | sed -e 's/^"//' -e 's/"$//')"
  fi
  if [[ -z "${AGENT_TOKEN}" ]]; then
    AGENT_TOKEN="$(awk -F'=' '/^[[:space:]]*Environment=AGENT_TOKEN=/{print $3; exit}' "${svc}" | sed -e 's/^"//' -e 's/"$//')"
  fi
}

load_existing_guard_conf() {
  local conf="/etc/gpu-cluster/ssh_guard.conf"
  if [[ ! -f "${conf}" ]]; then
    return 0
  fi
  # 若未显式传入，沿用节点当前 SSH Guard 的排除用户配置。
  if [[ "${SSH_GUARD_EXCLUDE_USERS}" == "root" ]]; then
    local v
    v="$(awk -F'=' '/^[[:space:]]*EXCLUDE_USERS=/{print $2; exit}' "${conf}" | sed -e 's/^"//' -e 's/"$//')"
    if [[ -n "${v}" ]]; then
      SSH_GUARD_EXCLUDE_USERS="${v}"
    fi
  fi
}

load_existing_service_env
load_existing_guard_conf

cleanup_runtime_limit_residue() {
  if [[ "${RESET_USER_CPU_QUOTA_ON_INSTALL}" != "1" ]]; then
    return 0
  fi

  echo "清理历史用户 CPU/内存运行态残留"
  while IFS=: read -r username _ uid _ _ home shell; do
    username="$(echo "${username}" | xargs)"
    uid="$(echo "${uid}" | xargs)"
    home="$(echo "${home}" | xargs)"
    shell="$(echo "${shell}" | xargs)"
    if [[ -z "${username}" || -z "${uid}" || -z "${home}" ]]; then
      continue
    fi
    if [[ ! "${uid}" =~ ^[0-9]+$ ]] || [[ "${uid}" -lt 1000 ]]; then
      continue
    fi
    if [[ "${home}" != /home/* ]]; then
      continue
    fi
    if [[ "${shell}" == *nologin* || "${shell}" == *false* ]]; then
      continue
    fi

    ${SUDO} systemctl set-property --runtime "user-${uid}.slice" CPUQuota= >/dev/null 2>&1 || true
    ${SUDO} systemctl set-property --runtime "user-${uid}.slice" MemoryAccounting=yes MemoryHigh=infinity MemoryMax=infinity >/dev/null 2>&1 || true
    ${SUDO} systemctl set-property --runtime "user@${uid}.service" MemoryAccounting=yes MemoryHigh=infinity MemoryMax=infinity >/dev/null 2>&1 || true
    ${SUDO} rm -f "/etc/systemd/system/user-${uid}.slice.d/90-gpuops-cpu-quota.conf" >/dev/null 2>&1 || true
    ${SUDO} rm -f "/etc/systemd/system/user-${uid}.slice.d/90-gpuops-memory-limit.conf" >/dev/null 2>&1 || true
    ${SUDO} rmdir "/etc/systemd/system/user-${uid}.slice.d" >/dev/null 2>&1 || true

    if [[ -w "/sys/fs/cgroup/user.slice/user-${uid}.slice/cpu.max" ]]; then
      echo "max 100000" | ${SUDO} tee "/sys/fs/cgroup/user.slice/user-${uid}.slice/cpu.max" >/dev/null || true
    fi
    if [[ -w "/sys/fs/cgroup/user-${uid}.slice/cpu.max" ]]; then
      echo "max 100000" | ${SUDO} tee "/sys/fs/cgroup/user-${uid}.slice/cpu.max" >/dev/null || true
    fi

    if [[ -w "/sys/fs/cgroup/cpu/gpuops/user-${uid}/cpu.cfs_quota_us" ]]; then
      echo "-1" | ${SUDO} tee "/sys/fs/cgroup/cpu/gpuops/user-${uid}/cpu.cfs_quota_us" >/dev/null || true
    fi
    if [[ -w "/sys/fs/cgroup/user.slice/user-${uid}.slice/memory.high" ]]; then
      echo "max" | ${SUDO} tee "/sys/fs/cgroup/user.slice/user-${uid}.slice/memory.high" >/dev/null || true
    fi
    if [[ -w "/sys/fs/cgroup/user.slice/user-${uid}.slice/memory.max" ]]; then
      echo "max" | ${SUDO} tee "/sys/fs/cgroup/user.slice/user-${uid}.slice/memory.max" >/dev/null || true
    fi
    if [[ -w "/sys/fs/cgroup/user.slice/user-${uid}.slice/user@${uid}.service/memory.high" ]]; then
      echo "max" | ${SUDO} tee "/sys/fs/cgroup/user.slice/user-${uid}.slice/user@${uid}.service/memory.high" >/dev/null || true
    fi
    if [[ -w "/sys/fs/cgroup/user.slice/user-${uid}.slice/user@${uid}.service/memory.max" ]]; then
      echo "max" | ${SUDO} tee "/sys/fs/cgroup/user.slice/user-${uid}.slice/user@${uid}.service/memory.max" >/dev/null || true
    fi
  done < /etc/passwd

  # 清理旧状态文件，避免 agent 重启时先恢复过期的运行态限制。
  ${SUDO} find /home -mindepth 2 -maxdepth 2 -type f \( -name ".cpu_quota" -o -name ".memory_limit" \) -delete >/dev/null 2>&1 || true
  ${SUDO} systemctl daemon-reload >/dev/null 2>&1 || true
}

clear_single_user_limits() {
  local username="$1"
  username="$(echo "${username}" | xargs)"
  if [[ -z "${username}" || "${username}" == "root" ]]; then
    return 0
  fi
  local uid
  uid="$(id -u "${username}" 2>/dev/null || true)"
  if [[ -z "${uid}" || ! "${uid}" =~ ^[0-9]+$ ]]; then
    echo "跳过：用户不存在 ${username}" >&2
    return 0
  fi

  ${SUDO} rm -f \
    "/etc/systemd/system/user-${uid}.slice.d/90-gpuops-cpu-quota.conf" \
    "/etc/systemd/system/user-${uid}.slice.d/90-gpuops-memory-limit.conf" \
    "/etc/systemd/system/user@${uid}.service.d/90-gpuops-memory-limit.conf" \
    "/home/${username}/.cpu_quota" \
    "/home/${username}/.memory_limit" >/dev/null 2>&1 || true
  ${SUDO} rmdir "/etc/systemd/system/user-${uid}.slice.d" "/etc/systemd/system/user@${uid}.service.d" >/dev/null 2>&1 || true
  ${SUDO} systemctl daemon-reload >/dev/null 2>&1 || true
  ${SUDO} systemctl set-property --runtime "user-${uid}.slice" CPUQuota=infinity MemoryAccounting=yes MemoryHigh=infinity MemoryMax=infinity >/dev/null 2>&1 || true
  ${SUDO} systemctl set-property --runtime "user@${uid}.service" MemoryAccounting=yes MemoryHigh=infinity MemoryMax=infinity >/dev/null 2>&1 || true

  for p in "/sys/fs/cgroup/user.slice/user-${uid}.slice" "/sys/fs/cgroup/user-${uid}.slice"; do
    [[ -d "${p}" ]] || continue
    [[ -w "${p}/cpu.max" ]] && echo "max 100000" | ${SUDO} tee "${p}/cpu.max" >/dev/null || true
    [[ -w "${p}/memory.high" ]] && echo "max" | ${SUDO} tee "${p}/memory.high" >/dev/null || true
    [[ -w "${p}/memory.max" ]] && echo "max" | ${SUDO} tee "${p}/memory.max" >/dev/null || true
  done
  echo "已清理用户资源限制：${username} uid=${uid}"
}

harden_project_workspace() {
  if [[ -d "${PROJECT_ROOT}" ]]; then
    find "${PROJECT_ROOT}" -xdev \( -type d -o -type f \) -exec chmod 700 {} + 2>/dev/null || true
  fi
}

build_agent_ldflags() {
  local build_at commit dirty modified=""
  build_at="$(date -u '+%Y%m%dT%H%M%SZ')"
  if git -C "${PROJECT_ROOT}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    commit="$(git -C "${PROJECT_ROOT}" rev-parse --short=12 HEAD 2>/dev/null || true)"
    dirty="$(git -C "${PROJECT_ROOT}" status --porcelain --untracked-files=no 2>/dev/null || true)"
    if [[ -n "${dirty}" ]]; then
      modified="true"
    fi
  fi

  local ldflags="-X main.agentBuildAt=${build_at}"
  if [[ -n "${commit:-}" ]]; then
    ldflags="${ldflags} -X main.agentCommit=${commit}"
  fi
  if [[ -n "${modified}" ]]; then
    ldflags="${ldflags} -X main.agentVCSModified=${modified}"
  fi
  echo "${ldflags}"
}

trim_text() {
  echo "$1" | xargs
}

collect_local_ipv4s() {
  {
    hostname -I 2>/dev/null || true
    ip -o -4 addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1 || true
  } | tr ' ' '\n' | sed '/^$/d' | awk '!seen[$0]++'
}

auto_detect_node_id() {
  local map_file="$1"
  local prefer_user="$2"
  if [[ ! -f "${map_file}" ]]; then
    return 1
  fi

  mapfile -t local_ips < <(collect_local_ipv4s)
  if [[ ${#local_ips[@]} -eq 0 ]]; then
    return 1
  fi

  local matches=()
  while IFS=',' read -r txt_file port ip node_id user; do
    txt_file="$(trim_text "${txt_file}")"
    ip="$(trim_text "${ip}")"
    node_id="$(trim_text "${node_id}")"
    user="$(trim_text "${user}")"
    if [[ "${txt_file}" == "txt文件名" || -z "${ip}" || -z "${node_id}" ]]; then
      continue
    fi
    for local_ip in "${local_ips[@]}"; do
      if [[ "${ip}" == "${local_ip}" ]]; then
        matches+=("${node_id}|${user}|${ip}")
      fi
    done
  done < "${map_file}"

  if [[ ${#matches[@]} -eq 0 ]]; then
    return 1
  fi

  if [[ ${#matches[@]} -eq 1 ]]; then
    NODE_ID="${matches[0]%%|*}"
    return 0
  fi

  if [[ -n "${prefer_user}" ]]; then
    local user_matches=()
    local item
    for item in "${matches[@]}"; do
      local m_user
      m_user="$(echo "${item}" | cut -d'|' -f2)"
      if [[ "${m_user}" == "${prefer_user}" ]]; then
        user_matches+=("${item}")
      fi
    done
    if [[ ${#user_matches[@]} -eq 1 ]]; then
      NODE_ID="${user_matches[0]%%|*}"
      return 0
    fi
  fi

  NODE_ID="${matches[0]%%|*}"
  echo "警告：根据 ${map_file} 匹配到多个节点，默认使用 NODE_ID=${NODE_ID}" >&2
  echo "候选如下：" >&2
  printf '%s\n' "${matches[@]}" >&2
  return 0
}

configure_shared_nfs() {
  if [[ "${ENABLE_SHARED_NFS}" != "1" ]]; then
    return 0
  fi
  if [[ -z "${NFS_SERVER}" || ! "${NODE_ID}" =~ ^[0-9]+$ ]]; then
    echo "NFS 配置非法：NFS_SERVER/NODE_ID 不能为空，且 NODE_ID 必须为数字" >&2
    return 2
  fi
  if [[ ! "${NFS_MOUNT_TIMEOUT_SECONDS}" =~ ^[0-9]+$ || "${NFS_MOUNT_TIMEOUT_SECONDS}" -le 0 ]]; then
    echo "NFS_MOUNT_TIMEOUT_SECONDS 必须为正整数" >&2
    return 2
  fi

  echo "[12/12] 配置共享 NFS"
  if ! command -v mount.nfs >/dev/null 2>&1; then
    if ! command -v apt-get >/dev/null 2>&1; then
      echo "未找到 mount.nfs，且系统不支持 apt-get，无法安装 nfs-common" >&2
      return 2
    fi
    echo "安装 NFS 客户端 nfs-common"
    DEBIAN_FRONTEND=noninteractive ${SUDO} apt-get update -y
    DEBIAN_FRONTEND=noninteractive ${SUDO} apt-get install -y nfs-common
  fi

  local node_source="${NFS_SERVER}:${NFS_NODE_EXPORT_ROOT%/}/${NODE_ID}"
  local cluster_source="${NFS_SERVER}:${NFS_CLUSTER_EXPORT}"
  ${SUDO} install -d -o root -g root -m 0755 /shared "${NFS_NODE_MOUNT}" "${NFS_CLUSTER_MOUNT}"

  local target current_source
  for target in "${NFS_NODE_MOUNT}" "${NFS_CLUSTER_MOUNT}"; do
    current_source="$(findmnt -rn -o SOURCE --mountpoint "${target}" 2>/dev/null || true)"
    if [[ -n "${current_source}" ]]; then
      local expected_source="${cluster_source}"
      if [[ "${target}" == "${NFS_NODE_MOUNT}" ]]; then
        expected_source="${node_source}"
      fi
      if [[ "${current_source}" != "${expected_source}" ]]; then
        echo "拒绝覆盖现有挂载：${target} 当前来源=${current_source}，期望=${expected_source}" >&2
        return 2
      fi
    fi
  done

  if [[ ! -e /etc/fstab.gpuops-before-shared-nfs ]]; then
    ${SUDO} cp -a /etc/fstab /etc/fstab.gpuops-before-shared-nfs
  fi
  local fstab_tmp
  fstab_tmp="$(mktemp)"
  awk -v node_mount="${NFS_NODE_MOUNT}" -v cluster_mount="${NFS_CLUSTER_MOUNT}" '
    /^[[:space:]]*#/ || NF == 0 { print; next }
    $2 != node_mount && $2 != cluster_mount { print }
  ' /etc/fstab >"${fstab_tmp}"
  printf '%s %s nfs %s 0 0\n' "${node_source}" "${NFS_NODE_MOUNT}" "${NFS_MOUNT_OPTIONS}" >>"${fstab_tmp}"
  printf '%s %s nfs %s 0 0\n' "${cluster_source}" "${NFS_CLUSTER_MOUNT}" "${NFS_MOUNT_OPTIONS}" >>"${fstab_tmp}"
  ${SUDO} install -m 0644 -o root -g root "${fstab_tmp}" /etc/fstab
  rm -f "${fstab_tmp}"
  ${SUDO} systemctl daemon-reload

  local expected_source
  for target in "${NFS_NODE_MOUNT}" "${NFS_CLUSTER_MOUNT}"; do
    expected_source="${cluster_source}"
    if [[ "${target}" == "${NFS_NODE_MOUNT}" ]]; then
      expected_source="${node_source}"
    fi
    current_source="$(findmnt -rn -o SOURCE --mountpoint "${target}" 2>/dev/null || true)"
    if [[ -z "${current_source}" ]]; then
      timeout "${NFS_MOUNT_TIMEOUT_SECONDS}" ${SUDO} mount "${target}"
      current_source="$(findmnt -rn -o SOURCE --mountpoint "${target}" 2>/dev/null || true)"
    fi
    if [[ "${current_source}" != "${expected_source}" ]]; then
      echo "NFS 挂载校验失败：${target} 来源=${current_source:-<未挂载>}，期望=${expected_source}" >&2
      return 2
    fi
    echo "NFS 挂载完成：${target} <- ${current_source}"
  done
}

usage() {
  cat <<USAGE
用法：
  # 手动指定 NODE_ID
  NODE_ID=node-01 \\
  CONTROLLER_URL=https://controller.example.org:60040 \\
  AGENT_TOKEN=<agent_token> \\
  bash scripts/install_agent_local.sh

  # 也支持把 KEY=VALUE 作为脚本参数传入（用于 sudo env_reset 场景）
  sudo -n /bin/bash /home/<user>/gpu-ops/scripts/install_agent_local.sh \\
    ENABLE_SYSTEM_MEMORY_RESERVE=1 SYSTEM_MEMORY_RESERVE_GB=70

  # 自动识别 NODE_ID（按本机 IP 匹配 my_ssh_keys/server_ssh_map.csv）
  CONTROLLER_URL=https://controller.example.org:60040 \\
  AGENT_TOKEN=<agent_token> \\
  bash scripts/install_agent_local.sh

可选环境变量：
  INSTALL_DEPS=1|0                是否先安装依赖（默认 1）
  INSTALL_NODE_DEPS=1|0           依赖安装时是否安装 Node/pnpm（默认 0，计算节点通常不需要）
  INSTALL_DOCKER_DEPS=1|0         依赖安装时是否安装 Docker（默认 0，计算节点通常不需要）
  NODE_ID_AUTO_DETECT=1|0         NODE_ID 为空时是否自动识别（默认 1）
  NODE_MAP_FILE=...               自动识别时使用的映射表（默认 my_ssh_keys/server_ssh_map.csv）
  NODE_MAP_USER=...               自动识别时用于歧义消解的用户名（默认当前系统用户）
  GO_PROXY=...                     Go 模块代理（默认 https://goproxy.cn,direct）
  GO_SUMDB=...                     Go sumdb（默认 sum.golang.google.cn）
  ACTION_POLL_INTERVAL_SECONDS=2   节点动作轮询周期（秒，默认 2）
  LOCAL_USERS_REFRESH_SECONDS=900  本地用户详情刷新周期（秒，默认 900）
  LOCAL_USERS_COLLECT_TIMEOUT_SECONDS=8 本地用户详情采集超时（秒，默认 8）
  SYSTEM_SERVICES_REFRESH_SECONDS=60 systemd 服务巡检刷新周期（秒，默认 60）
  SYSTEM_SERVICES_COLLECT_TIMEOUT_SECONDS=8 systemd 服务巡检超时（秒，默认 8）
  GPU_BUS_MAP_CACHE_SECONDS=600    GPU 总线映射缓存（秒，默认 600）
  GPU_INVENTORY_CACHE_SECONDS=1800 GPU 型号/数量缓存（秒，默认 1800）
  GPU_COMMAND_TIMEOUT_SECONDS=4    单次 nvidia-smi 超时（秒，默认 4）
  SKIP_CONTROLLER_HEALTHCHECK=1    跳过控制器健康检查
  SYNC_TIME_WITH_CONTROLLER=1      与控制器时间对齐（默认 1）
  PROJECT_ROOT=...                 项目根目录（默认脚本自动推断）
  ENABLE_SSH_GUARD=1|0             是否安装 SSH 登录拦截（默认 1）
  SSH_GUARD_EXCLUDE_USERS=\"...\"    拦截排除用户（默认 root）
  SSH_GUARD_EXEMPT_FILE=...         豁免账号缓存文件（默认 /var/lib/gpu-cluster/exempt_users.txt）
  SSH_GUARD_FAIL_OPEN=0|1          控制器不可达时是否放行（默认 0，严格模式）
  SSH_GUARD_REALTIME_LOOKUP=0|1    登录时是否实时请求控制器校验（默认 0，减少登录卡顿）
  SSH_GUARD_SYNC_INTERVAL=10s      白名单/黑名单缓存同步周期（默认 10s）
  SSH_GUARD_ENFORCE_INTERVAL=10s   在线会话巡检周期（默认 10s）
  ENABLE_HOST_SECURITY=1|0         安装主机安全基线（fail2ban，默认 1）
  SSH_FAIL2BAN_MAXRETRY=20         fail2ban SSH 最大重试次数（默认 20）
  SSH_FAIL2BAN_FINDTIME=5m         fail2ban SSH 统计窗口（默认 5m）
  SSH_FAIL2BAN_BANTIME=12h         fail2ban SSH 封禁时长（默认 12h）
  SSH_FAIL2BAN_IGNOREIP="..."      fail2ban 忽略网段（默认空，不忽略 localhost）
  ENABLE_SHARED_NFS=1|0            配置 /shared NFS（默认 0，需先完成服务端导出）
  NFS_SERVER=192.0.2.10         NFS 服务端地址
  NFS_NODE_EXPORT_ROOT=...         节点私有导出根目录（默认 /srv/gpu-ops/nodes）
	NFS_CLUSTER_EXPORT=...           集群公共导出目录（默认 /srv/gpu-ops/cluster）
  NFS_NODE_MOUNT=/shared/node      节点私有挂载点
  NFS_CLUSTER_MOUNT=/shared/cluster 集群公共挂载点
  NFS_MOUNT_OPTIONS=...            /etc/fstab NFS 参数
  ENABLE_SYSTEM_CPU_RESERVE=1      是否为系统保留 CPU（默认 1，开启）
  SYSTEM_CPU_RESERVE_PERCENT=5     为系统保留 CPU 百分比（默认 5）
  ENABLE_SYSTEM_MEMORY_RESERVE=1   是否为系统保留内存（默认 1，开启）
  SYSTEM_MEMORY_RESERVE_GB=8       为系统保留内存（GB，默认 8）
  ENABLE_USER_SLICE_CPU_RESERVE=1  兼容旧写法，等价于 ENABLE_SYSTEM_CPU_RESERVE
  USER_SLICE_CPU_RESERVE_PERCENT=95 兼容旧写法，表示 user.slice CPU 上限 95%，会自动换算成系统保留 5%
  ENABLE_USER_SLICE_MEMORY_RESERVE=1 兼容旧写法，等价于 ENABLE_SYSTEM_MEMORY_RESERVE
  USER_SLICE_MEMORY_RESERVE_GB=8   兼容旧写法，等价于 SYSTEM_MEMORY_RESERVE_GB
  RESET_USER_CPU_QUOTA_ON_INSTALL=1 安装时清理历史用户 CPU/内存运行态残留（默认 1）
  CLEAR_USER_LIMITS="u1,u2"       只清理指定用户 CPU/内存限制并退出
USAGE
}

if [[ -n "${CLI_INVALID_ARG}" ]]; then
  echo "未知参数：${CLI_INVALID_ARG}" >&2
  usage
  exit 2
fi

if [[ "${SHOW_USAGE}" == "1" ]]; then
  usage
  exit 0
fi

if [[ "$(id -u)" -ne 0 ]]; then
  SUDO="sudo"
else
  SUDO=""
fi

if [[ -n "${CLEAR_USER_LIMITS}" ]]; then
  users_to_clear="${CLEAR_USER_LIMITS//,/ }"
  for user_to_clear in ${users_to_clear}; do
    clear_single_user_limits "${user_to_clear}"
  done
  exit 0
fi

if [[ -z "${CONTROLLER_URL}" || -z "${AGENT_TOKEN}" ]]; then
  echo "缺少必需参数：CONTROLLER_URL/AGENT_TOKEN" >&2
  usage
  exit 2
fi

if [[ -z "${NODE_ID}" && "${NODE_ID_AUTO_DETECT}" == "1" ]]; then
  detect_user="${NODE_MAP_USER}"
  if [[ -z "${detect_user}" ]]; then
    detect_user="$(id -un 2>/dev/null || true)"
  fi
  if auto_detect_node_id "${NODE_MAP_FILE}" "${detect_user}"; then
    echo "自动识别 NODE_ID 成功：${NODE_ID}（映射表：${NODE_MAP_FILE}）"
  else
    echo "自动识别 NODE_ID 失败：请手动设置 NODE_ID，或检查映射表 ${NODE_MAP_FILE}" >&2
    usage
    exit 2
  fi
fi

if [[ -z "${NODE_ID}" ]]; then
  echo "NODE_ID 不能为空（可手动设置 NODE_ID，或启用 NODE_ID_AUTO_DETECT=1）" >&2
  usage
  exit 2
fi

if [[ ! -d "${NODE_AGENT_DIR}" ]]; then
  echo "node-agent 目录不存在：${NODE_AGENT_DIR}" >&2
  exit 2
fi

echo "[1/9] 基础信息"
echo "PROJECT_ROOT=${PROJECT_ROOT}"
echo "NODE_ID=${NODE_ID}"
echo "CONTROLLER_URL=${CONTROLLER_URL}"
echo "SERVICE_NAME=${SERVICE_NAME}"
echo "SYNC_TIME_WITH_CONTROLLER=${SYNC_TIME_WITH_CONTROLLER}"
echo "ENABLE_SSH_GUARD=${ENABLE_SSH_GUARD} (FAIL_OPEN=${SSH_GUARD_FAIL_OPEN})"
echo "ENABLE_SYSTEM_CPU_RESERVE=${ENABLE_SYSTEM_CPU_RESERVE} (SYSTEM_CPU_RESERVE_PERCENT=${SYSTEM_CPU_RESERVE_PERCENT})"
echo "ENABLE_SYSTEM_MEMORY_RESERVE=${ENABLE_SYSTEM_MEMORY_RESERVE} (SYSTEM_MEMORY_RESERVE_GB=${SYSTEM_MEMORY_RESERVE_GB})"
echo "RESET_USER_CPU_QUOTA_ON_INSTALL=${RESET_USER_CPU_QUOTA_ON_INSTALL}"
if [[ "${LEGACY_CPU_RESERVE_COMPAT_USED}" == "1" ]]; then
  echo "兼容旧变量：USER_SLICE_CPU_RESERVE_PERCENT=${LEGACY_USER_SLICE_CPU_RESERVE_PERCENT} -> SYSTEM_CPU_RESERVE_PERCENT=${SYSTEM_CPU_RESERVE_PERCENT}"
fi
if [[ "${LEGACY_MEMORY_RESERVE_COMPAT_USED}" == "1" ]]; then
  echo "兼容旧变量：USER_SLICE_MEMORY_RESERVE_GB=${LEGACY_USER_SLICE_MEMORY_RESERVE_GB} -> SYSTEM_MEMORY_RESERVE_GB=${SYSTEM_MEMORY_RESERVE_GB}"
fi
echo "INSTALL_NODE_DEPS=${INSTALL_NODE_DEPS}"
echo "INSTALL_DOCKER_DEPS=${INSTALL_DOCKER_DEPS}"
echo "NODE_ID_AUTO_DETECT=${NODE_ID_AUTO_DETECT}"
echo "ENABLE_SHARED_NFS=${ENABLE_SHARED_NFS} (NFS_SERVER=${NFS_SERVER})"

if [[ "${INSTALL_DEPS}" == "1" ]]; then
  echo "[2/9] 安装依赖"
  if [[ -f "${PROJECT_ROOT}/scripts/install_deps_ubuntu2204.sh" ]]; then
    INSTALL_NODE="${INSTALL_NODE_DEPS}" \
    INSTALL_DOCKER="${INSTALL_DOCKER_DEPS}" \
    bash "${PROJECT_ROOT}/scripts/install_deps_ubuntu2204.sh"
  else
    echo "未找到依赖脚本 scripts/install_deps_ubuntu2204.sh" >&2
    exit 2
  fi
else
  echo "[2/9] 跳过依赖安装"
fi

# 依赖脚本可能刚安装 Go 到 /usr/local/go/bin，但当前 shell 的 PATH 尚未更新。
if ! command -v go >/dev/null 2>&1; then
  if [[ -x /usr/local/go/bin/go ]]; then
    export PATH="/usr/local/go/bin:${PATH}"
    hash -r || true
  fi
fi
if ! command -v go >/dev/null 2>&1; then
  echo "未找到 go 命令，请确认 Go 已安装（例如 /usr/local/go/bin/go）" >&2
  echo "可手动执行：export PATH=/usr/local/go/bin:\$PATH" >&2
  exit 2
fi

echo "[3/9] 清理代理变量"
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY ALL_PROXY all_proxy || true

echo "[4/9] 配置 Go 源"
go env -w GOPROXY="${GO_PROXY}"
go env -w GOSUMDB="${GO_SUMDB}"
go env -w GO111MODULE=on
echo "GOPROXY=$(go env GOPROXY)"
echo "GOSUMDB=$(go env GOSUMDB)"

if [[ "${SYNC_TIME_WITH_CONTROLLER}" == "1" ]]; then
  echo "[5/9] 与控制器时间对齐"
  # 某些服务对 HEAD /healthz 返回 404，这里改用 GET 读取响应头更稳妥。
  date_header="$(curl -fsS -D - -o /dev/null --max-time 3 "${CONTROLLER_URL}/healthz" | awk -F': ' 'tolower($1)=="date"{print $2}' | tr -d '\r' | tail -n 1 || true)"
  if [[ -z "${date_header}" ]]; then
    echo "未获取到控制器 Date 头，跳过对时（可检查控制器连通性）"
  else
    if ${SUDO} date -u -s "${date_header}" >/dev/null 2>&1; then
      echo "已对时到控制器时间：${date_header}"
      ${SUDO} hwclock -w >/dev/null 2>&1 || true
    else
      echo "对时失败（date -s），继续执行后续步骤"
    fi
  fi
  echo "当前系统时间：$(date -u '+%F %T UTC')"
else
  echo "[5/9] 跳过与控制器对时"
fi

if [[ "${SKIP_CONTROLLER_HEALTHCHECK}" != "1" ]]; then
  echo "[6/9] 控制器健康检查"
  healthz_out="$(mktemp /tmp/agent_healthz.out.XXXXXX)"
  healthz_err="$(mktemp /tmp/agent_healthz.err.XXXXXX)"
  if ! curl -fsS --max-time 3 "${CONTROLLER_URL}/healthz" >"${healthz_out}" 2>"${healthz_err}"; then
    echo "控制器健康检查失败：${CONTROLLER_URL}/healthz" >&2
    echo "错误详情：$(cat "${healthz_err}" 2>/dev/null || true)" >&2
    rm -f "${healthz_out}" "${healthz_err}" >/dev/null 2>&1 || true
    echo "如需跳过，请设置 SKIP_CONTROLLER_HEALTHCHECK=1" >&2
    exit 3
  fi
  echo "健康检查通过：$(cat "${healthz_out}")"
  rm -f "${healthz_out}" "${healthz_err}" >/dev/null 2>&1 || true
else
  echo "[6/9] 跳过控制器健康检查"
fi

echo "[7/9] 编译 node-agent"
cd "${NODE_AGENT_DIR}"
go mod download
AGENT_LDFLAGS="$(build_agent_ldflags)"
echo "node-agent ldflags=${AGENT_LDFLAGS}"
go build -ldflags "${AGENT_LDFLAGS}" -o node-agent .

cleanup_runtime_limit_residue

echo "[8/9] 安装 systemd 服务"
${SUDO} install -m 0755 node-agent /usr/local/bin/node-agent
${SUDO} install -d -m 0755 /etc/gpu-cluster
node_agent_env_tmp="$(mktemp)"
cat >"${node_agent_env_tmp}" <<EOF_NODE_AGENT_ENV
NODE_ID=${NODE_ID}
CONTROLLER_URL=${CONTROLLER_URL}
AGENT_TOKEN=${AGENT_TOKEN}
ACTION_POLL_INTERVAL_SECONDS=${ACTION_POLL_INTERVAL_SECONDS}
LOCAL_USERS_REFRESH_SECONDS=${LOCAL_USERS_REFRESH_SECONDS}
LOCAL_USERS_COLLECT_TIMEOUT_SECONDS=${LOCAL_USERS_COLLECT_TIMEOUT_SECONDS}
SYSTEM_SERVICES_REFRESH_SECONDS=${SYSTEM_SERVICES_REFRESH_SECONDS}
SYSTEM_SERVICES_COLLECT_TIMEOUT_SECONDS=${SYSTEM_SERVICES_COLLECT_TIMEOUT_SECONDS}
GPU_BUS_MAP_CACHE_SECONDS=${GPU_BUS_MAP_CACHE_SECONDS}
GPU_INVENTORY_CACHE_SECONDS=${GPU_INVENTORY_CACHE_SECONDS}
GPU_COMMAND_TIMEOUT_SECONDS=${GPU_COMMAND_TIMEOUT_SECONDS}
EOF_NODE_AGENT_ENV
${SUDO} install -m 0600 -o root -g root "${node_agent_env_tmp}" "${NODE_AGENT_ENV_FILE}"
rm -f "${node_agent_env_tmp}"
public_env_tmp="$(mktemp)"
cat >"${public_env_tmp}" <<EOF_PUBLIC_ENV
CONTROLLER_URL=${CONTROLLER_URL}
NODE_ID=${NODE_ID}
EOF_PUBLIC_ENV
${SUDO} install -m 0644 -o root -g root "${public_env_tmp}" "${PUBLIC_ENV_FILE}"
rm -f "${public_env_tmp}"
${SUDO} tee "/etc/systemd/system/${SERVICE_NAME}.service" >/dev/null <<EOF_SERVICE
[Unit]
Description=GPU Cluster Node Agent
After=network.target

[Service]
Type=simple
User=root
EnvironmentFile=${NODE_AGENT_ENV_FILE}
ExecStart=/usr/local/bin/node-agent
Restart=always

[Install]
WantedBy=multi-user.target
EOF_SERVICE

${SUDO} systemctl daemon-reload
${SUDO} systemctl enable "${SERVICE_NAME}"
if ${SUDO} systemctl is-active --quiet "${SERVICE_NAME}"; then
  echo "检测到 ${SERVICE_NAME} 已在运行，执行重启以加载新二进制"
  ${SUDO} systemctl restart "${SERVICE_NAME}"
else
  echo "检测到 ${SERVICE_NAME} 未运行，执行启动"
  ${SUDO} systemctl start "${SERVICE_NAME}"
fi

echo "[9/9] 服务状态"
${SUDO} systemctl --no-pager --full status "${SERVICE_NAME}" || true
${SUDO} journalctl -u "${SERVICE_NAME}" -n 40 --no-pager || true

${SUDO} tee /usr/local/bin/gpuops-claim >/dev/null <<'EOF_CLAIM'
#!/bin/bash
set -euo pipefail

PUBLIC_CONF="/etc/gpu-cluster/public.env"
if [[ -r "${PUBLIC_CONF}" ]]; then
  # shellcheck disable=SC1090
  source "${PUBLIC_CONF}"
fi

TOKEN="${1:-}"
if [[ -z "${TOKEN}" ]]; then
  echo "用法：gpuops-claim <challenge_token>" >&2
  exit 2
fi

CONTROLLER_URL="${CONTROLLER_URL:-}"
NODE_ID="${NODE_ID:-}"
LOCAL_USER="$(id -un 2>/dev/null || whoami)"

if [[ -z "${CONTROLLER_URL}" || -z "${NODE_ID}" || -z "${LOCAL_USER}" ]]; then
  echo "缺少 CONTROLLER_URL/NODE_ID/LOCAL_USER，无法提交 challenge" >&2
  exit 2
fi

api="${CONTROLLER_URL%/}/api/registry/bind-claim"
payload="$(printf '{\"token\":\"%s\",\"node_id\":\"%s\",\"local_username\":\"%s\"}' "${TOKEN}" "${NODE_ID}" "${LOCAL_USER}")"
tmp_resp="$(mktemp)"
trap 'rm -f "${tmp_resp}"' EXIT
http_code="$(curl -sS -o "${tmp_resp}" -w '%{http_code}' -H "Content-Type: application/json" --data "${payload}" "${api}")"
resp="$(cat "${tmp_resp}")"
if [[ ! "${http_code}" =~ ^2[0-9][0-9]$ ]]; then
  if [[ -n "${resp}" ]]; then
    echo "${resp}" >&2
  fi
  echo "gpuops-claim 失败：http ${http_code}" >&2
  exit 1
fi
echo "${resp}"
msg="$(echo "${resp}" | sed -n 's/.*"message":"\([^"]*\)".*/\1/p')"
wait_s="$(echo "${resp}" | sed -n 's/.*"estimated_wait_seconds":\([0-9][0-9]*\).*/\1/p')"
if [[ -n "${msg}" ]]; then
  echo "提示：${msg}"
fi
if [[ -n "${wait_s}" ]]; then
  echo "建议等待：${wait_s} 秒"
fi
echo
EOF_CLAIM
${SUDO} chmod 0755 /usr/local/bin/gpuops-claim

if [[ "${ENABLE_SSH_GUARD}" == "1" ]]; then
  echo "[10/10] 安装 SSH Guard（PAM 登录拦截）"
  ${SUDO} mkdir -p /opt/gpu-cluster /etc/gpu-cluster /var/lib/gpu-cluster /etc/systemd/system

  ${SUDO} tee /etc/gpu-cluster/ssh_guard.conf >/dev/null <<EOF_GUARD_CONF
CONTROLLER_URL="${CONTROLLER_URL}"
NODE_ID="${NODE_ID}"
AGENT_TOKEN="${AGENT_TOKEN}"
EXCLUDE_USERS="${SSH_GUARD_EXCLUDE_USERS}"
FAIL_OPEN="${SSH_GUARD_FAIL_OPEN}"
ALLOWLIST_FILE="${SSH_GUARD_ALLOWLIST_FILE}"
DENYLIST_FILE="${SSH_GUARD_DENYLIST_FILE}"
EXEMPT_FILE="${SSH_GUARD_EXEMPT_FILE}"
GUARD_STATE_FILE="${SSH_GUARD_STATE_FILE}"
REALTIME_LOOKUP="${SSH_GUARD_REALTIME_LOOKUP}"
EOF_GUARD_CONF
  ${SUDO} chmod 0600 /etc/gpu-cluster/ssh_guard.conf
  ${SUDO} chown root:root /etc/gpu-cluster/ssh_guard.conf

  ${SUDO} tee /opt/gpu-cluster/sync_registered_users.sh >/dev/null <<'EOF_SYNC'
#!/bin/bash
set -euo pipefail

CONF="/etc/gpu-cluster/ssh_guard.conf"
if [[ -f "${CONF}" ]]; then
  # shellcheck disable=SC1090
  source "${CONF}"
fi

CONTROLLER_URL="${CONTROLLER_URL:-}"
NODE_ID="${NODE_ID:-}"
AGENT_TOKEN="${AGENT_TOKEN:-}"
ALLOWLIST_FILE="${ALLOWLIST_FILE:-/var/lib/gpu-cluster/registered_users.txt}"
DENYLIST_FILE="${DENYLIST_FILE:-/var/lib/gpu-cluster/blocked_users.txt}"
EXEMPT_FILE="${EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}"
GUARD_STATE_FILE="${GUARD_STATE_FILE:-/var/lib/gpu-cluster/guard_state.env}"

if [[ -z "${CONTROLLER_URL}" || -z "${NODE_ID}" ]]; then
  echo "missing CONTROLLER_URL/NODE_ID" >&2
  exit 2
fi

tmp="${ALLOWLIST_FILE}.tmp"
tmp_deny="${DENYLIST_FILE}.tmp"
tmp_exempt="${EXEMPT_FILE}.tmp"
tmp_state="${GUARD_STATE_FILE}.tmp"
mkdir -p "$(dirname "${ALLOWLIST_FILE}")"
mkdir -p "$(dirname "${DENYLIST_FILE}")"
mkdir -p "$(dirname "${EXEMPT_FILE}")"
mkdir -p "$(dirname "${GUARD_STATE_FILE}")"
curl_auth=()
if [[ -n "${AGENT_TOKEN}" ]]; then
  curl_auth=(-H "X-Agent-Token: ${AGENT_TOKEN}")
fi
if curl -fsS "${curl_auth[@]}" "${CONTROLLER_URL}/api/registry/nodes/${NODE_ID}/users.txt" -o "${tmp}"; then
  mv "${tmp}" "${ALLOWLIST_FILE}"
  chmod 0644 "${ALLOWLIST_FILE}"
else
  rm -f "${tmp}" || true
fi
if curl -fsS "${curl_auth[@]}" "${CONTROLLER_URL}/api/registry/nodes/${NODE_ID}/blocked.txt" -o "${tmp_deny}"; then
  mv "${tmp_deny}" "${DENYLIST_FILE}"
  chmod 0644 "${DENYLIST_FILE}"
else
  rm -f "${tmp_deny}" || true
fi
if curl -fsS "${curl_auth[@]}" "${CONTROLLER_URL}/api/registry/nodes/${NODE_ID}/exempt.txt" -o "${tmp_exempt}"; then
  mv "${tmp_exempt}" "${EXEMPT_FILE}"
  chmod 0644 "${EXEMPT_FILE}"
else
  rm -f "${tmp_exempt}" || true
fi

state_json="$(curl -fsS "${curl_auth[@]}" "${CONTROLLER_URL}/api/registry/nodes/${NODE_ID}/guard-state" 2>/dev/null || true)"
guard_enabled="0"
exclusive_enabled="0"
if [[ -n "${state_json}" ]]; then
  if echo "${state_json}" | grep -q '"guard_enabled":[[:space:]]*true'; then
    guard_enabled="1"
  fi
  if echo "${state_json}" | grep -q '"exclusive_enabled":[[:space:]]*true'; then
    exclusive_enabled="1"
  fi
fi
cat > "${tmp_state}" <<EOF_STATE
GUARD_ENABLED="${guard_enabled}"
EXCLUSIVE_ENABLED="${exclusive_enabled}"
UPDATED_AT="$(date '+%F %T')"
EOF_STATE
mv "${tmp_state}" "${GUARD_STATE_FILE}"
chmod 0644 "${GUARD_STATE_FILE}"
EOF_SYNC
  ${SUDO} chmod +x /opt/gpu-cluster/sync_registered_users.sh

  ${SUDO} tee /opt/gpu-cluster/ssh_login_check.sh >/dev/null <<'EOF_CHECK'
#!/bin/bash
set -euo pipefail

CONF="/etc/gpu-cluster/ssh_guard.conf"
if [[ -f "${CONF}" ]]; then
  # shellcheck disable=SC1090
  source "${CONF}"
fi

user="${PAM_USER:-}"
if [[ -z "${user}" ]]; then
  exit 0
fi

LOG_FILE="${LOG_FILE:-/var/log/gpu-ssh-guard.log}"
log() {
  printf '%s user=%s node=%s msg=%s\n' "$(date '+%F %T')" "${user:-}" "${NODE_ID:-}" "$1" >> "${LOG_FILE}" 2>/dev/null || true
}

EXCLUDE_USERS="${EXCLUDE_USERS:-root}"
EXEMPT_FILE="${EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}"
if [[ -f "${EXEMPT_FILE}" ]] && grep -Fxq "${user}" "${EXEMPT_FILE}"; then
  exit 0
fi
for u in ${EXCLUDE_USERS}; do
  if [[ "${user}" == "${u}" ]]; then
    exit 0
  fi
done

CONTROLLER_URL="${CONTROLLER_URL:-}"
NODE_ID="${NODE_ID:-}"
AGENT_TOKEN="${AGENT_TOKEN:-}"
FAIL_OPEN="${FAIL_OPEN:-0}"
ALLOWLIST_FILE="${ALLOWLIST_FILE:-/var/lib/gpu-cluster/registered_users.txt}"
DENYLIST_FILE="${DENYLIST_FILE:-/var/lib/gpu-cluster/blocked_users.txt}"
EXEMPT_FILE="${EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}"
GUARD_STATE_FILE="${GUARD_STATE_FILE:-/var/lib/gpu-cluster/guard_state.env}"
REALTIME_LOOKUP="${REALTIME_LOOKUP:-0}"

if [[ -z "${NODE_ID}" ]]; then
  exit 1
fi

curl_auth=()
if [[ -n "${AGENT_TOKEN}" ]]; then
  curl_auth=(-H "X-Agent-Token: ${AGENT_TOKEN}")
fi

GUARD_ENABLED="0"
EXCLUSIVE_ENABLED="0"
if [[ -f "${GUARD_STATE_FILE}" ]]; then
  # shellcheck disable=SC1090
  source "${GUARD_STATE_FILE}" || true
fi

if [[ -f "${EXEMPT_FILE}" ]]; then
  if grep -Fxq "${user}" "${EXEMPT_FILE}"; then
    log "exempt_allow_fallback"
    exit 0
  fi
fi
if [[ -f "${DENYLIST_FILE}" ]]; then
  if grep -Fxq "${user}" "${DENYLIST_FILE}"; then
    log "denylist_deny_fallback"
    exit 1
  fi
fi

# 拦截关闭且未开启独享时，不做未注册拦截，避免误踢与登录卡顿。
if [[ "${GUARD_ENABLED}" != "1" && "${EXCLUSIVE_ENABLED}" != "1" ]]; then
  log "guard_off_allow"
  exit 0
fi

if [[ -f "${ALLOWLIST_FILE}" ]]; then
  if grep -Fxq "${user}" "${ALLOWLIST_FILE}"; then
    log "allowlist_allow_fallback"
    exit 0
  fi
fi

resp=""
if [[ "${REALTIME_LOOKUP}" == "1" && -n "${CONTROLLER_URL}" ]]; then
  # 可选：本地缓存未命中时再实时校验。默认关闭，避免登录路径被网络抖动拖慢。
  resp="$(curl -fsS --max-time 1 "${curl_auth[@]}" "${CONTROLLER_URL}/api/registry/resolve?node_id=${NODE_ID}&local_username=${user}" 2>/dev/null || true)"
  if echo "${resp}" | grep -q '"registered":true'; then
    log "registry_allow_realtime"
    exit 0
  fi
  if [[ -n "${resp}" ]]; then
    log "registry_deny_realtime"
    exit 1
  fi
fi

if [[ "${FAIL_OPEN}" == "1" ]]; then
  log "fail_open_allow"
  exit 0
fi
log "fail_close_deny_or_allowlist_miss"
exit 1
EOF_CHECK
  ${SUDO} chmod +x /opt/gpu-cluster/ssh_login_check.sh
  ${SUDO} touch /var/log/gpu-ssh-guard.log || true
  ${SUDO} chmod 0644 /var/log/gpu-ssh-guard.log || true

  ${SUDO} tee /usr/local/bin/gpuops-claim >/dev/null <<'EOF_CLAIM'
#!/bin/bash
set -euo pipefail

PUBLIC_CONF="/etc/gpu-cluster/public.env"
if [[ -r "${PUBLIC_CONF}" ]]; then
  # shellcheck disable=SC1090
  source "${PUBLIC_CONF}"
fi

TOKEN="${1:-}"
if [[ -z "${TOKEN}" ]]; then
  echo "用法：gpuops-claim <challenge_token>" >&2
  exit 2
fi

CONTROLLER_URL="${CONTROLLER_URL:-}"
NODE_ID="${NODE_ID:-}"
LOCAL_USER="$(id -un 2>/dev/null || whoami)"

if [[ -z "${CONTROLLER_URL}" || -z "${NODE_ID}" || -z "${LOCAL_USER}" ]]; then
  echo "缺少 CONTROLLER_URL/NODE_ID/LOCAL_USER，无法提交 challenge" >&2
  exit 2
fi

api="${CONTROLLER_URL%/}/api/registry/bind-claim"
payload="$(printf '{\"token\":\"%s\",\"node_id\":\"%s\",\"local_username\":\"%s\"}' "${TOKEN}" "${NODE_ID}" "${LOCAL_USER}")"
tmp_resp="$(mktemp)"
trap 'rm -f "${tmp_resp}"' EXIT
http_code="$(curl -sS -o "${tmp_resp}" -w '%{http_code}' -H "Content-Type: application/json" --data "${payload}" "${api}")"
resp="$(cat "${tmp_resp}")"
if [[ ! "${http_code}" =~ ^2[0-9][0-9]$ ]]; then
  if [[ -n "${resp}" ]]; then
    echo "${resp}" >&2
  fi
  echo "gpuops-claim 失败：http ${http_code}" >&2
  exit 1
fi
echo "${resp}"
msg="$(echo "${resp}" | sed -n 's/.*"message":"\([^"]*\)".*/\1/p')"
wait_s="$(echo "${resp}" | sed -n 's/.*"estimated_wait_seconds":\([0-9][0-9]*\).*/\1/p')"
if [[ -n "${msg}" ]]; then
  echo "提示：${msg}"
fi
if [[ -n "${wait_s}" ]]; then
  echo "建议等待：${wait_s} 秒"
fi
echo
EOF_CLAIM
  ${SUDO} chmod 0755 /usr/local/bin/gpuops-claim

  ${SUDO} tee /opt/gpu-cluster/enforce_ssh_sessions.sh >/dev/null <<'EOF_ENFORCE'
#!/bin/bash
set -euo pipefail

CONF="/etc/gpu-cluster/ssh_guard.conf"
if [[ -f "${CONF}" ]]; then
  # shellcheck disable=SC1090
  source "${CONF}"
fi

CONTROLLER_URL="${CONTROLLER_URL:-}"
NODE_ID="${NODE_ID:-}"
AGENT_TOKEN="${AGENT_TOKEN:-}"
FAIL_OPEN="${FAIL_OPEN:-0}"
EXCLUDE_USERS="${EXCLUDE_USERS:-root}"
ALLOWLIST_FILE="${ALLOWLIST_FILE:-/var/lib/gpu-cluster/registered_users.txt}"
DENYLIST_FILE="${DENYLIST_FILE:-/var/lib/gpu-cluster/blocked_users.txt}"
EXEMPT_FILE="${EXEMPT_FILE:-/var/lib/gpu-cluster/exempt_users.txt}"
GUARD_STATE_FILE="${GUARD_STATE_FILE:-/var/lib/gpu-cluster/guard_state.env}"
REALTIME_LOOKUP="${REALTIME_LOOKUP:-0}"

if [[ -z "${NODE_ID}" ]]; then
  exit 0
fi

curl_auth=()
if [[ -n "${AGENT_TOKEN}" ]]; then
  curl_auth=(-H "X-Agent-Token: ${AGENT_TOKEN}")
fi

is_excluded() {
  local u="$1"
  if [[ -f "${EXEMPT_FILE}" ]] && grep -Fxq "${u}" "${EXEMPT_FILE}"; then
    return 0
  fi
  for x in ${EXCLUDE_USERS}; do
    if [[ "${u}" == "${x}" ]]; then
      return 0
    fi
  done
  return 1
}

check_allowed() {
  local u="$1"

  if [[ -f "${EXEMPT_FILE}" ]] && grep -Fxq "${u}" "${EXEMPT_FILE}"; then
    return 0
  fi
  if [[ -f "${DENYLIST_FILE}" ]] && grep -Fxq "${u}" "${DENYLIST_FILE}"; then
    return 1
  fi
  if [[ "${GUARD_ENABLED_CURRENT}" != "1" && "${EXCLUSIVE_ENABLED_CURRENT}" != "1" ]]; then
    return 0
  fi
  if [[ -f "${ALLOWLIST_FILE}" ]] && grep -Fxq "${u}" "${ALLOWLIST_FILE}"; then
    return 0
  fi

  if [[ "${REALTIME_LOOKUP}" == "1" && -n "${CONTROLLER_URL}" ]]; then
    local resp
    resp="$(curl -fsS --max-time 1 "${curl_auth[@]}" "${CONTROLLER_URL}/api/registry/resolve?node_id=${NODE_ID}&local_username=${u}" 2>/dev/null || true)"
    if [[ -n "${resp}" ]]; then
      if echo "${resp}" | grep -q '"registered":true'; then
        return 0
      fi
      return 1
    fi
  fi
  if [[ "${FAIL_OPEN}" == "1" ]]; then
    return 0
  fi
  return 1
}

GUARD_ENABLED_CURRENT="0"
EXCLUSIVE_ENABLED_CURRENT="0"
if [[ -f "${GUARD_STATE_FILE}" ]]; then
  # shellcheck disable=SC1090
  source "${GUARD_STATE_FILE}" || true
  GUARD_ENABLED_CURRENT="${GUARD_ENABLED:-0}"
  EXCLUSIVE_ENABLED_CURRENT="${EXCLUSIVE_ENABLED:-0}"
fi

# 拦截与独享都关闭时，仅维持黑名单踢人；若黑名单为空则直接退出，避免无意义巡检。
if [[ "${GUARD_ENABLED_CURRENT}" != "1" && "${EXCLUSIVE_ENABLED_CURRENT}" != "1" ]]; then
  if [[ ! -f "${DENYLIST_FILE}" ]] || [[ ! -s "${DENYLIST_FILE}" ]]; then
    exit 0
  fi
fi

while read -r user tty _; do
  user="$(echo "${user}" | xargs)"
  tty="$(echo "${tty}" | xargs)"
  if [[ -z "${user}" || -z "${tty}" ]]; then
    continue
  fi
  if is_excluded "${user}"; then
    continue
  fi
  if ! check_allowed "${user}"; then
    pkill -KILL -t "${tty}" >/dev/null 2>&1 || true
    pkill -KILL -f "^sshd: ${user}@" >/dev/null 2>&1 || true
  fi
done < <(who)
EOF_ENFORCE
  ${SUDO} chmod +x /opt/gpu-cluster/enforce_ssh_sessions.sh

  ${SUDO} tee /etc/systemd/system/gpu-ssh-guard-sync.service >/dev/null <<'EOF_SYNC_SVC'
[Unit]
Description=GPU SSH Guard List Sync
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/opt/gpu-cluster/sync_registered_users.sh
EOF_SYNC_SVC

  ${SUDO} tee /etc/systemd/system/gpu-ssh-guard-sync.timer >/dev/null <<EOF_SYNC_TIMER
[Unit]
Description=GPU SSH Guard List Sync Timer

[Timer]
OnBootSec=30
OnUnitActiveSec=${SSH_GUARD_SYNC_INTERVAL}
Unit=gpu-ssh-guard-sync.service

[Install]
WantedBy=timers.target
EOF_SYNC_TIMER

  ${SUDO} tee /etc/systemd/system/gpu-ssh-guard-enforce.service >/dev/null <<'EOF_ENFORCE_SVC'
[Unit]
Description=GPU SSH Guard Session Enforcer
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/opt/gpu-cluster/enforce_ssh_sessions.sh
EOF_ENFORCE_SVC

  ${SUDO} tee /etc/systemd/system/gpu-ssh-guard-enforce.timer >/dev/null <<EOF_ENFORCE_TIMER
[Unit]
Description=GPU SSH Guard Session Enforcer Timer

[Timer]
OnBootSec=40
OnUnitActiveSec=${SSH_GUARD_ENFORCE_INTERVAL}
Unit=gpu-ssh-guard-enforce.service

[Install]
WantedBy=timers.target
EOF_ENFORCE_TIMER

  if [[ -f /etc/pam.d/sshd ]]; then
    # 先移除旧规则（避免重复/位置错误），再插入到 common-account 前面，防止被 sufficient 规则短路。
    ${SUDO} sed -i '\#ssh_login_check.sh#d' /etc/pam.d/sshd
    if grep -q '^@include common-account' /etc/pam.d/sshd; then
      ${SUDO} sed -i '/^@include common-account/i account requisite pam_exec.so /opt/gpu-cluster/ssh_login_check.sh' /etc/pam.d/sshd
    else
      echo "account requisite pam_exec.so /opt/gpu-cluster/ssh_login_check.sh" | ${SUDO} tee -a /etc/pam.d/sshd >/dev/null
    fi
  fi

  # 确保 sshd 启用 PAM，否则 pam_exec 不生效
  if [[ -f /etc/ssh/sshd_config ]]; then
    if grep -Eq '^[[:space:]]*UsePAM[[:space:]]+no' /etc/ssh/sshd_config; then
      ${SUDO} sed -i 's/^[[:space:]]*UsePAM[[:space:]]\\+no/UsePAM yes/g' /etc/ssh/sshd_config
    elif ! grep -Eq '^[[:space:]]*UsePAM[[:space:]]+yes' /etc/ssh/sshd_config; then
      echo 'UsePAM yes' | ${SUDO} tee -a /etc/ssh/sshd_config >/dev/null
    fi
    # 关闭 DNS 反查与 GSSAPI，可显著降低 SSH 登录阶段卡顿。
    if grep -Eq '^[[:space:]]*UseDNS[[:space:]]+' /etc/ssh/sshd_config; then
      ${SUDO} sed -i 's/^[[:space:]]*UseDNS[[:space:]].*/UseDNS no/g' /etc/ssh/sshd_config
    else
      echo 'UseDNS no' | ${SUDO} tee -a /etc/ssh/sshd_config >/dev/null
    fi
    if grep -Eq '^[[:space:]]*GSSAPIAuthentication[[:space:]]+' /etc/ssh/sshd_config; then
      ${SUDO} sed -i 's/^[[:space:]]*GSSAPIAuthentication[[:space:]].*/GSSAPIAuthentication no/g' /etc/ssh/sshd_config
    else
      echo 'GSSAPIAuthentication no' | ${SUDO} tee -a /etc/ssh/sshd_config >/dev/null
    fi
  fi

  ${SUDO} systemctl daemon-reload
  ${SUDO} systemctl enable --now gpu-ssh-guard-sync.timer
  ${SUDO} systemctl enable --now gpu-ssh-guard-enforce.timer
  ${SUDO} systemctl start gpu-ssh-guard-sync.service || true
  ${SUDO} systemctl start gpu-ssh-guard-enforce.service || true

  # 某些节点的 /run/sshd 未由 tmpfiles 创建；直接重启会先停掉旧 sshd，
  # 随后因配置或运行目录问题启动失败，导致远程节点失联。
  # 必须先补齐运行目录并校验配置，校验失败时保留当前 SSH 服务。
  ${SUDO} install -d -o root -g root -m 0755 /run/sshd
  if [[ -x /usr/sbin/sshd ]] && ! ${SUDO} /usr/sbin/sshd -t; then
    echo "警告：sshd 配置校验失败，跳过 SSH 服务重启以避免远程失联" >&2
  else
    ${SUDO} systemctl restart ssh || ${SUDO} systemctl restart sshd || true
  fi
fi

if [[ "${ENABLE_HOST_SECURITY}" == "1" ]]; then
  echo "[11/11] 安装主机安全基线（fail2ban + CPU 预留）"
  if command -v apt-get >/dev/null 2>&1; then
    ${SUDO} apt-get update -y >/dev/null 2>&1 || true
    ${SUDO} apt-get install -y fail2ban >/dev/null 2>&1 || true
  fi

  if command -v fail2ban-client >/dev/null 2>&1; then
    ${SUDO} mkdir -p /etc/fail2ban/jail.d
    ${SUDO} tee /etc/fail2ban/jail.d/gpu-ssh.local >/dev/null <<EOF_FAIL2BAN
[sshd]
enabled = true
backend = systemd
port = ssh
maxretry = ${SSH_FAIL2BAN_MAXRETRY}
findtime = ${SSH_FAIL2BAN_FINDTIME}
bantime = ${SSH_FAIL2BAN_BANTIME}
ignoreip = ${SSH_FAIL2BAN_IGNOREIP}

[recidive]
enabled = true
backend = systemd
logpath = /var/log/fail2ban.log
maxretry = 5
findtime = 1d
bantime = 7d
EOF_FAIL2BAN
    ${SUDO} systemctl enable --now fail2ban || true
    ${SUDO} systemctl restart fail2ban || true
  else
    echo "警告：未安装 fail2ban，跳过 SSH 防爆破基线"
  fi

  # 兼容旧版文件名，避免和新拆分的 CPU/内存配置冲突。
  ${SUDO} rm -f /etc/systemd/system/user.slice.d/20-gpu-reserve.conf || true

  if [[ "${ENABLE_SYSTEM_CPU_RESERVE}" == "1" ]]; then
    cpu_cnt="$(nproc 2>/dev/null || echo 1)"
    if [[ ! "${cpu_cnt}" =~ ^[0-9]+$ || "${cpu_cnt}" -le 0 ]]; then
      cpu_cnt=1
    fi
    reserve_pct="${SYSTEM_CPU_RESERVE_PERCENT}"
    if [[ ! "${reserve_pct}" =~ ^[0-9]+$ || "${reserve_pct}" -lt 0 || "${reserve_pct}" -gt 99 ]]; then
      reserve_pct=5
    fi
    user_slice_pct=$((100 - reserve_pct))
    quota_pct=$((cpu_cnt * user_slice_pct))
    ${SUDO} mkdir -p /etc/systemd/system/user.slice.d
    ${SUDO} tee /etc/systemd/system/user.slice.d/20-gpu-cpu-reserve.conf >/dev/null <<EOF_USER_SLICE_CPU
[Slice]
CPUAccounting=true
CPUQuota=${quota_pct}%
EOF_USER_SLICE_CPU
    ${SUDO} systemctl daemon-reload || true
    ${SUDO} systemctl set-property --runtime user.slice "CPUQuota=${quota_pct}%" >/dev/null 2>&1 || true
    echo "已设置 user.slice CPUQuota=${quota_pct}%（约保留 ${reserve_pct}% CPU 给系统）"
  else
    # 默认不限制 user.slice，避免普通 SSH 会话出现明显卡顿。
    ${SUDO} rm -f /etc/systemd/system/user.slice.d/20-gpu-cpu-reserve.conf || true
    ${SUDO} systemctl daemon-reload || true
    ${SUDO} systemctl set-property --runtime user.slice "CPUQuota=infinity" >/dev/null 2>&1 || true
    echo "已关闭系统 CPU 预留（恢复为不限制）"
  fi

  if [[ "${ENABLE_SYSTEM_MEMORY_RESERVE}" == "1" ]]; then
    reserve_gb_raw="${SYSTEM_MEMORY_RESERVE_GB}"
    if [[ ! "${reserve_gb_raw}" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
      reserve_gb_raw="8"
    fi

    total_mem_kb="$(awk '/MemTotal:/ {print $2}' /proc/meminfo 2>/dev/null || true)"
    if [[ ! "${total_mem_kb}" =~ ^[0-9]+$ || "${total_mem_kb}" -le 0 ]]; then
      echo "警告：读取总内存失败，跳过 user.slice 内存预留设置"
    else
      reserve_bytes="$(awk -v gb="${reserve_gb_raw}" 'BEGIN{printf "%.0f", gb*1024*1024*1024}')"
      total_bytes="$((total_mem_kb * 1024))"
      memory_max_bytes="$((total_bytes - reserve_bytes))"
      min_bytes=$((512 * 1024 * 1024))
      if (( memory_max_bytes < min_bytes )); then
        memory_max_bytes="${min_bytes}"
      fi

      ${SUDO} mkdir -p /etc/systemd/system/user.slice.d
      ${SUDO} tee /etc/systemd/system/user.slice.d/30-gpu-memory-reserve.conf >/dev/null <<EOF_USER_SLICE_MEM
[Slice]
MemoryAccounting=true
MemoryMax=${memory_max_bytes}
EOF_USER_SLICE_MEM

      ${SUDO} systemctl daemon-reload || true
      ${SUDO} systemctl set-property --runtime user.slice "MemoryMax=${memory_max_bytes}" >/dev/null 2>&1 || true

      memory_max_gb="$(awk -v b="${memory_max_bytes}" 'BEGIN{printf "%.2f", b/1024/1024/1024}')"
      total_mem_gb="$(awk -v kb="${total_mem_kb}" 'BEGIN{printf "%.2f", kb/1024/1024}')"
      echo "已设置 user.slice MemoryMax=${memory_max_gb}G（总内存约 ${total_mem_gb}G，系统保留 ${reserve_gb_raw}G）"
    fi
  else
    ${SUDO} rm -f /etc/systemd/system/user.slice.d/30-gpu-memory-reserve.conf || true
    ${SUDO} systemctl daemon-reload || true
    ${SUDO} systemctl set-property --runtime user.slice "MemoryMax=infinity" >/dev/null 2>&1 || true
    echo "已关闭系统内存预留（恢复为不限制）"
  fi
fi

echo

harden_project_workspace

configure_shared_nfs

echo "部署完成。"
