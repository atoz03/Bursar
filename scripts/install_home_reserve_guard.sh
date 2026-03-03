#!/usr/bin/env bash
# 安装 /home 预留空间保护：
# - 允许用户登录（仅提示告警）
# - 低于阈值时将非豁免用户 home 目录改为只读，防止继续写爆 /home
# - 空间恢复后自动还原原始权限
set -euo pipefail

HOME_RESERVE_GB="${HOME_RESERVE_GB:-8}"
HOME_RESERVE_CHECK_PATH="${HOME_RESERVE_CHECK_PATH:-/home}"
HOME_RESERVE_EXEMPT_USERS="${HOME_RESERVE_EXEMPT_USERS:-root gpuops}"
HOME_RESERVE_ENFORCE_INTERVAL="${HOME_RESERVE_ENFORCE_INTERVAL:-30s}"
HOME_RESERVE_STATE_DIR="${HOME_RESERVE_STATE_DIR:-/var/lib/gpu-cluster/home-reserve}"

if [[ ! "${HOME_RESERVE_GB}" =~ ^[0-9]+$ ]]; then
  echo "HOME_RESERVE_GB 必须是非负整数，当前=${HOME_RESERVE_GB}" >&2
  exit 2
fi

SUDO=""
if [[ "$(id -u)" -ne 0 ]]; then
  SUDO="sudo"
fi

echo "[home-reserve] 配置：path=${HOME_RESERVE_CHECK_PATH} reserve=${HOME_RESERVE_GB}G exempt=[${HOME_RESERVE_EXEMPT_USERS}] interval=${HOME_RESERVE_ENFORCE_INTERVAL}"
${SUDO} mkdir -p /etc/gpu-cluster /opt/gpu-cluster /var/log /var/lib/gpu-cluster

${SUDO} tee /etc/gpu-cluster/home_reserve.conf >/dev/null <<EOF_CONF
HOME_RESERVE_GB="${HOME_RESERVE_GB}"
HOME_RESERVE_CHECK_PATH="${HOME_RESERVE_CHECK_PATH}"
HOME_RESERVE_EXEMPT_USERS="${HOME_RESERVE_EXEMPT_USERS}"
HOME_RESERVE_STATE_DIR="${HOME_RESERVE_STATE_DIR}"
EOF_CONF

${SUDO} tee /opt/gpu-cluster/home_reserve_login_check.sh >/dev/null <<'EOF_LOGIN'
#!/usr/bin/env bash
set -euo pipefail

CONF="/etc/gpu-cluster/home_reserve.conf"
if [[ -f "${CONF}" ]]; then
  # shellcheck disable=SC1090
  source "${CONF}"
fi

user="${PAM_USER:-}"
[[ -z "${user}" ]] && exit 0

HOME_RESERVE_GB="${HOME_RESERVE_GB:-8}"
HOME_RESERVE_CHECK_PATH="${HOME_RESERVE_CHECK_PATH:-/home}"
HOME_RESERVE_EXEMPT_USERS="${HOME_RESERVE_EXEMPT_USERS:-root gpuops}"

if ! [[ "${HOME_RESERVE_GB}" =~ ^[0-9]+$ ]]; then
  HOME_RESERVE_GB=8
fi
if (( HOME_RESERVE_GB <= 0 )); then
  exit 0
fi

for u in ${HOME_RESERVE_EXEMPT_USERS}; do
  if [[ "${user}" == "${u}" ]]; then
    exit 0
  fi
done

avail_bytes="$(df -PB1 "${HOME_RESERVE_CHECK_PATH}" 2>/dev/null | awk 'NR==2{print $4}')"
if ! [[ "${avail_bytes}" =~ ^[0-9]+$ ]]; then
  exit 0
fi
reserve_bytes=$((HOME_RESERVE_GB * 1024 * 1024 * 1024))
if (( avail_bytes < reserve_bytes )); then
  avail_gb="$(awk -v b="${avail_bytes}" 'BEGIN{printf "%.2f", b/1024/1024/1024}')"
  echo "系统提示：${HOME_RESERVE_CHECK_PATH} 可用空间仅 ${avail_gb}G，低于保留阈值 ${HOME_RESERVE_GB}G。你仍可登录，但 home 目录将进入只读保护，禁止继续写入。" >&2
fi
exit 0
EOF_LOGIN
${SUDO} chmod +x /opt/gpu-cluster/home_reserve_login_check.sh

${SUDO} tee /opt/gpu-cluster/enforce_home_reserve_storage.sh >/dev/null <<'EOF_ENFORCE'
#!/usr/bin/env bash
set -euo pipefail

CONF="/etc/gpu-cluster/home_reserve.conf"
if [[ -f "${CONF}" ]]; then
  # shellcheck disable=SC1090
  source "${CONF}"
fi

HOME_RESERVE_GB="${HOME_RESERVE_GB:-8}"
HOME_RESERVE_CHECK_PATH="${HOME_RESERVE_CHECK_PATH:-/home}"
HOME_RESERVE_EXEMPT_USERS="${HOME_RESERVE_EXEMPT_USERS:-root gpuops}"
HOME_RESERVE_STATE_DIR="${HOME_RESERVE_STATE_DIR:-/var/lib/gpu-cluster/home-reserve}"

mkdir -p "${HOME_RESERVE_STATE_DIR}"

is_exempt() {
  local user="$1"
  for x in ${HOME_RESERVE_EXEMPT_USERS}; do
    if [[ "${user}" == "${x}" ]]; then
      return 0
    fi
  done
  return 1
}

list_home_users() {
  awk -F: '($3>=1000 && $6 ~ /^\/home\// && $7 !~ /(nologin|false)$/){print $1":"$6}' /etc/passwd
}

