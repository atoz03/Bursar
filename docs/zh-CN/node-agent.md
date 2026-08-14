# Node Agent

[English](../node-agent.md) | **简体中文**

Node Agent 运行在每个 Linux 计算节点上，向控制器上报硬件、进程、GPU、磁盘、SSH 和服务状态，并执行控制器下发的策略动作。

## 为什么 Agent 不容器化

Agent 需要写入 cgroup v2、操作 systemd unit、读取主机进程表和 `/etc/passwd`、修改 SSH 配置，并访问 NVIDIA 驱动栈。把它放进容器需要开放到几乎没有隔离意义的主机权限。请用 systemd 直接安装在主机上。只有控制器提供容器镜像。

## 前置条件

先运行只读的前置检查脚本，它不会修改任何内容：

```bash
bash scripts/node_prereq_check.sh
```

节点需要满足：

- Linux 且有 systemd（推荐），或 `/sys/fs/cgroup` 下的 cgroup v2；
- 网络可达控制器的内部监听；
- GPU 节点上有 `nvidia-smi`；
- Agent 服务具备 root 权限。

## 安装

在节点上从已检出的仓库执行：

```bash
sudo env \
  NODE_ID=node-01 \
  CONTROLLER_URL=https://controller.example.org:8081 \
  AGENT_TOKEN='<node-or-global-agent-token>' \
  bash scripts/install_agent_local.sh
```

安装脚本会构建二进制、写入 `/etc/gpu-cluster/node-agent.env`、安装 `gpu-node-agent` systemd unit，并可选安装 SSH Guard 与主机安全钩子。

`NODE_ID` 是运维方选定的稳定标识，不要求是 IP 地址或 SSH 端口。一旦节点以某个标识上报，就不要再更改：用量历史、账号映射和节点专属积分都以它为键。

## 环境变量

在 `/etc/gpu-cluster/node-agent.env` 中配置。

### 必填

| 变量 | 含义 |
| --- | --- |
| `NODE_ID` | 稳定的节点标识 |
| `CONTROLLER_URL` | Agent 上报目标控制器监听的基础 URL |
| `AGENT_TOKEN` | 全局 Agent token，或 `agent_node_tokens` 中本节点的 token |

### 时间与采样

| 变量 | 默认值 | 含义 |
| --- | --- | --- |
| `INTERVAL_SECONDS` | `60` | 上报周期，控制器据此把采样折算为成本 |
| `ACTION_POLL_INTERVAL_SECONDS` | `1`（安装脚本设为 `2`） | 动作轮询周期 |
| `CPU_MIN_PERCENT` | `1.0` | 忽略 CPU 占用低于此值的进程 |
| `STATE_DIR` | 由安装脚本管理 | 本地状态目录 |

### 采集缓存

| 变量 | 默认值 | 含义 |
| --- | --- | --- |
| `LOCAL_USERS_REFRESH_SECONDS` | `900` | 本地账号列表刷新周期 |
| `LOCAL_USERS_COLLECT_TIMEOUT_SECONDS` | `8` | 采集本地账号的超时 |
| `SYSTEM_SERVICES_REFRESH_SECONDS` | `1800`（安装脚本设为 `60`） | 服务状态刷新周期 |
| `SYSTEM_SERVICES_COLLECT_TIMEOUT_SECONDS` | `8` | 采集服务状态的超时 |
| `SYSTEM_SERVICES_CHECK_UNITS` | 内置列表 | 逗号分隔的待监控 systemd unit |
| `DISK_QUOTA_REFRESH_SECONDS` | `120` | 磁盘配额刷新周期 |
| `GPU_BUS_MAP_CACHE_SECONDS` | `600` | GPU 总线 ID 映射缓存时长 |
| `GPU_INVENTORY_CACHE_SECONDS` | `1800` | GPU 清单缓存时长 |
| `GPU_COMMAND_TIMEOUT_SECONDS` | `4` | `nvidia-smi` 调用超时 |

### 安装脚本选项

| 变量 | 默认值 | 含义 |
| --- | --- | --- |
| `ENABLE_SSH_GUARD` | `1` | 安装 SSH 登录拦截 |
| `SSH_GUARD_EXCLUDE_USERS` | `root` | 永不被拦截的账号 |
| `SSH_GUARD_FAIL_OPEN` | `0` | 控制器不可达时是否放行登录 |
| `SSH_GUARD_REALTIME_LOOKUP` | `0` | 登录时实时查询控制器而不是使用缓存 |
| `SSH_GUARD_SYNC_INTERVAL` | `10s` | 白名单/黑名单同步周期 |
| `SSH_GUARD_ENFORCE_INTERVAL` | `10s` | 在线会话巡检周期 |
| `ENABLE_HOST_SECURITY` | `1` | 安装主机安全信号采集 |
| `ENABLE_SHARED_NFS` | `0` | 挂载共享工作目录导出 |
| `SKIP_CONTROLLER_HEALTHCHECK` | `0` | 安装时跳过控制器可达性检查 |

## SSH Guard

当 `ENABLE_SSH_GUARD=1` 时，节点会基于从控制器 registry 接口同步来的白名单、黑名单和豁免名单缓存，拦截未在平台登记的账号登录。

`SSH_GUARD_FAIL_OPEN` 决定控制器不可达时的行为：

- `0`（默认）：拒绝所有非排除账号登录。更安全，但控制器故障可能导致整个集群被锁在门外。
- `1`：放行登录。安全性略低，但故障不会演变成锁定事故。

在生产节点上启用 Guard 之前，务必确保存在带外访问路径——控制台，或一个位于 `SSH_GUARD_EXCLUDE_USERS` 中且你持有密钥的账号。

## Token

先使用全局 `agent_token`，再迁移到每节点 token：

1. 在控制器上填写 `agent_node_tokens`，保持 `agent_node_token_enforce: false`；
2. 下发每节点 token 并重启 Agent；
3. 确认全部节点上报正常；
4. 设置 `agent_node_token_enforce: true` 并重启控制器。

`scripts/generate_node_agent_tokens.sh` 用于生成 token 映射。轮换全局 token 时，先把旧值放入 `agent_legacy_tokens`，滚动完成后清空。

## 服务运维

```bash
sudo systemctl status gpu-node-agent
sudo systemctl restart gpu-node-agent
sudo journalctl -u gpu-node-agent -f
```

## 排障

**控制器上看不到该节点。** 检查节点到 `CONTROLLER_URL` 的连通性，确认 `AGENT_TOKEN` 与控制器配置一致，并在 journal 中查找 `401`。若已开启 `agent_node_token_enforce`，token 必须是该 `NODE_ID` 注册的那一个。

**GPU 用量缺失。** 确认以 root 执行 `nvidia-smi --query-compute-apps=pid,used_memory --format=csv` 正常。驱动响应慢的节点可以调大 `GPU_COMMAND_TIMEOUT_SECONDS`。

**CPU 限制未生效。** 检查 systemd 是否暴露用户 slice：`systemctl show user-$(id -u <user>).slice -p CPUQuota`。没有 systemd 的节点需要挂载并可写 cgroup v2。

**重试后出现重复扣费。** 上报携带全局唯一 `report_id`，控制器据此去重。若仍出现重复，请确认该节点没有运行两个 Agent 实例。

**启用 Guard 后用户无法登录。** 临时设置 `SSH_GUARD_FAIL_OPEN=1`，或把账号加入 `SSH_GUARD_EXCLUDE_USERS`，并确认 registry 列表正在同步。

控制器侧的现象见[故障排查](troubleshooting.md)。
