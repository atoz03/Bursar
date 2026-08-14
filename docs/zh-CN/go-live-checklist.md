# 上线检查清单

[English](../go-live-checklist.md) | **简体中文**

在生产集群启用策略执行前，请逐项确认。建议先在少量节点试点，再逐步扩大。

## 1. 环境

- [ ] PostgreSQL 部署在独立实例上，并已规划备份，必要时规划复制。
- [ ] 控制器能连通 PostgreSQL，各节点能连通控制器监听地址。
- [ ] 每个节点至少满足一种限制路径：systemd（推荐）、`/sys/fs/cgroup` 下的 cgroup v2，或带 cpu 控制器的 cgroup v1。
- [ ] 已在每个节点执行 `bash scripts/node_prereq_check.sh`。该脚本不做任何修改。
- [ ] 控制器与所有节点时间已同步。计量窗口依赖时间。

## 2. 密钥

- [ ] `agent_token`、`admin_token`、`auth_secret` 是三个互不相同的值，由 `openssl rand -hex 32` 生成。
- [ ] 没有残留任何占位值。只要有必填就绪检查未通过，首次 Setup 就拒绝保存；密钥中含有 `replace-with`、`change-me`、`example`，或长度不足 16 字符，都会导致该检查失败。
- [ ] 若启用 HA，`ha_token` 已设置且与其它 token 不同。
- [ ] 密钥存放在仓库之外，文件权限受限，并已记录在密钥管理系统中。
- [ ] 已确认 `config/*.local.yaml` 与 `.env` 被 Git 忽略。

## 3. 网络与 TLS

- [ ] Web 监听置于 HTTPS 反向代理之后。
- [ ] `cookie_secure: true`。
- [ ] `internal_listen_addr` 已配置证书与私钥，并通过防火墙限定在节点与控制器网络内。
- [ ] 管理员路由不可从公网访问。
- [ ] 已在代理层配置登录限流。
- [ ] 若启用 Turnstile，`turnstile_expected_hostnames` 与用户实际输入的主机名一致，且浏览器与控制器都能访问 `challenges.cloudflare.com:443`。

## 4. 数据库

- [ ] 启动时迁移全部成功执行——否则控制器会拒绝启动。
- [ ] `resource_prices` 覆盖集群中的每种 GPU 型号，并包含保留条目 `CPU_CORE`。
- [ ] 已为 `usage_records` 设置保留策略。

## 5. 功能闭环

启用策略执行前，请端到端验证以下每一项：

- [ ] `GET /healthz` 返回 `{"ok":true}`。
- [ ] `GET /readyz` 返回 `{"ok":true,"database":true}`。
- [ ] `GET /metrics` 在携带运维凭据时返回计数器，不带凭据时返回 `401`。
- [ ] `POST /api/admin/bootstrap` 已创建第一个管理员，且 Web 登录可用。
- [ ] 首次 Setup 已完成，公开注册行为符合预期。
- [ ] `GET /api/admin/nodes` 显示所有预期节点都在近期上报。
- [ ] 重放同一条上报（相同 `report_id`）不会重复扣费。
- [ ] `nvidia-smi --query-compute-apps` 可见的 GPU 进程能产生一条成本合理的 `usage_records` 记录。
- [ ] 高 CPU 占用的进程能产生一条纯 CPU 的用量记录。
- [ ] 把测试用户降到 `limited` 后，shell 钩子拦截新的 GPU 任务并下发 `set_cpu_quota`。在节点上确认：`systemctl show user-<uid>.slice -p CPUQuota`。
- [ ] 把测试用户降到 `blocked` 后，宽限期结束会下发 `kill_process`，并施加严格 CPU 限制。
- [ ] 已为一名真实用户走完注册、绑定审核与账号开通全流程。

## 6. 安全

- [ ] 所有管理员账号都已启用两步验证。
- [ ] 高级用户权限位只授予各自需要的能力。
- [ ] 每节点 Agent token 已配置齐全。先以 `agent_node_token_enforce: false` 运行，待所有节点均已上报后改为 `true` 并重启。
- [ ] SSH guard 行为是经过考虑的决定：你在知道 `SSH_GUARD_FAIL_OPEN=0` 会把控制器故障放大为全集群锁死的前提下，选择了对应取值。
- [ ] 启用 guard 之前，确保每个节点都有带外访问方式——控制台，或一个持有密钥的排除账号。
- [ ] 已阅读安全模型并接受残余风险，见[安全模型](security.md)。

## 7. 备份

- [ ] `scripts/gpuops_backup.sh` 由定时器执行，仓库位于独立磁盘或独立主机。
- [ ] restic 密码保存在密钥管理系统中，且不止一人能够取回。
- [ ] `scripts/gpuops_backup_verify.sh` 至少成功恢复过一次快照。
- [ ] 管理端 → 集群总览显示的备份与验证时间戳是最新的。
- [ ] 任一时间戳过期时会触发告警。

## 8. 灰度上线

- [ ] 前 1–3 天保持 `dry_run: true`。此时记录用量并计算成本，但不扣减。
- [ ] 已抽样比对用量记录与节点实际运行情况。
- [ ] `warning_threshold`、`limited_threshold`、`kill_grace_period_seconds` 与每月欠费上限已按用户的实际使用习惯调优。
- [ ] 试点用户已在真实条件下运行，且扣费结果合理。
- [ ] 已向用户说明变化内容、如何查询余额、如何申请更多积分。
- [ ] 以上都完成之后，再设置 `dry_run: false` 并重启。

## 9. 上线之后

- [ ] 监控已对 `gpuops_controller_last_report_unix` 落后设置告警。
- [ ] 有明确责任人负责每日的节点健康与安全事件巡检。
- [ ] 回滚路径已写入文档：设置 `dry_run: true` 并重启，即可立即停止扣费。
- [ ] 已安排恢复与接管演练，见[备份与 HA](backup-and-ha.md)。
