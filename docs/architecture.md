# Architecture

**English** | [简体中文](zh-CN/architecture.md)

GPU Ops separates centralized policy and persistence from node-local observation and enforcement. It is intentionally compatible with direct SSH workflows.

## Components

### Controller

The Go controller:

- serves the Web UI and HTTP APIs;
- authenticates administrators, delegated operators, users, agents, and HA peers;
- applies PostgreSQL migrations at startup;
- stores identities, account mappings, points, prices, usage, policies, audit records, and runtime settings;
- calculates usage cost and queues node actions;
- runs monthly points, retention, reminder, and HA schedulers;
- reads backup status files produced by system-level backup jobs.

### Node Agent

The Go agent runs on each Linux compute node. It collects hardware and process state, reports usage, polls actions, and applies supported CPU, memory, disk, GPU, process, and SSH policies. Metrics collection can run with reduced privileges, but the packaged installation uses root because enforcement requires OS-level access.

### Web UI

The Vue application is compiled into `web/dist`. The controller serves the static files and the API from the same origin, which keeps cookie and CSRF handling simple. During development, Vite proxies API traffic to `127.0.0.1:8080`.

### PostgreSQL

PostgreSQL is the authoritative store for control-plane state. Migrations in `database/migrations/` are ordered and applied automatically. Database availability is part of `/readyz`.

## Network topology

```text
Browser ──HTTPS──> reverse proxy ──HTTP──> controller web listener (:8080)
                                                  │
                                                  └──> PostgreSQL (:5432)

Node Agent ──HTTPS + X-Agent-Token──> controller internal listener (:8081)
HA peer   ──HTTPS + X-HA-Token──────> controller internal listener (:8081)
```

`internal_listen_addr` is optional. When it is empty, agent, registry, and HA endpoints are mounted on the Web listener. When it is set, those endpoints move to a separate listener that requires a certificate and private key. Production deployments should use the separate internal listener and restrict it to node and controller networks.

The public Web listener does not provide TLS itself. Put it behind a trusted HTTPS reverse proxy and bind it to loopback or a private interface when possible.

## Main data flows

### Agent reporting and action delivery

1. The agent samples node, user, process, GPU, disk, SSH, and service state.
2. It sends a report with a globally unique `report_id` to `POST /api/metrics`.
3. The controller deduplicates the report, resolves local-to-platform identity, records usage, and calculates cost.
4. The response and the action-poll endpoint deliver pending policy actions.
5. The agent applies actions locally and reports subsequent state.

### Registration and account mapping

1. A user registers and verifies an email address.
2. An administrator reviews the registration.
3. The user declares existing node accounts or requests new ones.
4. The controller stores `(node_id, local_username) → platform user` mappings.
5. Node registry endpoints provide SSH guard state and account lists.

### Points and enforcement

Usage cost is charged against node-specific, carryover, and general points according to policy. Balance state can lead to warnings, CPU/memory restrictions, GPU restrictions, or process termination. Operators should validate accounting in `dry_run` mode before enabling enforcement.

## Authentication domains

| Actor | Mechanism | Scope |
| --- | --- | --- |
| Browser user/admin | Signed HttpOnly session cookie + CSRF header | Web and role-permitted API routes |
| Bootstrap/operator script | `Authorization: Bearer <admin_token>` | Super-admin API access |
| Node Agent | `X-Agent-Token` | Internal metrics, actions, and registry routes |
| HA peer | `X-HA-Token` | Internal HA status |
| Anonymous client | None | Health, readiness, public settings, registration and login |

Per-node tokens can be introduced in audit mode and then enforced after every node has migrated. Legacy tokens provide a bounded rotation window.

## Trust boundaries

- A controller administrator can change policies and trigger destructive node actions.
- A root node agent can inspect processes and alter OS controls on its node.
- PostgreSQL contains operational and identity data; database access is equivalent to control-plane access.
- SMTP credentials are runtime settings in PostgreSQL; protect database backups accordingly.
- The per-username balance and usage endpoints require a credential: an operator token, an administrator session, or the named user's own session. Node-side helper scripts authenticate with an agent token read from a node-local file, which is readable by node users — treat it as a node-scope credential, not a per-user one.
- The backup repository and HA standby are separate privileged systems and must not share all credentials or the same failure domain with the primary.

## Repository boundaries

GPU Ops does not include a production database, TLS PKI, reverse proxy, identity provider, email provider, NFS server, container registry, or monitoring stack. The repository supplies application components and reference scripts; operators own infrastructure integration and policy decisions.

## Design constraints

- Linux and systemd/cgroup behavior varies; enforcement capabilities must be tested on every node image.
- `node_id` is a stable operator-defined identifier, not an IP address or SSH port requirement.
- Migrations are forward-applied; database rollback requires a tested restore.
- HA synchronization is operational automation, not consensus. Prevent simultaneous active primaries at the network and process level.
