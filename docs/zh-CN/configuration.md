# 配置参考

[English](../configuration.md) | **简体中文**

控制器启动时读取 YAML 配置，通过 `--config <path>` 或 `CONTROLLER_CONFIG` 指定文件。启动配置的变更需要重启服务；Setup 中保存的运行时设置存放在 PostgreSQL 中。

请把 `config/controller.yaml` 当作安全示例，真实值放在 `config/controller.local.yaml` 或密钥管理系统中。

## 环境变量覆盖

少量配置项可以通过环境变量提供，使容器部署无需携带含密钥的 YAML 文件。覆盖在 YAML 解析之后、校验之前生效。

| 变量 | 覆盖的配置项 |
| --- | --- |
| `GPUOPS_LISTEN_ADDR` | `listen_addr` |
| `GPUOPS_INTERNAL_LISTEN_ADDR` | `internal_listen_addr` |
| `GPUOPS_INTERNAL_TLS_CERT_FILE` | `internal_tls_cert_file` |
| `GPUOPS_INTERNAL_TLS_KEY_FILE` | `internal_tls_key_file` |
| `GPUOPS_DATABASE_DSN` | `database_dsn` |
| `GPUOPS_AGENT_TOKEN` | `agent_token` |
| `GPUOPS_ADMIN_TOKEN` | `admin_token` |
| `GPUOPS_AUTH_SECRET` | `auth_secret` |
| `GPUOPS_HA_TOKEN` | `ha_token` |
| `GPUOPS_MIGRATION_DIR` | `migration_dir` |
| `GPUOPS_WEB_DIR` | `web_dir` |
| `GPUOPS_COOKIE_SECURE` | `cookie_secure` |
| `GPUOPS_DRY_RUN` | `dry_run` |

未设置或为空的变量会被忽略，因此空的环境变量不会清空 YAML 中的值。两个布尔变量接受 `true`/`false`、`1`/`0`、`yes`/`no` 或 `on`/`off`，其它取值会导致启动失败。表格之外的配置项只能来自 YAML。

另有两个环境变量影响进程行为：`CONTROLLER_CONFIG` 选择配置文件，`GIN_MODE` 覆盖 HTTP 框架模式（默认 `release`）。

## 必填启动配置

| 配置项 | 用途 | 生产建议 |
| --- | --- | --- |
| `listen_addr` | Web/API 监听地址 | 置于 HTTPS 代理之后，尽量绑定回环或内网地址 |
| `database_dsn` | PostgreSQL 连接串 | 专用角色、启用 TLS、最小网络暴露 |
| `agent_token` | 全局 Agent 鉴权 | 独立随机密钥；逐步迁移到每节点 token |
| `admin_token` | 引导与管理 Bearer 访问 | 独立随机密钥；不要在浏览器中使用 |
| `auth_secret` | 会话 cookie 签名 | 至少 16 字符，建议 32 字节随机值 |

Setup 会把明显的占位值与过短的密钥判定为未就绪。

## 监听与 TLS

| 配置项 | 默认/示例 | 说明 |
| --- | --- | --- |
| `listen_addr` | `0.0.0.0:8080` | HTTP Web 监听，TLS 通常由反向代理终止 |
| `internal_listen_addr` | 空 | 设置后启用独立的 Agent/registry/HA 监听 |
| `internal_tls_cert_file` | 空 | 启用内部监听时必填 |
| `internal_tls_key_file` | 空 | 启用内部监听时必填，注意文件权限 |

内部监听关闭时，内部 API 出于兼容保留在 Web 监听上；启用后会从 Web 监听移除。

## Agent 鉴权与轮换

| 配置项 | 含义 |
| --- | --- |
| `agent_token` | 全局 token，也是迁移期的兜底 |
| `agent_legacy_tokens` | 临时接受的旧全局 token |
| `agent_node_tokens` | `node_id: token` 映射 |
| `agent_node_token_enforce` | 节点 token 不匹配时直接拒绝 |

安全的每节点迁移流程：

1. 在 enforce 为 `false` 时填充 `agent_node_tokens`；
2. 下发每节点 token，确认全部心跳正常；
3. 设置 `agent_node_token_enforce: true` 并重启；
4. 回滚窗口结束后移除过期的全局/legacy token。

## 会话与注册安全

| 配置项 | 含义 |
| --- | --- |
| `session_hours` | 浏览器会话有效期，`0` 表示禁用会话，最大 `720` |
| `cookie_secure` | 仅通过 HTTPS 发送会话 cookie，生产环境应开启 |
| `turnstile_site_key` | Cloudflare Turnstile 公钥 |
| `turnstile_secret_key` | Cloudflare Turnstile 密钥 |
| `turnstile_expected_hostnames` | 校验时接受的浏览器 hostname |