restore_user_mode() {
  local user="$1"
  local home="$2"
  local mode_file="${HOME_RESERVE_STATE_DIR}/${user}.mode"
  if [[ ! -f "${mode_file}" ]]; then
    return 0
  fi
  local old_mode
  old_mode="$(tr -d ' \t\r\n' < "${mode_file}" 2>/dev/null || true)"
  if [[ "${old_mode}" =~ ^[0-7]{3,4}$ && -d "${home}" ]]; then
    chmod "${old_mode}" "${home}" >/dev/null 2>&1 || true
  fi
  rm -f "${mode_file}" || true
}

apply_readonly_mode() {
  local user="$1"
  local home="$2"
  [[ -d "${home}" ]] || return 0
  local now_mode
  now_mode="$(stat -c '%a' "${home}" 2>/dev/null || true)"
  [[ "${now_mode}" =~ ^[0-7]{3,4}$ ]] || return 0
  local mode_file="${HOME_RESERVE_STATE_DIR}/${user}.mode"
  if [[ ! -f "${mode_file}" ]]; then
    printf '%s\n' "${now_mode}" > "${mode_file}"
    chmod 0600 "${mode_file}" >/dev/null 2>&1 || true
  fi
  local mode_dec desired_dec desired_mode
  mode_dec=$((8#${now_mode}))
  # 仅移除写位（0222），保留其他权限位（含特殊位）。
  desired_dec=$((mode_dec & 07555))
  desired_mode="$(printf '%03o' "${desired_dec}")"
  if [[ "${desired_mode}" != "${now_mode}" ]]; then
    chmod "${desired_mode}" "${home}" >/dev/null 2>&1 || true
  fi
}

if ! [[ "${HOME_RESERVE_GB}" =~ ^[0-9]+$ ]]; then
  HOME_RESERVE_GB=8
fi

avail_bytes="$(df -PB1 "${HOME_RESERVE_CHECK_PATH}" 2>/dev/null | awk 'NR==2{print $4}')"
if ! [[ "${avail_bytes}" =~ ^[0-9]+$ ]]; then
  exit 0
fi
reserve_bytes=$((HOME_RESERVE_GB * 1024 * 1024 * 1024))
low_space=0
if (( HOME_RESERVE_GB > 0 && avail_bytes < reserve_bytes )); then
  low_space=1
fi

while IFS=: read -r user home; do
  user="$(echo "${user}" | xargs)"
  home="$(echo "${home}" | xargs)"
  [[ -z "${user}" || -z "${home}" ]] && continue
  if is_exempt "${user}"; then
    restore_user_mode "${user}" "${home}"
    continue
  fi
  if (( low_space == 1 )); then
    apply_readonly_mode "${user}" "${home}"
  else
    restore_user_mode "${user}" "${home}"
  fi
done < <(list_home_users)

# 清理已不存在账号的残留状态文件
for mf in "${HOME_RESERVE_STATE_DIR}"/*.mode; do
  [[ -e "${mf}" ]] || break
  u="$(basename "${mf}" .mode)"
  if ! getent passwd "${u}" >/dev/null 2>&1; then
    rm -f "${mf}" || true
  fi
done
EOF_ENFORCE
${SUDO} chmod +x /opt/gpu-cluster/enforce_home_reserve_storage.sh

${SUDO} tee /etc/systemd/system/gpu-home-reserve-enforce.service >/dev/null <<'EOF_SVC'
[Unit]
Description=GPU Home Reserve Storage Enforcer
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/opt/gpu-cluster/enforce_home_reserve_storage.sh
EOF_SVC

${SUDO} tee /etc/systemd/system/gpu-home-reserve-enforce.timer >/dev/null <<EOF_TIMER
[Unit]
Description=GPU Home Reserve Storage Enforcer Timer

[Timer]
OnBootSec=45
OnUnitActiveSec=${HOME_RESERVE_ENFORCE_INTERVAL}
Unit=gpu-home-reserve-enforce.service

[Install]
WantedBy=timers.target
EOF_TIMER

if [[ -f /etc/pam.d/sshd ]]; then
  ${SUDO} sed -i '\#home_reserve_login_check.sh#d' /etc/pam.d/sshd
  if grep -q '^@include common-account' /etc/pam.d/sshd; then
    ${SUDO} sed -i '/^@include common-account/i account optional pam_exec.so quiet /opt/gpu-cluster/home_reserve_login_check.sh' /etc/pam.d/sshd
  else
    echo "account optional pam_exec.so quiet /opt/gpu-cluster/home_reserve_login_check.sh" | ${SUDO} tee -a /etc/pam.d/sshd >/dev/null
  fi
fi

if [[ -f /etc/ssh/sshd_config ]]; then
  if grep -Eq '^[[:space:]]*UsePAM[[:space:]]+no' /etc/ssh/sshd_config; then
    ${SUDO} sed -i 's/^[[:space:]]*UsePAM[[:space:]]\\+no/UsePAM yes/g' /etc/ssh/sshd_config
  elif ! grep -Eq '^[[:space:]]*UsePAM[[:space:]]+yes' /etc/ssh/sshd_config; then
    echo 'UsePAM yes' | ${SUDO} tee -a /etc/ssh/sshd_config >/dev/null
  fi
fi

${SUDO} systemctl daemon-reload
${SUDO} systemctl enable --now gpu-home-reserve-enforce.timer
${SUDO} systemctl start gpu-home-reserve-enforce.service || true
${SUDO} systemctl restart ssh || ${SUDO} systemctl restart sshd || true

echo "[home-reserve] 已启用（低于 ${HOME_RESERVE_GB}G 时仅限制非豁免用户 /home 写入，登录仍允许）"
