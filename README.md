# GPU Ops

轻量 GPU 集群运维平台：保留 SSH 使用习惯，后台完成监控、计费、配额控制、账号映射与管理。

## 功能概览

- **节点 Agent（Go）**：每分钟采集 GPU/CPU 进程并上报控制器
- **控制器（Go + Gin + PostgreSQL）**：落库、计费、限制动作下发、管理 API
- **Web 管理端（Vue3）**：管理员与普通用户分角色界面
- **用户能力**：注册、登录、找回密码、修改密码、查询个人余额/用量、管理个人服务器账号映射
- **管理员能力**：运营看板、节点状态、价格配置、注册审核、账号映射管理、SSH 白/黑/豁免名单、邮件配置与测试发送、容灾同步状态查看

---

## 安全准则与判罚规则

### 1) SSH 爆破防护（fail2ban）
- 判定条件：同一来源在 `5m` 内失败超过 `20` 次。
- 判罚：封禁 `12h`。
- 适用范围：控制节点与计算节点（安装脚本会自动部署并开机自启）。

### 2) SSH 登录准入（平台注册拦截）
- 计算节点支持“是否拦截平台未注册用户”开关（节点状态监控页逐节点控制）。
- 默认：`关闭`（不拦截未注册用户）。
- 开关行为：
  - `关闭 -> 开启`：立即清除该节点全部 SSH 会话，所有用户需重新登录后按新规则校验。
  - `开启 -> 关闭`：不清除现有会话，当前在线用户不受影响。
- 无论开关状态，黑名单仍会被拒绝，豁免名单仍优先放行。

### 3) 疑似恶意行为审计
- 判定条件（命中任一即记录为恶意风险）：
  - 节点总 CPU 负载达到或超过 `95%`
  - 5 分钟内 SSH 失败登录次数超过 `20`
  - 检测到 SSH 爆破行为（同来源高频失败/多账号试探）
  - 检测到异常端口扫描
  - 检测到磁盘打满风险（磁盘使用率极高或剩余空间过低）
- 记录内容：节点、触发时间、相关用户、Top CPU 用户占比、触发原因。
- 作用：在“节点状态监控 -> 节点详情”中生成“疑似恶意用户名单”和“安全审计日志”，支持管理员一键拉黑（SSH 黑名单）。

### 4) CPU 预留策略
- 节点安装时默认给 `user.slice` 设置 CPU 上限约 `95%`，预留约 `5%` 给系统，降低 SSH 卡顿风险。

---

## 脚本作用与用法（`scripts/`）

| 脚本 | 作用 | 常用用法 |
|---|---|---|
| `scripts/install_deps_ubuntu2204.sh` | 安装基础依赖（Go/Node/pnpm/Docker，可选） | `bash scripts/install_deps_ubuntu2204.sh` |
| `scripts/install_controller_local.sh` | 本机安装控制器到 systemd（后台+开机自启） | `bash scripts/install_controller_local.sh` |
| `scripts/install_agent_local.sh` | 在计算节点本机安装 node-agent、SSH Guard 与安全基线 | `CONTROLLER_URL=... AGENT_TOKEN=... bash scripts/install_agent_local.sh` |
| `scripts/install_home_reserve_guard.sh` | 安装 `/home` 预留空间保护（低于阈值限制非豁免用户 `/home` 写入） | `HOME_RESERVE_GB=8 bash scripts/install_home_reserve_guard.sh` |
| `scripts/deploy_controller.sh` | 远程部署控制器二进制和 systemd | `HOST=<ip> bash scripts/deploy_controller.sh` |
| `scripts/deploy_agent.sh` | 批量远程部署 node-agent 到多节点 | `NODES='60000:ip1 60001:ip2' ... bash scripts/deploy_agent.sh` |
| `scripts/distribute_workspace.sh` | 按 `my_ssh_keys/server_ssh_map.csv` 把工作区分发到所有节点 | `bash scripts/distribute_workspace.sh` |
| `scripts/deploy_installed_nodes_only.sh` | 并发分发到所有节点；仅对已安装 `gpu-node-agent` 的节点重装，输出部署报告 | `CONTROLLER_URL=... AGENT_TOKEN=... bash scripts/deploy_installed_nodes_only.sh` |
| `scripts/check_server_connectivity.sh` | 按 `server_ssh_map.csv` 检查所有节点 SSH 连通性并输出报告 | `bash scripts/check_server_connectivity.sh` |
| `scripts/check_status.sh` | 快速检查控制器健康、节点、metrics | `bash scripts/check_status.sh` |
| `scripts/node_prereq_check.sh` | 节点环境预检查（systemd/cgroup/cpu 控制能力） | `bash scripts/node_prereq_check.sh` |
| `scripts/build_linux.sh` | 构建 Linux 可执行文件 | `bash scripts/build_linux.sh` |

