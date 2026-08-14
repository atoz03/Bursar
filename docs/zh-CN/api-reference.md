# API 参考

[English](../api-reference.md) | **简体中文**

面向 Agent、节点侧辅助程序与运维脚本的 HTTP 契约。本文是重要接口的目录，不是全部路由的穷举清单。

## 鉴权

| 调用方 | 方式 |
| --- | --- |
| Node Agent | `X-Agent-Token: <token>` |
| 运维脚本 | `Authorization: Bearer <admin_token>` |
| 浏览器 | `POST /api/auth/login` 下发的 HttpOnly 会话 cookie |
| HA 对端 | `X-HA-Token: <token>` |

**CSRF。** 使用会话 cookie 鉴权时，所有非 GET 请求都必须携带 `X-CSRF-Token`，取值来自 `GET /api/auth/me` 的 `csrf_token` 字段。缺失时返回 `403 csrf_required`。

**监听归属。** 配置了 `internal_listen_addr` 时，Agent、registry 与 HA 路由只在内部监听（默认 `8081`）上提供。留空时，它们同时挂载在 Web 监听（默认 `8080`）上。

## 健康与指标

### `GET /healthz`

无需鉴权。进程能提供 HTTP 服务时返回 `{"ok":true}`，不检查数据库。

### `GET /readyz`

无需鉴权。返回 `{"ok":true,"database":true}`；PostgreSQL 不可达时返回 `503` 与 `{"ok":false,"database":false}`。负载均衡与容器健康检查请使用该接口。

### `GET /metrics`

需要 `admin_token`、Agent token 或管理员会话。以 Prometheus 文本格式返回聚合计数器。指标清单与抓取配置见[运维指南](operations.md)。

## 会话

### `POST /api/admin/bootstrap`

创建第一个管理员。仅在管理员表为空时允许，且只接受 `Authorization: Bearer <admin_token>`——会话无法自举。

```json
{"username":"admin","password":"<strong-password>"}
```

### `POST /api/auth/login`

```json
{"username":"admin","password":"..."}
```

返回 `{"ok":true,"role":"admin"}`，并设置 HttpOnly、`SameSite=Lax` 的会话 cookie。

### `GET /api/auth/me`

返回当前会话与 CSRF token：

```json
{"authenticated":true,"username":"admin","role":"admin","expires_at":"2026-08-14T16:00:00Z","csrf_token":"..."}
```

### `POST /api/auth/logout`

返回 `{"ok":true}` 并清除 cookie。

## Agent 上报

### `POST /api/metrics`

需要 `X-Agent-Token`。`report_id` 必填且必须全局唯一——控制器以它去重，避免 Agent 重试导致重复扣费。

```json
{
  "node_id": "node-01",
  "timestamp": "2026-08-14T16:00:00Z",
  "report_id": "2f6c7b3b3c3b4a8b8f1c5c3c1b2a9d10",
  "interval_seconds": 60,
  "users": [
    {
      "username": "alice",
      "pid": 12345,
      "cpu_percent": 120.5,
      "memory_mb": 2048,
      "gpu_usage": [
        {"gpu_id": 0, "gpu_model": "NVIDIA A100-SXM4-80GB", "gpu_bus_id": "00000000:3B:00.0", "memory_mb": 4096}
      ]
    }
  ]
}
```

响应：

```json
{
  "actions": [
    {"type": "notify", "username": "alice", "message": "..."},
    {"type": "set_cpu_quota", "username": "alice", "cpu_quota_percent": 50, "reason": "..."}
  ]
}
```

`node_id` 是由运维方选定的稳定标识。当存在 `(node_id, local_username)` 映射时，控制器向映射到的平台账号扣费，但动作仍以本地用户名下发，以便 Agent 执行。

把采样折算为成本时，上报中的 `interval_seconds` 优先于控制器的 `sample_interval_seconds`。

### `GET /api/node/actions`

需要 `X-Agent-Token`。返回该节点的待执行动作。

## 用户接口

### `GET /api/users/:username/balance`

需要 `admin_token`、Agent token、管理员会话，或该用户本人的会话。用户不存在时返回 `404`——此接口不会创建用户。

```json
{
  "username": "alice",
  "balance": 80.0,
  "general_balance": 80.0,
  "carryover_balance": 20.0,
  "exclusive_balance": 0.0,
  "total_balance": 100.0,
  "status": "warning",
  "warning_threshold_points": 100,
  "limited_threshold_points": 3,
  "monthly_max_overdraft_limit": 0,
  "current_overdraft_points": 0,
  "overdraft_exceeded": false,
  "manual_blocked": false
}
```

### `GET /api/users/:username/usage`

鉴权方式相同。`limit` 默认 200，最大 5000。

```json
{"records":[{"node_id":"node-01","username":"alice","timestamp":"2026-08-14T16:00:00Z","cpu_percent":120.5,"memory_mb":2048,"gpu_usage":"[]","cost":0.6}]}
```

### `POST /api/users/:username/recharge`

需要 `admin_token`、管理员会话，或持有 `manage_platform_users` 权限位的高级用户会话。

```json
{"amount": 100, "method": "admin"}
```

### 会话作用域的用户路由

均需会话 cookie：`GET /api/user/me`、`/me/balance`、`/me/usage`、`/me/points-increments`、`/me/profile`、`/me/profile-change-requests`、`/accounts`、`/requests`。

## 注册与账号绑定

以下三个路由都需要会话 cookie。申请人取自会话，绝不取自请求体——请求中的 `billing_username` 字段会被忽略并覆盖。

### `POST /api/user/requests/bind`

申报已有的节点账号以供审核。单次最多 200 条。

```json
{
  "items": [
    {"node_id": "node-01", "local_username": "alice"},
    {"node_id": "node-05", "local_username": "alice2"}
  ],
  "message": "optional note"
}
```

