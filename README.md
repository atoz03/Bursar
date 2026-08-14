# GPU Ops

轻量 GPU 集群运维平台：保留 SSH 使用习惯，后台完成监控、计费、配额控制、账号映射与管理。

## 功能概览

- **节点 Agent（Go）**：每分钟采集 GPU/CPU 进程并上报控制器
- **控制器（Go + Gin + PostgreSQL）**：落库、计费、限制动作下发、管理 API
- **Web 管理端（Vue3）**：管理员与普通用户分角色界面
- **用户能力**：注册、登录、找回密码、修改密码、查询个人余额/用量、管理个人服务器账号映射
- **管理员能力**：运营看板、节点状态、价格配置、注册审核、账号映射管理、SSH 白/黑/豁免名单、邮件配置与测试发送、容灾同步状态实时查看与配置（`/admin/ha` 支持自动/手动同步、日志、回切）

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

### 4) 资源预留策略
- 默认开启 CPU 预留：`ENABLE_SYSTEM_CPU_RESERVE=1`，并通过 `SYSTEM_CPU_RESERVE_PERCENT=5` 约保留 `5%` CPU 给系统（脚本会据此设置 `user.slice` 的 CPU 上限）。
- 默认内存预留：`ENABLE_SYSTEM_MEMORY_RESERVE=1`，并通过 `SYSTEM_MEMORY_RESERVE_GB=8` 给系统保留 `8G` 内存，避免用户任务把节点内存打满导致卡死。

---

## 脚本作用与用法（`scripts/`）

| 脚本 | 作用 | 常用用法 |
|---|---|---|
| `scripts/install_deps_ubuntu2204.sh` | 安装基础依赖（Go/Node/pnpm/Docker，可选） | `bash scripts/install_deps_ubuntu2204.sh` |
| `scripts/install_controller_local.sh` | 本机安装控制器到 systemd（后台+开机自启） | `bash scripts/install_controller_local.sh` |
| `scripts/install_agent_local.sh` | 在计算节点本机安装 node-agent、SSH Guard 与安全基线 | `CONTROLLER_URL=... AGENT_TOKEN=... bash scripts/install_agent_local.sh` |
| `scripts/deploy_controller.sh` | 远程部署控制器二进制和 systemd | `HOST=<ip> bash scripts/deploy_controller.sh` |
| `scripts/deploy_agent.sh` | 批量远程部署 node-agent 到多节点 | `NODES='60000:ip1 60001:ip2' ... bash scripts/deploy_agent.sh` |
| `scripts/distribute_workspace.sh` | 按 `my_ssh_keys/server_ssh_map.csv` 把工作区分发到节点；默认拒绝裸跑，必须显式确认 | `CONFIRM_DISTRIBUTE_WORKSPACE=1 NODE_IDS='60020' bash scripts/distribute_workspace.sh` |
| `scripts/deploy_installed_nodes_only.sh` | 并发分发到节点；仅对已安装 `gpu-node-agent` 的节点重装，支持 `NODE_IDS='60020 60002'`，输出部署报告 | `CONTROLLER_URL=... AGENT_TOKEN=... bash scripts/deploy_installed_nodes_only.sh` |
| `scripts/check_server_connectivity.sh` | 按 `server_ssh_map.csv` 检查所有节点 SSH 连通性并输出报告 | `bash scripts/check_server_connectivity.sh` |
| `scripts/check_status.sh` | 快速检查控制器健康、节点、metrics | `bash scripts/check_status.sh` |
| `scripts/node_prereq_check.sh` | 节点环境预检查（systemd/cgroup/cpu 控制能力） | `bash scripts/node_prereq_check.sh` |
| `scripts/build_linux.sh` | 构建 Linux 可执行文件 | `bash scripts/build_linux.sh` |

