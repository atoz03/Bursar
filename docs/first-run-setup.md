# First-run Administrator Setup

**English** | [简体中文](zh-CN/first-run-setup.md)

The Setup wizard makes instance-specific runtime choices explicit while keeping startup secrets out of the browser.

## Before opening Setup

The controller must already have:

- a working PostgreSQL connection;
- non-placeholder `agent_token`;
- non-placeholder `admin_token`;
- a strong `auth_secret`;
- a valid internal TLS certificate/key pair if `internal_listen_addr` is enabled.

These values remain in startup YAML or your secret manager. Setup returns only readiness status and non-secret paths/addresses.

## Bootstrap the first administrator

Use the startup `admin_token` once:

```bash
curl -fsS -X POST https://ops.example.org/api/admin/bootstrap \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <admin_token>' \
  -d '{"username":"admin","password":"<strong-password>"}'
```

Bootstrap is rejected after an administrator exists. Store recovery access to the bearer token securely; do not embed it in frontend code or browser storage.

## Wizard flow

After the first administrator signs in, the router redirects to `/admin/setup` until Setup completes.

### 1. Platform identity

- **Platform name:** 1–80 characters; used in the UI and email subjects.
- **Registration email domains:** optional list without `@`. Empty means any valid non-disposable domain.
- **SSH host:** optional hostname or IP displayed in provisioning instructions. Do not include scheme, user, port, or path.

### 2. Pricing and user policy

- Set known GPU model prices.
- Set `CPU_CORE` for core-minute accounting.
- Publish the user guideline shown in the application.

Unknown GPU models fall back to startup `default_price_per_minute`. Test model matching before charging users.

### 3. Mail

Mail is optional. When enabled, provide SMTP host, port, user, password, sender email, and sender name. Saving with mail disabled clears stored SMTP credentials. Send a test message from the mail settings page after Setup.

### 4. HA and readiness

Leave HA disabled for a single controller. Enabling it requires explicit role, peer, SSH, script, and sync choices described in [Backup and HA](backup-and-ha.md).

Required readiness checks must pass before Setup can be completed. Internal TLS is reported as an optional check because the single-port development mode is supported, but it is recommended in production.

## Registration behavior

Before completion, public registration returns `setup_required`. After completion, registration follows the configured domain and disposable-email policy. Changing the allowed-domain list affects new registrations, not existing accounts.

## Reopening Setup

Super administrators can revisit `/admin/setup`. Saving is repeatable, but it changes active runtime settings. Review price, mail, and HA changes as production changes and test them immediately.

## API access

The relevant endpoints are:

- `GET /api/public/settings` — public, non-secret platform identity and Setup state;
- `GET /api/admin/setup` — super administrator;
- `POST /api/admin/setup` — super administrator, validates and saves the complete form;
- `POST /api/admin/bootstrap` — one-time administrator creation.

Browser requests use the signed session and CSRF token. Operator scripts may use the administrative bearer token. See [API reference](api-reference.md).

## Failure recovery

- If a readiness check fails, correct startup YAML and restart the controller.
- If SMTP validation fails, disable mail to complete Setup, then configure it separately.
- If HA values are incomplete, keep HA disabled rather than saving placeholders.
- If the administrator password is lost, follow your database and operational recovery policy; do not delete administrator rows casually to reopen bootstrap.