补充说明：
- `install_agent_local.sh` 默认支持 `NODE_ID` 自动识别（按本机 IP 匹配 `my_ssh_keys/server_ssh_map.csv`）。
- `install_agent_local.sh` 已内置 SSH 防爆破（`fail2ban`）和 `user.slice` CPU 预留（默认给系统保留约 5%）。
- `install_agent_local.sh` 与 `install_controller_local.sh` 会自动调用 `install_home_reserve_guard.sh`，可通过 `HOME_RESERVE_GB` 一行调整预留空间。
- `distribute_workspace.sh` 支持并发，默认 `PARALLEL=6`。
- `distribute_workspace.sh`/`check_server_connectivity.sh` 默认都是全量读取 `my_ssh_keys/server_ssh_map.csv`，不是只跑单节点。

---

## 🚀 快速开始（本机）

### 🧰 0) 安装依赖（Ubuntu 22.04，清华源优先，固定版本）

> 建议先配置 apt 为清华源，再安装基础依赖；以下版本为本项目推荐固定版本。

```bash
cd /home/gpuops/gpu-ops
bash scripts/install_deps_ubuntu2204.sh
```

可选参数（示例）：

```bash
# 跳过 Docker
INSTALL_DOCKER=0 bash scripts/install_deps_ubuntu2204.sh

# 指定版本
GO_VERSION=1.22.5 NODE_MAJOR=20 PNPM_VERSION=10.28.2 bash scripts/install_deps_ubuntu2204.sh
```

脚本等价于下方手动步骤，若你想逐条执行可继续参考：

```bash
# 0.1 切换 apt 清华源（Ubuntu 22.04 / jammy）
sudo cp /etc/apt/sources.list /etc/apt/sources.list.bak.$(date +%s)
sudo tee /etc/apt/sources.list >/dev/null <<'EOF'
deb https://mirrors.tuna.tsinghua.edu.cn/ubuntu/ jammy main restricted universe multiverse
deb https://mirrors.tuna.tsinghua.edu.cn/ubuntu/ jammy-updates main restricted universe multiverse
deb https://mirrors.tuna.tsinghua.edu.cn/ubuntu/ jammy-backports main restricted universe multiverse
deb https://mirrors.tuna.tsinghua.edu.cn/ubuntu/ jammy-security main restricted universe multiverse
EOF

# 0.2 基础依赖
sudo apt-get update
sudo apt-get install -y --no-install-recommends \
  ca-certificates curl wget git jq build-essential docker.io docker-compose-plugin

# 0.3 安装 Go 1.22.5（清华失败自动切阿里/腾讯）
cd /tmp
rm -f go.tgz
wget -O go.tgz https://mirrors.tuna.tsinghua.edu.cn/golang/go1.22.5.linux-amd64.tar.gz \
|| wget -O go.tgz https://mirrors.aliyun.com/golang/go1.22.5.linux-amd64.tar.gz \
|| wget -O go.tgz https://mirrors.cloud.tencent.com/golang/go1.22.5.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf /tmp/go.tgz
echo 'export PATH=/usr/local/go/bin:$PATH' | sudo tee /etc/profile.d/go.sh >/dev/null
export PATH=/usr/local/go/bin:$PATH
grep -q '/usr/local/go/bin' ~/.bashrc || echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc
hash -r
go version   # 期望：go1.22.5

# 0.4 安装 Node 20 + pnpm 10.28.2
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt-get install -y nodejs
sudo corepack enable
corepack prepare pnpm@10.28.2 --activate
node -v      # 期望：v20.x
pnpm -v      # 期望：10.28.2

# 0.5 Go 网络建议（国内）
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=off
```

### 🗄️ 1) 启动 PostgreSQL

```bash
cd /home/gpuops/gpu-ops
docker-compose up -d

docker-compose ps -a
docker-compose logs --tail=200 postgres
```

默认数据库：`gpuops`，账号密码：`gpuops/gpuops`，端口：`5432`。

