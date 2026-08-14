#!/usr/bin/env bash
# 在备机上拉取主控发布产物，确保 controller 二进制与前端 dist 与主控一致。
set -euo pipefail

PRIMARY_HOST="${PRIMARY_HOST:-}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REMOTE_BIN="${REMOTE_BIN:-/usr/local/bin/gpu-controller}"
LOCAL_BIN="${LOCAL_BIN:-/usr/local/bin/gpu-controller}"
SYNC_WEB_DIST="${SYNC_WEB_DIST:-1}"
REMOTE_WEB_DIST="${REMOTE_WEB_DIST:-/opt/gpu-ops/web/dist/}"
LOCAL_WEB_DIST="${LOCAL_WEB_DIST:-${ROOT_DIR}/web/dist/}"
RESTART_SERVICE="${RESTART_SERVICE:-1}"
SERVICE_NAME="${SERVICE_NAME:-gpu-controller}"

if [[ -z "${PRIMARY_HOST}" ]]; then
  echo "PRIMARY_HOST 不能为空（示例：PRIMARY_HOST=gpuops@192.0.2.10）" >&2
  exit 2
fi

for cmd in ssh scp sha256sum; do
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "缺少命令：${cmd}" >&2
    exit 2
  fi
done

SUDO=""
if [[ "$(id -u)" -ne 0 ]]; then
  SUDO="sudo"
fi

TMP_BIN="$(mktemp /tmp/gpu-controller.primary.XXXXXX)"
trap 'rm -f "${TMP_BIN}"' EXIT

echo "[1/4] 拉取主控二进制：${PRIMARY_HOST}:${REMOTE_BIN}"
scp -q "${PRIMARY_HOST}:${REMOTE_BIN}" "${TMP_BIN}"

echo "[2/4] 安装到本机：${LOCAL_BIN}"
${SUDO} install -m 0755 "${TMP_BIN}" "${LOCAL_BIN}"

REMOTE_HASH="$(ssh -o BatchMode=yes "${PRIMARY_HOST}" "sha256sum '${REMOTE_BIN}' | awk '{print \\$1}'")"
LOCAL_HASH="$(sha256sum "${LOCAL_BIN}" | awk '{print $1}')"
if [[ "${REMOTE_HASH}" != "${LOCAL_HASH}" ]]; then
  echo "二进制校验失败：remote=${REMOTE_HASH} local=${LOCAL_HASH}" >&2
  exit 3
fi

if [[ "${SYNC_WEB_DIST}" == "1" ]]; then
  echo "[3/4] 同步前端 dist"
  if command -v rsync >/dev/null 2>&1; then
    mkdir -p "${LOCAL_WEB_DIST}"
    rsync -az --delete "${PRIMARY_HOST}:${REMOTE_WEB_DIST}" "${LOCAL_WEB_DIST}"
  else
    echo "未安装 rsync，跳过 dist 同步（可安装后重试）"
  fi
else
  echo "[3/4] 跳过前端 dist 同步（SYNC_WEB_DIST=${SYNC_WEB_DIST}）"
fi

if [[ "${RESTART_SERVICE}" == "1" ]]; then
  echo "[4/4] 重启服务：${SERVICE_NAME}"
  ${SUDO} systemctl restart "${SERVICE_NAME}"
  ${SUDO} systemctl --no-pager --full status "${SERVICE_NAME}" || true
else
  echo "[4/4] 跳过重启服务（RESTART_SERVICE=${RESTART_SERVICE}）"
fi

REMOTE_VER="$(ssh -o BatchMode=yes "${PRIMARY_HOST}" "'${REMOTE_BIN}' --version 2>/dev/null || true")"
LOCAL_VER="$("${LOCAL_BIN}" --version 2>/dev/null || true)"

echo "完成：主备二进制已对齐。"
echo "remote_sha256=${REMOTE_HASH}"
echo "local_sha256=${LOCAL_HASH}"
echo "remote_version=${REMOTE_VER}"
echo "local_version=${LOCAL_VER}"
