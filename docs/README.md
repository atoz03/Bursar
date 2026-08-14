# GPU Ops Documentation

**English** | [简体中文](zh-CN/README.md)

This directory is the canonical documentation set for GPU Ops. `README.md` files and English documents are the source language; complete Simplified Chinese translations live under `docs/zh-CN/`.

## Choose a reading path

### Evaluate the project

1. [Project README](../README.md)
2. [Architecture](architecture.md)
3. [Security model](security.md)
4. [Getting started](getting-started.md)

### Deploy and operate

1. [Installation](installation.md)
2. [Configuration](configuration.md)
3. [First-run Setup](first-run-setup.md)
4. [Node Agent](node-agent.md)
5. [Go-live checklist](go-live-checklist.md)
6. [Operations](operations.md)
7. [Backup and HA](backup-and-ha.md)
8. [Troubleshooting](troubleshooting.md)

### Use or administer the platform

- [Administrator guide](admin-guide.md)
- [User guide](user-guide.md)
- [API reference](api-reference.md)

### Contribute

- [Development guide](development.md)
- [Container and source deployment](installation.md)
- [Contributing](../CONTRIBUTING.md)
- [Security policy](../SECURITY.md)
- [Code of Conduct](../CODE_OF_CONDUCT.md)
- [Changelog](../CHANGELOG.md)

## Document ownership

| Topic | Canonical file |
| --- | --- |
| Product overview and five-minute path | `README.md` |
| Components, data flow, trust boundaries | `architecture.md` |
| Installation and lifecycle | `installation.md`, `operations.md` |
| Startup YAML and runtime settings | `configuration.md`, `first-run-setup.md` |
| Node behavior | `node-agent.md` |
| Web workflows | `admin-guide.md`, `user-guide.md` |
| HTTP contract | `api-reference.md` |
| Security and disclosure | `security.md`, root `SECURITY.md` |

When behavior changes, update the English source and the matching `zh-CN` document in the same pull request. Run `bash scripts/check_docs.sh` before submitting.