补充说明：
- `install_agent_local.sh` 默认支持 `NODE_ID` 自动识别（按本机 IP 匹配 `my_ssh_keys/server_ssh_map.csv`）。
- `install_agent_local.sh` 已内置 SSH 防爆破（`fail2ban`）和 `user.slice` 资源预留（默认给系统保留 `5%` CPU + `8G` 内存，可通过 `SYSTEM_CPU_RESERVE_PERCENT`、`SYSTEM_MEMORY_RESERVE_GB` 调整）。
- `distribute_workspace.sh` 支持并发，默认 `PARALLEL=6`，且默认拒绝执行真实分发；先用 `DRY_RUN=1` 检查目标，确认后必须加 `CONFIRM_DISTRIBUTE_WORKSPACE=1`。
- `distribute_workspace.sh`、`deploy_installed_nodes_only.sh` 都支持 `NODE_IDS` 过滤，例如 `NODE_IDS='60020 60002'` 或 `NODE_IDS='60020,60002'`。
- `check_server_connectivity.sh` 默认仍是全量读取 `my_ssh_keys/server_ssh_map.csv`。

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
if docker compose version >/dev/null 2>&1; then
  DC="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  DC="docker-compose"
else
  echo "未检测到 docker compose / docker-compose，请先安装 docker-compose-plugin"
  exit 1
fi

$DC up -d

$DC ps -a
$DC logs --tail=200 postgres
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

#### 可选：启用 Cloudflare Turnstile

登录与注册默认使用本地简易验证；同时配置 sitekey 和 secret key 后自动切换为 Turnstile：

```yaml
turnstile_site_key: "<Cloudflare sitekey>"
turnstile_secret_key: "<Cloudflare secret key>"
turnstile_expected_hostnames:
  - "192.0.2.20"
```

Turnstile widget 的允许主机也填写 `192.0.2.20`，不要填写协议或端口。内网页面不需要暴露到公网，但用户浏览器和 Controller 都必须能通过 HTTPS 访问 `challenges.cloudflare.com`。生产 secret key 只保存在 Controller 配置中，不能写入前端。

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
if docker compose version >/dev/null 2>&1; then
  DC="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  DC="docker-compose"
else
  echo "未检测到 docker compose / docker-compose，请先安装 docker-compose-plugin"
  exit 1
fi
$DC up -d

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
- 容灾同步：`GET/POST /api/admin/ha/sync/config`、`GET /api/admin/ha/sync/runs`、`POST /api/admin/ha/sync/now`、`POST /api/admin/ha/failover/activate`
- 运营统计：`GET /api/admin/stats/users`、`GET /api/admin/stats/monthly`、`GET /api/admin/stats/recharges`
- 积分管理：`GET /api/admin/points/users`、`POST /api/admin/points/adjust`、`POST /api/admin/points/batch-grant`
- 月初发放：`GET/POST /api/admin/points/monthly-config`、`GET /api/admin/points/monthly-reset/status`、`POST /api/admin/points/monthly-reset`
- 邮件：`GET/POST /api/admin/mail/settings`、`POST /api/admin/mail/test`

完整字段说明见：`docs/api-reference.md`

---

## 🧪 计算节点快速测试与构建（首次安装）

```bash
# 1) 在控制节点检查分发目标（不会实际写远端）
cd /home/gpuops/gpu-ops && DRY_RUN=1 NODE_IDS="60020" bash scripts/distribute_workspace.sh

# 确认无误后，才对“新节点”实际分发（示例只分发 60020）
cd /home/gpuops/gpu-ops && CONFIRM_DISTRIBUTE_WORKSPACE=1 NODE_IDS="60020" bash scripts/distribute_workspace.sh

# 2) 登录到对应计算节点，在节点本机执行首次安装（示例节点 60020）
cd /home/gpuops/gpu-ops && \
NODE_ID=60020 \
SSH_GUARD_EXCLUDE_USERS="root gpuops" \
ENABLE_SYSTEM_CPU_RESERVE=1 \
SYSTEM_CPU_RESERVE_PERCENT=5 \
ENABLE_SYSTEM_MEMORY_RESERVE=1 \
SYSTEM_MEMORY_RESERVE_GB=8 \
CONTROLLER_URL=http://192.0.2.10:60039 \
AGENT_TOKEN=<agent_token> \
bash scripts/install_agent_local.sh && \
sudo systemctl status gpu-node-agent --no-pager
```

