# Contributing to GPU Ops

**English** | [简体中文](CONTRIBUTING.zh-CN.md)

Thank you for improving GPU Ops. Read the [Code of Conduct](CODE_OF_CONDUCT.md) before contributing. Do not open a public issue for a vulnerability; follow the [Security Policy](SECURITY.md) instead.

## Before you start

1. Search existing issues and pull requests to avoid duplicate work.
2. For a large feature, data-model change, or incompatible behavior, open an issue first and describe the motivation, scope, and migration plan.
3. Fork the repository and create a short-lived branch from the latest `main`.

## Local development

See the [development guide](docs/development.md) for prerequisites and setup. The common validation commands are:

```bash
go test ./controller/... ./node-agent/...
go vet ./controller/... ./node-agent/...
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
bash -n scripts/*.sh
```

For documentation-only changes, run at least:

```bash
bash scripts/check_docs.sh
```

## Change requirements

- Keep the implementation simple, explicit, and consistent with the existing structure.
- Add or update tests for behavior changes. If automated coverage is impractical, document the manual verification in the pull request.
- Update both the English source documentation and its Chinese translation when configuration, APIs, UI workflows, or operations change.
- Never commit real tokens, private keys, database DSNs, host inventories, user data, backups, or generated artifacts.
- Explain every new dependency and verify that its license is compatible with Apache-2.0 distribution.
- Add a new ordered migration for database changes; do not rewrite a migration that may already have been applied.

## Commits and pull requests

Concise Conventional Commits-style subjects are recommended:

```text
feat: add node maintenance mode
fix: prevent duplicate usage billing
docs: document first-run setup
```

A pull request should describe the problem, solution, user-visible behavior, compatibility impact, tests, and any deployment or migration steps. UI screenshots are welcome, but they must not expose real accounts, hosts, or operational data.

By submitting a contribution, you agree that it is provided under the repository's [Apache License 2.0](LICENSE).