Turnstile 的两个密钥必须同时配置，hostname 不能包含协议、端口或路径。未启用 Turnstile 时使用内置的本地验证流程。

## 计费与策略执行

| 配置项 | 含义 |
| --- | --- |
| `warning_threshold` | 进入预警状态的余额阈值，必须为正数 |
| `limited_threshold` | 进入受限状态的余额阈值，必须小于预警阈值 |
| `cpu_price_per_core_minute` | CPU 核分钟兜底价格，数据库中的 `CPU_CORE` 优先 |
| `sample_interval_seconds` | 兜底上报周期，1–600 秒 |
| `enable_cpu_control` | 是否允许下发 CPU 限制动作 |
| `cpu_limit_percent_limited` | 受限状态的 CPU 配额，1–100 |
| `cpu_limit_percent_blocked` | 封禁状态的 CPU 配额，1–100 |
| `overdraft_memory_limit_gb` | 欠费后的内存上限，`0` 表示关闭 |
| `kill_grace_period_seconds` | 进入封禁状态后下发 kill 前的宽限期 |
| `dry_run` | 记录用量但不扣减积分 |
| `default_balance` | 新观测到的用户的初始余额 |
| `default_price_per_minute` | GPU 分钟兜底价格 |

PostgreSQL 中配置的价格优先于 YAML 兜底值。请在 dry-run 模式下验证型号匹配与成本。

## SMTP 默认值

`smtp_host`、`smtp_port`、`smtp_user`、`smtp_pass`、`from_email` 和 `from_name` 用于初始化邮件行为。管理员可以在 Setup 或邮件设置页面中修改。作为运行时设置存储的 SMTP 密码依靠数据库访问控制与备份保护，应用层不做额外加密。

把全部 SMTP 身份字段留空即可禁用邮件功能，此时密码找回与邮箱验证无法通过邮件完成。

## 文件与目录

| 配置项 | 含义 |
| --- | --- |
| `migration_dir` | 覆盖迁移目录，留空时自动探测仓库路径 |
| `web_dir` | 覆盖已编译 Web 目录，留空时自动探测 `../web/dist` |
| `backup_status_file` | 备份任务写入的 JSON 状态 |
| `backup_verify_status_file` | 恢复演练写入的 JSON 状态 |
| `shared_node_root` | 服务端每节点共享工作目录根路径 |
| `shared_cluster_root` | 服务端集群级共享工作目录根路径 |

控制器服务账号只需要已启用功能所需的文件系统权限。启用共享目录创建时，请审阅自动生成的最小化 sudoers 规则。

## HA 启动配置

| 配置项 | 含义 |
| --- | --- |
| `ha_enabled` | 启用 HA 相关行为 |
| `ha_node` | 稳定的控制器标识 |
| `ha_role` | `primary` 或 `standby` |
| `ha_peer_url` | 对端控制器基础 URL |
| `ha_token` | 共享 HA 鉴权密钥 |

同步调度与对端 SSH 等细节属于 Setup 中保存的运行时配置，见[备份与 HA](backup-and-ha.md)。

## 运行时平台设置

Setup 把以下内容保存在 PostgreSQL 中：

- 平台展示名称；
- 允许注册的邮箱域名；
- 共享 SSH 入口主机名；
- Setup 完成状态；
- 用户准则；
- 资源价格；
- SMTP 设置；
- HA 同步设置。

注册域名列表为空时，接受任何语法合法且未被一次性邮箱策略拦截的邮箱。使用同一数据库的控制器共享这些运行时设置。

## 密钥处理

- 绝不提交真实配置。`.gitignore` 已排除 `config/*.local.yaml` 与本地环境文件。
- 在有受保护文件或密钥管理系统时，不要在多用户 shell 的命令行中传递密钥。
- Agent、admin、会话、HA、备份、数据库和 SMTP 凭据必须各不相同。
- 限制配置文件与密钥文件只对服务账号可读。
- 怀疑泄露后、以及仓库或基础设施移交后，立即轮换凭据。
- 恢复加密数据所需的密钥要与加密备份分开保存。

## 校验

控制器在开启监听前校验配置：

```bash
go run ./controller --config config/controller.local.yaml
```

已安装的服务可以这样查看失败原因：

```bash
journalctl -u gpu-controller -n 100 --no-pager
```
