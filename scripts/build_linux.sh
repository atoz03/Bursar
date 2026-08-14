#!/bin/bash
# 构建 Linux 可部署二进制（建议在 CI 或任意有 Go 的机器执行）

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-${ROOT_DIR}/bin}"

mkdir -p "${OUT_DIR}"

GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
BUILD_AT="$(date -u '+%Y%m%dT%H%M%SZ')"
GIT_COMMIT="$(git -c safe.directory="${ROOT_DIR}" -C "${ROOT_DIR}" rev-parse --short=12 HEAD 2>/dev/null || true)"
GIT_DIRTY=""
if [[ -n "$(git -c safe.directory="${ROOT_DIR}" -C "${ROOT_DIR}" status --porcelain --untracked-files=no 2>/dev/null || true)" ]]; then
  GIT_DIRTY="true"
fi

agent_ldflags="-X main.agentBuildAt=${BUILD_AT}"
if [[ -n "${GIT_COMMIT}" ]]; then
  agent_ldflags="${agent_ldflags} -X main.agentCommit=${GIT_COMMIT}"
fi
if [[ -n "${GIT_DIRTY}" ]]; then
  agent_ldflags="${agent_ldflags} -X main.agentVCSModified=${GIT_DIRTY}"
fi

echo "==> 构建 controller (${GOOS}/${GOARCH})"
(cd "${ROOT_DIR}/controller" && GOOS="${GOOS}" GOARCH="${GOARCH}" go build -buildvcs=false -o "${OUT_DIR}/controller" .)

echo "==> 构建 node-agent (${GOOS}/${GOARCH})"
(cd "${ROOT_DIR}/node-agent" && GOOS="${GOOS}" GOARCH="${GOARCH}" go build -buildvcs=false -ldflags "${agent_ldflags}" -o "${OUT_DIR}/node-agent" .)

echo "输出目录：${OUT_DIR}"
