# 安全模型

[English](../security.md) | **简体中文**

本文说明 GPU Ops 保护什么、不保护什么，以及运维方需要自行提供的加固基线。漏洞私下报告方式见 [SECURITY.zh-CN.md](../../SECURITY.zh-CN.md)。

## GPU Ops 的前提假设

GPU Ops 是你自己机器的控制面，它假设：

- 控制器运行在你可控网络中的可信主机上；
- 计算节点与控制器由同一团队管理；
- PostgreSQL、TLS 证书、DNS 和备份由你负责运维；
- 用户拥有计算节点的 shell 权限，但不被视为拥有 root 的攻击者。

GPU Ops 不是多租户隔离边界。能够在计算节点提权到 root 的用户，可以绕过节点本地的所有策略执行。

## 信任边界

| 边界 | 持有者 | 被攻破的影响 |
| --- | --- | --- |
| `admin_token` | 运维人员、部署脚本 | 完整超级管理员 API 权限，绕过角色检查与 2FA |
| 管理员会话 | 浏览器 cookie | 受权限位与 CSRF 限制的管理操作 |
| `agent_token` / 每节点 token | 各计算节点的 root | 上报用量、读取该节点的 registry 列表、轮询动作 |
| `ha_token` | 备用控制器 | 读取 HA 状态与同步信息 |
| PostgreSQL 凭据 | 控制器主机 | 等同控制面失守：身份、积分、SMTP 设置全部暴露 |
| 计算节点 root | 节点管理员 | 完全控制该节点，包括 Agent 及其 token |

`admin_token` 是系统中权限最高的凭据。它被设计为绕过角色权限与两步验证，因为在没有任何管理员账号时也必须能够引导和恢复。请把它当作 root 口令：存放在仓库之外、限制文件权限，怀疑泄露后立即轮换。

## 鉴权机制

- **浏览器会话。** 登录后下发 HMAC-SHA256 签名、HttpOnly、`SameSite=Lax` 的 cookie，签名覆盖用户名、角色、过期时间和每会话 nonce。角色与权限在每次请求时从数据库重新解析，因此权限变更立即生效。
- **CSRF。** 使用会话 cookie 的所有非 GET 请求必须携带 `X-CSRF-Token`，其值来自 `GET /api/auth/me`。
- **两步验证。** 支持按账号启用 TOTP，管理员可以强制要求。
- **token 比较。** 所有 token 校验（admin、agent、legacy、每节点、HA）均使用常量时间比较。
- **密码策略。** 12–64 字符，必须同时包含大写字母、小写字母、数字和特殊字符，注册、重置和修改密码时统一强制。

## 网络暴露面

控制器有两个监听：

| 监听 | 默认端口 | 内容 |
| --- | --- | --- |
| Web | `8080` | Web UI、用户与管理员 API、登录、注册 |
| 内部 | `8081`（可选） | Agent 指标、节点动作、registry 列表、HA 状态 |

`internal_listen_addr` 为空时，内部路由挂载在 Web 监听上。这对评估很方便，但意味着同一个对外端口同时承载浏览器与 Agent 流量。生产部署应设置 `internal_listen_addr`、提供证书与私钥，并用防火墙把该监听限制在节点与控制器网络内。

Web 监听自身不终止 TLS，请置于反向代理之后，并在启用 HTTPS 后设置 `cookie_secure: true`。

## 接口暴露一览

| 路由 | 所需凭据 |
| --- | --- |
| `GET /healthz`、`GET /readyz` | 无 |
| `GET /metrics` | `admin_token`、Agent token 或管理员会话 |
| `GET /api/public/settings`、登录、注册、密码重置 | 无（受注册策略限流） |
| `GET /api/users/:username/balance`、`.../usage` | `admin_token`、Agent token、管理员会话，或该用户本人的会话 |
| `POST /api/registry/bind-claim` | 控制器签发的一次性绑定 challenge token |
| `/api/user/*` | 会话 cookie |
| `/api/admin/*` | `admin_token`，或具备对应权限位的管理员/高级用户会话 |
| `POST /api/metrics`、`/api/node/*`、`/api/registry/*` | `X-Agent-Token` |
| `GET /api/ha/status` | `X-HA-Token` |

## 本次发布已修复的问题

以下问题是在准备开源发布时发现的，已在代码中修复：

