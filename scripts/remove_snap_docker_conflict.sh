#!/usr/bin/env bash
# 移除抢占 /run/docker.sock 的空 Snap Docker，并恢复系统 Docker。
set -Eeuo pipefail
umask 077

if [[ "$(id -u)" -ne 0 ]]; then
  echo "请使用 sudo 运行此脚本" >&2
  exit 2
fi

for command_name in docker nsenter pgrep snap systemctl; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "缺少命令：${command_name}" >&2
    exit 2
  }
done

if ! snap list docker >/dev/null 2>&1; then
  echo "Snap Docker 未安装，无需移除" >&2
  exit 2
fi

mapfile -t snap_containers < <(/usr/bin/docker -H unix:///run/docker.sock ps --all --quiet)
if (( ${#snap_containers[@]} > 0 )); then
  echo "当前 Socket 对应的 Snap Docker 中存在 ${#snap_containers[@]} 个容器，拒绝自动移除" >&2
  /usr/bin/docker -H unix:///run/docker.sock ps --all >&2
  exit 3
fi

system_docker_pid="$(systemctl show docker.service --property MainPID --value)"
if [[ -z "${system_docker_pid}" || "${system_docker_pid}" == "0" ]]; then
  echo "系统 Docker daemon 未运行" >&2
  exit 3
fi

postgres_pid="$(pgrep --oldest --exact postgres || true)"
if [[ -z "${postgres_pid}" ]]; then
  echo "未找到生产 PostgreSQL 进程" >&2
  exit 3
fi
postgres_cgroup="$(<"/proc/${postgres_pid}/cgroup")"
if [[ "${postgres_cgroup}" =~ docker-([0-9a-f]{64})\.scope ]]; then
  postgres_container="${BASH_REMATCH[1]}"
elif [[ "${postgres_cgroup}" =~ /docker/([0-9a-f]{64}) ]]; then
  postgres_container="${BASH_REMATCH[1]}"
else
  echo "PostgreSQL 进程不属于可识别的系统 Docker 容器" >&2
  exit 3
fi

backup_dir="/var/lib/gpu-controller/emergency"
backup_file="${backup_dir}/gpuops-before-snap-removal-$(date +%Y%m%d-%H%M%S).dump"
install -d -m 0700 "${backup_dir}"
echo "正在生成重启前数据库保险备份：${backup_file}"
nsenter --target "${postgres_pid}" --mount --uts --ipc --net --pid \
  pg_dump --username=gpuops --dbname=gpuops --format=custom --compress=6 \
    --no-owner --no-privileges >"${backup_file}"
nsenter --target "${postgres_pid}" --mount --uts --ipc --net --pid \
  pg_restore --list <"${backup_file}" >/dev/null
echo "保险备份已校验：$(du -h "${backup_file}" | awk '{print $1}')"

echo "停止并移除 Snap Docker（不会移除 snapd）"
snap stop --disable docker
set +e
snap remove docker --purge
snap_remove_rc=$?
set -e

echo "重建系统 Docker Socket；PostgreSQL 将短暂中断"
systemctl stop docker.service docker.socket
systemctl start docker.socket
systemctl start docker.service

if ! docker inspect "${postgres_container}" >/dev/null 2>&1; then
  echo "系统 Docker 已恢复，但找不到原 PostgreSQL 容器 ${postgres_container}" >&2
  exit 4
fi
if [[ "$(docker inspect --format '{{.State.Running}}' "${postgres_container}")" != "true" ]]; then
  docker start "${postgres_container}" >/dev/null
fi

database_ready=0
for _ in $(seq 1 60); do
  if docker exec "${postgres_container}" pg_isready --username=gpuops --dbname=gpuops >/dev/null 2>&1; then
    database_ready=1
    break
  fi
  sleep 1
done
if [[ "${database_ready}" != "1" ]]; then
  echo "PostgreSQL 容器已启动，但数据库在 60 秒内未就绪" >&2
  exit 4
fi

if (( snap_remove_rc != 0 )); then
  echo "系统 Docker 已恢复，但 Snap 包移除命令失败，请检查：snap changes" >&2
  exit "${snap_remove_rc}"
fi

echo "系统 Docker 与 PostgreSQL 已恢复，开始首份平台备份"
systemctl start gpuops-backup.service
systemctl --no-pager --full status gpuops-backup.service
echo "完成。紧急数据库备份保留在：${backup_file}"
