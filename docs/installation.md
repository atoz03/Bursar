# Installation

**English** | [简体中文](zh-CN/installation.md)

Bursar supports two ways to run the controller:

| Path | Use it when |
| --- | --- |
| [**Source build**](#source-deployment) | You want systemd supervision, host-native operation, and the same lifecycle as the node agents. This is the path the scripts and HA tooling assume. |
| [**Docker**](#docker-deployment) | You want the fastest reproducible start and already run containers. Fewer moving parts, but the HA and backup scripts expect a host-installed controller. |

The node agent is never containerised. It needs cgroup, systemd, SSH, and driver access on the host. See [Node Agent](node-agent.md).

---

# Docker deployment

## 1. Prepare secrets

```bash
git clone https://github.com/atoz03/gpu-ops.git
cd gpu-ops
cp .env.example .env
```

Fill in `.env`. Every secret must be a distinct random value:

```bash
openssl rand -hex 24   # POSTGRES_PASSWORD
openssl rand -hex 32   # GPUOPS_AGENT_TOKEN
openssl rand -hex 32   # GPUOPS_ADMIN_TOKEN
openssl rand -hex 32   # GPUOPS_AUTH_SECRET
```

Compose refuses to start if any of them is empty. `.env` is git-ignored.

## 2. Start the stack

```bash
docker compose --profile full up -d --build
docker compose --profile full ps
```

The controller image is a three-stage build: pnpm builds the Web UI, Go builds a static binary, and the runtime stage is distroless. Migrations run automatically at startup.

Wait for the controller to report `healthy`, then:

```bash
curl -fsS http://127.0.0.1:8080/readyz
```

## 3. Bootstrap and complete Setup

```bash
curl -fsS -X POST http://127.0.0.1:8080/api/admin/bootstrap \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $(grep '^GPUOPS_ADMIN_TOKEN=' .env | cut -d= -f2)" \
  -d '{"username":"admin","password":"<strong-password>"}'
```

Open <http://127.0.0.1:8080/login> and follow [First-run Setup](first-run-setup.md).

## 4. Configuration in containers

The image bundles `config/controller.yaml` for non-secret defaults. Everything a deployment must change is supplied through the environment:

| Variable | Overrides |
| --- | --- |
| `GPUOPS_DATABASE_DSN` | `database_dsn` |
| `GPUOPS_AGENT_TOKEN` | `agent_token` |
| `GPUOPS_ADMIN_TOKEN` | `admin_token` |
| `GPUOPS_AUTH_SECRET` | `auth_secret` |
| `GPUOPS_HA_TOKEN` | `ha_token` |
| `GPUOPS_LISTEN_ADDR` | `listen_addr` |
| `GPUOPS_INTERNAL_LISTEN_ADDR` | `internal_listen_addr` |
| `GPUOPS_INTERNAL_TLS_CERT_FILE` | `internal_tls_cert_file` |
| `GPUOPS_INTERNAL_TLS_KEY_FILE` | `internal_tls_key_file` |
| `GPUOPS_MIGRATION_DIR` | `migration_dir` |
| `GPUOPS_WEB_DIR` | `web_dir` |
| `GPUOPS_COOKIE_SECURE` | `cookie_secure` |
| `GPUOPS_DRY_RUN` | `dry_run` |

An unset or empty variable leaves the YAML value alone. To change anything else — thresholds, prices, CPU control, SMTP — mount your own file over `/app/config/controller.yaml`:

```yaml
    volumes:
      - ./config/controller.local.yaml:/app/config/controller.yaml:ro
```

## 5. Production notes for the container path

- Put an HTTPS reverse proxy in front and set `GPUOPS_COOKIE_SECURE=true`.
- Keep `POSTGRES_PUBLISH` on `127.0.0.1` so the database is not reachable off-host.
- To enable the internal listener, mount certificates and set `GPUOPS_INTERNAL_LISTEN_ADDR`, then publish `8081` to node networks only.
- The container database volume `pgdata` is not a backup. Set up [encrypted backups](backup-and-ha.md).
- Health checks call `/app/controller --healthcheck`; the image has no shell.

---

# Source deployment

This is a production-oriented deployment on systemd-based Linux. Adapt paths, users, network controls, and certificates to your environment.

## Deployment checklist

Before installing, decide:

- controller DNS name and HTTPS reverse proxy;
- PostgreSQL location, credentials, TLS, retention, and backup ownership;
- separate internal DNS/IP and TLS certificate for agents, if used;
- a non-login service account for the controller;
- stable node IDs and per-node agent tokens;
- shared workspace ownership, if NFS integration is enabled;
- independent encrypted backup repository;
- whether HA is required now or can remain disabled.

## Supported baseline

- Controller: systemd Linux, Go 1.26.6, Node.js 20.20+, pnpm 10.28.2, PostgreSQL 18
- Compute nodes: systemd Linux; `nvidia-smi` for NVIDIA GPU discovery; cgroup v2 preferred
- Build host: Git, Go, Node.js, pnpm

The scripts target Ubuntu 22.04-style hosts. Review them before using another distribution.

## 1. Install dependencies

On a dedicated build/controller host:

```bash
sudo bash scripts/install_deps_ubuntu2204.sh
```

The script can install Go, Node.js, pnpm, and Docker. To skip components already managed by your platform:

```bash
sudo env INSTALL_DOCKER=0 INSTALL_GO=0 bash scripts/install_deps_ubuntu2204.sh
```

Global dependency changes are operationally significant; inspect the script and pin packages through your normal configuration-management process for production.

## 2. Provision PostgreSQL

Use a managed or separately administered PostgreSQL instance when possible. Create a database and role dedicated to Bursar, enforce network access controls, and produce a DSN such as:

```text
postgres://gpuops:<password>@db.example.org:5432/gpuops?sslmode=require
```

The repository Compose file is a local-development convenience. Its credentials come from `.env` and are not production secrets.

## 3. Create controller configuration

```bash
cp config/controller.yaml config/controller.local.yaml
chmod 600 config/controller.local.yaml
```

At minimum replace:

- `database_dsn`
- `agent_token`
- `admin_token`
- `auth_secret`
- all HA placeholders, or leave HA disabled

Generate independent secrets with `openssl rand -hex 32`. Configure `listen_addr`, TLS topology, and `cookie_secure` according to [Configuration](configuration.md).

## 4. Build the Web UI

```bash
corepack enable
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
```

## 5. Install the controller service

Create a dedicated system user using your operating-system policy, then run the local installer from the repository:

```bash
sudo env \
  CONFIG_PATH="$PWD/config/controller.local.yaml" \
  RUN_USER=gpuops \
  RUN_GROUP=gpuops \
  BUILD_WEB=0 \
  bash scripts/install_controller_local.sh
```

The script builds `/usr/local/bin/gpu-controller`, writes a systemd unit, optionally installs shared-workspace sudo rules, and starts the service. Review `ENABLE_SHARED_WORKSPACE_SUDOERS` and `ENABLE_HOST_SECURITY` before execution.

Verify:

```bash
systemctl status gpu-controller
journalctl -u gpu-controller -n 100 --no-pager
curl -fsS http://127.0.0.1:8080/readyz
/usr/local/bin/gpu-controller --version
```

## 6. Publish the Web listener

Terminate public TLS at a reverse proxy and forward to the controller Web listener. Preserve `Host`, `X-Forwarded-For`, and `X-Forwarded-Proto`. A minimal Nginx location is:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

Configure certificates and modern TLS policy in the proxy. Set `cookie_secure: true` after HTTPS is active. Do not expose PostgreSQL or the controller's internal listener to the public Internet.

## 7. Configure the internal listener

Recommended production configuration:

```yaml
internal_listen_addr: "0.0.0.0:8081"
internal_tls_cert_file: "/etc/gpu-ops/tls/internal.crt"
internal_tls_key_file: "/etc/gpu-ops/tls/internal.key"
```

The certificate must be trusted by compute nodes. Restrict port `8081` to enrolled nodes and HA peers. Restart the controller after changing startup YAML.

## 8. Bootstrap and complete Setup

Create the first administrator with the bootstrap API, sign in through the HTTPS hostname, and follow [First-run Setup](first-run-setup.md). Bootstrap is deliberately one-time.

## 9. Enroll nodes

For each Linux compute node:

```bash
bash scripts/node_prereq_check.sh
sudo env \
  NODE_ID=node-01 \
  CONTROLLER_URL=https://controller-internal.example.org:8081 \
  AGENT_TOKEN='<token-for-node-01>' \
  bash scripts/install_agent_local.sh
```

Use the staged token-enforcement process in [Node Agent](node-agent.md). Test SSH guard, CPU, memory, disk, and GPU controls on a non-critical node first.

## 10. Backups and rollout

Install encrypted backups, perform an isolated restore drill, then follow the [go-live checklist](go-live-checklist.md). Start with `dry_run: true` for at least one representative accounting window.

## Upgrade model

The project does not publish container images; you build from a tagged source tree either way. Upgrade by:

1. backing up PostgreSQL and configuration;
2. reviewing `CHANGELOG.md` and migrations;
3. building and testing the target commit;
4. replacing the controller binary and Web assets, or rebuilding the image, and restarting;
5. upgrading agents in controlled batches;
6. verifying versions, readiness, node heartbeats, and accounting.

Detailed commands are in [Operations](operations.md).
