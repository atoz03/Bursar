# Security Model

**English** | [简体中文](zh-CN/security.md)

This document describes what Bursar protects, what it does not protect, and the hardening baseline an operator is expected to provide. For private vulnerability reporting, see [SECURITY.md](../SECURITY.md).

## What Bursar assumes

Bursar is a control plane for machines you already own. It assumes:

- the controller runs on a trusted host on a network you control;
- compute nodes are administered by the same team that administers the controller;
- PostgreSQL, TLS certificates, DNS, and backups are operated by you;
- users have shell access to compute nodes and are not treated as adversaries with root.

Bursar is not a multi-tenant isolation boundary. A user who can escalate to root on a compute node can defeat node-local enforcement.

## Trust boundaries

| Boundary | Holder | Compromise impact |
| --- | --- | --- |
| `admin_token` | Operators, deployment scripts | Full super-admin API access, bypassing role checks and 2FA |
| Administrator session | Browser cookie | Role-limited administration, subject to permission bits and CSRF |
| `agent_token` / per-node token | Root on each compute node | Report usage, read that node's registry lists, poll actions |
| `ha_token` | Standby controller | Read HA status and synchronization state |
| PostgreSQL credentials | Controller host | Equivalent to control-plane compromise: identities, points, SMTP settings |
| Root on a compute node | Node administrator | Full control of that node, including the agent and its token |

`admin_token` is the strongest credential in the system. It bypasses role permissions and two-factor authentication by design, because bootstrap and recovery must work when no administrator account exists. Treat it like a root password: store it outside the repository, restrict file permissions, and rotate it after any suspected exposure.

## Authentication

- **Browser sessions.** Login issues an HMAC-SHA256 signed, HttpOnly, `SameSite=Lax` cookie. The signature covers username, role, expiry, and a per-session nonce. Role and permissions are re-resolved from the database on every request, so privilege changes take effect immediately.
- **CSRF.** Every non-GET request made with a session cookie must carry `X-CSRF-Token` matching the session nonce, which the client reads from `GET /api/auth/me`.
- **Two-factor authentication.** TOTP is available per account and can be required by an administrator.
- **Token comparison.** All token checks — admin, agent, legacy, per-node, and HA — use constant-time comparison.
- **Password policy.** 12–64 characters with uppercase, lowercase, digit, and special character, enforced on registration, reset, and change.

## Network exposure

The controller has two listeners:

| Listener | Default | Contents |
| --- | --- | --- |
| Web | `8080` | Web UI, user and administrator APIs, login, registration |
| Internal | `8081` (optional) | Agent metrics, node actions, registry lists, HA status |

When `internal_listen_addr` is empty, the internal routes are mounted on the Web listener. This is convenient for evaluation but means a single exposed port carries both browser and agent traffic. Production deployments should set `internal_listen_addr`, supply a certificate and key, and restrict that listener to node and controller networks with firewall rules.

The Web listener does not terminate TLS. Put it behind a reverse proxy that does, and set `cookie_secure: true` once HTTPS is in place.

## Endpoint exposure summary

| Route | Credential |
| --- | --- |
| `GET /healthz`, `GET /readyz` | None |
| `GET /metrics` | `admin_token`, agent token, or administrator session |
| `GET /api/public/settings`, login, registration, password reset | None (rate-limited by registration policy) |
| `GET /api/users/:username/balance`, `.../usage` | `admin_token`, agent token, administrator session, or the user's own session |
| `POST /api/registry/bind-claim` | One-time bind challenge token issued by the controller |
| `/api/user/*` | Session cookie |
| `/api/admin/*` | `admin_token` or an administrator/power-user session with the required permission bit |
| `POST /api/metrics`, `/api/node/*`, `/api/registry/*` | `X-Agent-Token` |
| `GET /api/ha/status` | `X-HA-Token` |

## Fixed in this release

These issues were found while preparing the public release and are fixed in the code:

