#!/usr/bin/env bash
# 安装每日加密备份和每周隔离恢复演练。必须显式提供独立备份仓库。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKUP_REPOSITORY="${BACKUP_REPOSITORY:-}"
BACKUP_DATA_PATHS="${BACKUP_DATA_PATHS:-/srv/gpu-ops/nodes /srv/gpu-ops/cluster}"
BACKUP_RUN_USER="${BACKUP_RUN_USER:-root}"
BACKUP_STATUS_GROUP="${BACKUP_STATUS_GROUP:-gpuops}"

if [[ -z "${BACKUP_REPOSITORY}" ]]; then
  echo "必须设置 BACKUP_REPOSITORY，且应位于独立磁盘、SFTP 或对象存储。" >&2
  echo "示例：BACKUP_REPOSITORY=sftp:backup@10.0.0.2:/srv/restic/gpu-ops" >&2
  exit 2
fi
if [[ "${BACKUP_REPOSITORY}" == /var/lib/docker/* || "${BACKUP_REPOSITORY}" == /home/* ]]; then
  echo "拒绝将备份仓库放在控制器系统盘：${BACKUP_REPOSITORY}" >&2
  exit 2
fi
if [[ "${BACKUP_REPOSITORY}" == /* ]]; then
  read -r -a protected_paths <<<"${BACKUP_DATA_PATHS}"
  for protected_path in "${protected_paths[@]}"; do
    if [[ "${BACKUP_REPOSITORY}" == "${protected_path}" || "${BACKUP_REPOSITORY}" == "${protected_path}"/* ]]; then
      echo "备份仓库不能位于被备份目录内部：${BACKUP_REPOSITORY}" >&2
      exit 2
    fi
  done
fi

SUDO=""
if [[ "$(id -u)" -ne 0 ]]; then
  SUDO="sudo"
fi

if command -v apt-get >/dev/null 2>&1; then
  ${SUDO} apt-get update -y
  ${SUDO} apt-get install -y restic jq openssl
fi

${SUDO} install -d -m 0750 -o root -g "${BACKUP_STATUS_GROUP}" /etc/gpu-controller /var/lib/gpu-controller
if [[ ! -f /etc/gpu-controller/restic-password ]]; then
  openssl rand -base64 48 | ${SUDO} tee /etc/gpu-controller/restic-password >/dev/null
fi
${SUDO} chmod 0600 /etc/gpu-controller/restic-password

env_tmp="$(mktemp /tmp/gpuops-backup-env.XXXXXX)"
trap 'rm -f "${env_tmp}"' EXIT
printf 'RESTIC_REPOSITORY=%q\n' "${BACKUP_REPOSITORY}" >"${env_tmp}"
printf 'RESTIC_PASSWORD_FILE=%q\n' /etc/gpu-controller/restic-password >>"${env_tmp}"
printf 'BACKUP_DATA_PATHS=%q\n' "${BACKUP_DATA_PATHS}" >>"${env_tmp}"
printf 'BACKUP_STATUS_GROUP=%q\n' "${BACKUP_STATUS_GROUP}" >>"${env_tmp}"
printf 'CONTROLLER_CONFIG_PATH=%q\n' "${ROOT_DIR}/config/controller.local.yaml" >>"${env_tmp}"
${SUDO} install -m 0600 -o root -g root "${env_tmp}" /etc/gpu-controller/backup.env

${SUDO} install -m 0750 "${ROOT_DIR}/scripts/gpuops_backup.sh" /usr/local/sbin/gpuops-backup
${SUDO} install -m 0750 "${ROOT_DIR}/scripts/gpuops_backup_verify.sh" /usr/local/sbin/gpuops-backup-verify

if ! ${SUDO} env BACKUP_ENV_FILE=/etc/gpu-controller/backup.env bash -c 'set -a; source "$BACKUP_ENV_FILE"; set +a; restic snapshots >/dev/null 2>&1'; then
  ${SUDO} env BACKUP_ENV_FILE=/etc/gpu-controller/backup.env bash -c 'set -a; source "$BACKUP_ENV_FILE"; set +a; restic init'
fi

${SUDO} tee /etc/systemd/system/gpuops-backup.service >/dev/null <<EOF
[Unit]
Description=GPU Ops encrypted backup
After=docker.service network-online.target
Requires=docker.service

[Service]
Type=oneshot
User=${BACKUP_RUN_USER}
Environment=BACKUP_ENV_FILE=/etc/gpu-controller/backup.env
ExecStart=/usr/local/sbin/gpuops-backup
Nice=10
IOSchedulingClass=idle
EOF

${SUDO} tee /etc/systemd/system/gpuops-backup.timer >/dev/null <<'EOF'
[Unit]
Description=Daily GPU Ops backup

[Timer]
OnCalendar=*-*-* 02:00:00
RandomizedDelaySec=15m
Persistent=true

[Install]
WantedBy=timers.target
EOF

${SUDO} tee /etc/systemd/system/gpuops-backup-verify.service >/dev/null <<EOF
[Unit]
Description=GPU Ops isolated restore verification
After=docker.service network-online.target
Requires=docker.service

[Service]
Type=oneshot
User=${BACKUP_RUN_USER}
Environment=BACKUP_ENV_FILE=/etc/gpu-controller/backup.env
ExecStart=/usr/local/sbin/gpuops-backup-verify
Nice=10
IOSchedulingClass=idle
EOF

${SUDO} tee /etc/systemd/system/gpuops-backup-verify.timer >/dev/null <<'EOF'
[Unit]
Description=Weekly GPU Ops restore verification

[Timer]
OnCalendar=Sun *-*-* 04:00:00
RandomizedDelaySec=30m
Persistent=true

[Install]
WantedBy=timers.target
EOF

${SUDO} systemctl daemon-reload
${SUDO} systemctl enable --now gpuops-backup.timer gpuops-backup-verify.timer
echo "备份定时器已安装。建议立即执行：sudo systemctl start gpuops-backup.service"
