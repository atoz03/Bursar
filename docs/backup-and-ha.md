# Backup and High Availability

**English** | [简体中文](zh-CN/backup-and-ha.md)

Backups protect against data loss. HA reduces downtime. They solve different problems, and HA is not a substitute for a tested restore.

## What is worth backing up

| Item | Why |
| --- | --- |
| PostgreSQL database | Identities, account mappings, points, prices, usage history, policies, runtime settings including SMTP credentials |
| Controller configuration | Startup secrets and listener configuration |
| `web/dist` | Convenience only; it can be rebuilt from source |

By default the backup scripts protect the platform itself, not user research data. `BACKUP_DATA_PATHS` is empty on purpose — user data belongs to your storage system and is usually far too large for this pipeline. Set it only if you have deliberately decided otherwise.

## Backup with restic

`scripts/gpuops_backup.sh` produces encrypted, deduplicated snapshots with a retention policy. It requires `docker`, `restic`, `jq`, and `flock`.

### Configure

Create `/etc/gpu-controller/backup.env`:

```bash
RESTIC_REPOSITORY=sftp:backup@backup.example.org:/srv/restic/gpu-ops
RESTIC_PASSWORD_FILE=/etc/gpu-controller/restic-password
POSTGRES_CONTAINER=gpuops-postgres
POSTGRES_DATABASE=gpuops
POSTGRES_USER=gpuops
CONTROLLER_CONFIG_PATH=/opt/gpu-controller/controller.yaml
KEEP_DAILY=7
KEEP_WEEKLY=4
KEEP_MONTHLY=12
```

The repository must live on a separate disk or a separate host. A backup on the same disk as the database does not survive the failure it exists to protect against.

Restrict the password file: `chmod 600 /etc/gpu-controller/restic-password`. Losing it makes every snapshot permanently unreadable — store a copy in your organisation's secret manager.

### Install and run

```bash
sudo bash scripts/install_backup_local.sh
sudo bash scripts/gpuops_backup.sh
```

The installer registers a systemd timer. Each run writes `backup_status_file`, which the controller reads and shows on Admin → Status.

### Verify restores

A backup you have not restored is a hypothesis. `scripts/gpuops_backup_verify.sh` restores the newest database snapshot into a throwaway PostgreSQL container and confirms it loads:

```bash
sudo bash scripts/gpuops_backup_verify.sh
```

It writes `backup_verify_status_file`, also shown on Admin → Status. Run it on a schedule, not just after setup, and alert when the verify timestamp goes stale.

### Manual restore

```bash
restic snapshots
restic restore <snapshot-id> --target /var/tmp/gpuops-restore
docker compose --profile full stop controller
psql "postgres://gpuops@127.0.0.1:5432/gpuops" < /var/tmp/gpuops-restore/.../gpuops.sql
docker compose --profile full start controller
curl -fsS http://127.0.0.1:8080/readyz
```

Stop the controller before restoring. Restoring underneath a running controller produces inconsistent state.

## High availability

HA in Bursar is operational automation, not a consensus protocol. There is no quorum, no automatic leader election, and no split-brain protection in the application. Preventing two simultaneously active primaries is your responsibility, at the network and process level.

### Model

- One controller runs as `primary` with `ha_role: primary`.
- One controller runs as `standby` with `ha_role: standby`.
- The primary periodically synchronises state to the standby.
- `GET /api/ha/status` on the internal listener reports synchronisation state, authenticated with `X-HA-Token`.

### Configure

On both controllers:

```yaml
ha_enabled: true
ha_node: "controller-primary"     # or controller-standby
ha_role: "primary"                # or standby
ha_peer_url: "https://standby.example.org:8081"
ha_token: "<shared-ha-token>"
```

Generate `ha_token` with `openssl rand -hex 32`. It is a distinct secret from `agent_token` and `admin_token`.

### Deploy a standby

```bash
sudo bash scripts/deploy_dr_standby.sh
sudo bash scripts/bootstrap_dr_standby_local.sh
```

`scripts/ha_sync_worker.sh` runs the synchronisation loop. `scripts/gpuops_ha_apply.sh` is a restricted root helper that applies synchronised artifacts; it validates file ownership and refuses unexpected paths.

In the primary-to-standby direction the worker also copies `database/migrations/` to the standby before the binary, and aborts if a checksum manifest of the `*.sql` files differs between the two hosts. It never deletes extra migration files on the standby, because they may already be recorded as applied. Set `SYNC_MIGRATIONS=0` to skip this step.

### Release to primary and standby

After the node agents have been updated, `scripts/finish_pending_release.sh` completes a release in one pass. It refuses to run if tracked files have uncommitted changes or `web/dist` has not been built. It then takes an immediate backup, installs and restarts the primary controller, checks that the installed binary reports the expected commit, checks that the newest migration in `database/migrations/` has been applied, runs a primary-to-standby sync, and waits until the HA status reports a reachable peer with a matching version.

```bash
DR_HOST=192.0.2.20 DR_SSH_USER=gpuops DR_KEY_FILE=/etc/gpu-ops/standby_ed25519 \
PRIMARY_HOST=192.0.2.10 bash scripts/finish_pending_release.sh
```

Optional variables include `CONTROLLER_URL`, `CONFIG_PATH`, `POSTGRES_CONTAINER`, `DR_CONTROLLER_PORT`, `PRIMARY_CONTROLLER_PORT`, `REMOTE_CONFIG_PATH`, and `DR_RUN_USER` (the local account that runs the sync worker).

### Failover

Failover is manual and deliberate:

1. Confirm the primary is genuinely down. A network partition looks identical to a dead host from the standby's perspective.
2. Stop the controller on the primary if it is reachable, so it cannot resume writing.
3. Promote the standby: set `ha_role: primary` and restart it.
4. Move the VIP or update DNS. `scripts/install_ha_vip_local.sh` manages a VIP with a health check against `/readyz`.
5. Confirm agents reconnect and reports resume.

Failing back is the same procedure in reverse, after reconciling any divergence.

### What HA does not do

- It does not fail over automatically.
- It does not prevent split brain. Two active primaries writing to two databases will diverge irreconcilably.
- It does not replace backups. Synchronisation faithfully copies corruption and accidental deletions.
- It does not cover PostgreSQL replication. If you need database-level HA, configure it in PostgreSQL.

## Drill checklist

Run quarterly:

- [ ] Restore the newest snapshot into a scratch environment and start a controller against it.
- [ ] Confirm the restored database has recent usage records and the expected user count.
- [ ] Promote the standby in a maintenance window and confirm agents reconnect.
- [ ] Confirm the backup and verify timestamps on Admin → Status are current.
- [ ] Confirm the restic password is recoverable from your secret manager by someone other than the person who set it up.