1. **Anonymous disclosure of any user's balance and usage.** `GET /api/users/:username/balance` and `GET /api/users/:username/usage` had no authentication. Any client that could reach the Web listener could enumerate usernames and read balances, account status, and per-node usage history. Both routes now require a credential, and non-administrator sessions may only read their own record.
2. **Anonymous write through a read endpoint.** The balance handler called an upsert helper that created a `users` row for any username it had not seen. An unauthenticated caller could therefore insert unlimited rows. The handler now performs a read-only lookup and returns `404` for unknown users.
3. **Unauthenticated metrics exposure.** `GET /metrics` was public. The payload is aggregate counters only — no usernames or hostnames — but it revealed fleet activity and enforcement volume. It now requires an operator credential.
4. **Non-constant-time token comparison.** The admin token and all agent tokens were compared with `==`, which short-circuits on the first differing byte. Only the HA token used constant-time comparison. All comparisons are now constant-time, and candidate sets are scanned without early exit.
5. **Missing `SameSite` attribute.** Session cookies relied solely on the CSRF token check. They are now issued with `SameSite=Lax` as well.
6. **Debug-mode HTTP server.** Gin ran in debug mode, printing the full route table and debug warnings at startup. Release mode is now the default; set `GIN_MODE=debug` explicitly when debugging.
7. **Hardcoded database password in the Compose file.** The development password was committed as a literal. It now comes from `.env`, and Compose fails fast when it is unset.

## Known residual risks

These are accepted behaviors, not defects. Decide whether each is acceptable for your deployment.

- **Sessions cannot be revoked before expiry.** The session cookie is self-contained. Changing a password or disabling an account does not invalidate cookies already issued; role and permission changes do take effect immediately because they are re-read from the database. Reduce `session_hours` if this matters.
- **`admin_token` bypasses roles and 2FA.** See above. There is no audit trail distinguishing which operator used it.
- **`cookie_secure` defaults to `false`.** This is required for plain-HTTP local evaluation. Set it to `true` for every deployment behind HTTPS.
- **`POST /api/registry/bind-claim` is reachable without an agent token.** It is authenticated by a one-time challenge token instead, because the claiming client is a user process on a node. It is rate-limited by challenge validity, not by IP.
- **Single-port mode exposes internal routes on the public listener.** They still require an agent token, but the attack surface is larger. Use `internal_listen_addr` in production.
- **Node agents run as root.** Enforcement requires cgroup, systemd, and process control. A compromised controller can direct a node agent to terminate processes and change limits.
- **SMTP credentials are stored in PostgreSQL** as runtime settings. Protect database backups accordingly.
- **No rate limiting on login.** Registration has abuse controls (domain allow-lists, disposable-domain blocking, optional Turnstile); password login does not. Put a rate limiter in the reverse proxy.

## Operator hardening baseline

1. Generate `agent_token`, `admin_token`, and `auth_secret` as three different values with `openssl rand -hex 32`. First-run Setup refuses to save while any of them is still a placeholder or shorter than 16 characters.
2. Terminate TLS at a reverse proxy and set `cookie_secure: true`.
3. Set `internal_listen_addr` with a certificate and key; firewall it to node networks.
4. Issue a distinct token per node with `agent_node_tokens`, then set `agent_node_token_enforce: true` once every node reports successfully.
5. Restrict the PostgreSQL network path and require TLS in the DSN.
6. Keep `dry_run: true` until accounting has been validated against real workloads.
7. Configure encrypted, independently stored backups and rehearse a restore. See [Backup and HA](backup-and-ha.md).
8. Enable two-factor authentication for every administrator account.
9. Add a login rate limiter at the proxy.
10. Complete the [go-live checklist](go-live-checklist.md) before enforcement is enabled.

## Reporting

Report suspected vulnerabilities privately as described in [SECURITY.md](../SECURITY.md). Never attach tokens, keys, host inventories, database dumps, or user data to public issues.