1. **任意用户余额与用量可匿名读取。** `GET /api/users/:username/balance` 与 `GET /api/users/:username/usage` 此前没有任何鉴权。任何能访问 Web 监听的客户端都可以枚举用户名，读取余额、账号状态和按节点的用量历史。现在两个接口都需要凭据，非管理员会话只能读取自己的记录。
2. **读接口造成匿名写入。** 余额处理函数调用了 upsert 辅助方法，对任何没见过的用户名都会创建一行 `users` 记录，未认证调用方因此可以无限插入数据行。现在改为只读查询，用户不存在时返回 `404`。
3. **监控指标未鉴权暴露。** `GET /metrics` 此前公开。其内容只有聚合计数器，不含用户名或主机名，但仍然泄露集群活跃度与策略执行量。现在需要运维凭据。
4. **token 比较非常量时间。** admin token 与全部 agent token 使用 `==` 比较，会在首个不同字节处短路返回，此前只有 HA token 使用常量时间比较。现在全部改为常量时间比较，且候选集合不提前退出。
5. **缺少 `SameSite` 属性。** 会话 cookie 此前仅依赖 CSRF token 校验，现在额外带上 `SameSite=Lax`。
6. **HTTP 框架运行在 debug 模式。** Gin 以 debug 模式启动，会打印完整路由表与调试告警。现在默认 release 模式，调试时显式设置 `GIN_MODE=debug`。
7. **Compose 文件硬编码数据库密码。** 开发密码以字面量提交在仓库中。现在改为从 `.env` 读取，未设置时 Compose 直接失败。

## 已知残余风险

以下是被接受的设计行为，而非缺陷。请自行判断是否可接受。

- **会话在过期前无法吊销。** 会话 cookie 是自包含的，修改密码或禁用账号不会让已签发的 cookie 失效；角色与权限变更会立即生效，因为它们每次都从数据库读取。如果这一点重要，请降低 `session_hours`。
- **`admin_token` 绕过角色与 2FA。** 如上所述，且审计日志无法区分是哪位运维人员使用了它。
- **`cookie_secure` 默认为 `false`。** 这是本地 HTTP 评估所必需的。任何 HTTPS 部署都应设置为 `true`。
- **`POST /api/registry/bind-claim` 无需 Agent token 即可访问。** 它改用一次性 challenge token 鉴权，因为调用方是节点上的用户进程。其限流依赖 challenge 有效期，而不是来源 IP。
- **单端口模式会把内部路由暴露在公开监听上。** 这些路由仍然需要 Agent token，但攻击面更大。生产环境请使用 `internal_listen_addr`。
- **Node Agent 以 root 运行。** 策略执行需要 cgroup、systemd 和进程控制权限。被攻破的控制器可以指挥 Agent 终止进程、修改限制。
- **SMTP 凭据存储在 PostgreSQL** 中作为运行时设置，数据库备份需同等保护。
- **登录接口没有限流。** 注册有防滥用措施（域名白名单、一次性邮箱拦截、可选 Turnstile），密码登录没有。请在反向代理上配置限流。

## 运维加固基线

1. 用 `openssl rand -hex 32` 生成三个互不相同的 `agent_token`、`admin_token` 和 `auth_secret`。只要其中任何一个仍是占位值或长度不足 16 字符，首次 Setup 就拒绝保存。
2. 在反向代理终止 TLS，并设置 `cookie_secure: true`。
3. 配置 `internal_listen_addr` 及证书与私钥，并用防火墙限制到节点网络。
4. 用 `agent_node_tokens` 为每个节点分配独立 token，待全部节点上报正常后设置 `agent_node_token_enforce: true`。
5. 限制 PostgreSQL 的网络路径，并在 DSN 中要求 TLS。
6. 在计费经过验证之前保持 `dry_run: true`。
7. 配置加密且独立存放的备份，并演练恢复流程，见[备份与 HA](backup-and-ha.md)。
8. 为每个管理员账号启用两步验证。
9. 在代理层增加登录限流。
10. 启用强制策略前完成[上线检查清单](go-live-checklist.md)。

## 报告

请按 [SECURITY.zh-CN.md](../../SECURITY.zh-CN.md) 私下报告疑似漏洞。不要在公开 Issue 中附带 token、私钥、主机清单、数据库转储或用户数据。
