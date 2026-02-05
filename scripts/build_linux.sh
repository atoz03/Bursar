#!/bin/bash
# 构建 Linux 可部署二进制（建议在 CI 或任意有 Go 的机器执行）

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-${ROOT_DIR}/bin}"

mkdir -p "${OUT_DIR}"

GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"

echo "==> 构建 controller (${GOOS}/${GOARCH})"
(cd "${ROOT_DIR}/controller" && GOOS="${GOOS}" GOARCH="${GOARCH}" go build -o "${OUT_DIR}/controller" .)

echo "==> 构建 node-agent (${GOOS}/${GOARCH})"
(cd "${ROOT_DIR}/node-agent" && GOOS="${GOOS}" GOARCH="${GOARCH}" go build -o "${OUT_DIR}/node-agent" .)

echo "输出目录：${OUT_DIR}"

