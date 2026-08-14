#!/usr/bin/env bash
# 在控制器主机安装 Keepalived + VRRP VIP，提供主备自动切换与主修复后自动回切。
set -euo pipefail

HA_ROLE_RAW="${HA_ROLE:-}"
HA_VIP="${HA_VIP:-}"
HA_PEER_IP="${HA_PEER_IP:-}"
HA_INTERFACE="${HA_INTERFACE:-}"
HA_ROUTER_ID="${HA_ROUTER_ID:-61}"
HA_AUTH_PASS="${HA_AUTH_PASS:-gpuhavip}"
PRIMARY_PRIORITY="${PRIMARY_PRIORITY:-180}"
STANDBY_PRIORITY="${STANDBY_PRIORITY:-120}"
CONTROLLER_SERVICE="${CONTROLLER_SERVICE:-gpu-controller}"
CONTROLLER_HEALTH_URL="${CONTROLLER_HEALTH_URL:-http://127.0.0.1:8080/readyz}"
CHECK_INTERVAL="${CHECK_INTERVAL:-2}"
CHECK_TIMEOUT="${CHECK_TIMEOUT:-2}"
CHECK_FALL="${CHECK_FALL:-2}"
CHECK_RISE="${CHECK_RISE:-2}"

ROLE="$(echo "${HA_ROLE_RAW}" | tr '[:upper:]' '[:lower:]' | xargs)"
if [[ "${ROLE}" != "primary" && "${ROLE}" != "standby" ]]; then
  echo "HA_ROLE 仅支持 primary/standby，当前：${HA_ROLE_RAW}" >&2
  exit 2
fi
if [[ -z "${HA_VIP}" ]]; then
  echo "HA_VIP 不能为空（示例：192.0.2.30/24）" >&2
  exit 2
fi
if [[ -z "${HA_PEER_IP}" ]]; then
  echo "HA_PEER_IP 不能为空（示例：192.0.2.20）" >&2
  exit 2
fi
if [[ "${#HA_AUTH_PASS}" -lt 1 || "${#HA_AUTH_PASS}" -gt 8 ]]; then
  echo "HA_AUTH_PASS 长度必须在 1~8 字符（VRRP PASS 限制）" >&2
  exit 2
fi

if [[ -z "${HA_INTERFACE}" ]]; then
  HA_INTERFACE="$(ip route show default 2>/dev/null | awk '{print $5; exit}')"
fi
if [[ -z "${HA_INTERFACE}" ]]; then
  echo "无法自动识别网卡，请显式设置 HA_INTERFACE" >&2
  exit 2
fi

LOCAL_IP="$(ip -4 -o addr show dev "${HA_INTERFACE}" 2>/dev/null | awk '{print $4}' | head -n1 | cut -d/ -f1)"
if [[ -z "${LOCAL_IP}" ]]; then
  echo "网卡 ${HA_INTERFACE} 未找到 IPv4 地址，请检查 HA_INTERFACE 配置" >&2
  exit 2
fi

PRIORITY="${STANDBY_PRIORITY}"
if [[ "${ROLE}" == "primary" ]]; then
  PRIORITY="${PRIMARY_PRIORITY}"
fi

SUDO=""
if [[ "$(id -u)" -ne 0 ]]; then
  SUDO="sudo"
fi

if command -v apt-get >/dev/null 2>&1; then
  ${SUDO} apt-get update -y >/dev/null 2>&1 || true
  ${SUDO} apt-get install -y keepalived curl >/dev/null
fi

CHECK_SCRIPT_PATH="/usr/local/bin/gpu-controller-ha-check.sh"
TMP_CHECK_SCRIPT="$(mktemp /tmp/gpu-controller-ha-check.XXXXXX)"
cat >"${TMP_CHECK_SCRIPT}" <<EOF_CHECK
#!/usr/bin/env bash
set -euo pipefail
systemctl is-active --quiet ${CONTROLLER_SERVICE}
curl -fsS --max-time ${CHECK_TIMEOUT} "${CONTROLLER_HEALTH_URL}" >/dev/null
EOF_CHECK
${SUDO} install -m 0755 "${TMP_CHECK_SCRIPT}" "${CHECK_SCRIPT_PATH}"
rm -f "${TMP_CHECK_SCRIPT}"

${SUDO} mkdir -p /etc/keepalived
${SUDO} tee /etc/keepalived/keepalived.conf >/dev/null <<EOF_CFG
global_defs {
  router_id GPUOPS_${ROLE}
  enable_script_security
  script_user root
}

vrrp_script chk_gpu_controller {
  script "${CHECK_SCRIPT_PATH}"
  interval ${CHECK_INTERVAL}
  timeout ${CHECK_TIMEOUT}
  fall ${CHECK_FALL}
  rise ${CHECK_RISE}
  weight -80
}

vrrp_instance VI_GPUOPS {
  state BACKUP
  interface ${HA_INTERFACE}
  virtual_router_id ${HA_ROUTER_ID}
  priority ${PRIORITY}
  advert_int 1
  authentication {
    auth_type PASS
    auth_pass ${HA_AUTH_PASS}
  }
  unicast_src_ip ${LOCAL_IP}
  unicast_peer {
    ${HA_PEER_IP}
  }
  virtual_ipaddress {
    ${HA_VIP} dev ${HA_INTERFACE} label ${HA_INTERFACE}:gpuops
  }
  track_script {
    chk_gpu_controller
  }
}
EOF_CFG

${SUDO} systemctl daemon-reload
${SUDO} systemctl enable --now keepalived
${SUDO} systemctl restart keepalived

echo "完成：Keepalived 已安装并启动。"
echo "角色=${ROLE} 本机IP=${LOCAL_IP} 对端IP=${HA_PEER_IP} VIP=${HA_VIP} 网卡=${HA_INTERFACE}"
echo "查看状态："
echo "  systemctl status keepalived --no-pager"
echo "  ip -4 addr show dev ${HA_INTERFACE} | grep -n \"${HA_VIP%%/*}\" || true"
echo "  journalctl -u keepalived -n 100 --no-pager"