首次安装完成后，如需让该节点以后支持控制节点上的 `deploy_installed_nodes_only.sh` 远程重装，再在该节点额外执行一次 sudoers 放行：

```bash
cd /home/gpuops/gpu-ops && \
echo "gpuops ALL=(root) NOPASSWD: /bin/bash /home/gpuops/gpu-ops/scripts/install_agent_local.sh, /bin/bash /home/gpuops/gpu-ops/scripts/install_agent_local.sh *" | sudo tee /etc/sudoers.d/gpu-deploy >/dev/null && \
sudo chmod 440 /etc/sudoers.d/gpu-deploy && \
sudo chown root:root /etc/sudoers.d/gpu-deploy && \
sudo visudo -cf /etc/sudoers.d/gpu-deploy
```

说明：
- `scripts/install_agent_local.sh` 已经会自动 `enable --now gpu-node-agent`，首次安装时不需要再手动执行一次 `sudo systemctl enable --now gpu-node-agent`。
- `deploy_installed_nodes_only.sh` 只负责“已安装过 agent 的节点”的后续重装；新节点第一次安装，先按上面的“分发代码 + 节点本机执行 install_agent_local.sh”做。
- `SYSTEM_CPU_RESERVE_PERCENT` 默认 `5`，表示给系统保留约 `5%` CPU；`SYSTEM_MEMORY_RESERVE_GB` 默认 `8`，可按节点规格改成 `6`、`10` 等。
- 兼容旧变量：`USER_SLICE_CPU_RESERVE_PERCENT=90` 会自动换算成系统保留 `10%` CPU，`USER_SLICE_MEMORY_RESERVE_GB=8` 等价于 `SYSTEM_MEMORY_RESERVE_GB=8`。

---

## 🧪 控制节点快速测试与构建

```bash
# "60020 60000 60002 60005 60014 60016 60001" 第一批节点
# "60006 60010" 第二批节点
# "60020 60018 60017 60015 60014 60012 60010 60008 60007 60006 60005 60004 60003 60002 60001 60000" 全部节点
# 60013 60009 60003 特殊节点
# DRY_RUN=1 NODE_IDS="60020" bash ./scripts/distribute_workspace.sh
# 一键并发分发并“仅重装已安装过 agent 的节点”，结果写入当前目录“计算节点部署情况.txt”
# cd /home/gpuops/gpu-ops && CONTROLLER_URL=http://192.0.2.10:60039 AGENT_TOKEN=<agent_token> SSH_GUARD_EXCLUDE_USERS="root gpuops" ENABLE_SYSTEM_CPU_RESERVE=1 SYSTEM_CPU_RESERVE_PERCENT=5 ENABLE_SYSTEM_MEMORY_RESERVE=1 SYSTEM_MEMORY_RESERVE_GB=8 PARALLEL=8 bash scripts/deploy_installed_nodes_only.sh
# 当只部署 60003 时，SSH Guard 排除用户需要额外包含 operator：
# cd /home/gpuops/gpu-ops && NODE_IDS="60003" CONTROLLER_URL=http://192.0.2.10:60039 AGENT_TOKEN=<agent_token> SSH_GUARD_EXCLUDE_USERS="root gpuops operator" ENABLE_SYSTEM_CPU_RESERVE=1 SYSTEM_CPU_RESERVE_PERCENT=5 ENABLE_SYSTEM_MEMORY_RESERVE=1 SYSTEM_MEMORY_RESERVE_GB=8 PARALLEL=8 bash scripts/deploy_installed_nodes_only.sh

# 控制节点先构建前端
cd /home/gpuops/gpu-ops/web && pnpm build
# 控制节点后台运行 + 开机自启（推荐）
cd /home/gpuops/gpu-ops && bash scripts/install_controller_local.sh
# 控制节点重启服务
sudo systemctl restart gpu-controller
# 常规更新不要跑 distribute_workspace.sh。
# 只用 deploy_installed_nodes_only.sh：它会分发必要代码、清理节点旧残留、统一权限并重装 agent。
# 注意：SYSTEM_MEMORY_RESERVE_GB 的含义是“给系统预留多少内存”，不是“把用户限制到多少内存”
NODE_IDS="60020 60018 60017 60015 60014 60012 60010 60008 60007 60006 60005 60004 60003 60002 60001 60000"
SSH_GUARD_EXCLUDE_USERS="root gpuops"
if [[ " ${NODE_IDS} " == *" 60003 "* ]]; then
  SSH_GUARD_EXCLUDE_USERS="${SSH_GUARD_EXCLUDE_USERS} operator"
fi
cd /home/gpuops/gpu-ops && \
  NODE_IDS="${NODE_IDS}" \
  CONTROLLER_URL=http://192.0.2.10:60039 \
  AGENT_TOKEN=<agent_token> \
  SSH_GUARD_EXCLUDE_USERS="${SSH_GUARD_EXCLUDE_USERS}" \
  ENABLE_SYSTEM_CPU_RESERVE=1 \
  SYSTEM_CPU_RESERVE_PERCENT=10 \
  ENABLE_SYSTEM_MEMORY_RESERVE=1 \
  SYSTEM_MEMORY_RESERVE_GB=12 \
  PARALLEL=8 \
  bash scripts/deploy_installed_nodes_only.sh
```
```bash
# 停止控制节点服务（需要时）
sudo systemctl stop gpu-controller && sudo systemctl disable gpu-controller && sudo systemctl status gpu-controller --no-pager
# 停止计算节点服务（node-agent + SSH Guard，需要时）
sudo systemctl stop gpu-node-agent gpu-ssh-guard-sync.timer gpu-ssh-guard-enforce.timer && sudo systemctl disable gpu-node-agent gpu-ssh-guard-sync.timer gpu-ssh-guard-enforce.timer && sudo systemctl status gpu-node-agent gpu-ssh-guard-sync.timer gpu-ssh-guard-enforce.timer --no-pager
# 前台运行（与上面后台方式二选一）
cd /home/gpuops/gpu-ops/controller && go run . --config ../config/controller.yaml
# 看 agent 主服务是否在运行
sudo systemctl status gpu-node-agent --no-pager
# 只看是否 active（适合脚本）
systemctl is-active gpu-node-agent
# 看 SSH Guard 定时器是否在跑
systemctl is-active gpu-ssh-guard-sync.timer gpu-ssh-guard-enforce.timer
systemctl list-timers --all | grep -E 'gpu-ssh-guard-(sync|enforce)\.timer'
```

