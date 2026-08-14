# GPU Ops

GPU Ops 是一个面向共享 GPU 服务器的轻量运维平台。它保留用户直接通过 SSH 使用计算节点的习惯，由控制器统一完成资源监控、积分计费、配额限制、账号映射和安全审计。

## 主要能力

- Go 节点 Agent：采集 GPU、CPU、内存、磁盘、登录会话和安全事件。
- Go 控制器：提供管理 API、计费、策略下发、账号开通、邮件通知和 PostgreSQL 持久化。
- Vue 3 管理端：包含运营看板、节点管理、积分管理、注册审核、账号映射和系统设置。
- 首次运行向导：管理员首次登录后配置平台名称、注册邮箱域名、SSH 入口、资源价格、用户准则、SMTP 和可选 HA。
- 安全能力：节点独立 token、Web 会话、CSRF、2FA、Turnstile、SSH 名单与主机安全事件。
- 运维能力：主备同步、加密备份、恢复验证和批量 Agent 部署。

## 目录结构

```text
gpu-ops/
├── controller/       # 控制器与 API
├── node-agent/       # 计算节点 Agent
├── web/              # Vue 3 前端
├── database/         # Schema 与增量迁移
├── config/           # 配置示例
├── scripts/          # 构建、安装、部署、HA 与备份脚本
├── systemd/          # systemd 服务模板
└── docs/             # 管理、用户与 API 文档
```

## 环境要求

- Go 1.26.6
- Node.js 24 与 pnpm 10
- PostgreSQL 18
- Linux 计算节点；GPU 采集依赖 `nvidia-smi`

## 快速启动

### 1. 准备配置

复制示例配置，并生成三组不同的随机密钥：

```bash
cp config/controller.yaml config/controller.local.yaml
openssl rand -hex 32  # agent_token
openssl rand -hex 32  # admin_token
openssl rand -hex 32  # auth_secret
```

将结果分别写入 `config/controller.local.yaml`。生产环境还应修改数据库账号密码，并根据部署方式配置 HTTPS、内部监听和共享目录。

真实配置、私钥、节点映射与生成的 token 文件已经被 `.gitignore` 排除，不要提交到版本库。

### 2. 启动数据库

开发环境可以使用项目内的 Compose 配置：

```bash
docker compose up -d postgres
```

### 3. 构建前端

```bash
cd web
corepack enable
pnpm install --frozen-lockfile
pnpm build
cd ..
```

### 4. 启动控制器

```bash
cd controller
go run . --config ../config/controller.local.yaml
```

控制器默认监听 `http://127.0.0.1:60039`。可用以下命令检查状态：

```bash
curl -fsS http://127.0.0.1:60039/readyz
```

### 5. 创建首个管理员

首次创建管理员必须使用配置中的 `admin_token`：

```bash
curl -fsS -X POST http://127.0.0.1:60039/api/admin/bootstrap \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <admin_token>' \
  -d '{"username":"admin","password":"<strong-password>"}'
```

随后打开 `http://127.0.0.1:60039/login` 登录。首个管理员会被自动引导到“系统设置”页面；完成必填配置前，公开注册入口保持关闭。

## 首次设置页面

Setup 页面集中处理运行期配置：

1. 平台名称、允许注册的邮箱域名和统一 SSH 入口。
2. GPU/CPU 价格与用户准则。
3. 可选 SMTP 邮件通知。
4. 可选主备同步，以及启动配置安全检查。

数据库 DSN、监听地址、`agent_token`、`admin_token`、`auth_secret` 和 TLS 私钥属于启动前配置。页面只显示它们是否安全就绪，不会将值返回浏览器。

邮箱域名列表留空时允许任意格式合法且未被一次性邮箱黑名单拦截的邮箱；内部部署可以填写一个或多个组织域名。

## 接入计算节点

在计算节点上构建并运行 Agent：

```bash
cd node-agent
go build -o gpu-node-agent .
NODE_ID=node-01 \
CONTROLLER_URL=https://controller.example.org:60040 \
AGENT_TOKEN=<agent_token> \
./gpu-node-agent
```

正式安装可使用：

```bash
NODE_ID=node-01 \
CONTROLLER_URL=https://controller.example.org:60040 \
AGENT_TOKEN=<agent_token> \
bash scripts/install_agent_local.sh
```

生产环境建议启用独立内部监听、TLS 和每节点独立 token。批量部署脚本默认读取本地 `my_ssh_keys/server_ssh_map.csv`，该目录不会进入 Git。

## 高可用与备份

单控制器部署保持 `ha_enabled: false`。主备部署需要显式提供对端主机、SSH 用户、私钥路径和同步脚本路径；仓库内不包含任何真实基础设施参数。

相关脚本：

- `scripts/deploy_dr_standby.sh`：分发并引导 standby。
- `scripts/ha_sync_worker.sh`：执行版本、前端和数据库同步。
- `scripts/install_ha_vip_local.sh`：安装 Keepalived VIP。
- `scripts/install_backup_local.sh`：安装 restic 加密备份和恢复验证。

备份仓库必须位于独立磁盘或异机，不能与控制器数据共用故障域。

## 开发与验证

```bash
# Go
(cd controller && go test ./...)
(cd node-agent && go test ./...)

# 前端
(cd web && pnpm install --frozen-lockfile && pnpm build)

# Shell 语法
bash -n scripts/*.sh
```

CI 会执行控制器测试、Agent 测试和前端构建。

## 安全文档

- [上线检查清单](docs/go-live-checklist.md)
- [管理员指南](docs/admin-guide.md)
- [用户指南](docs/user-guide.md)
- [API 参考](docs/api-reference.md)

提交安全问题时，请避免在公开 Issue 中粘贴 token、私钥、数据库 DSN、真实主机地址或用户数据。
