# 快速入门

[English](../getting-started.md) | **简体中文**

本文用 Docker 中的 PostgreSQL 加上源码运行的控制器，搭建一个本地评估环境。如果你希望控制器也运行在容器里，请改用 [Docker 部署](installation.md#docker-部署)——准备好密钥后只需一条 `docker compose` 命令。

## 环境要求

- Go 1.26.6
- Node.js 20.20 或更高版本
- pnpm 10.28.2
- Docker Compose
- OpenSSL、curl 和 Git

验证工具链：

```bash
go version
node --version
pnpm --version
docker compose version
```

## 1. 准备仓库

```bash
git clone https://github.com/atoz03/gpu-ops.git
cd gpu-ops
cp .env.example .env
cp config/controller.yaml config/controller.local.yaml
```

在 `.env` 中设置 `POSTGRES_PASSWORD`，否则 Compose 不会启动。

生成三个不同的密钥：

```bash
openssl rand -hex 32
openssl rand -hex 32
openssl rand -hex 32
```

分别写入 `agent_token`、`admin_token` 和 `auth_secret`，不要复用同一个值。

本地评估时，示例 DSN 与 Compose 凭据已经匹配。请保持 `internal_listen_addr` 为空、`cookie_secure: false`、`ha_enabled: false`，并在用量记录确认无误前设置 `dry_run: true`。

## 2. 启动 PostgreSQL

```bash
docker compose up -d postgres
docker compose ps
```

等待容器状态变为 `healthy`。排查启动失败：

```bash
docker compose logs postgres
```

## 3. 构建 Web

```bash
corepack enable
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
```

从仓库目录启动时，控制器会自动探测 `web/dist`。

## 4. 运行控制器

```bash
go run ./controller --config config/controller.local.yaml
```

另开终端检查存活与数据库就绪状态：

```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS -H 'Authorization: Bearer <admin_token>' http://127.0.0.1:8080/metrics | head
```

`/metrics` 需要运维凭据；`/healthz` 和 `/readyz` 不需要。

就绪时的预期响应：

```json
{"ok":true,"database":true}
```

## 5. 创建首个管理员

只有管理员表为空时，bootstrap 接口才可用：

```bash
curl -fsS -X POST http://127.0.0.1:8080/api/admin/bootstrap \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <admin_token>' \
  -d '{"username":"admin","password":"<strong-password>"}'
```

密码需为 12–64 位，且同时包含大写字母、小写字母、数字和特殊字符。

## 6. 完成 Setup

打开 <http://127.0.0.1:8080/login>。首次登录后管理员会被重定向到 `/admin/setup`，需要配置：

1. 平台名称、允许注册的邮箱域名，以及可选的共享 SSH 入口；
2. 资源价格与用户准则；
3. 可选 SMTP；
4. 可选 HA 与必填启动检查。

Setup 完成前，公开注册保持关闭。字段行为见[首次 Setup](first-run-setup.md)。

## 7. 可选：开发用 Agent

在 Linux 主机上手动构建并运行 Agent：

```bash
go build -o /tmp/gpu-node-agent ./node-agent
NODE_ID=node-01 \
CONTROLLER_URL=http://127.0.0.1:8080 \
AGENT_TOKEN='<agent_token>' \
/tmp/gpu-node-agent
```

这仅适用于 Agent 与控制器共处本地可信环境的场景。生产节点请按[安装指南](installation.md)和 [Node Agent 指南](node-agent.md)部署。

## 8. 停止评估环境

用 `Ctrl+C` 停止控制器，然后在不删除数据卷的前提下停止 PostgreSQL：

```bash
docker compose down
```

`docker compose down -v` 会永久删除开发数据库卷，只有在你明确需要一个干净数据库时才使用。

## 下一步

- [配置参考](configuration.md)
- [生产安装](installation.md)
- [安全模型](security.md)
- [故障排查](troubleshooting.md)
