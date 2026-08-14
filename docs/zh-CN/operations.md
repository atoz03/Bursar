# 运维指南

[English](../operations.md) | **简体中文**

已上线部署的日常生命周期、升级与恢复流程。

## 服务生命周期

### 源码部署的控制器

```bash
sudo systemctl status gpu-controller
sudo systemctl restart gpu-controller
sudo journalctl -u gpu-controller -f
```

### Docker 部署的控制器

```bash
docker compose --profile full ps
docker compose --profile full restart controller
docker compose --profile full logs -f controller
```

### Node Agent

```bash
sudo systemctl status gpu-node-agent
sudo journalctl -u gpu-node-agent -f
```

## 健康检查

| 接口 | 含义 |
| --- | --- |
| `GET /healthz` | 进程正在提供 HTTP 服务 |
| `GET /readyz` | 进程正常**且** PostgreSQL 有响应 |
| `GET /metrics` | 聚合计数器，需要运维凭据 |

```bash
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS -H "Authorization: Bearer <admin_token>" http://127.0.0.1:8080/metrics
```

负载均衡与容器健康检查请使用 `/readyz`。`/healthz` 在数据库故障时仍返回成功，不能替代就绪检查。

`scripts/check_status.sh` 提供更完整的集群巡检，并读取 `CONTROLLER_URL`。

## 监控

控制器暴露一组 Prometheus 格式的计数器：

| 指标 | 含义 |
| --- | --- |
| `gpuops_controller_reports_total` | 已接受的 Agent 上报数 |
| `gpuops_controller_reports_duplicate_total` | 被判定为重复而拒绝的上报数 |
| `gpuops_controller_usage_records_total` | 写入的用量记录数 |
| `gpuops_controller_actions_notify_total` | 下发的通知动作数 |
| `gpuops_controller_actions_block_user_total` | 下发的封禁动作数 |
| `gpuops_controller_actions_unblock_user_total` | 下发的解封动作数 |
| `gpuops_controller_actions_kill_process_total` | 下发的进程终止动作数 |
| `gpuops_controller_actions_set_cpu_quota_total` | 下发的 CPU 配额动作数 |
| `gpuops_controller_actions_set_memory_limit_total` | 下发的内存限制动作数 |
| `gpuops_controller_last_report_unix` | 最近一次上报的 Unix 时间戳 |

抓取需要凭据，Prometheus 任务示例：

```yaml
scrape_configs:
  - job_name: gpu-ops
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/gpuops-admin-token
    static_configs:
      - targets: ["controller.example.org:8080"]
```

请对 `gpuops_controller_last_report_unix` 落后设置告警：全集群范围的停止上报意味着计费与策略执行已经悄悄中断。

## 例行巡检

**每天。** 确认所有预期节点都在上报（管理端 → 节点，可查看 Agent 健康与最近上报时间）。查看安全事件。

**每周。** 处理待审核的注册与账号绑定申请。在管理端 → 集群总览查看备份与恢复演练状态。

**每月。** 确认月度积分重置按配置执行。核对价格与实际硬件清单。复核管理员账号及其权限位。

## 升级

先做数据库备份。迁移只向前执行，回滚意味着恢复备份。

### 源码方式

```bash
git fetch --tags
git checkout <tag>
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
go build -o /opt/gpu-controller/controller ./controller
sudo systemctl restart gpu-controller
curl -fsS http://127.0.0.1:8080/readyz
```

### Docker 方式

```bash
git fetch --tags && git checkout <tag>
docker compose --profile full build controller
docker compose --profile full up -d controller
docker compose --profile full ps
```

### Node Agent

先升级控制器再升级 Agent。先滚动一个节点，确认上报正常后再继续。`scripts/deploy_installed_nodes_only.sh` 只对已安装 Agent 的节点执行滚动。

### 版本偏差

控制器接受较旧 Agent 的上报。不要用较新的 Agent 对接较旧的控制器：Agent 可能发送控制器无法识别的字段。

## 数据库维护

控制器启动时按顺序自动执行 `database/migrations/` 中的迁移。任一迁移失败都会导致启动失败，控制器不会带着半迁移的 schema 运行。

用量记录会持续增长。请在管理端 → 进程审计中配置保留策略，或调用保留策略接口。`POST /api/admin/usage/delete-range` 会不可逆地删除某个日期区间——执行前先备份，并用区间估算接口确认范围。

## 密钥轮换

**Agent token。** 把当前 token 放入 `agent_legacy_tokens`，把新值设为 `agent_token`，重启控制器，向所有节点下发新 token，然后清空 `agent_legacy_tokens` 并再次重启。

**Admin token。** 修改 `admin_token` 后重启。所有持有旧值的脚本都必须同步更新，没有轮换窗口。

**Auth secret。** 修改 `auth_secret` 会立即让所有浏览器会话失效，全部用户需要重新登录。这也是强制全局登出的最快方式。

**数据库密码。** 先在 PostgreSQL 中修改，再更新 DSN（或 `GPUOPS_DATABASE_DSN`），然后重启。

## 事故处理

**Agent 停止上报。** 先检查 `/readyz`。若数据库故障，先修复数据库——没有数据库控制器无法接受上报。上报中断期间策略执行暂停，该时段的用量不会被记录，也无法事后补齐。

**扣费异常。** 设置 `dry_run: true` 并重启，先停止扣费再排查。调整余额前，请把 `usage_records` 与节点实际情况比对。

**怀疑凭据泄露。** 按上文轮换对应 token。轮换 `auth_secret` 同时会强制所有人登出。检查安全事件与管理员审计记录。

**误触发大范围策略执行。** 在管理端 → 平台用户中解封受影响用户，然后设置 `dry_run: true`，直到策略修正完毕。

## 相关文档

- [备份与 HA](backup-and-ha.md)
- [故障排查](troubleshooting.md)
- [安全模型](security.md)
