# Getting Started

**English** | [简体中文](zh-CN/getting-started.md)

This guide creates a local evaluation environment with PostgreSQL in Docker and the controller running from source. If you would rather run the controller in a container too, use the [Docker deployment](installation.md#docker-deployment) instead — it is a single `docker compose` command once secrets are in place.

## Prerequisites

- Go 1.26.6
- Node.js 20.20 or newer
- pnpm 10.28.2
- Docker with Compose
- OpenSSL, curl, and Git

Verify the toolchain:

```bash
go version
node --version
pnpm --version
docker compose version
```

## 1. Prepare the repository

```bash
git clone https://github.com/atoz03/gpu-ops.git
cd gpu-ops
cp .env.example .env
cp config/controller.yaml config/controller.local.yaml
```

Set `POSTGRES_PASSWORD` in `.env`; Compose will not start without it.

Generate three different secrets:

```bash
openssl rand -hex 32
openssl rand -hex 32
openssl rand -hex 32
```

Set them as `agent_token`, `admin_token`, and `auth_secret`. Do not reuse one value for multiple purposes.

For local evaluation, the example DSN and Compose credentials already match. Keep `internal_listen_addr` empty, `cookie_secure: false`, `ha_enabled: false`, and set `dry_run: true` until usage records look correct.

## 2. Start PostgreSQL

```bash
docker compose up -d postgres
docker compose ps
```

Wait until the container reports `healthy`. To inspect startup failures:

```bash
docker compose logs postgres
```

## 3. Build the Web UI

```bash
corepack enable
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
```

The controller auto-detects `web/dist` when launched from the repository.

## 4. Run the controller

```bash
go run ./controller --config config/controller.local.yaml
```

Check liveness and database readiness from another terminal:

```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS -H 'Authorization: Bearer <admin_token>' http://127.0.0.1:8080/metrics | head
```

`/metrics` requires an operator credential; `/healthz` and `/readyz` do not.

Expected readiness response:

```json
{"ok":true,"database":true}
```

## 5. Bootstrap an administrator

The bootstrap endpoint works only while the administrator table is empty:

```bash
curl -fsS -X POST http://127.0.0.1:8080/api/admin/bootstrap \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <admin_token>' \
  -d '{"username":"admin","password":"<strong-password>"}'
```

Use a 12–64 character password containing uppercase, lowercase, number, and special characters.

## 6. Complete Setup

Open <http://127.0.0.1:8080/login>. After the first login, administrators are redirected to `/admin/setup`. Configure:

1. platform name, accepted registration domains, and optional shared SSH host;
2. resource prices and user guidelines;
3. optional SMTP;
4. optional HA and required startup checks.

Public registration remains closed until Setup completes. See [First-run Setup](first-run-setup.md) for field behavior.

## 7. Optional development Agent

On a Linux host, build and run an agent manually:

```bash
go build -o /tmp/gpu-node-agent ./node-agent
NODE_ID=node-01 \
CONTROLLER_URL=http://127.0.0.1:8080 \
AGENT_TOKEN='<agent_token>' \
/tmp/gpu-node-agent
```

This is suitable only when the agent and controller share the local trusted environment. Production nodes should use [the installation guide](installation.md) and the [Node Agent guide](node-agent.md).

## 8. Stop the evaluation stack

Stop the controller with `Ctrl+C`, then stop PostgreSQL without deleting its volume:

```bash
docker compose down
```

`docker compose down -v` permanently deletes the development database volume; use it only when you explicitly want a clean database.

## Next steps

- [Configuration reference](configuration.md)
- [Production installation](installation.md)
- [Security model](security.md)
- [Troubleshooting](troubleshooting.md)