说明：
- 计算节点安全防护由 `install_agent_local.sh` 安装并启用（SSH Guard + fail2ban + user.slice 资源预留）。
- 计算节点需启用 `gpu-node-agent` 开机自启（上面的 `systemctl enable --now gpu-node-agent`）。
- 控制节点需启用 `gpu-controller` 开机自启（`install_controller_local.sh` 会配置并启用 systemd 服务）。

---


## 🚀 60019 容灾一键部署（推荐）

> 容灾节点必须是独立主机。同步器会拒绝 localhost、本机任一 IP 和占位地址；在没有成功同步记录前，不应执行回切。

你给定的容灾节点（`60019`）可直接执行以下命令完成：
- 容灾控制器部署（端口可改，默认 `60019`，不是 `60039`）
- 主备工具版本一致化校验（go/node/pnpm/docker/psql）
- 首次主→备全量同步（controller/web/dist/数据库）
- 自动同步策略写入（默认每天凌晨 `03:00`）

```bash
cd /home/gpuops/gpu-ops
PRIMARY_HOST=<主控制节点IP> \
DR_SSH_USER=<60019-SSH用户> \
DR_CONTROLLER_PORT=60019 \
SYNC_INTERVAL_DAYS=1 \
SYNC_START_HOUR=3 \
bash scripts/deploy_dr_node_60019.sh
```

说明：
- 默认读取容灾私钥：`/home/gpuops/gpu-ops/my_ssh_keys/node_60019.txt`。
- 可在管理界面 `/admin/ha` 修改“每隔几天同步、几点开始、容灾端口、手动正反向同步、查看上次同步时间与日志”。  

## 🛡️ 将 `60009` 改为容灾节点（退出计算节点）