### 🧠 2) 启动控制器

```bash
cd /home/gpuops/gpu-ops/controller
go run . --config ../config/controller.yaml
```

健康检查：

```bash
curl -s http://127.0.0.1:60039/healthz
```

### 🖥️ 3) 构建前端（首次或前端改动后）

```bash
cd /home/gpuops/gpu-ops/web
pnpm install
pnpm build
```

说明：控制器只托管 `web/dist`，前端改动后需重新 `pnpm build`，然后重启控制器。

### 🔐 4) 初始化管理员账号（仅首次）

```bash
# admin_token 请从 config/controller.yaml 读取
curl -fsS -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -X POST http://127.0.0.1:60039/api/admin/bootstrap \
  -d '{"username":"admin","password":"ChangeMe_123456"}'
```

登录地址：`http://127.0.0.1:60039/login`

### 🤖 5) 本机模拟启动 Agent

```bash
cd /home/gpuops/gpu-ops/node-agent
NODE_ID=60000 \
CONTROLLER_URL=http://192.0.2.10:60039 \
AGENT_TOKEN=<agent_token> \
go run .
```

`AGENT_TOKEN` 必须与 `config/controller.yaml` 的 `agent_token` 一致。

计算节点本地安装时，推荐用“一条命令”启动（避免代理导致下载超时）：

```bash
cd /home/<用户名>/gpu-ops/node-agent
env -u http_proxy -u https_proxy -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u all_proxy \
GOPROXY=https://goproxy.cn,direct \
GOSUMDB=sum.golang.google.cn \
GO111MODULE=on \
NODE_ID=60001 \
CONTROLLER_URL=http://192.0.2.10:60039 \
AGENT_TOKEN=<agent_token> \
go run .
```

---

## 🔁 日常启动（开发环境）

```bash
# 1) 控制端：数据库
cd /home/gpuops/gpu-ops
docker-compose up -d

# 2) 控制端：控制器
cd /home/gpuops/gpu-ops/controller
go run . --config ../config/controller.yaml

# 3) 节点端：Agent（如未用 systemd 托管）
cd /home/gpuops/gpu-ops/node-agent
NODE_ID=60000 CONTROLLER_URL=http://127.0.0.1:60039 AGENT_TOKEN=<agent_token> go run .
```

> `pnpm build` 不需要每次开机执行，只有前端代码变更后需要。

---

## 🧭 主要页面

- 登录：`/login`
- 用户注册：`/register`
- 找回密码：`/forgot-password`
- 管理员运营看板：`/admin/board`
- 积分管理：`/admin/points`（支持设置博士/硕士月初发放积分、查看上次发放时间）
- 节点状态：`/admin/nodes`
- 账号映射管理：`/admin/accounts`、`/user/accounts`
- SSH 名单（白/黑/豁免）：`/admin/whitelist`
- 容灾同步：`/admin/ha`
- 邮件设置与测试发送：`/admin/mail`

---

## 🔌 API 速查

- Agent 上报：`POST /api/metrics`
- 用户自助：`POST /api/auth/register`、`POST /api/auth/forgot-password`、`POST /api/auth/reset-password`
- 登录会话：`POST /api/auth/login`、`GET /api/auth/me`、`POST /api/auth/change-password`
- 用户查询：`GET /api/user/me/balance`、`GET /api/user/me/usage`
- 账号映射：
  - 用户：`GET/POST/PUT/DELETE /api/user/accounts`
  - 管理员：`GET/POST/PUT/DELETE /api/admin/accounts`
- 白名单：`GET/POST/DELETE /api/admin/whitelist`
- 黑名单：`GET/POST/DELETE /api/admin/blacklist`
- 豁免名单：`GET/POST/DELETE /api/admin/exemptions`
- 容灾状态：`GET /api/admin/ha/status`（管理员）、`GET /api/ha/status`（主备内部）
- 运营统计：`GET /api/admin/stats/users`、`GET /api/admin/stats/monthly`、`GET /api/admin/stats/recharges`
- 积分管理：`GET /api/admin/points/users`、`POST /api/admin/points/adjust`、`POST /api/admin/points/batch-grant`
- 月初发放：`GET/POST /api/admin/points/monthly-config`、`GET /api/admin/points/monthly-reset/status`、`POST /api/admin/points/monthly-reset`
- 邮件：`GET/POST /api/admin/mail/settings`、`POST /api/admin/mail/test`

