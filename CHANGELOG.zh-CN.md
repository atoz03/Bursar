# 变更日志

[English](CHANGELOG.md) | **简体中文**

本项目的重要变化记录在此。版本发布遵循[语义化版本](https://semver.org/lang/zh-CN/)，发布内容以 GitHub Release 和对应 Git 标签为准。

## [Unreleased]

### 新增

- 管理员首次登录 Setup 向导，集中配置平台、注册域名、SSH 入口、价格、用户准则、SMTP 和 HA。
- 完整的双语开源文档、贡献流程、安全披露策略与 Apache-2.0 许可证。
- 容器部署路径：多阶段 `Dockerfile`，以及同时拉起 PostgreSQL 与控制器的 `docker compose` 配置。
- 面向监听地址、路径与密钥的 `GPUOPS_*` 环境变量覆盖，使容器无需修改 YAML 即可完成配置。
- `controller --healthcheck`：distroless 运行镜像中没有 shell，容器健康检查改由二进制自身执行。
- `scripts/check_docs.sh`：校验相对链接、中英文档对齐与语言切换头，CI 每次推送都会执行。

### 变更

- 统一使用可公开复用的配置示例、主机名、路径和测试数据。
- **破坏性变更：** 默认端口调整为 `8080`（Web/API）与 `8081`（内部 Agent/HA 监听），取代原来的 `60039`/`60040`。已有部署显式配置 `listen_addr` 与 `internal_listen_addr` 即可保持不变；升级前请同步更新防火墙规则、各节点的 `CONTROLLER_URL` 以及反向代理配置。
- 未显式设置 `GIN_MODE` 时，控制器默认以 release 模式运行。
- Vite 开发代理指向 `127.0.0.1:8080`。此前指向 8000 端口，而控制器从未监听过该端口。
- `tools/balance-query` 与 `tools/check_quota.sh` 改为携带运维凭据，从 `GPUOPS_QUERY_TOKEN` 或 `/etc/gpu-ops/query-token` 读取，因为余额接口不再匿名开放。

### 修复

- `docker-compose.yml` 不再内置数据库密码；必须提供 `POSTGRES_PASSWORD`，且 PostgreSQL 默认只发布到 `127.0.0.1`。

### 安全

- 强化真实配置、密钥、节点清单、备份和本地导出文件的忽略规则。
- Setup 完成前关闭公开注册，并检查启动密钥是否已替换。
- `GET /api/users/:username/balance` 与 `GET /api/users/:username/usage` 改为需要鉴权。此前这两个接口可匿名访问，会向未认证调用方泄露任意用户的余额、状态与用量历史。
- `GET /api/users/:username/balance` 不再作为读接口的副作用创建用户行，关闭了匿名写入 `users` 表的路径。
- `GET /metrics` 改为需要运维凭据（`admin_token`、Agent token 或管理员会话）。
- 全部凭据比较——admin、agent、legacy、每节点 token、管理员自举以及 CSRF nonce——统一使用常量时间比较；此前仅 HA token 与 TOTP 验证码如此。
- 会话 cookie 在原有 CSRF token 校验之外，追加 `SameSite=Lax` 属性。

[Unreleased]: https://github.com/atoz03/gpu-ops/commits/main