### `POST /api/user/requests/open`

申请在某节点上开通新账号。`node_id` 或 `local_username` 为空时会被置为 `待分配`，因此尚不确定需要哪个节点的用户也能提交申请。`message` 作为申请理由参与校验。

```json
{"node_id":"node-01","local_username":"alice","message":"申请理由"}
```

### `GET /api/user/requests`

返回调用者自己的申请。`limit` 默认 200，最大 5000。

### `POST /api/admin/requests/:id/approve` 与 `/reject`

需要审核权限。通过 `bind` 申请会写入 `user_node_accounts`，该表同时驱动计费归属与 SSH 登录校验。此外还提供 `/reopen` 与 `/batch-review`；新账号注册申请由 `POST /api/admin/registration-requests/:id/approve` 与 `/reject` 处理。

## 节点 registry

以下路由需要 `X-Agent-Token`，且应通过内部监听访问。

| 路由 | 用途 |
| --- | --- |
| `GET /api/registry/nodes/:node_id/users.txt` | 该节点已登记的本地用户名，每行一个 |
| `GET /api/registry/nodes/:node_id/blocked.txt` | 被拒绝的本地用户名 |
| `GET /api/registry/nodes/:node_id/exempt.txt` | 豁免的本地用户名 |
| `GET /api/registry/nodes/:node_id/guard-state` | 当前 SSH guard 状态 |
| `GET /api/registry/resolve?node_id=&local_username=` | 把本地账号解析为平台账号 |

### `POST /api/registry/bind-claim`

由请求体中的一次性绑定挑战 token 鉴权，而不是 Agent token，因为调用方是节点上的用户进程。

```json
{"token":"<challenge-token>","node_id":"node-01","local_username":"alice"}
```

## 管理员接口

### 价格

```http
POST /api/admin/prices
```

```json
{"gpu_model":"RTX 3090","price_per_minute":0.2}
```

CPU 计费使用保留型号名 `CPU_CORE`，按核分钟计价（100% CPU ≈ 1 核）。执行 `set_cpu_quota` 需要节点上有 systemd `CPUQuota` 或可写的 cgroup，且 Agent 以 root 运行。

### 进程审计

| 路由 | 用途 |
| --- | --- |
| `GET /api/admin/usage` | 用量与终止记录。过滤参数：`billing_username`、`local_username`、`unregistered_only=1`、`limit`（默认 200，最大 5000） |
| `GET /api/admin/usage/export.csv` | CSV 导出。额外支持 `from`、`to`、`limit`（默认 20000，最大 200000） |
| `GET /api/admin/usage/days` | 有记录的日期，含行数与预估 CSV 体积 |
| `GET /api/admin/usage/range-estimate` | 导出或删除前，预览某日期区间的行数与体积 |
| `POST /api/admin/usage/delete-range` | 不可逆地删除区间内的 `usage_records` |
| `GET`/`POST /api/admin/usage/retention` | 读取或设置保留策略 |

删除请求与响应：

```json
{"from":"2026-08-01","to":"2026-08-03","billing_username":"alice","unregistered_only":false,"confirm":true}
```

```json
{"ok":true,"records_before":1200,"deleted_records":1200}
```

### 节点

`GET /api/admin/nodes` 返回上报状态——最近上报时间、GPU 与 CPU 进程数、最近一次上报的成本。`limit` 默认 200，最大 2000。

按节点的路由涵盖详情、价格、CPU 限制、内存限制、GPU 可见性、磁盘配额、SSH 独享、查看权限、积分拦截与安全事件。

### HA 与备份

| 路由 | 用途 |
| --- | --- |
| `GET /api/admin/ha/status` | 主备可达性、摘要比对与一致性 |
| `GET`/`POST /api/admin/ha/sync/config` | 同步配置 |
| `GET /api/admin/ha/sync/runs` | 同步历史，含每步结果 |
| `POST /api/admin/ha/sync/now` | 触发同步；`direction` 取 `primary_to_standby` 或 `standby_to_primary` |
| `POST /api/admin/ha/failover/activate` | 仅备机可用，主机返回 `409`。请求体必须为 `{"confirm":"ACTIVATE_STANDBY"}` |
| `GET /api/admin/backup/status` | 最近快照与恢复演练状态 |

同步配置：

```json
{
  "enabled": true,
  "interval_days": 1,
  "start_hour": 3,
  "dr_host": "192.0.2.20",
  "dr_ssh_user": "gpuops",
  "dr_ssh_port": 22,
  "dr_key_file": "/etc/gpu-ops/standby_ed25519",
  "dr_controller_port": 8080,
  "primary_host": "192.0.2.10",
  "primary_controller_port": 8080,
  "script_path": "/opt/gpu-ops/scripts/ha_sync_worker.sh",
  "sync_web_dist": true,
  "sync_database": true,
  "auto_failover": true
}
```

只有当 36 小时内存在一次成功备份、且 8 天内存在一次成功的隔离恢复演练时，`GET /api/admin/backup/status` 才会返回 `ready=true`。

## 错误响应

| 状态码 | 响应体 | 含义 |
| --- | --- | --- |
| `400` | `{"error":"..."}` | 请求格式错误或参数非法 |
| `401` | `{"error":"unauthorized"}` | 缺少或提供了无效凭据 |
| `403` | `{"error":"forbidden"}` | 已鉴权但缺少所需权限 |
| `403` | `{"error":"csrf_required"}` | 会话请求缺少匹配的 `X-CSRF-Token` |
| `404` | `{"error":"..."}` | 资源不存在 |
| `409` | `{"error":"..."}` | 状态冲突，例如在主机上执行接管 |
| `503` | `{"ok":false,"database":false}` | 就绪检查失败 |
