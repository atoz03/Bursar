# Configuration Reference

**English** | [简体中文](zh-CN/configuration.md)

The controller reads YAML at startup. Select the file with `--config <path>` or `CONTROLLER_CONFIG`. Startup configuration requires a service restart; runtime settings saved through Setup are stored in PostgreSQL.

Use `config/controller.yaml` as the safe example and keep real values in `config/controller.local.yaml` or your secret-management system.

## Environment overrides

A small set of keys can be supplied through the environment so that a container deployment does not need a secret-bearing YAML file. Overrides are applied after the YAML is parsed and before validation.

| Variable | Overrides |
| --- | --- |
| `GPUOPS_LISTEN_ADDR` | `listen_addr` |
| `GPUOPS_INTERNAL_LISTEN_ADDR` | `internal_listen_addr` |
| `GPUOPS_INTERNAL_TLS_CERT_FILE` | `internal_tls_cert_file` |
| `GPUOPS_INTERNAL_TLS_KEY_FILE` | `internal_tls_key_file` |
| `GPUOPS_DATABASE_DSN` | `database_dsn` |
| `GPUOPS_AGENT_TOKEN` | `agent_token` |
| `GPUOPS_ADMIN_TOKEN` | `admin_token` |
| `GPUOPS_AUTH_SECRET` | `auth_secret` |
| `GPUOPS_HA_TOKEN` | `ha_token` |
| `GPUOPS_MIGRATION_DIR` | `migration_dir` |
| `GPUOPS_WEB_DIR` | `web_dir` |
| `GPUOPS_COOKIE_SECURE` | `cookie_secure` |
| `GPUOPS_DRY_RUN` | `dry_run` |

An unset or empty variable is ignored, so an empty environment value never clears a YAML value. The two boolean variables accept `true`/`false`, `1`/`0`, `yes`/`no`, or `on`/`off`; any other value fails startup. Everything not in this table must come from YAML.

Two other environment variables affect the process: `CONTROLLER_CONFIG` selects the configuration file, and `GIN_MODE` overrides the HTTP framework mode, which defaults to `release`.

## Required startup settings

| Key | Purpose | Production guidance |
| --- | --- | --- |
| `listen_addr` | Web/API listen address | Bind behind an HTTPS proxy, preferably to loopback/private IP |
| `database_dsn` | PostgreSQL connection | Dedicated role, TLS, least network exposure |
| `agent_token` | Global Agent authentication | Independent random secret; migrate to per-node tokens |
| `admin_token` | Bootstrap and administrative bearer access | Independent random secret; do not use in browsers |
| `auth_secret` | Session-cookie signing | At least 16 characters; use 32 random bytes |

Setup treats obvious placeholders and short secrets as not ready.

## Listener and TLS

| Key | Default/example | Notes |
| --- | --- | --- |
| `listen_addr` | `0.0.0.0:8080` | HTTP Web listener; TLS is normally terminated by a reverse proxy |
| `internal_listen_addr` | empty | Separate Agent/registry/HA listener when set |
| `internal_tls_cert_file` | empty | Required with an internal listener |
| `internal_tls_key_file` | empty | Required with an internal listener; restrict file permissions |

When the internal listener is disabled, internal APIs remain on the Web listener for compatibility. When enabled, they are removed from the Web listener.

## Agent authentication and rotation

| Key | Meaning |
| --- | --- |
| `agent_token` | Global token and migration fallback |
| `agent_legacy_tokens` | Temporarily accepted old global tokens |
| `agent_node_tokens` | Map of `node_id: token` |
| `agent_node_token_enforce` | Reject a node unless its dedicated token matches |

Safe per-node migration:

1. Populate `agent_node_tokens` while enforcement is `false`.
2. Distribute node-specific tokens and verify every heartbeat.
3. Set `agent_node_token_enforce: true` and restart.
4. Remove obsolete global/legacy tokens after the rollback window.

## Sessions and registration security

