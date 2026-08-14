# 首次管理员 Setup

[English](../first-run-setup.md) | **简体中文**

Setup 向导让实例相关的运行时选择显式化，同时把启动密钥挡在浏览器之外。

## 打开 Setup 之前

控制器必须已经具备：

- 可用的 PostgreSQL 连接；
- 非占位值的 `agent_token`；
- 非占位值的 `admin_token`；
- 足够强的 `auth_secret`；
- 若启用 `internal_listen_addr`，还需有效的内部 TLS 证书与私钥。

这些值始终保存在启动 YAML 或密钥管理系统中。Setup 只返回就绪状态和非敏感的路径/地址。

## 创建首个管理员

使用启动配置中的 `admin_token`，只执行一次：

```bash
curl -fsS -X POST https://ops.example.org/api/admin/bootstrap \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <admin_token>' \
  -d '{"username":"admin","password":"<strong-password>"}'
```

管理员存在后 bootstrap 会被拒绝。请妥善保存该 Bearer token 的恢复途径，不要嵌入前端代码或浏览器存储。

## 向导流程

首个管理员登录后，路由会持续重定向到 `/admin/setup`，直到 Setup 完成。

### 1. 平台信息

- **平台名称：** 1–80 字符，用于界面与邮件主题。
- **注册邮箱域名：** 可选列表，不含 `@`。留空表示接受任何合法且非一次性的域名。
- **SSH 主机：** 可选主机名或 IP，用于开号说明展示。不要包含协议、用户名、端口或路径。

### 2. 价格与用户策略

- 设置已知 GPU 型号的价格。
- 设置 `CPU_CORE` 用于核分钟计费。
- 发布应用内展示的用户准则。

未知 GPU 型号会回退到启动配置中的 `default_price_per_minute`。正式扣费前请先验证型号匹配。

### 3. 邮件

邮件为可选项。启用时需提供 SMTP 主机、端口、用户、密码、发件邮箱与发件人名称。在禁用邮件的状态下保存会清除已存储的 SMTP 凭据。Setup 完成后请在邮件设置页面发送一封测试邮件。

### 4. HA 与就绪检查

单控制器部署保持 HA 关闭。启用 HA 需要显式配置角色、对端、SSH、脚本与同步选项，详见[备份与 HA](backup-and-ha.md)。

必填就绪检查必须全部通过，Setup 才能完成。内部 TLS 被标记为可选检查，因为单端口开发模式受支持，但生产环境仍然推荐启用。

## 注册行为

Setup 完成前，公开注册返回 `setup_required`。完成后，注册遵循配置的域名与一次性邮箱策略。修改允许域名列表只影响新注册，不影响已有账号。

## 重新打开 Setup

超级管理员可以再次访问 `/admin/setup`。保存操作可重复执行，但会修改正在生效的运行时设置。价格、邮件和 HA 的变更应当按生产变更对待，并立即验证。

## API 访问

相关接口：

- `GET /api/public/settings` — 公开，返回非敏感的平台信息与 Setup 状态；
- `GET /api/admin/setup` — 超级管理员；
- `POST /api/admin/setup` — 超级管理员，校验并保存完整表单；
- `POST /api/admin/bootstrap` — 一次性创建管理员。

浏览器请求使用签名会话与 CSRF token，运维脚本可使用管理 Bearer token，详见 [API 参考](api-reference.md)。

## 失败恢复

- 就绪检查失败：修正启动 YAML 并重启控制器。
- SMTP 校验失败：先禁用邮件完成 Setup，之后单独配置。
- HA 配置不完整：保持 HA 关闭，不要保存占位值。
- 管理员密码丢失：按数据库与运维恢复策略处理，不要为了重新触发 bootstrap 而随意删除管理员记录。
