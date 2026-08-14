# Changelog

**English** | [简体中文](CHANGELOG.zh-CN.md)

Notable changes are recorded here. Releases follow [Semantic Versioning](https://semver.org/); GitHub Releases and their Git tags are the authoritative release artifacts.

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

[3.2.0]: https://github.com/atoz03/gpu-ops/releases/tag/v3.2.0
