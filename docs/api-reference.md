# API Reference

**English** | [简体中文](zh-CN/api-reference.md)

The HTTP contract for agents, node-side helpers, and operator scripts. This is a catalogue of the important endpoints, not an exhaustive list of every route.

## Authentication

| Caller | Mechanism |
| --- | --- |
| Node agent | `X-Agent-Token: <token>` |
| Operator script | `Authorization: Bearer <admin_token>` |
| Browser | HttpOnly session cookie issued by `POST /api/auth/login` |
| HA peer | `X-HA-Token: <token>` |

**CSRF.** When authenticating with a session cookie, every non-GET request must carry `X-CSRF-Token`, taken from the `csrf_token` field of `GET /api/auth/me`. Requests without it return `403 csrf_required`.

**Listener placement.** When `internal_listen_addr` is configured, agent, registry, and HA routes are served only on the internal listener (default `8081`). When it is empty, they are also mounted on the Web listener (default `8080`).

## Health and metrics

### `GET /healthz`

No authentication. Returns `{"ok":true}` while the process is serving HTTP. It does not check the database.

### `GET /readyz`

No authentication. Returns `{"ok":true,"database":true}`, or `503` with `{"ok":false,"database":false}` when PostgreSQL is unreachable. Use this for load balancer and container health checks.

### `GET /metrics`

Requires `admin_token`, an agent token, or an administrator session. Returns aggregate counters in Prometheus text format. See [Operations](operations.md) for the metric list and a scrape configuration.

## Sessions

### `POST /api/admin/bootstrap`

Creates the first administrator. Allowed only while the administrator table is empty, and only with `Authorization: Bearer <admin_token>` — a session cannot bootstrap itself.

```json
{"username":"admin","password":"<strong-password>"}
```

### `POST /api/auth/login`

```json
{"username":"admin","password":"..."}
```

Returns `{"ok":true,"role":"admin"}` and sets an HttpOnly, `SameSite=Lax` session cookie.

### `GET /api/auth/me`

Returns the current session and the CSRF token:

```json
{"authenticated":true,"username":"admin","role":"admin","expires_at":"2026-08-14T16:00:00Z","csrf_token":"..."}
```

### `POST /api/auth/logout`

Returns `{"ok":true}` and clears the cookie.

## Agent reporting

### `POST /api/metrics`

Requires `X-Agent-Token`. `report_id` is mandatory and must be globally unique — the controller deduplicates on it so that an agent retry cannot charge twice.

```json
{
  "node_id": "node-01",
  "timestamp": "2026-08-14T16:00:00Z",
  "report_id": "2f6c7b3b3c3b4a8b8f1c5c3c1b2a9d10",
  "interval_seconds": 60,
  "users": [
    {
      "username": "alice",
      "pid": 12345,
      "cpu_percent": 120.5,
      "memory_mb": 2048,
      "gpu_usage": [
        {"gpu_id": 0, "gpu_model": "NVIDIA A100-SXM4-80GB", "gpu_bus_id": "00000000:3B:00.0", "memory_mb": 4096}
      ]
    }
  ]
}
```

Response:

```json
{
  "actions": [
    {"type": "notify", "username": "alice", "message": "..."},
    {"type": "set_cpu_quota", "username": "alice", "cpu_quota_percent": 50, "reason": "..."}
  ]
}
```

`node_id` is a stable operator-chosen identifier. When a `(node_id, local_username)` mapping exists, the controller charges the mapped platform account, but actions are still addressed to the local username so the agent can apply them.

`interval_seconds` from the report takes precedence over the controller's `sample_interval_seconds` when converting samples to cost.

### `GET /api/node/actions`

Requires `X-Agent-Token`. Returns pending actions for the calling node.

## User endpoints

### `GET /api/users/:username/balance`

Requires `admin_token`, an agent token, an administrator session, or the named user's own session. Returns `404` when the user does not exist — this endpoint does not create users.

```json
{
  "username": "alice",
  "balance": 80.0,
  "general_balance": 80.0,
  "carryover_balance": 20.0,
  "exclusive_balance": 0.0,
  "total_balance": 100.0,
  "status": "warning",
  "warning_threshold_points": 100,
  "limited_threshold_points": 3,
  "monthly_max_overdraft_limit": 0,
  "current_overdraft_points": 0,
  "overdraft_exceeded": false,
  "manual_blocked": false
}
```

### `GET /api/users/:username/usage`

Same authentication. `limit` defaults to 200, maximum 5000.

```json
{"records":[{"node_id":"node-01","username":"alice","timestamp":"2026-08-14T16:00:00Z","cpu_percent":120.5,"memory_mb":2048,"gpu_usage":"[]","cost":0.6}]}
```

### `POST /api/users/:username/recharge`

Requires `admin_token`, an administrator session, or a power-user session holding the `manage_platform_users` permission bit.

```json
{"amount": 100, "method": "admin"}
```

### Session-scoped user routes

All require a session cookie: `GET /api/user/me`, `/me/balance`, `/me/usage`, `/me/points-increments`, `/me/profile`, `/me/profile-change-requests`, `/accounts`, `/requests`.

## Registration and account binding

These three routes require a session cookie. The requesting account is taken from the session, never from the body — a `billing_username` field in the request is ignored and overwritten.

### `POST /api/user/requests/bind`

Declares existing node accounts for review. At most 200 items per request.

