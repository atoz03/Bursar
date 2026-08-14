# Changelog

**English** | [简体中文](CHANGELOG.zh-CN.md)

Notable project changes are recorded here. Releases follow [Semantic Versioning](https://semver.org/); GitHub Releases and their Git tags are the authoritative release artifacts.

## [Unreleased]

### Added

- A first-run administrator Setup wizard for platform identity, registration domains, SSH entry, pricing, user guidelines, SMTP, and HA.
- Complete bilingual open-source documentation, contribution workflow, security policy, and Apache-2.0 licensing.
- A container deployment path: multi-stage `Dockerfile` and a `docker compose` profile that runs PostgreSQL and the controller together.
- `GPUOPS_*` environment overrides for listeners, paths, and secrets, so a container can be configured without editing YAML.
- `controller --healthcheck`, used by the container health check because the distroless runtime image has no shell.
- `scripts/check_docs.sh`, which verifies relative links, English/Chinese parity, and language-switcher headers; CI runs it on every push.

### Changed

- Public examples now use reusable hostnames, paths, identities, and test data.
- **Breaking:** default ports changed to `8080` (Web/API) and `8081` (internal agent/HA listener), replacing `60039`/`60040`. Existing deployments keep working by setting `listen_addr` and `internal_listen_addr` explicitly; update firewall rules, `CONTROLLER_URL` on every node, and reverse proxies before upgrading.
- The controller now runs Gin in release mode unless `GIN_MODE` is set explicitly.
- The Vite dev proxy targets `127.0.0.1:8080`. It previously pointed at port 8000, where no controller has ever listened.
- `tools/balance-query` and `tools/check_quota.sh` send an operator credential, read from `GPUOPS_QUERY_TOKEN` or `/etc/gpu-ops/query-token`, because the balance endpoint is no longer anonymous.

### Fixed

- `docker-compose.yml` no longer ships a hardcoded database password; `POSTGRES_PASSWORD` must be supplied and PostgreSQL publishes to `127.0.0.1` by default.

### Security

- Expanded ignore rules for local configuration, credentials, node inventories, backups, and exports.
- Public registration remains closed until Setup is complete and startup secrets pass readiness checks.
- `GET /api/users/:username/balance` and `GET /api/users/:username/usage` now require authentication. They were reachable anonymously, which disclosed any user's balance, status, and usage history to unauthenticated callers.
- `GET /api/users/:username/balance` no longer creates a user row as a side effect of a read, closing an anonymous write path into the `users` table.
- `GET /metrics` now requires an operator credential (`admin_token`, agent token, or an administrator session).
- All credential comparisons — admin, agent, legacy, per-node, administrator bootstrap, and the CSRF nonce — use constant-time comparison; previously only the HA token and TOTP codes did.
- Session cookies are issued with `SameSite=Lax` in addition to the existing CSRF token check.

[Unreleased]: https://github.com/atoz03/gpu-ops/commits/main