目标：
- `60009` 不再作为计算节点上报（不再运行 `gpu-node-agent`）。
- `60009` 改为控制器备机（`ha_role=standby`）。
- 备机 controller 版本与主控完全一致（可对齐二进制 + 前端 dist）。
- 自动实时探测主控健康，主控故障自动切到备机，主控恢复后自动回切。
- 管理后台在 `/admin/ha` 实时查看主备状态（自动刷新）。

### 1) 在 `60009` 上停止并禁用计算节点服务

```bash
sudo systemctl stop gpu-node-agent gpu-ssh-guard-sync.timer gpu-ssh-guard-enforce.timer
sudo systemctl disable gpu-node-agent gpu-ssh-guard-sync.timer gpu-ssh-guard-enforce.timer
systemctl is-active gpu-node-agent gpu-ssh-guard-sync.timer gpu-ssh-guard-enforce.timer
```

### 2) （可选）从控制器清理 `60009` 的“计算节点实时状态”

> 下面只清理节点运行态/策略/映射，不删除历史 `usage_records` 计费记录。

```bash
cd /home/gpuops/gpu-ops
if docker compose version >/dev/null 2>&1; then
  DC="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  DC="docker-compose"
else
  echo "未检测到 docker compose / docker-compose，请先安装 docker-compose-plugin"
  exit 1
fi
$DC exec -T postgres psql -U gpuops -d gpuops <<'SQL'
BEGIN;
DELETE FROM node_user_cpu_limits      WHERE node_id='60009';
DELETE FROM node_user_memory_limits   WHERE node_id='60009';
DELETE FROM node_user_disk_quotas     WHERE node_id='60009';
DELETE FROM node_local_users          WHERE node_id='60009';
DELETE FROM node_runtime_snapshots    WHERE node_id='60009';
DELETE FROM node_security_events      WHERE node_id='60009';
DELETE FROM node_policies             WHERE node_id='60009';
DELETE FROM node_exclusive_gpu_users  WHERE node_id='60009';
DELETE FROM node_exclusive_users      WHERE node_id='60009';
DELETE FROM node_view_acl             WHERE node_id='60009';
DELETE FROM user_node_accounts        WHERE node_id='60009';
DELETE FROM nodes                     WHERE node_id='60009';
COMMIT;
SQL
```

### 3) 配置主备控制器（主机与 `60009` 互指）

先生成一份共享 token（两边相同）：

```bash
openssl rand -hex 32
```

主控制器（primary）配置示例：

```bash
cd /home/gpuops/gpu-ops
sed -i 's/^ha_enabled:.*/ha_enabled: true/' config/controller.yaml
sed -i 's/^ha_node:.*/ha_node: "controller-primary"/' config/controller.yaml
sed -i 's/^ha_role:.*/ha_role: "primary"/' config/controller.yaml
sed -i 's#^ha_peer_url:.*#ha_peer_url: "http://<60009-IP>:60039"#' config/controller.yaml
sed -i 's#^ha_token:.*#ha_token: "<同一份HA_TOKEN>"#' config/controller.yaml
sudo systemctl restart gpu-controller
```

`60009`（standby）配置示例：

```bash
cd /home/gpuops/gpu-ops
sed -i 's/^ha_enabled:.*/ha_enabled: true/' config/controller.yaml
sed -i 's/^ha_node:.*/ha_node: "controller-60009"/' config/controller.yaml
sed -i 's/^ha_role:.*/ha_role: "standby"/' config/controller.yaml
sed -i 's#^ha_peer_url:.*#ha_peer_url: "http://<PRIMARY-IP>:60039"#' config/controller.yaml
sed -i 's#^ha_token:.*#ha_token: "<同一份HA_TOKEN>"#' config/controller.yaml
cd /home/gpuops/gpu-ops && bash scripts/install_controller_local.sh
sudo systemctl restart gpu-controller
```

说明：
- 建议主备控制器连接同一个 PostgreSQL（`database_dsn` 一致），这样 `/admin/ha` 更容易保持“已同步”。
- 主备间 `/api/ha/status` 使用独立 `X-HA-Token` 鉴权，因此两边 `ha_token` 必须一致。

### 4) 验证容灾状态

