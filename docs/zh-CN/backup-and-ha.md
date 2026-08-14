# 备份与高可用

[English](../backup-and-ha.md) | **简体中文**

备份用于防止数据丢失，HA 用于降低停机时间。两者解决不同问题，HA 不能替代经过验证的恢复流程。

## 需要备份什么

| 内容 | 原因 |
| --- | --- |
| PostgreSQL 数据库 | 身份、账号映射、积分、价格、用量历史、策略，以及包含 SMTP 凭据的运行时设置 |
| 控制器配置 | 启动密钥与监听配置 |
| `web/dist` | 仅为省事，可以从源码重新构建 |

备份脚本默认只保护平台自身，不包含用户科研数据。`BACKUP_DATA_PATHS` 有意留空——用户数据属于你的存储系统，通常也远超这条流水线的处理能力。只有在明确决定后才设置它。

## 使用 restic 备份

`scripts/gpuops_backup.sh` 生成加密、去重、带保留策略的快照，依赖 `docker`、`restic`、`jq` 和 `flock`。

### 配置

创建 `/etc/gpu-controller/backup.env`：

```bash
RESTIC_REPOSITORY=sftp:backup@backup.example.org:/srv/restic/gpu-ops
RESTIC_PASSWORD_FILE=/etc/gpu-controller/restic-password
POSTGRES_CONTAINER=gpuops-postgres
POSTGRES_DATABASE=gpuops
POSTGRES_USER=gpuops
CONTROLLER_CONFIG_PATH=/opt/gpu-controller/controller.yaml
KEEP_DAILY=7
KEEP_WEEKLY=4
KEEP_MONTHLY=12
```

仓库必须位于独立磁盘或独立主机。与数据库同盘的备份，无法在它本该应对的故障中幸存。

限制密码文件权限：`chmod 600 /etc/gpu-controller/restic-password`。一旦丢失，所有快照将永久不可读——请在组织的密钥管理系统中另存一份。

### 安装与运行

```bash
sudo bash scripts/install_backup_local.sh
sudo bash scripts/gpuops_backup.sh
```

安装脚本会注册 systemd timer。每次运行都会写入 `backup_status_file`，控制器读取后展示在管理端 → 集群总览页面。

### 验证恢复

没有恢复过的备份只是一个假设。`scripts/gpuops_backup_verify.sh` 会把最新的数据库快照恢复到一次性 PostgreSQL 容器中，确认它可以正常加载：

```bash
sudo bash scripts/gpuops_backup_verify.sh
```

它写入 `backup_verify_status_file`，同样展示在管理端 → 集群总览。请定期执行，而不是只在初始化时跑一次，并在验证时间戳过期时告警。

### 手动恢复

```bash
restic snapshots
restic restore <snapshot-id> --target /var/tmp/gpuops-restore
docker compose --profile full stop controller
psql "postgres://gpuops@127.0.0.1:5432/gpuops" < /var/tmp/gpuops-restore/.../gpuops.sql
docker compose --profile full start controller
curl -fsS http://127.0.0.1:8080/readyz
```

恢复前必须停止控制器。在控制器运行时恢复数据库会产生不一致状态。

## 高可用

Bursar 的 HA 是运维自动化，而不是共识协议。它没有 quorum、没有自动选主，应用层也没有脑裂保护。防止出现两个同时活跃的主控制器是你的责任，需要在网络与进程层面落实。

### 模型

- 一个控制器以 `ha_role: primary` 作为主机运行。
- 一个控制器以 `ha_role: standby` 作为备机运行。
- 主机定期把状态同步到备机。
- 内部监听上的 `GET /api/ha/status` 返回同步状态，用 `X-HA-Token` 鉴权。

### 配置

两台控制器上分别配置：

```yaml
ha_enabled: true
ha_node: "controller-primary"     # 或 controller-standby
ha_role: "primary"                # 或 standby
ha_peer_url: "https://standby.example.org:8081"
ha_token: "<shared-ha-token>"
```

用 `openssl rand -hex 32` 生成 `ha_token`，它是独立于 `agent_token` 和 `admin_token` 的密钥。

### 部署备机

```bash
sudo bash scripts/deploy_dr_standby.sh
sudo bash scripts/bootstrap_dr_standby_local.sh
```

`scripts/ha_sync_worker.sh` 运行同步循环。`scripts/gpuops_ha_apply.sh` 是受限的 root helper，用于应用同步产物；它会校验文件属主并拒绝非预期路径。

### 接管

接管是手动且需要明确确认的：

1. 确认主机确实已经宕机。从备机视角看，网络分区与主机死亡完全一样。
2. 如果主机仍可访问，先停止其控制器服务，避免它继续写入。
3. 提升备机：设置 `ha_role: primary` 并重启。
4. 迁移 VIP 或更新 DNS。`scripts/install_ha_vip_local.sh` 基于 `/readyz` 健康检查管理 VIP。
5. 确认 Agent 重新连接、上报恢复。

回切是同样的流程反向执行，并需要先处理数据分歧。

### HA 不提供什么

- 不自动接管。
- 不防止脑裂。两个主控制器同时写入两个数据库会产生无法调和的分歧。
- 不能替代备份。同步会忠实地复制损坏数据和误删除。
- 不涵盖 PostgreSQL 复制。若需要数据库级 HA，请在 PostgreSQL 中配置。

## 演练清单

建议每季度执行：

- [ ] 把最新快照恢复到临时环境，并用它启动一个控制器。
- [ ] 确认恢复出的数据库包含近期用量记录和预期的用户数量。
- [ ] 在维护窗口内提升备机，确认 Agent 重新连接。
- [ ] 确认管理端 → 集群总览上的备份与验证时间戳是最新的。
- [ ] 确认除了配置者之外，还有其他人能够从密钥管理系统中取回 restic 密码。
