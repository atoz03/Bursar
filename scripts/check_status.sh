#!/bin/bash
# 系统自检（控制器 + 数据库 + 基本 API）

set -euo pipefail

CONTROLLER_URL="${CONTROLLER_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${ADMIN_TOKEN:-dev-admin-token}"

# 通过标准输入传递请求头，避免令牌出现在其他本机账号可见的进程参数中。
admin_get() {
  curl -fsS -H @- "${CONTROLLER_URL}$1" <<<"Authorization: Bearer ${ADMIN_TOKEN}"
}

echo "==> healthz"
curl -fsS "${CONTROLLER_URL}/healthz" && echo

echo "==> metrics"
curl -fsS "${CONTROLLER_URL}/metrics" | head -n 20
echo

echo "==> prices"
admin_get "/api/admin/prices" && echo

echo "==> users"
admin_get "/api/admin/users" && echo

echo "==> nodes"
admin_get "/api/admin/nodes?limit=5" && echo

echo "==> usage"
admin_get "/api/admin/usage?limit=5" && echo
