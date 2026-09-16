# Changelog

**English** | [简体中文](CHANGELOG.zh-CN.md)

Notable changes are recorded here. Releases follow [Semantic Versioning](https://semver.org/); GitHub Releases and their Git tags are the authoritative release artifacts.

## [Unreleased]

### Added

- **Rack power dashboard** (`/admin/racks`) — racks, slot assignments, allocated power budgets, live GPU power draw, and flags for unreported or under-budgeted nodes. Racks start empty.
- **Low-balance email alerts** — a points warning threshold in Mail settings; one email per downward crossing, queued transactionally and retried.
- **GPU visibility "none"** — a node-user GPU policy can now hide every GPU instead of only restricting to a subset (`deny_all`).
- **Durable node account actions** — account creation and UID/GID alignment are persisted, leased, retried, and completed only by a node receipt (`POST /api/node/actions/:id/result`).
- **Node recommendation panel** in account provisioning, and a structured new-account request form (research direction, workload, usage intensity).
- **`scripts/finish_pending_release.sh`** — backup, primary install, commit and migration verification, and primary-to-standby sync in one pass. The HA sync worker now also synchronises and verifies migration files.

### Changed

- The operations dashboard is reorganised around user activity, process time, balances, and daily trends, scoped to the nodes the viewer may see.
- Cluster status, node management, and the interface demo are localised.
- Workspace distribution to compute nodes no longer copies `docs/`, and the previous node deployment report is archived instead of overwritten.
- Controller builds embed the commit, build time, and dirty-tree flag.

### Fixed

- Disk capacity alerts are latched per mount point, so repeated reports and restarts no longer duplicate critical events, and they no longer inflate suspected-user summaries.
- Date-only `to` filters include the whole day at PostgreSQL microsecond precision.
- Queued node actions use the same normalised payload as immediately delivered ones.
- An empty GPU allow-list was treated as "no restriction" at several layers; a corrupted agent policy state file now keeps existing restrictions instead of lifting them.
- Leftover deployment-specific branding was removed from the interface demo.
- An HA standby no longer re-sends balance alert emails copied from the primary's database.

### Security

- Node security event details and suspicious-account summaries are HTML-escaped before rendering. Process command lines captured by mining detection could previously inject markup into an administrator's session.
- A deny-all GPU visibility policy also carries a sentinel GPU index, so agents that predate `deny_all` hide every GPU instead of removing the restriction.
- GPU policy state files are written atomically with mode `0640`, so an interrupted write can no longer leave a state file that blocks every later policy update.
- Statistics routes reject date ranges longer than 1,830 days.
- Node-scoped dashboard totals no longer include node-exclusive points or grants from nodes the viewer cannot see.
- Operator scripts pass the admin token to `curl` on standard input instead of in process arguments.
- Node action delivery tokens are compared in constant time.

### Upgrade notes

- Migrations `0081`–`0086` run automatically at startup.
- Roll out node agents before the controller. An older agent does not acknowledge durable account actions, so those actions fail once they reach their retry limit.

## [3.2.0] — 2026-08-14

First public release. The version number continues the internal series this project was developed under; there are no earlier public releases.

### Included

- **Controller** — HTTP API, policy engine, usage accounting, ordered forward-only migrations applied at startup, scheduled jobs, and same-origin hosting of the Web UI.
- **Node agent** — metric collection, action execution, CPU and memory limits, SSH state reporting, and security signals.
- **Web UI** — Vue 3 and Element Plus, built to `web/dist` and served by the controller.
- **Two deployment paths** — a source build supervised by systemd, and a container image with a `docker compose` profile that runs PostgreSQL and the controller together.
- **First-run Setup** — a wizard for platform identity, registration domains, SSH entry, pricing, user guidelines, SMTP, and HA. It refuses to save while a required readiness check fails.
- **Bilingual documentation** — English is the source language and `docs/zh-CN/` is a complete mirror, enforced by `scripts/check_docs.sh` in CI.

### Defaults worth knowing

- The Web and API listener defaults to `8080`; the internal agent and HA listener defaults to `8081`.
- `dry_run` should stay `true` for the first days of a rollout. Usage is recorded and cost is calculated, but nothing is deducted.
- The balance, usage, and `/metrics` endpoints all require a credential. None of them is anonymous.
- Session cookies are `HttpOnly` and `SameSite=Lax`, and every non-GET session request must carry `X-CSRF-Token`.
- All credential comparisons are constant-time.

### Known limitations

- HA is operational automation, not a consensus protocol: no quorum, no automatic leader election, and no split-brain protection. Preventing two simultaneously active primaries is the operator's responsibility.
- The platform is not a multi-tenant isolation boundary. A user who can escalate to root on a compute node can defeat node-local enforcement.
- Migrations are forward-only. Rolling back means restoring a backup.

### Upgrading from an internal deployment

- Ports moved to `8080` and `8081` from `60039` and `60040`. Set `listen_addr` and `internal_listen_addr` explicitly to keep the old values, and update firewall rules, `CONTROLLER_URL` on every node, and reverse proxies before upgrading.
- Node-side helpers now send an operator credential, read from `GPUOPS_QUERY_TOKEN` or `/etc/gpu-ops/query-token`, because the balance endpoint is no longer anonymous.
- `docker-compose.yml` no longer carries a database password. Supply `POSTGRES_PASSWORD` in `.env`.

[3.2.0]: https://github.com/atoz03/Bursar/releases/tag/v3.2.0