完整字段说明见：`docs/api-reference.md`

---

## 🧪 计算节点快速测试与构建（首次安装）

```bash
cd /home/gpuops/gpu-ops && \
echo "gpuops ALL=(root) NOPASSWD: /bin/bash /home/gpuops/gpu-ops/scripts/install_agent_local.sh, /bin/bash /home/gpuops/gpu-ops/scripts/install_agent_local.sh *" | sudo tee /etc/sudoers.d/gpu-deploy >/dev/null && \
sudo chmod 440 /etc/sudoers.d/gpu-deploy && \
sudo chown root:root /etc/sudoers.d/gpu-deploy && \
sudo visudo -cf /etc/sudoers.d/gpu-deploy && \
SSH_GUARD_EXCLUDE_USERS="root gpuops" HOME_RESERVE_GB=8 CONTROLLER_URL=http://192.0.2.10:60039 AGENT_TOKEN=6af5911256a62c73d8ecaaf60ffec363f23247a0cf262a8f7c78b188fdeaaf4b bash scripts/install_agent_local.sh && \
sudo systemctl enable --now gpu-node-agent && \
sudo systemctl status gpu-node-agent --no-pager
```

说明：`scripts/install_agent_local.sh` 默认已经会安装并启用 `gpu-node-agent`，上述命令用于手动确认或补开启；`HOME_RESERVE_GB` 可改成任意整数（单位 GB），例如 `HOME_RESERVE_GB=12`。

---

## 🧪 控制节点快速测试与构建

```bash
# 分发最新版脚本给计算节点
bash ./scripts/distribute_workspace.sh
# 一键并发分发并“仅重装已安装过 agent 的节点”，结果写入当前目录“计算节点部署情况.txt”
cd /home/gpuops/gpu-ops && CONTROLLER_URL=http://192.0.2.10:60039 AGENT_TOKEN=6af5911256a62c73d8ecaaf60ffec363f23247a0cf262a8f7c78b188fdeaaf4b SSH_GUARD_EXCLUDE_USERS="root gpuops" HOME_RESERVE_GB=8 PARALLEL=8 bash scripts/deploy_installed_nodes_only.sh
# 计算节点服务开机自启（在每个计算节点执行一次）
sudo systemctl enable --now gpu-node-agent
sudo systemctl status gpu-node-agent --no-pager
# 先构建前端
cd /home/gpuops/gpu-ops/web && pnpm build
# 后台运行 + 开机自启（推荐）
cd /home/gpuops/gpu-ops && HOME_RESERVE_GB=8 bash scripts/install_controller_local.sh
# 重启服务
sudo systemctl restart gpu-controller
# 停止控制节点服务（需要时）
sudo systemctl stop gpu-controller && sudo systemctl disable gpu-controller && sudo systemctl status gpu-controller --no-pager
# 停止计算节点服务（node-agent + SSH Guard + /home 预留保护定时任务，需要时）
sudo systemctl stop gpu-node-agent gpu-ssh-guard-sync.timer gpu-ssh-guard-enforce.timer gpu-home-reserve-enforce.timer && sudo systemctl disable gpu-node-agent gpu-ssh-guard-sync.timer gpu-ssh-guard-enforce.timer gpu-home-reserve-enforce.timer && sudo systemctl status gpu-node-agent gpu-ssh-guard-sync.timer gpu-ssh-guard-enforce.timer gpu-home-reserve-enforce.timer --no-pager
# 前台运行（与上面后台方式二选一）
cd /home/gpuops/gpu-ops/controller && go run . --config ../config/controller.yaml
# 看 agent 主服务是否在运行
sudo systemctl status gpu-node-agent --no-pager
# 只看是否 active（适合脚本）
systemctl is-active gpu-node-agent
# 看 SSH Guard 与 /home 预留保护定时器是否在跑
systemctl is-active gpu-ssh-guard-sync.timer gpu-ssh-guard-enforce.timer gpu-home-reserve-enforce.timer
systemctl list-timers --all | grep -E 'gpu-ssh-guard-(sync|enforce)\.timer|gpu-home-reserve-enforce\.timer'

```