```json
{
  "items": [
    {"node_id": "node-01", "local_username": "alice"},
    {"node_id": "node-05", "local_username": "alice2"}
  ],
  "message": "optional note"
}
```

### `POST /api/user/requests/open`

Requests a new account on a node. An empty `node_id` or `local_username` becomes `待分配` ("to be assigned"), so a user who does not yet know which node they need can still file the request. `message` is validated as the justification.

```json
{"node_id":"node-01","local_username":"alice","message":"reason for the request"}
```

### `GET /api/user/requests`

Returns the caller's own requests. `limit` defaults to 200, maximum 5000.

### `POST /api/admin/requests/:id/approve` and `/reject`

Requires the review permission. Approving a `bind` request writes `user_node_accounts`, which drives both billing attribution and SSH login validation. `/reopen` and `/batch-review` are also available, and `POST /api/admin/registration-requests/:id/approve` and `/reject` handle new-account registrations.

## Node registry

These routes require `X-Agent-Token` and should be reached over the internal listener.

| Route | Purpose |
| --- | --- |
| `GET /api/registry/nodes/:node_id/users.txt` | Registered local usernames for the node, one per line |
| `GET /api/registry/nodes/:node_id/blocked.txt` | Denied local usernames |
| `GET /api/registry/nodes/:node_id/exempt.txt` | Exempt local usernames |
| `GET /api/registry/nodes/:node_id/guard-state` | Current SSH guard state |
| `GET /api/registry/resolve?node_id=&local_username=` | Resolve a local account to its platform account |

### `POST /api/registry/bind-claim`

Authenticated by a one-time bind challenge token in the body, not by an agent token, because the caller is a user process on a node.

```json
{"token":"<challenge-token>","node_id":"node-01","local_username":"alice"}
```

## Administrator endpoints

### Pricing

```http
POST /api/admin/prices
```

```json
{"gpu_model":"RTX 3090","price_per_minute":0.2}
```

CPU billing uses the reserved model name `CPU_CORE`, priced per core-minute (100% CPU ≈ 1 core). Applying `set_cpu_quota` requires systemd `CPUQuota` or a writable cgroup on the node, with the agent running as root.

### Usage audit

| Route | Purpose |
| --- | --- |
| `GET /api/admin/usage` | Usage and kill records. Filters: `billing_username`, `local_username`, `unregistered_only=1`, `limit` (default 200, max 5000) |
| `GET /api/admin/usage/export.csv` | CSV export. Adds `from`, `to`, `limit` (default 20000, max 200000) |
| `GET /api/admin/usage/days` | Dates that have records, with row counts and estimated CSV size |
| `GET /api/admin/usage/range-estimate` | Row count and size for a date range, before exporting or deleting |
| `POST /api/admin/usage/delete-range` | Irreversibly deletes `usage_records` in a range |
| `GET`/`POST /api/admin/usage/retention` | Read or set the retention policy |

Delete request and response:

```json
{"from":"2026-08-01","to":"2026-08-03","billing_username":"alice","unregistered_only":false,"confirm":true}
```

```json
{"ok":true,"records_before":1200,"deleted_records":1200}
```

### Nodes

`GET /api/admin/nodes` returns reporting state — last seen, GPU and CPU process counts, cost from the latest report. `limit` defaults to 200, maximum 2000.

Per-node routes cover detail, price, CPU limits, memory limits, GPU visibility, disk quota, SSH exclusivity, view access, points interception, and security events.

### HA and backup

| Route | Purpose |
| --- | --- |
| `GET /api/admin/ha/status` | Primary/standby reachability, digest comparison, consistency |
| `GET`/`POST /api/admin/ha/sync/config` | Synchronisation configuration |
| `GET /api/admin/ha/sync/runs` | Synchronisation history with per-step results |
| `POST /api/admin/ha/sync/now` | Trigger a sync; `direction` is `primary_to_standby` or `standby_to_primary` |
| `POST /api/admin/ha/failover/activate` | Standby only; primary returns `409`. Body must be `{"confirm":"ACTIVATE_STANDBY"}` |
| `GET /api/admin/backup/status` | Latest snapshot and restore-drill state |

Sync configuration:

```json
{
  "enabled": true,
  "interval_days": 1,
  "start_hour": 3,
  "dr_host": "192.0.2.20",
  "dr_ssh_user": "gpuops",
  "dr_ssh_port": 22,
  "dr_key_file": "/etc/gpu-ops/standby_ed25519",
  "dr_controller_port": 8080,
  "primary_host": "192.0.2.10",
  "primary_controller_port": 8080,
  "script_path": "/opt/gpu-ops/scripts/ha_sync_worker.sh",
  "sync_web_dist": true,
  "sync_database": true,
  "auto_failover": true
}
```

`GET /api/admin/backup/status` reports `ready=true` only when a successful backup exists within 36 hours and a successful isolated restore drill within 8 days.

## Error responses

| Status | Body | Meaning |
| --- | --- | --- |
| `400` | `{"error":"..."}` | Malformed request or invalid parameter |
| `401` | `{"error":"unauthorized"}` | Missing or invalid credential |
| `403` | `{"error":"forbidden"}` | Authenticated but lacking the required permission |
| `403` | `{"error":"csrf_required"}` | Session request without a matching `X-CSRF-Token` |
| `404` | `{"error":"..."}` | Resource does not exist |
| `409` | `{"error":"..."}` | Conflicting state, such as failover on a primary |
| `503` | `{"ok":false,"database":false}` | Readiness failure |
