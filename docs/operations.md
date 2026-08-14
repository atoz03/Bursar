# Operations

**English** | [简体中文](zh-CN/operations.md)

Day-to-day lifecycle, upgrade, and recovery procedures for a running deployment.

## Service lifecycle

### Controller from source

```bash
sudo systemctl status gpu-controller
sudo systemctl restart gpu-controller
sudo journalctl -u gpu-controller -f
```

### Controller in Docker

```bash
docker compose --profile full ps
docker compose --profile full restart controller
docker compose --profile full logs -f controller
```

### Node agent

```bash
sudo systemctl status gpu-node-agent
sudo journalctl -u gpu-node-agent -f
```

## Health checks

| Endpoint | Meaning |
| --- | --- |
| `GET /healthz` | The process is serving HTTP |
| `GET /readyz` | The process is serving HTTP **and** PostgreSQL responds |
| `GET /metrics` | Aggregate counters; requires an operator credential |

```bash
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS -H "Authorization: Bearer <admin_token>" http://127.0.0.1:8080/metrics
```

Use `/readyz` for load-balancer and container health checks. `/healthz` returns success even when the database is down, so it cannot stand in for readiness.

`scripts/check_status.sh` runs a broader fleet check and honours `CONTROLLER_URL`.

## Monitoring

The controller exposes a small set of Prometheus-format counters:

| Metric | Meaning |
| --- | --- |
| `gpuops_controller_reports_total` | Accepted agent reports |
| `gpuops_controller_reports_duplicate_total` | Reports rejected as duplicates |
| `gpuops_controller_usage_records_total` | Usage rows written |
| `gpuops_controller_actions_notify_total` | Notify actions issued |
| `gpuops_controller_actions_block_user_total` | Block actions issued |
| `gpuops_controller_actions_unblock_user_total` | Unblock actions issued |
| `gpuops_controller_actions_kill_process_total` | Process termination actions issued |
| `gpuops_controller_actions_set_cpu_quota_total` | CPU quota actions issued |
| `gpuops_controller_actions_set_memory_limit_total` | Memory limit actions issued |
| `gpuops_controller_last_report_unix` | Unix timestamp of the most recent report |

Scraping requires a credential. In a Prometheus job:

```yaml
scrape_configs:
  - job_name: gpu-ops
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/gpuops-admin-token
    static_configs:
      - targets: ["controller.example.org:8080"]
```

Alert on `gpuops_controller_last_report_unix` falling behind: a fleet-wide gap means agents stopped reporting, which silently stops accounting and enforcement.

## Routine checks

**Daily.** Confirm every expected node is reporting (Admin → Nodes shows agent health and last-seen). Review security events.

**Weekly.** Review pending registrations and account-binding requests. Check backup and backup-verify status on the Admin → Status page.

**Monthly.** Confirm the monthly points reset ran as configured. Review prices against the actual hardware inventory. Review administrator accounts and their permission bits.

## Upgrades

Take a database backup first. Migrations are forward-only; rolling back means restoring.

### From source

```bash
git fetch --tags
git checkout <tag>
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
go build -o /opt/gpu-controller/controller ./controller
sudo systemctl restart gpu-controller
curl -fsS http://127.0.0.1:8080/readyz
```

### With Docker

```bash
git fetch --tags && git checkout <tag>
docker compose --profile full build controller
docker compose --profile full up -d controller
docker compose --profile full ps
```

### Node agents

Upgrade agents after the controller. Roll one node first, confirm it reports, then continue. `scripts/deploy_installed_nodes_only.sh` rolls out to nodes that already have the agent installed.

### Version skew

The controller accepts reports from older agents. Do not run a newer agent against an older controller: agents may send fields the controller does not understand.

## Database maintenance

Migrations run automatically at controller startup, in order, from `database/migrations/`. Startup fails if a migration fails; the controller does not start with a partially migrated schema.

Usage records grow continuously. Configure retention under Admin → Usage, or call the retention API. `POST /api/admin/usage/delete-range` deletes a date range irreversibly — take a backup first and check the range estimate endpoint before running it.

## Rotating secrets

**Agent token.** Put the current token in `agent_legacy_tokens`, set the new value as `agent_token`, restart the controller, roll the new token to every node, then clear `agent_legacy_tokens` and restart again.

**Admin token.** Change `admin_token` and restart. Every script holding the old value must be updated; there is no rotation window.

**Auth secret.** Changing `auth_secret` invalidates all browser sessions immediately. Everyone must sign in again. This is also the fastest way to force a global logout.

**Database password.** Update PostgreSQL, then the DSN (or `GPUOPS_DATABASE_DSN`), then restart.

## Incident playbook

**Agents stopped reporting.** Check `/readyz`. If the database is down, fix it first — the controller cannot accept reports without it. Enforcement pauses while reports are missing; usage during the gap is not recorded and cannot be reconstructed.

**Wrong charges.** Set `dry_run: true` and restart to stop deductions while you investigate. Compare `usage_records` against node reality before adjusting balances.

**Suspected credential compromise.** Rotate the affected token as above. For `auth_secret`, rotating also logs everyone out. Review security events and administrator audit notes.

**Accidental mass enforcement.** Unblock affected users from Admin → Users, then set `dry_run: true` until the policy is corrected.

## Related

- [Backup and HA](backup-and-ha.md)
- [Troubleshooting](troubleshooting.md)
- [Security model](security.md)
