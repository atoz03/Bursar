# 故障排查

[English](../troubleshooting.md) | **简体中文**

按现象出现的位置分组。请从 `/readyz` 开始——控制器侧的问题大多是数据库问题。

## 控制器无法启动

**`配置校验失败`。** `Validate()` 拒绝了配置，错误信息会指出具体字段。常见原因：`agent_token`、`admin_token` 或 `auth_secret` 为空；`session_hours > 0` 时 `auth_secret` 短于 16 字符；`limited_threshold` 不小于 `warning_threshold`；配置了 `internal_listen_addr` 却没有提供证书与私钥。

**`未找到默认配置文件`。** 控制器依次查找 `CONTROLLER_CONFIG`、`../config/controller.yaml`、`config/controller.yaml`。请显式传入 `--config`。

**`连接数据库失败`。** 检查 DSN、PostgreSQL 是否运行、凭据是否匹配。在 Docker 中，DSN 的主机名是服务名 `postgres`，不是 `127.0.0.1`。

**`数据库迁移失败`。** 某个迁移执行出错，控制器拒绝带着半迁移的 schema 启动。请阅读错误信息，必要时从备份恢复，不要通过删除迁移记录强行启动。

**环境变量覆盖不生效。** 只有 `GPUOPS_*` 变量会被读取，且必须非空。空值等同于未设置，会保留 YAML 中的值。`GPUOPS_DRY_RUN` 和 `GPUOPS_COOKIE_SECURE` 必须是布尔词（`true`/`false`/`1`/`0`/`yes`/`no`/`on`/`off`），其它取值会导致启动失败。

## 就绪与健康检查

| 现象 | 含义 |
| --- | --- |
| `/healthz` 正常、`/readyz` 返回 503 | 进程正常，PostgreSQL 不可达 |
| 两者都失败 | 控制器未监听，或端口不对 |
| `/readyz` 正常但界面空白 | 未找到或未构建 `web/dist` |

默认端口现在是 `8080`。在此变更之前创建的部署使用 `60039`——如果之前可用的 URL 突然无响应，请检查 `listen_addr`。

## Web 界面

**页面空白或所有路由 404。** 控制器只有找到构建产物才会托管界面。请执行 `pnpm --dir web build`，或显式设置 `web_dir`。自动探测按工作目录依次尝试 `../web/dist` 和 `web/dist`，因此从非预期目录启动控制器会静默关闭界面托管。

**`pnpm dev` 下 API 调用失败。** Vite 把 `/api`、`/metrics` 和 `/healthz` 代理到 `127.0.0.1:8080`，控制器必须监听在那里。

**登录后立刻掉线。** 在明文 HTTP 上设置了 `cookie_secure: true`，浏览器会丢弃该 cookie。要么提供 HTTPS，要么本地调试时设为 `false`。

**每次写操作都返回 `csrf_required`。** 客户端必须携带来自 `GET /api/auth/me` 的 `X-CSRF-Token`。使用不带该头的工具重放会话 cookie 时也会出现这个错误。

**所有人同时被登出。** `auth_secret` 发生了变化。会话用它签名，修改后全部失效。

## 鉴权与访问

**带 token 访问 `/api/admin/*` 返回 `401 unauthorized`。** 请求头必须严格是 `Authorization: Bearer <admin_token>`，且 token 需与配置逐字节一致，包括长度。

**高级用户返回 `403 forbidden`。** 账号鉴权通过，但缺少该路由所需的权限位。请在管理端 → 高级用户中授予。

**`/api/users/<name>/balance` 返回 `401`。** 该接口现在需要凭据：`admin_token`、Agent token、管理员会话，或该用户本人的会话。节点侧辅助脚本从 `GPUOPS_QUERY_TOKEN` 或 `/etc/gpu-ops/query-token` 读取凭据。

**余额查询返回 `404 用户不存在`。** 该用户还没有记录行。这个接口不再作为副作用创建用户；账号开通或首次被 Agent 上报后，记录才会出现。

**抓取 `/metrics` 返回 `401`。** 采集端必须发送 `Authorization: Bearer <admin_token>` 或 `X-Agent-Token`。

## Agent

**节点始终不出现。** 检查节点到 `CONTROLLER_URL` 的连通性，再查看 Agent 日志中的 `401`。开启 `agent_node_token_enforce` 后，token 必须是该 `NODE_ID` 注册的那一个。

**改名后节点消失。** `node_id` 是身份键，改名等于新建一个节点，旧历史会成为孤儿数据。

**上报正常但没有扣费。** 检查 `dry_run`。dry-run 模式下会记录用量并计算成本，但不扣减余额。

**扣费明显偏高或偏低。** 控制器用上报中的 `interval_seconds` 折算，缺失时回退到 `sample_interval_seconds`。Agent 实际周期与配置不一致会等比例放大或缩小每一笔扣费。另请确认 `resource_prices` 中的 GPU 型号与节点上报一致，否则会使用兜底的 `default_price_per_minute`。

**重复扣费。** 去重以 `report_id` 为键，出现重复几乎总是意味着一个节点上运行了两个 Agent 实例。

## 数据库

**`too many connections`。** 其它客户端占用了连接池，或多个控制器指向同一个数据库。一个数据库只应有一个控制器写入。

**查询逐渐变慢。** `usage_records` 会无限增长，请在管理端 → 进程审计中配置保留策略。

**删除用量区间后磁盘空间没有释放。** PostgreSQL 会在内部复用这些空间。若需要归还给文件系统，请执行 `VACUUM`。

## Docker

**`set POSTGRES_PASSWORD in .env`。** 缺少密钥时 Compose 拒绝启动。执行 `cp .env.example .env` 并填写全部值。

**控制器容器反复重启。** 执行 `docker compose --profile full logs controller`，通常是配置值被拒绝或数据库不可达。

**健康检查始终不通过。** 检查执行 `/app/controller --healthcheck`，它按 `GPUOPS_LISTEN_ADDR` 的端口探测 `/readyz`。覆盖该变量后检查也会随之变化。`start_period` 设为 30 秒以容纳迁移时间；首次启动时迁移积压过多可能超过这个窗口。

**端口被占用。** 修改 `.env` 中的 `CONTROLLER_PUBLISH`。它只改变宿主机侧端口，容器内仍监听 8080。

## SSH Guard

**整台节点无人能登录。** Guard 会拒绝未登记账号，而 `SSH_GUARD_FAIL_OPEN=0` 在控制器不可达时会拒绝所有人。请使用控制台访问、设置 `SSH_GUARD_FAIL_OPEN=1`，或把账号加入 `SSH_GUARD_EXCLUDE_USERS`。

**已登记用户仍被拦截。** 节点使用每 `SSH_GUARD_SYNC_INTERVAL` 同步一次的缓存列表。请检查 registry 接口对该节点返回的用户是否符合预期。

## 收集诊断信息

```bash
curl -fsS http://127.0.0.1:8080/readyz
sudo journalctl -u gpu-controller -n 200 --no-pager
sudo journalctl -u gpu-node-agent -n 200 --no-pager
docker compose --profile full logs --tail 200 controller
bash scripts/check_status.sh
```

在公开 Issue 中分享这些内容前，请先脱敏 token、DSN、主机名和用户名。
