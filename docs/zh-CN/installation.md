# 安装指南

[English](../installation.md) | **简体中文**

Bursar 支持两种运行控制器的方式：

| 方式 | 适用场景 |
| --- | --- |
| [**源码构建**](#源码部署) | 需要 systemd 托管、主机原生运行，并与 Node Agent 保持一致的生命周期。脚本与 HA 工具按这种方式设计。 |
| [**Docker**](#docker-部署) | 追求最快、可复现的启动，且已有容器运行环境。组件更少，但 HA 与备份脚本假设控制器安装在主机上。 |

Node Agent 永远不容器化，它需要主机上的 cgroup、systemd、SSH 和驱动访问权限，详见 [Node Agent](node-agent.md)。

---

# Docker 部署

## 1. 准备密钥

```bash
git clone https://github.com/atoz03/gpu-ops.git
cd gpu-ops
cp .env.example .env
```

填写 `.env`，每个密钥都必须是互不相同的随机值：

```bash
openssl rand -hex 24   # POSTGRES_PASSWORD
openssl rand -hex 32   # GPUOPS_AGENT_TOKEN
openssl rand -hex 32   # GPUOPS_ADMIN_TOKEN
openssl rand -hex 32   # GPUOPS_AUTH_SECRET
```

任意一项为空时 Compose 会直接失败。`.env` 已被 Git 忽略。

## 2. 启动服务栈

```bash
docker compose --profile full up -d --build
docker compose --profile full ps
```

控制器镜像是三阶段构建：pnpm 构建 Web UI、Go 构建静态二进制、运行阶段使用 distroless 基础镜像。启动时自动执行数据库迁移。

等待控制器状态变为 `healthy`，然后：

```bash
curl -fsS http://127.0.0.1:8080/readyz
```

## 3. 创建管理员并完成 Setup

```bash
curl -fsS -X POST http://127.0.0.1:8080/api/admin/bootstrap \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $(grep '^GPUOPS_ADMIN_TOKEN=' .env | cut -d= -f2)" \
  -d '{"username":"admin","password":"<strong-password>"}'
```

打开 <http://127.0.0.1:8080/login>，按[首次 Setup](first-run-setup.md) 完成配置。

## 4. 容器中的配置方式

镜像内置 `config/controller.yaml` 提供非敏感默认值，部署时必须修改的项通过环境变量提供：

| 变量 | 覆盖的配置项 |
| --- | --- |
| `GPUOPS_DATABASE_DSN` | `database_dsn` |
| `GPUOPS_AGENT_TOKEN` | `agent_token` |
| `GPUOPS_ADMIN_TOKEN` | `admin_token` |
| `GPUOPS_AUTH_SECRET` | `auth_secret` |
| `GPUOPS_HA_TOKEN` | `ha_token` |
| `GPUOPS_LISTEN_ADDR` | `listen_addr` |
| `GPUOPS_INTERNAL_LISTEN_ADDR` | `internal_listen_addr` |
| `GPUOPS_INTERNAL_TLS_CERT_FILE` | `internal_tls_cert_file` |
| `GPUOPS_INTERNAL_TLS_KEY_FILE` | `internal_tls_key_file` |
| `GPUOPS_MIGRATION_DIR` | `migration_dir` |
| `GPUOPS_WEB_DIR` | `web_dir` |
| `GPUOPS_COOKIE_SECURE` | `cookie_secure` |
| `GPUOPS_DRY_RUN` | `dry_run` |

未设置或为空的变量不会覆盖 YAML 中的值。若要修改其它配置（阈值、价格、CPU 控制、SMTP 等），请挂载自己的文件覆盖 `/app/config/controller.yaml`：

```yaml
    volumes:
      - ./config/controller.local.yaml:/app/config/controller.yaml:ro
```

## 5. 容器方式的生产注意事项

- 前置 HTTPS 反向代理，并设置 `GPUOPS_COOKIE_SECURE=true`。
- 保持 `POSTGRES_PUBLISH` 为 `127.0.0.1`，避免数据库对外可达。
- 启用内部监听时需挂载证书并设置 `GPUOPS_INTERNAL_LISTEN_ADDR`，且只向节点网络发布 `8081`。
- 容器数据卷 `pgdata` 不是备份，请配置[加密备份](backup-and-ha.md)。
- 健康检查执行 `/app/controller --healthcheck`，镜像内没有 shell。

---

# 源码部署

这是面向生产、基于 systemd 的 Linux 部署方式。请根据自身环境调整路径、用户、网络管控和证书。

## 部署前决策

- 控制器 DNS 名称与 HTTPS 反向代理；
- PostgreSQL 位置、凭据、TLS、保留策略与备份归属；
- 若使用内部监听，独立的内网 DNS/IP 与 TLS 证书；
- 控制器使用的非登录服务账号；
- 稳定的 node ID 与每节点 Agent token；
- 若启用 NFS 集成，共享工作目录的归属；
- 独立的加密备份仓库；
- 现在是否需要 HA。

## 支持的基线

- 控制器：systemd Linux、Go 1.26.6、Node.js 20.20+、pnpm 10.28.2、PostgreSQL 18
- 计算节点：systemd Linux；NVIDIA GPU 发现需要 `nvidia-smi`；建议 cgroup v2
- 构建机：Git、Go、Node.js、pnpm

脚本以 Ubuntu 22.04 类主机为目标，用于其它发行版前请先审阅。

## 1. 安装依赖

在专用的构建/控制器主机上：

```bash
sudo bash scripts/install_deps_ubuntu2204.sh
```

脚本可安装 Go、Node.js、pnpm 和 Docker。跳过已由平台管理的组件：

```bash
sudo env INSTALL_DOCKER=0 INSTALL_GO=0 bash scripts/install_deps_ubuntu2204.sh
```

全局依赖变更影响面较大；生产环境请先审阅脚本，并通过既有的配置管理流程固定版本。

## 2. 准备 PostgreSQL

尽量使用托管或独立管理的 PostgreSQL 实例。为 Bursar 创建专用数据库与角色，配置网络访问控制，得到类似这样的 DSN：

```text
postgres://gpuops:<password>@db.example.org:5432/gpuops?sslmode=require
```

仓库中的 Compose 文件只是本地开发便利工具，其凭据来自 `.env`，不是生产密钥。

## 3. 创建控制器配置

```bash
cp config/controller.yaml config/controller.local.yaml
chmod 600 config/controller.local.yaml
```

至少需要替换：

- `database_dsn`
- `agent_token`
- `admin_token`
- `auth_secret`
- 全部 HA 占位值，或保持 HA 关闭

用 `openssl rand -hex 32` 生成互相独立的密钥。`listen_addr`、TLS 拓扑与 `cookie_secure` 按[配置参考](configuration.md)设置。

## 4. 构建 Web

```bash
corepack enable
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
```

## 5. 安装控制器服务

按操作系统策略创建专用系统用户，然后从仓库运行本地安装脚本：

```bash
sudo env \
  CONFIG_PATH="$PWD/config/controller.local.yaml" \
  RUN_USER=gpuops \
  RUN_GROUP=gpuops \
  BUILD_WEB=0 \
  bash scripts/install_controller_local.sh
```

脚本会构建 `/usr/local/bin/gpu-controller`、写入 systemd unit、可选安装共享目录 sudo 规则并启动服务。执行前请确认 `ENABLE_SHARED_WORKSPACE_SUDOERS` 与 `ENABLE_HOST_SECURITY` 的取值。

验证：

```bash
systemctl status gpu-controller
journalctl -u gpu-controller -n 100 --no-pager
curl -fsS http://127.0.0.1:8080/readyz
/usr/local/bin/gpu-controller --version
```

## 6. 发布 Web 监听

在反向代理终止公网 TLS 并转发到控制器 Web 监听，保留 `Host`、`X-Forwarded-For` 和 `X-Forwarded-Proto`。最小 Nginx 配置：

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

在代理侧配置证书与现代 TLS 策略。HTTPS 生效后设置 `cookie_secure: true`。不要把 PostgreSQL 或控制器内部监听暴露到公网。

## 7. 配置内部监听

推荐的生产配置：

```yaml
internal_listen_addr: "0.0.0.0:8081"
internal_tls_cert_file: "/etc/gpu-ops/tls/internal.crt"
internal_tls_key_file: "/etc/gpu-ops/tls/internal.key"
```

证书必须被计算节点信任。将 `8081` 端口限制为已接入节点与 HA 对端可访问。修改启动 YAML 后需重启控制器。

## 8. 创建管理员并完成 Setup

用 bootstrap 接口创建首个管理员，通过 HTTPS 主机名登录，并按[首次 Setup](first-run-setup.md) 完成配置。bootstrap 被刻意设计为只能执行一次。

## 9. 接入节点

在每个 Linux 计算节点上：

```bash
bash scripts/node_prereq_check.sh
sudo env \
  NODE_ID=node-01 \
  CONTROLLER_URL=https://controller-internal.example.org:8081 \
  AGENT_TOKEN='<token-for-node-01>' \
  bash scripts/install_agent_local.sh
```

按 [Node Agent](node-agent.md) 中的分阶段 token 强制流程操作。先在非关键节点上测试 SSH Guard、CPU、内存、磁盘和 GPU 控制。

## 10. 备份与灰度

安装加密备份、完成一次隔离恢复演练，然后执行[上线检查清单](go-live-checklist.md)。至少用一个完整计费窗口以 `dry_run: true` 试运行。

## 升级模型

本项目不发布容器镜像，两种方式都从带标签的源码树构建。升级步骤：

1. 备份 PostgreSQL 与配置；
2. 审阅 `CHANGELOG.md` 与迁移；
3. 构建并测试目标提交；
4. 替换控制器二进制与 Web 资源，或重新构建镜像，然后重启；
5. 分批升级 Agent；
6. 验证版本、就绪状态、节点心跳与计费。

详细命令见[运维指南](operations.md)。