```bash
curl -s -H "Authorization: Bearer <admin_token>" http://127.0.0.1:60039/api/admin/ha/status | jq
```

后台页面：`/admin/ha`（自动刷新）。

### 5) 让备机版本与主控完全一致（推荐）

在 `60009`（standby）执行：

```bash
cd /home/gpuops/gpu-ops
PRIMARY_HOST=gpuops@<PRIMARY-IP> \
SYNC_WEB_DIST=1 \
RESTART_SERVICE=1 \
bash scripts/sync_controller_release_from_primary.sh
```

说明：
- 该脚本会把主控 `/usr/local/bin/gpu-controller` 拉到备机并校验 `sha256` 一致。
- 默认同步 `web/dist`，并重启 `gpu-controller`。
- `--version` 信息也会在脚本末尾输出，便于人工核对。

### 6) 开启自动故障切换/恢复回切（VIP + Keepalived）

先选一个业务 VIP（示例 `192.0.2.10/24`），并让所有 Node Agent / 管理访问都走这个 VIP 的 `:60039`。

主控（primary）执行：

```bash
cd /home/gpuops/gpu-ops
HA_ROLE=primary \
HA_INTERFACE=eth0 \
HA_VIP=192.0.2.10/24 \
HA_PEER_IP=<60009-IP> \
HA_AUTH_PASS=gpuhavip \
CONTROLLER_HEALTH_URL=http://127.0.0.1:60039/readyz \
bash scripts/install_ha_vip_local.sh
```

备机（standby）执行：

```bash
cd /home/gpuops/gpu-ops
HA_ROLE=standby \
HA_INTERFACE=eth0 \
HA_VIP=192.0.2.10/24 \
HA_PEER_IP=<PRIMARY-IP> \
HA_AUTH_PASS=gpuhavip \
CONTROLLER_HEALTH_URL=http://127.0.0.1:60039/readyz \
bash scripts/install_ha_vip_local.sh
```

说明：
- `install_ha_vip_local.sh` 会安装 `keepalived`，每 2 秒检测本机 `gpu-controller + /readyz`（包含数据库连通性）。
- primary 优先级更高：故障时 VIP 自动漂移到 standby；primary 修复后会自动抢回 VIP（回切）。

### 7) 演练自动切换与回切

在 primary 执行（模拟故障）：

```bash
sudo systemctl stop gpu-controller
```

在 standby 看 VIP 是否接管：

```bash
ip -4 addr show dev <网卡名> | grep 192.0.2.10
```

修复后在 primary 执行：

```bash
sudo systemctl start gpu-controller
```

等待数秒后，VIP 应自动回到 primary。

## 🔐 加密备份与恢复演练

平台使用 restic 保存 PostgreSQL 归档、控制器配置、当前控制器二进制、前端产物和 systemd/Keepalived 配置，默认保留 7 个日备份、4 个周备份和 12 个月备份。用户科研数据不属于平台备份范围，不会默认扫描 `/srv/gpu-ops/nodes` 或 `/srv/gpu-ops/cluster`。备份仓库应位于独立磁盘、SFTP 或对象存储。

```bash
cd /home/gpuops/gpu-ops
BACKUP_REPOSITORY='sftp:backup@<backup-host>:/srv/restic/gpu-ops' \
bash scripts/install_backup_local.sh

# 安装后立即生成首份备份并验证
sudo systemctl start gpuops-backup.service
sudo systemctl start gpuops-backup-verify.service
```

- 每日备份：`gpuops-backup.timer`，默认 02:00。
- 每周隔离恢复：`gpuops-backup-verify.timer`，默认周日 04:00。
- 恢复演练会启动一次性 PostgreSQL 容器，不覆盖生产数据库。
- 管理员可在 `/admin/ha` 查看最近快照和恢复演练状态。
- Restic 密码文件必须另行离线保管；密码与仓库同时丢失时无法恢复。
- 如确实需要附带其他小型目录，可在安装时显式设置 `BACKUP_DATA_PATHS='/path/one /path/two'`；不要用它备份大规模科研数据。
- 备份脚本默认按宿主机发布端口 `5432` 自动识别 PostgreSQL 容器；存在多个候选时必须显式设置 `POSTGRES_CONTAINER=<容器名>`。

