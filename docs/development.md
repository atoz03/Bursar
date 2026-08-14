# Development Guide

**English** | [简体中文](zh-CN/development.md)

Setup, layout, and validation for contributors. For workflow and review expectations, see [CONTRIBUTING.md](../CONTRIBUTING.md).

## Prerequisites

| Tool | Version |
| --- | --- |
| Go | 1.26.6 |
| Node.js | 20.20 or newer |
| pnpm | 10.28.2 |
| Docker with Compose | any recent release |
| PostgreSQL | supplied by Compose for development |

```bash
go version
node --version
pnpm --version
docker compose version
```

## Repository layout

| Path | Contents |
| --- | --- |
| `controller/` | Go module: HTTP API, policy engine, accounting, migrations runner, schedulers, Web hosting |
| `node-agent/` | Go module: metrics collection, action execution, limits, SSH state, security signals |
| `web/` | Vue 3 + Element Plus single-page application |
| `database/` | Schema and ordered migrations |
| `scripts/` | Build, install, deploy, backup, HA, and fleet utilities |
| `config/` | Non-secret controller configuration example |
| `systemd/` | Unit files for the controller and the agent |
| `tools/` | Node-side helper scripts |
| `docs/` | English source documentation; `docs/zh-CN/` holds the translations |

`go.work` ties the two Go modules together, so `go test ./controller/... ./node-agent/...` works from the repository root.

Two files dominate the controller: `controller/database.go` (all persistence) and `controller/api.go` (routes, middleware, and most handlers). New handlers generally belong in a topic file — `points_handlers.go`, `registry_handlers.go`, `auth_handlers.go` — rather than growing `api.go` further.

## Setup

```bash
git clone https://github.com/atoz03/gpu-ops.git
cd gpu-ops
cp .env.example .env          # fill in POSTGRES_PASSWORD
docker compose up -d postgres

cp config/controller.yaml config/controller.local.yaml
# set agent_token, admin_token, auth_secret to three different random values
openssl rand -hex 32

corepack enable
pnpm --dir web install --frozen-lockfile
pnpm --dir web build

go run ./controller --config config/controller.local.yaml
```

`config/*.local.yaml` and `.env` are git-ignored.

### Front-end development

```bash
pnpm --dir web dev
```

Vite serves on `5173` and proxies `/api`, `/metrics`, and `/healthz` to the controller on `127.0.0.1:8080`. Keep the controller running in another terminal.

## Validation

Run before every pull request:

```bash
go vet ./controller/... ./node-agent/...
go test ./controller/... ./node-agent/...
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
bash -n scripts/*.sh
bash scripts/check_docs.sh
```

CI runs the same checks plus a container image build.

## Conventions

**Language.** English is the documentation source language; every document in `docs/` has a counterpart in `docs/zh-CN/`. Code comments and log messages in this codebase are Chinese — match the surrounding file rather than mixing languages within one file.

**Configuration.** Add new options to the `Config` struct, `config/controller.yaml`, `Validate()`, and [configuration.md](configuration.md) together. Add a `GPUOPS_*` override in `applyEnvOverrides` only for values a container deployment must set without editing YAML — typically secrets, listeners, and paths.

**Migrations.** Add a new numbered file in `database/migrations/`. Never edit a migration that may already have been applied; the controller applies them in order at startup and refuses to start if one fails.

**Authentication.** Reuse the existing middleware rather than checking credentials inside a handler: `authSession`, `authAdmin`, `authAgent`, `authNodeAgent`, `authHA`, `authOperator`, `authSelfOrOperator`, and the `require*Permission` family. Compare tokens with `constantTimeTokenEqual`, never `==`.

**Routes.** Adding a route means updating [api-reference.md](api-reference.md) in both languages. Decide deliberately whether it belongs on the Web router, the internal router, or both.

## Testing

Tests are standard `go test`. Existing coverage focuses on billing, TOTP, Turnstile, registration security, memory limits, HA safety, setup, node monitoring, and configuration overrides. There is no database fixture harness, so tests exercise pure logic; behavior that requires PostgreSQL is verified manually — describe that verification in the pull request.

```bash
go test ./controller/... -run TestBilling -v
go test ./node-agent/... -v
```

## Building release binaries

```bash
bash scripts/build_linux.sh
```

Version strings are compiled into each binary (`controller/version.go`, `node-agent/version.go`). Bump them when cutting a release, and update `.version` and both changelogs.

## Building the container image

```bash
docker compose --profile full build controller
docker compose --profile full up -d
curl -fsS http://127.0.0.1:8080/readyz
```

The image is a three-stage build: pnpm builds the UI, Go builds a static binary, and the runtime stage is distroless with the binary, `web/dist`, and `database/migrations`. It has no shell, so container health checks call `/app/controller --healthcheck`.