说明：
- 计算节点安全防护由 `install_agent_local.sh` 安装并启用（SSH Guard + fail2ban + user.slice CPU 预留）。
- 计算节点需启用 `gpu-node-agent` 开机自启（上面的 `systemctl enable --now gpu-node-agent`）。
- 控制节点需启用 `gpu-controller` 开机自启（`install_controller_local.sh` 会配置并启用 systemd 服务）。
- 两类节点都会安装 `/home` 预留保护（默认 `HOME_RESERVE_GB=8`，`HOME_RESERVE_EXEMPT_USERS` 默认 `root gpuops`）。

---

## 🛠️ scripts 目录说明（作用 + 用法）

> 路径：`/home/gpuops/gpu-ops/scripts`

| 脚本 | 作用 | 常用用法 |
|---|---|---|
| `install_deps_ubuntu2204.sh` | Ubuntu 22.04 一键安装项目依赖（Go/Node/pnpm/Docker 等） | `bash scripts/install_deps_ubuntu2204.sh` |
| `install_controller_local.sh` | 在控制器本机安装并启用 `gpu-controller` systemd 服务 | `cd /home/gpuops/gpu-ops && bash scripts/install_controller_local.sh` |
| `install_agent_local.sh` | 在计算节点本机一键安装并启用 `gpu-node-agent` | `NODE_ID=60001 CONTROLLER_URL=http://<控制器IP>:60039 AGENT_TOKEN=<token> bash scripts/install_agent_local.sh` |
| `deploy_agent.sh` | 从控制端批量部署 agent 到多台节点 | `NODES='60000:192.0.2.10 60001:192.0.2.10' AGENT_TOKEN=<token> CONTROLLER_URL=http://<控制器IP>:60039 bash scripts/deploy_agent.sh` |
| `deploy_controller.sh` | 部署 controller 二进制与配置到远端控制器主机 | `HOST=<控制器主机> CONTROLLER_BIN=./controller/controller bash scripts/deploy_controller.sh` |
| `distribute_workspace.sh` | 将当前仓库分发到各计算节点 `/home/<用户>/<项目目录>`（支持并发，默认 `PARALLEL=6`） | `PARALLEL=8 bash scripts/distribute_workspace.sh` |
| `deploy_installed_nodes_only.sh` | 并发分发并仅重装“已安装过 gpu-node-agent”的节点，未安装节点仅更新目录，自动生成 `计算节点部署情况.txt` | `CONTROLLER_URL=http://<控制器IP>:60039 AGENT_TOKEN=<token> SSH_GUARD_EXCLUDE_USERS='root ...' PARALLEL=8 bash scripts/deploy_installed_nodes_only.sh` |
| `build_linux.sh` | 构建 Linux 可部署二进制（controller + node-agent） | `bash scripts/build_linux.sh` |
| `node_prereq_check.sh` | 计算节点上线前检查（只检查，不改系统） | `bash scripts/node_prereq_check.sh` |
| `check_server_connectivity.sh` | 检查节点 SSH 连通性并输出报告 | `bash scripts/check_server_connectivity.sh` |
| `check_status.sh` | 控制器与核心 API 快速自检 | `CONTROLLER_URL=http://127.0.0.1:60039 ADMIN_TOKEN=<admin_token> bash scripts/check_status.sh` |
| `deploy_hook.sh` | 批量部署用户侧 shell hook（示例） | `HOOK_SRC=./tools/check_quota.sh bash scripts/deploy_hook.sh` |
| `install-githooks.sh` | 安装仓库 git hooks（开发者本地） | `bash scripts/install-githooks.sh` |

推荐顺序（新环境）：

1. 控制器机：`install_deps_ubuntu2204.sh` → `pnpm build` → `install_controller_local.sh`
2. 验证：`check_status.sh` + 管理员页面 `/admin/nodes`

---

## 📚 文档导航

- `docs/plan.md`：总体方案
- `docs/runbook.md`：上线运行手册
- `docs/admin-guide.md`：管理员查阅手册（模块功能 + 安全规则 + 阈值速查）
- `docs/user-guide.md`：用户手册
- `docs/go-live-checklist.md`：上线检查项

文档维护约定（必须）：
- 后台功能、权限、安全规则或阈值有变更时，需同步更新 `docs/admin-guide.md`。

---

## 🗂️ 目录结构

```text
gpu-ops/
├── controller/      # 控制器
├── node-agent/      # 节点 Agent
├── web/             # 前端
├── database/        # schema + migrations
├── scripts/         # 部署/运维脚本
├── tools/           # 用户侧工具
├── config/          # 配置
├── systemd/         # service 示例
└── docs/            # 文档
```
