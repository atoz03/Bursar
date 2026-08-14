# 变更日志

[English](CHANGELOG.md) | **简体中文**

本项目的重要变化记录在此。版本发布遵循[语义化版本](https://semver.org/lang/zh-CN/)，发布内容以 GitHub Release 和对应 Git 标签为准。

## [3.2.0] — 2026-08-14

首个公开版本。版本号沿用本项目内部开发时的序列，此前没有公开发布过。

### 包含内容

- **控制器** — HTTP API、策略引擎、用量计费、启动时按顺序执行的只向前迁移、定时任务，以及同源托管 Web 界面。
- **Node Agent** — 指标采集、动作执行、CPU 与内存限制、SSH 状态上报和安全信号。
- **Web 界面** — Vue 3 与 Element Plus，构建到 `web/dist` 并由控制器托管。
- **两种部署方式** — 由 systemd 托管的源码构建，以及包含 PostgreSQL 与控制器的 `docker compose` 容器方案。
- **首次 Setup** — 集中配置平台标识、注册域名、SSH 入口、价格、用户准则、SMTP 和 HA 的向导。只要有必填就绪检查未通过，它就拒绝保存。
- **双语文档** — 英文是源语言，`docs/zh-CN/` 是完整镜像，由 CI 中的 `scripts/check_docs.sh` 强制保持一致。

### 需要了解的默认值

- Web 与 API 监听默认 `8080`，内部 Agent 与 HA 监听默认 `8081`。
- 上线初期请保持 `dry_run: true`。此时记录用量并计算成本，但不扣减。
- 余额、用量与 `/metrics` 接口都需要凭据，没有任何一个是匿名开放的。
- 会话 cookie 为 `HttpOnly` 且 `SameSite=Lax`，所有非 GET 的会话请求都必须携带 `X-CSRF-Token`。
- 全部凭据比较均为常量时间比较。

### 已知限制

- HA 是运维自动化，而不是共识协议：没有 quorum、没有自动选主，也没有脑裂保护。防止出现两个同时活跃的主控制器是运维方的责任。
- 本平台不是多租户隔离边界。能够在计算节点提权到 root 的用户，可以绕过节点本地的所有策略执行。
- 迁移只向前执行。回滚意味着恢复备份。

### 从内部部署升级

- 端口从 `60039`/`60040` 调整为 `8080`/`8081`。显式配置 `listen_addr` 与 `internal_listen_addr` 即可保持原值；升级前请同步更新防火墙规则、各节点的 `CONTROLLER_URL` 以及反向代理配置。
- 节点侧辅助工具改为携带运维凭据，从 `GPUOPS_QUERY_TOKEN` 或 `/etc/gpu-ops/query-token` 读取，因为余额接口不再匿名开放。
- `docker-compose.yml` 不再内置数据库密码，请在 `.env` 中提供 `POSTGRES_PASSWORD`。

[3.2.0]: https://github.com/atoz03/gpu-ops/releases/tag/v3.2.0
