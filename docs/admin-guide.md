# Administrator Guide

**English** | [简体中文](zh-CN/admin-guide.md)

Modules, permissions, and security rules for administrators and delegated operators.

## Roles

### Administrator (`admin`)

Full access to every module: node policy, points, registration review, account mapping, security response, mail configuration, and HA.

### Power user (`power_user`)

Access only to the capabilities granted by permission bits:

| Permission | Grants |
| --- | --- |
| `view_board` | Operations dashboard |
| `view_nodes` | Node status |
| `manage_nodes` | Node policy changes and remote actions |
| `review_requests` | Registration and profile review |
| `manage_points` | Points administration |
| `manage_platform_users` | Platform user administration |

Points administration is subdivided further — user lists, filtered batches, all-user batches, records, monthly configuration, and special rules each have their own bit — so a delegated operator can be given exactly one of them.

Requests authenticated with `admin_token` bypass all of these checks. See [Security model](security.md).

## Module map

### Overview

| Page | Path |
| --- | --- |
| Operations dashboard | `/admin/board` |
| Cluster overview | `/admin/status` |

### Resources and billing

| Page | Path |
| --- | --- |
| Node management | `/admin/nodes` |
| Process audit | `/admin/usage` |
| Points management | `/admin/points` |

### Accounts and access

| Page | Path |
| --- | --- |
| Platform users | `/admin/users` |
| Account mapping | `/admin/accounts` |
| Account provisioning | `/admin/account-provision` |
| Registration and profile review | `/admin/requests` |
| Power users | `/admin/power-users` |
| SSH lists (allow / deny / exempt) | `/admin/whitelist` |

### System and continuity

| Page | Path |
| --- | --- |
| HA synchronisation | `/admin/ha` |
| Announcements | `/admin/announcements` |
| User guidelines | `/admin/guideline` |
| Administrator notebook | `/admin/notebook` |
| Mail settings | `/admin/mail` |
| First-run Setup | `/admin/setup` |

## Node management

Per-node policy switches:

- **SSH guard** — block accounts that are not registered on the platform.
- **Points enforcement** — whether usage is charged and limits are applied on this node.
- **Pricing** — GPU price and CPU core-minute price, set per node.
- **Exclusive users** — reserve a node for specific users.
- **Visibility** — restrict which power users can see the node.

Remote actions: sync now, disconnect SSH sessions, and terminate a user's processes. These act on the node immediately. Validate with `dry_run: true` before relying on them.

The node list flags risk with an emoji next to the node ID, and a banner summarises nodes with security events in the last 7 days.

## Points management

- Adjust an individual user's general, carryover, or node-exclusive points.
- Apply a batch change across all users.
- Configure the monthly reset by degree category, with per-user special rules.
- Configure the carryover ceiling. This is a cap on the accumulated carryover pool, not a cap on how much may be added each month.
- Configure the monthly maximum overdraft. Exceeding it triggers termination of all of that user's processes.

Deduction order is **node-exclusive → carryover → general**. Node-exclusive points do not participate in monthly carryover and do not expire monthly.

Every points operation is recorded with timestamp, operation, target, delta, and points type.

## Accounts and access

**Platform users.** Status, block and unblock, delete and restore, duplicate-identity detection.

**Account mapping.** Maintains `(node_id, local_username) → platform user`. The rebinding monitor flags a local account that rebinds frequently within a time window or that has been associated with several platform accounts. Flagged accounts carry a red dot and can be blocked directly from the risk panel.

**Account provisioning.** Provisions a node account and delivers credentials as ciphertext in the platform plus an extraction code by email. If the account is already mapped to the same platform user, a second confirmation allows regenerating and resending.

**SSH lists.** Allow, deny, and exemption lists in one place, with a reason and source recorded per entry.

## Security events

### Suspected malicious users

This is not a manual label. The controller aggregates `node_security_events` over a 7-day window, grouped by `node_id + username`:

| Field | Meaning |
| --- | --- |
| `hit_count` | Number of triggers in the window |
| `last_seen_at` | Most recent trigger |
| `reason_hints` | Aggregated reasons |
| `phenomena` | Suspected behaviour, such as mining or SSH brute force |
| `mining_suspected` | Whether a `suspected_mining` event was hit in the window |

Only events that carry a username reach this list. Port-scan events usually have no associated user and appear only in the security audit log.

Default limits: 200 rows on node detail, 500 on the security events page.

### Detection rules

Values below are the constants in `controller/api.go`. Verify them again if you change detection logic.

| Event | Condition | Severity | Cooldown |
| --- | --- | --- | --- |
| `suspected_mining` | Process command line matches mining keywords (`xmrig`, `cpuminer`, `minerd`, `ethminer`, `lolminer`, `nbminer`, `stratum`, `randomx`, `ethash`, …) | `critical` | 2 min |
| `high_cpu_load` | Node CPU load ≥ 95% of total capacity | `warning` | 5 min |
| `ssh_failed_login_spike` | More than 20 failed SSH logins in 5 minutes | `critical` | 30 min |
| `ssh_bruteforce` | Agent brute-force signal active | `critical` | 30 min |
| `abnormal_port_scan` | Agent port-scan signal active | `critical` | 30 min |
| `disk_full_risk` | `/`, `/home`, or `/mnt` at ≥ 98% used or ≤ 1 GB free | `critical` | 10 min |

Cooldown suppresses repeat writes of the same event type on the same node, so a persistent condition does not flood the log. Agent-signal events additionally hold for 10 minutes before being treated as cleared.

## Response playbooks

### Suspected mining

1. Open node detail and review the suspected-user list and the security audit log.
2. Check the command samples and trigger reasons.
3. Block the account, disconnect SSH, and terminate its processes.
4. Review the account's mapping history and recent points consumption.
5. Notify the user and freeze related accounts if warranted.

### SSH attack spike

1. Review `ssh_failed_login_spike` and `ssh_bruteforce` events.
2. Add suspect accounts to the deny list.
3. Confirm the SSH guard is enabled on that node.
4. Re-check the allow and exemption lists so an exemption is not acting as a back door.

### Disk nearly full

1. Open the `disk_full_risk` event for the affected mount and free space.
2. Identify heavy `/home` consumers on node detail.
3. Ask the user to clean up, or clean up on their behalf; restrict the account temporarily if needed.

## Keeping this document accurate

Update this guide whenever modules, permissions, security rules, thresholds, or response actions change. The code paths that trigger an update:

- `web/src/views/pages/Admin*.vue` — module behaviour
- `web/src/router/index.ts`, `web/src/views/Layout.vue` — menu and permission entry points
- `controller/api.go`, `controller/database.go` — security rules, thresholds, event logic
- `config/controller.yaml` — meaning of policy parameters

Before each release, confirm menu paths match, the threshold table matches the code, the suspected-user definition matches the query, and any new module is listed here — in both languages.

## Related

- [User guide](user-guide.md)
- [API reference](api-reference.md)
- [Operations](operations.md)
- [Security model](security.md)
