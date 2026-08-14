# 开发指南

[English](../development.md) | **简体中文**

面向贡献者的环境搭建、仓库结构与验证流程。工作流与评审要求见 [CONTRIBUTING.zh-CN.md](../../CONTRIBUTING.zh-CN.md)。

## 环境要求

| 工具 | 版本 |
| --- | --- |
| Go | 1.26.6 |
| Node.js | 20.20 或更高 |
| pnpm | 10.28.2 |
| Docker Compose | 近期版本即可 |
| PostgreSQL | 开发环境由 Compose 提供 |

```bash
go version
node --version
pnpm --version
docker compose version
```

## 仓库结构

| 路径 | 内容 |
| --- | --- |
| `controller/` | Go 模块：HTTP API、策略引擎、计费、迁移执行、定时任务、Web 托管 |
| `node-agent/` | Go 模块：指标采集、动作执行、限制、SSH 状态、安全信号 |
| `web/` | Vue 3 + Element Plus 单页应用 |
| `database/` | Schema 与顺序迁移 |
| `scripts/` | 构建、安装、部署、备份、HA 与批量运维工具 |
| `config/` | 不含密钥的控制器配置示例 |
| `systemd/` | 控制器与 Agent 的 unit 文件 |
| `tools/` | 节点侧辅助脚本 |
| `docs/` | 英文源文档；`docs/zh-CN/` 存放翻译 |

`go.work` 把两个 Go 模块关联在一起，因此可以在仓库根目录执行 `go test ./controller/... ./node-agent/...`。

控制器中有两个体量很大的文件：`controller/database.go`（全部持久化逻辑）和 `controller/api.go`（路由、中间件与大部分处理函数）。新增处理函数通常应放进主题文件——`points_handlers.go`、`registry_handlers.go`、`auth_handlers.go`——而不是继续扩大 `api.go`。

## 环境搭建

```bash
git clone https://github.com/atoz03/Bursar.git
cd Bursar
cp .env.example .env          # 填写 POSTGRES_PASSWORD
docker compose up -d postgres

cp config/controller.yaml config/controller.local.yaml
# 把 agent_token、admin_token、auth_secret 设为三个不同的随机值
openssl rand -hex 32

corepack enable
pnpm --dir web install --frozen-lockfile
pnpm --dir web build

go run ./controller --config config/controller.local.yaml
```

`config/*.local.yaml` 与 `.env` 已被 Git 忽略。

### 前端开发

```bash
pnpm --dir web dev
```

Vite 监听 `5173`，把 `/api`、`/metrics` 和 `/healthz` 代理到 `127.0.0.1:8080` 的控制器。请在另一个终端保持控制器运行。

## 验证

提交 Pull Request 前请执行：

```bash
go vet ./controller/... ./node-agent/...
go test ./controller/... ./node-agent/...
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
bash -n scripts/*.sh
bash scripts/check_docs.sh
```

CI 会执行同样的检查，并额外构建容器镜像。

## 约定

**语言。** 英文是文档源语言，`docs/` 中的每篇文档在 `docs/zh-CN/` 都有对应翻译。本代码库的代码注释与日志信息使用中文——请与所在文件保持一致，不要在同一个文件中混用。

**配置。** 新增配置项时，需要同时修改 `Config` 结构体、`config/controller.yaml`、`Validate()` 和[配置参考](configuration.md)。只有容器部署必须在不修改 YAML 的前提下设置的值（通常是密钥、监听地址和路径）才需要在 `applyEnvOverrides` 中增加 `GPUOPS_*` 覆盖。

**迁移。** 在 `database/migrations/` 中新增编号文件。绝不要修改可能已被执行过的迁移；控制器启动时按顺序执行，任一失败即拒绝启动。

**鉴权。** 复用现有中间件，不要在处理函数内部自行校验凭据：`authSession`、`authAdmin`、`authAgent`、`authNodeAgent`、`authHA`、`authOperator`、`authSelfOrOperator` 以及 `require*Permission` 系列。token 比较必须使用 `constantTimeTokenEqual`，不要用 `==`。

**路由。** 新增路由意味着需要同步更新两种语言的 [API 参考](api-reference.md)，并明确决定它属于 Web 路由、内部路由，还是两者都有。

## 测试

测试使用标准 `go test`。现有覆盖集中在计费、TOTP、Turnstile、注册安全、内存限制、HA 安全约束、Setup、节点监控和配置覆盖上。项目没有数据库夹具框架，因此测试只覆盖纯逻辑；依赖 PostgreSQL 的行为需要手工验证，并在 Pull Request 中说明验证过程。

```bash
go test ./controller/... -run TestBilling -v
go test ./node-agent/... -v
```

## 构建发布二进制

```bash
bash scripts/build_linux.sh
```

版本字符串编译进各自二进制（`controller/version.go`、`node-agent/version.go`）。发布新版本时需要更新它们，并同步更新 `.version` 与两份变更日志。

## 构建容器镜像

```bash
docker compose --profile full build controller
docker compose --profile full up -d
curl -fsS http://127.0.0.1:8080/readyz
```

镜像是三阶段构建：pnpm 构建界面、Go 构建静态二进制、运行阶段使用 distroless 并包含二进制、`web/dist` 和 `database/migrations`。镜像内没有 shell，因此容器健康检查调用 `/app/controller --healthcheck`。
