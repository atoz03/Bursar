#!/usr/bin/env bash
# 一键安装本项目依赖（Ubuntu 22.04）
# - 默认切换 apt 为清华源
# - 安装基础依赖、Go、Node.js、pnpm、Docker
# - 可重复执行（幂等）

set -euo pipefail

GO_VERSION="${GO_VERSION:-1.22.5}"
NODE_MAJOR="${NODE_MAJOR:-20}"
PNPM_VERSION="${PNPM_VERSION:-10.28.2}"
USE_TUNA_APT="${USE_TUNA_APT:-1}"
INSTALL_DOCKER="${INSTALL_DOCKER:-1}"
INSTALL_GO="${INSTALL_GO:-1}"
INSTALL_NODE="${INSTALL_NODE:-1}"

if [[ "$(id -u)" -ne 0 ]]; then
  SUDO="sudo"
else
  SUDO=""
fi

run_root_env_bash() {
  if [[ -n "${SUDO}" ]]; then
    ${SUDO} -E bash -
  else
    bash -
  fi
}

require_ubuntu_2204() {
  if [[ ! -f /etc/os-release ]]; then
    echo "无法识别系统：缺少 /etc/os-release" >&2
    exit 2
  fi
  # shellcheck disable=SC1091
  source /etc/os-release
  if [[ "${ID:-}" != "ubuntu" || "${VERSION_ID:-}" != "22.04" ]]; then
    echo "当前系统是 ${PRETTY_NAME:-unknown}，本脚本仅针对 Ubuntu 22.04 测试。" >&2
    echo "如仍要继续，请设置 SKIP_OS_CHECK=1" >&2
    if [[ "${SKIP_OS_CHECK:-0}" != "1" ]]; then
      exit 2
    fi
  fi
}

setup_tuna_apt() {
  if [[ "${USE_TUNA_APT}" != "1" ]]; then
    return 0
  fi
  echo "[1/6] 配置 apt 清华源..."
  ${SUDO} cp /etc/apt/sources.list "/etc/apt/sources.list.bak.$(date +%s)"
  ${SUDO} tee /etc/apt/sources.list >/dev/null <<'APT'
deb https://mirrors.tuna.tsinghua.edu.cn/ubuntu/ jammy main restricted universe multiverse
deb https://mirrors.tuna.tsinghua.edu.cn/ubuntu/ jammy-updates main restricted universe multiverse
deb https://mirrors.tuna.tsinghua.edu.cn/ubuntu/ jammy-backports main restricted universe multiverse
deb https://mirrors.tuna.tsinghua.edu.cn/ubuntu/ jammy-security main restricted universe multiverse
APT
}

install_base_packages() {
  echo "[2/6] 安装基础依赖..."
  ${SUDO} apt-get update
  ${SUDO} apt-get install -y --no-install-recommends \
    ca-certificates curl wget git jq build-essential software-properties-common gnupg lsb-release
}

install_go() {
  if [[ "${INSTALL_GO}" != "1" ]]; then
    echo "[3/6] 跳过 Go 安装"
    return 0
  fi
  if command -v go >/dev/null 2>&1; then
    current="$(go version | awk '{print $3}' | sed 's/^go//')"
    if [[ "${current}" == "${GO_VERSION}" ]]; then
      echo "[3/6] Go ${GO_VERSION} 已安装，跳过"
      return 0
    fi
  fi

  echo "[3/6] 安装 Go ${GO_VERSION}..."
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' EXIT
  cd "${tmpdir}"
  wget -O go.tgz "https://mirrors.tuna.tsinghua.edu.cn/golang/go${GO_VERSION}.linux-amd64.tar.gz" \
    || wget -O go.tgz "https://mirrors.aliyun.com/golang/go${GO_VERSION}.linux-amd64.tar.gz" \
    || wget -O go.tgz "https://mirrors.cloud.tencent.com/golang/go${GO_VERSION}.linux-amd64.tar.gz"
  ${SUDO} rm -rf /usr/local/go
  ${SUDO} tar -C /usr/local -xzf go.tgz
  echo 'export PATH=/usr/local/go/bin:$PATH' | ${SUDO} tee /etc/profile.d/go.sh >/dev/null
  if ! grep -q '/usr/local/go/bin' "${HOME}/.bashrc"; then
    echo 'export PATH=/usr/local/go/bin:$PATH' >> "${HOME}/.bashrc"
  fi
  export PATH=/usr/local/go/bin:$PATH
  hash -r
  go version
  go env -w GOPROXY=https://goproxy.cn,direct
  go env -w GOSUMDB=off
}

install_node_and_pnpm() {
  if [[ "${INSTALL_NODE}" != "1" ]]; then
    echo "[4/6] 跳过 Node/pnpm 安装"
    return 0
  fi
  echo "[4/6] 安装 Node.js ${NODE_MAJOR}.x 与 pnpm ${PNPM_VERSION}..."
  curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | run_root_env_bash
  ${SUDO} apt-get install -y nodejs
  ${SUDO} corepack enable
  corepack prepare "pnpm@${PNPM_VERSION}" --activate
  node -v
  pnpm -v
  pnpm config set registry https://registry.npmmirror.com
}

install_docker() {
  if [[ "${INSTALL_DOCKER}" != "1" ]]; then
    echo "[5/6] 跳过 Docker 安装"
    return 0
  fi
  echo "[5/6] 安装 Docker..."
  ${SUDO} apt-get install -y docker.io
  if ${SUDO} apt-get install -y docker-compose-plugin; then
    :
  else
    echo "docker-compose-plugin 不可用，回退安装 docker-compose..."
    ${SUDO} apt-get install -y docker-compose || true
  fi
  ${SUDO} systemctl enable docker >/dev/null 2>&1 || true
  ${SUDO} systemctl start docker >/dev/null 2>&1 || true
  if getent group docker >/dev/null 2>&1; then
    ${SUDO} usermod -aG docker "${USER}" || true
  fi
  docker --version || true
  docker compose version 2>/dev/null || docker-compose --version 2>/dev/null || true
}

summary() {
  echo "[6/6] 完成"
  echo "Go:      $(command -v go >/dev/null 2>&1 && go version || echo 'not installed')"
  echo "Node:    $(command -v node >/dev/null 2>&1 && node -v || echo 'not installed')"
  echo "pnpm:    $(command -v pnpm >/dev/null 2>&1 && pnpm -v || echo 'not installed')"
  echo "Docker:  $(command -v docker >/dev/null 2>&1 && docker --version || echo 'not installed')"
  echo
  echo "如果你刚加入 docker 组，请重新登录 shell 后再执行 docker 命令。"
}

require_ubuntu_2204
setup_tuna_apt
install_base_packages
install_go
install_node_and_pnpm
install_docker
summary