| Key | Meaning |
| --- | --- |
| `session_hours` | Browser session lifetime, `0` disables browser sessions, maximum `720` |
| `cookie_secure` | Send session cookies only over HTTPS; enable in production |
| `turnstile_site_key` | Cloudflare Turnstile public key |
| `turnstile_secret_key` | Cloudflare Turnstile secret |
| `turnstile_expected_hostnames` | Exact browser hostnames accepted from verification |

Turnstile keys must be set together, and hostnames contain no scheme, port, or path. With Turnstile disabled, the application uses its local challenge flow.

## Accounting and enforcement

| Key | Meaning |
| --- | --- |
| `warning_threshold` | Balance below which warning state begins; must be positive |
| `limited_threshold` | Balance below which limited state begins; lower than warning threshold |
| `cpu_price_per_core_minute` | Fallback CPU core-minute price; database `CPU_CORE` wins |
| `sample_interval_seconds` | Fallback report interval, 1–600 seconds |
| `enable_cpu_control` | Permit CPU limit actions |
| `cpu_limit_percent_limited` | CPU quota in limited state, 1–100 |
| `cpu_limit_percent_blocked` | CPU quota in blocked state, 1–100 |
| `overdraft_memory_limit_gb` | Memory limit after overdraft; `0` disables |
| `kill_grace_period_seconds` | Delay before kill actions after blocked state |
| `dry_run` | Record usage without charging points |
| `default_balance` | Initial balance for a newly observed user |
| `default_price_per_minute` | Fallback GPU-minute price |

Prices configured in PostgreSQL override YAML fallbacks. Validate model matching and cost in dry-run mode.

## SMTP defaults

`smtp_host`, `smtp_port`, `smtp_user`, `smtp_pass`, `from_email`, and `from_name` seed mail behavior. The administrator can change mail settings in Setup or the mail page. SMTP passwords stored as runtime settings are protected by database access controls and backups, not by application-level encryption.

Leave all SMTP identity fields empty to disable mail. Password reset and email verification then cannot complete through email.

## Files and directories

| Key | Meaning |
| --- | --- |
| `migration_dir` | Override migration directory; empty enables repository auto-detection |
| `web_dir` | Override compiled Web directory; empty enables `../web/dist` auto-detection |
| `backup_status_file` | JSON status written by the backup job |
| `backup_verify_status_file` | JSON status written by restore verification |
| `shared_node_root` | Server-side per-node shared-workspace root |
| `shared_cluster_root` | Server-side cluster-wide shared-workspace root |

The controller service account needs only the filesystem permissions required by enabled features. Review the narrowly generated sudoers rules when shared workspace creation is enabled.

## HA bootstrap settings

| Key | Meaning |
| --- | --- |
| `ha_enabled` | Enables HA-related controller behavior |
| `ha_node` | Stable controller identity |
| `ha_role` | `primary` or `standby` |
| `ha_peer_url` | Peer controller base URL |
| `ha_token` | Shared HA authentication secret |

Detailed sync scheduling and peer SSH settings are runtime configuration stored from Setup. See [Backup and HA](backup-and-ha.md).

## Runtime platform settings

Setup stores these values in PostgreSQL:

- platform display name;
- allowed registration email domains;
- shared SSH entry hostname;
- setup completion state;
- user guidelines;
- resource prices;
- SMTP settings;
- HA synchronization settings.

An empty registration-domain list accepts any syntactically valid email not blocked by the disposable-domain policy. Runtime settings are shared by controllers using the same database.

## Secret handling

- Never commit real configuration. `.gitignore` excludes `config/*.local.yaml` and local environment files.
- Do not pass secrets on a multi-user shell command line when a protected file or secret manager is available.
- Use different values for Agent, admin, session, HA, backup, database, and SMTP credentials.
- Restrict configuration and key files to the service account.
- Rotate credentials after suspected exposure and after transferring repository or infrastructure ownership.
- Back up the secrets required to restore encrypted data, separately from the encrypted backup itself.

## Validation

The controller validates configuration before opening a listener:

```bash
go run ./controller --config config/controller.local.yaml
```

For an installed service, inspect failures with:

```bash
journalctl -u gpu-controller -n 100 --no-pager
```
