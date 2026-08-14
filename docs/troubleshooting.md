# Troubleshooting

**English** | [简体中文](zh-CN/troubleshooting.md)

Symptoms grouped by where they appear. Start with `/readyz` — most controller-side problems are database problems.

## Controller will not start

**`配置校验失败` / config validation failed.** `Validate()` rejected the configuration. The message names the field. Common causes: `agent_token`, `admin_token`, or `auth_secret` empty; `auth_secret` shorter than 16 characters while `session_hours > 0`; `limited_threshold` not below `warning_threshold`; `internal_listen_addr` set without a certificate and key.

**`未找到默认配置文件` / no default config file.** The controller looks for `CONTROLLER_CONFIG`, then `../config/controller.yaml`, then `config/controller.yaml`. Pass `--config` explicitly.

**`连接数据库失败` / database connection failed.** Check the DSN, that PostgreSQL is running, and that credentials match. In Docker, the DSN host is the service name `postgres`, not `127.0.0.1`.

**`数据库迁移失败` / migration failed.** A migration errored, and the controller refuses to start on a partially migrated schema. Read the error, restore from backup if needed, and do not delete migration rows to force startup.

**Environment overrides are ignored.** Only `GPUOPS_*` variables are read, and only when non-empty. An empty value means "unset" and leaves the YAML value in place. `GPUOPS_DRY_RUN` and `GPUOPS_COOKIE_SECURE` must be a boolean word (`true`/`false`/`1`/`0`/`yes`/`no`/`on`/`off`); anything else fails startup.

## Readiness and health

| Response | Meaning |
| --- | --- |
| `/healthz` OK, `/readyz` 503 | The process is up; PostgreSQL is unreachable |
| Both fail | The controller is not listening, or you have the wrong port |
| `/readyz` OK but the UI is blank | `web/dist` was not found or not built |

The default port is now `8080`. Deployments created before this change used `60039` — check `listen_addr` if a previously working URL stops responding.

## Web UI

**Blank page or 404 on every route.** The controller only serves the UI when it finds a build. Run `pnpm --dir web build`, or set `web_dir` explicitly. Auto-detection tries `../web/dist` then `web/dist` relative to the working directory, so starting the controller from an unexpected directory silently disables the UI.

**API calls fail in `pnpm dev`.** Vite proxies `/api`, `/metrics`, and `/healthz` to `127.0.0.1:8080`. The controller must be listening there.

**Logged out immediately after logging in.** With `cookie_secure: true` over plain HTTP the browser discards the cookie. Either serve HTTPS or set `cookie_secure: false` for local work.

**`csrf_required` on every write.** The client must send `X-CSRF-Token` from `GET /api/auth/me`. This also appears when a session cookie is being replayed by a tool that does not carry the header.

**Everyone was logged out at once.** `auth_secret` changed. Sessions are signed with it, so changing it invalidates all of them.

## Authentication and access

**`401 unauthorized` on `/api/admin/*` with a token.** The header must be exactly `Authorization: Bearer <admin_token>`, and the token must match `admin_token` byte for byte, including length.

**`403 forbidden` for a power user.** The account authenticated but lacks the permission bit for that route. Grant it under Admin → Power users.

**`401` on `/api/users/<name>/balance`.** This endpoint now requires a credential. Use `admin_token`, an agent token, an administrator session, or the user's own session. Node-side helpers read a token from `GPUOPS_QUERY_TOKEN` or `/etc/gpu-ops/query-token`.

**`404 用户不存在` on a balance query.** The user has no row yet. The endpoint no longer creates one as a side effect; the row appears once the user is provisioned or first reported by an agent.

**`401` when scraping `/metrics`.** Scrapers must send `Authorization: Bearer <admin_token>` or `X-Agent-Token`.

## Agents

**A node never appears.** Check reachability to `CONTROLLER_URL` from the node, then the agent journal for `401`. With `agent_node_token_enforce: true`, the token must be the one registered for that exact `NODE_ID`.

**A node disappears after a rename.** `node_id` is the identity key. Renaming creates a new node and orphans the old history.

**Reports arrive but nothing is charged.** Check `dry_run`. In dry-run mode usage is recorded and cost is calculated, but balances are not deducted.

**Charges look too high or too low.** The controller converts samples using `interval_seconds` from the report, falling back to `sample_interval_seconds`. A mismatch between the agent's real interval and the configured value scales every charge. Also confirm the GPU model in `resource_prices` matches what the node reports, or the fallback `default_price_per_minute` is being used.

**Duplicate charges.** Deduplication is keyed on `report_id`. Duplicates almost always mean two agent instances are running on one node.

## Database

**`too many connections`.** Other clients are consuming the pool, or several controllers point at one database. Only one controller should write to a database.

**Queries slow down over time.** `usage_records` grows without bound. Configure retention under Admin → Usage.

**Deleting a usage range did not free disk space.** PostgreSQL reuses the space internally. Run `VACUUM` if you need it returned to the filesystem.

## Docker

**`set POSTGRES_PASSWORD in .env`.** Compose refuses to start without the secrets. `cp .env.example .env` and fill in every value.

**The controller container restarts repeatedly.** `docker compose --profile full logs controller`. Usually a rejected configuration value or an unreachable database.

**Health check never passes.** The check runs `/app/controller --healthcheck`, which probes `/readyz` on the port from `GPUOPS_LISTEN_ADDR`. If you override that variable, the check follows it. `start_period` is 30s to allow for migrations; a very large migration backlog on first start can exceed it.

**Port already in use.** Change `CONTROLLER_PUBLISH` in `.env`. It only changes the host-side port; the container still listens on 8080.

## SSH guard

**Nobody can log in to a node.** The guard denies unregistered accounts, and `SSH_GUARD_FAIL_OPEN=0` denies everyone when the controller is unreachable. Use console access, set `SSH_GUARD_FAIL_OPEN=1`, or add the account to `SSH_GUARD_EXCLUDE_USERS`.

**A registered user is still blocked.** The node uses cached lists synced every `SSH_GUARD_SYNC_INTERVAL`. Check that the registry endpoints return the expected users for that node.

## Collecting diagnostics

```bash
curl -fsS http://127.0.0.1:8080/readyz
sudo journalctl -u gpu-controller -n 200 --no-pager
sudo journalctl -u gpu-node-agent -n 200 --no-pager
docker compose --profile full logs --tail 200 controller
bash scripts/check_status.sh
```

Redact tokens, DSNs, hostnames, and usernames before sharing any of this in a public issue.
