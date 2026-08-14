# Go-Live Checklist

**English** | [简体中文](zh-CN/go-live-checklist.md)

Work through this before enabling enforcement on a production fleet. Pilot on a few nodes first, then expand.

## 1. Environment

- [ ] PostgreSQL is available on a dedicated instance, with a plan for backups and, if needed, replication.
- [ ] The controller reaches PostgreSQL, and nodes reach the controller listener.
- [ ] Each node satisfies at least one enforcement path: systemd (recommended), cgroup v2 at `/sys/fs/cgroup`, or cgroup v1 with the cpu controller.
- [ ] `bash scripts/node_prereq_check.sh` has been run on every node. It changes nothing.
- [ ] Time is synchronised across the controller and all nodes. Accounting windows depend on it.

## 2. Secrets

- [ ] `agent_token`, `admin_token`, and `auth_secret` are three different values from `openssl rand -hex 32`.
- [ ] No placeholder value remains. First-run Setup refuses to save while a required readiness check fails, and a secret containing `replace-with`, `change-me`, or `example`, or shorter than 16 characters, fails that check.
- [ ] `ha_token` is set and distinct, if HA is enabled.
- [ ] Secrets live outside the repository, with restricted file permissions, and are recorded in your secret manager.
- [ ] `config/*.local.yaml` and `.env` are confirmed git-ignored.

## 3. Network and TLS

- [ ] The Web listener sits behind an HTTPS reverse proxy.
- [ ] `cookie_secure: true`.
- [ ] `internal_listen_addr` is configured with a certificate and key, and firewalled to node and controller networks.
- [ ] Administrator routes are not reachable from the public internet.
- [ ] A login rate limiter is configured at the proxy.
- [ ] If Turnstile is enabled, `turnstile_expected_hostnames` matches the hostname users actually type, and both the browser and the controller can reach `challenges.cloudflare.com:443`.

## 4. Database

- [ ] Migrations applied cleanly at startup — the controller refuses to start otherwise.
- [ ] `resource_prices` contains every GPU model in the fleet plus the reserved `CPU_CORE` entry.
- [ ] A retention policy is set for `usage_records`.

## 5. Functional loop

Verify each of these end to end before enforcement:

- [ ] `GET /healthz` returns `{"ok":true}`.
- [ ] `GET /readyz` returns `{"ok":true,"database":true}`.
- [ ] `GET /metrics` returns counters when called with an operator credential, and `401` without one.
- [ ] `POST /api/admin/bootstrap` created the first administrator, and the Web login works.
- [ ] First-run Setup is complete; public registration behaves as intended.
- [ ] `GET /api/admin/nodes` shows every expected node reporting recently.
- [ ] Replaying an identical report (same `report_id`) does not charge twice.
- [ ] A GPU process visible to `nvidia-smi --query-compute-apps` produces a `usage_records` row with a plausible cost.
- [ ] A CPU-heavy process produces a CPU-only usage row.
- [ ] Dropping a test user to `limited` blocks new GPU jobs via the shell hook and issues `set_cpu_quota`. Confirm on the node: `systemctl show user-<uid>.slice -p CPUQuota`.
- [ ] Dropping a test user to `blocked` issues `kill_process` after the grace period and applies the hard CPU limit.
- [ ] Registration, bind review, and account provisioning complete for one real user.

## 6. Security

- [ ] Two-factor authentication is enabled for every administrator account.
- [ ] Power-user permission bits grant only what each operator needs.
- [ ] Per-node agent tokens are populated. Run with `agent_node_token_enforce: false` until every node reports, then set it to `true` and restart.
- [ ] SSH guard behaviour is deliberate: you have chosen a `SSH_GUARD_FAIL_OPEN` value knowing that `0` turns a controller outage into a fleet-wide lockout.
- [ ] Out-of-band access to every node exists — console, or an excluded account with a key you hold — before the guard is enabled.
- [ ] The security model has been read and residual risks accepted. See [Security model](security.md).

## 7. Backups

- [ ] `scripts/gpuops_backup.sh` runs on a timer, to a repository on a separate disk or host.
- [ ] The restic password is stored in a secret manager and is recoverable by more than one person.
- [ ] `scripts/gpuops_backup_verify.sh` has restored a snapshot successfully at least once.
- [ ] Admin → Status shows current backup and verify timestamps.
- [ ] An alert fires when either timestamp goes stale.

## 8. Rollout

- [ ] `dry_run: true` for the first 1–3 days. Usage is recorded and cost is calculated, but nothing is deducted.
- [ ] Sampled usage records have been compared against what the nodes actually did.
- [ ] `warning_threshold`, `limited_threshold`, `kill_grace_period_seconds`, and the monthly overdraft ceiling are tuned to your users' working patterns.
- [ ] A pilot group has run under real conditions and their charges look right.
- [ ] Users have been told what changes, how to check their balance, and how to request more points.
- [ ] Only then: set `dry_run: false` and restart.

## 9. After go-live

- [ ] Monitoring alerts on `gpuops_controller_last_report_unix` falling behind.
- [ ] A named person owns the daily node-health and security-event review.
- [ ] The rollback path is written down: set `dry_run: true` and restart to stop deductions immediately.
- [ ] A restore and failover drill is scheduled. See [Backup and HA](backup-and-ha.md).