---

## 🛠️ scripts 目录说明（作用 + 用法）

> 路径：`/home/gpuops/gpu-ops/scripts`

| 脚本 | 作用 | 常用用法 |
|---|---|---|
| `install_deps_ubuntu2204.sh` | Ubuntu 22.04 一键安装项目依赖（Go/Node/pnpm/Docker 等） | `bash scripts/install_deps_ubuntu2204.sh` |
| `install_controller_local.sh` | 在控制器本机安装并启用 `gpu-controller` systemd 服务 | `cd /home/gpuops/gpu-ops && bash scripts/install_controller_local.sh` |
| `sync_controller_release_from_primary.sh` | 备机从主控拉取 controller 二进制（+可选 web/dist），确保版本完全一致 | `PRIMARY_HOST=gpuops@<PRIMARY-IP> bash scripts/sync_controller_release_from_primary.sh` |
| `ha_sync_worker.sh` | 容灾同步执行器（主→备 / 备→主，含版本一致性校验、二进制/前端/数据库同步） | `HA_SYNC_DIRECTION=primary_to_standby DR_HOST=<DR-IP> DR_SSH_USER=<user> DR_KEY_FILE=... bash scripts/ha_sync_worker.sh` |
| `deploy_dr_node_60019.sh` | 一键部署 60019 容灾节点并写入自动同步策略（默认每天 03:00） | `PRIMARY_HOST=<PRIMARY-IP> DR_SSH_USER=<user> DR_CONTROLLER_PORT=60019 bash scripts/deploy_dr_node_60019.sh` |
| `install_ha_vip_local.sh` | 安装 Keepalived VRRP VIP，主备自动切换与修复回切 | `HA_ROLE=primary HA_INTERFACE=eth0 HA_VIP=192.0.2.10/24 HA_PEER_IP=<standby-ip> bash scripts/install_ha_vip_local.sh` |
| `install_backup_local.sh` | 安装 restic 每日加密备份与每周隔离恢复演练 | `BACKUP_REPOSITORY=sftp:backup@<host>:/srv/restic/gpu-ops bash scripts/install_backup_local.sh` |
| `gpuops_backup.sh` | 备份 PostgreSQL 与控制器运行配置并执行保留策略 | `sudo systemctl start gpuops-backup.service` |
| `gpuops_backup_verify.sh` | 在一次性 PostgreSQL 容器内执行完整恢复校验 | `sudo systemctl start gpuops-backup-verify.service` |
| `install_agent_local.sh` | 在计算节点本机一键安装并启用 `gpu-node-agent` | `NODE_ID=60001 CONTROLLER_URL=http://<控制器IP>:60039 AGENT_TOKEN=<token> bash scripts/install_agent_local.sh` |
| `deploy_agent.sh` | 从控制端批量部署 agent 到多台节点 | `NODES='60000:192.0.2.10 60001:192.0.2.10' AGENT_TOKEN=<token> CONTROLLER_URL=http://<控制器IP>:60039 bash scripts/deploy_agent.sh` |
| `deploy_controller.sh` | 部署 controller 二进制与配置到远端控制器主机 | `HOST=<控制器主机> CONTROLLER_BIN=./controller/controller bash scripts/deploy_controller.sh` |
| `distribute_workspace.sh` | 将当前仓库分发到各计算节点 `/home/<用户>/<项目目录>`（默认拒绝裸跑；支持并发和 `NODE_IDS`） | `DRY_RUN=1 NODE_IDS='60020 60002' PARALLEL=8 bash scripts/distribute_workspace.sh` |
| `deploy_installed_nodes_only.sh` | 并发分发并仅重装“已安装过 gpu-node-agent”的节点，未安装节点仅更新目录，自动生成 `计算节点部署情况.txt`，支持 `NODE_IDS` 过滤 | `NODE_IDS='60020 60002' CONTROLLER_URL=http://<控制器IP>:60039 AGENT_TOKEN=<token> SSH_GUARD_EXCLUDE_USERS='root ...' PARALLEL=8 bash scripts/deploy_installed_nodes_only.sh` |
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
