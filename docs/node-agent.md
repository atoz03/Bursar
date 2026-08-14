# Node Agent

**English** | [简体中文](zh-CN/node-agent.md)

The node agent runs on every Linux compute node. It reports hardware, process, GPU, disk, SSH, and service state to the controller, and applies the policy actions the controller returns.

## Why the agent is not containerised

The agent needs write access to cgroup v2, systemd units, the host process table, `/etc/passwd`, SSH configuration, and the NVIDIA driver stack. Running it in a container would require so much host access that the isolation would be meaningless. Install it directly on the host with systemd. Only the controller ships as a container image.

## Prerequisites

Run the read-only prerequisite check first. It inspects the node and changes nothing:

```bash
bash scripts/node_prereq_check.sh
```

The node needs:

- Linux with systemd (recommended) or cgroup v2 at `/sys/fs/cgroup`;
- network reachability to the controller's internal listener;
- `nvidia-smi` on GPU nodes;
- root privileges for the agent service.

## Installation

From a checked-out repository on the node:

```bash
sudo env \
  NODE_ID=node-01 \
  CONTROLLER_URL=https://controller.example.org:8081 \
  AGENT_TOKEN='<node-or-global-agent-token>' \
  bash scripts/install_agent_local.sh
```

The installer builds the binary, writes `/etc/gpu-cluster/node-agent.env`, installs the `gpu-node-agent` systemd unit, and optionally installs the SSH guard and host-security hooks.

`NODE_ID` is a stable operator-chosen identifier. It is not required to be an IP address or an SSH port. Once a node reports under an identifier, keep it: usage history, account mappings, and per-node points are keyed to it.

## Environment variables

Set these in `/etc/gpu-cluster/node-agent.env`.

### Required

| Variable | Meaning |
| --- | --- |
| `NODE_ID` | Stable node identifier |
| `CONTROLLER_URL` | Base URL of the controller listener the agent reports to |
| `AGENT_TOKEN` | Global agent token, or this node's token from `agent_node_tokens` |

### Timing and sampling

| Variable | Default | Meaning |
| --- | --- | --- |
| `INTERVAL_SECONDS` | `60` | Reporting interval; the controller uses it to convert samples to cost |
| `ACTION_POLL_INTERVAL_SECONDS` | `1` (installer sets `2`) | Action poll interval |
| `CPU_MIN_PERCENT` | `1.0` | Ignore processes below this CPU percentage |
| `STATE_DIR` | installer-managed | Local state directory |

### Collection caches

| Variable | Default | Meaning |
| --- | --- | --- |
| `LOCAL_USERS_REFRESH_SECONDS` | `900` | Local account list refresh interval |
| `LOCAL_USERS_COLLECT_TIMEOUT_SECONDS` | `8` | Timeout for collecting local accounts |
| `SYSTEM_SERVICES_REFRESH_SECONDS` | `1800` (installer sets `60`) | Service status refresh interval |
| `SYSTEM_SERVICES_COLLECT_TIMEOUT_SECONDS` | `8` | Timeout for collecting service status |
| `SYSTEM_SERVICES_CHECK_UNITS` | built-in list | Comma-separated systemd units to monitor |
| `DISK_QUOTA_REFRESH_SECONDS` | `120` | Disk quota refresh interval |
| `GPU_BUS_MAP_CACHE_SECONDS` | `600` | GPU bus-ID map cache lifetime |
| `GPU_INVENTORY_CACHE_SECONDS` | `1800` | GPU inventory cache lifetime |
| `GPU_COMMAND_TIMEOUT_SECONDS` | `4` | Timeout for `nvidia-smi` calls |

### Installer options

| Variable | Default | Meaning |
| --- | --- | --- |
| `ENABLE_SSH_GUARD` | `1` | Install SSH login enforcement |
| `SSH_GUARD_EXCLUDE_USERS` | `root` | Accounts the guard never blocks |
| `SSH_GUARD_FAIL_OPEN` | `0` | Allow logins when the controller is unreachable |
| `SSH_GUARD_REALTIME_LOOKUP` | `0` | Query the controller during login instead of using the cache |
| `SSH_GUARD_SYNC_INTERVAL` | `10s` | Allow/deny list sync interval |
| `SSH_GUARD_ENFORCE_INTERVAL` | `10s` | Active-session sweep interval |
| `ENABLE_HOST_SECURITY` | `1` | Install host-security signal collection |
| `ENABLE_SHARED_NFS` | `0` | Mount the shared workspace exports |
| `SKIP_CONTROLLER_HEALTHCHECK` | `0` | Skip the controller reachability check during install |

## SSH guard

With `ENABLE_SSH_GUARD=1`, the node blocks SSH logins for accounts that are not registered on the platform, using cached allow, deny, and exemption lists synchronised from the controller's registry endpoints.

`SSH_GUARD_FAIL_OPEN` decides what happens when the controller is unreachable:

- `0` (default): logins for non-excluded accounts are denied. Safer, but a controller outage can lock everyone out of the fleet.
- `1`: logins are allowed. Less safe, but an outage does not become a lockout.

Always keep a working out-of-band path to the node — console access, or an account in `SSH_GUARD_EXCLUDE_USERS` with a key you hold — before enabling the guard on a production node.

## Tokens

Start with the global `agent_token`, then migrate to per-node tokens:

1. Fill `agent_node_tokens` on the controller, keeping `agent_node_token_enforce: false`.
2. Distribute each node's token and restart the agents.
3. Confirm every node is reporting.
4. Set `agent_node_token_enforce: true` and restart the controller.

`scripts/generate_node_agent_tokens.sh` generates the token map. To rotate the global token, put the old value in `agent_legacy_tokens` during the rollout window, then clear it.

## Operating the service

```bash
sudo systemctl status gpu-node-agent
sudo systemctl restart gpu-node-agent
sudo journalctl -u gpu-node-agent -f
```

## Troubleshooting

**The node does not appear in the controller.** Check `CONTROLLER_URL` reachability from the node, confirm `AGENT_TOKEN` matches the controller configuration, and look for `401` responses in the journal. If `agent_node_token_enforce` is on, the token must be the one registered for this exact `NODE_ID`.

**GPU usage is missing.** Confirm `nvidia-smi --query-compute-apps=pid,used_memory --format=csv` works as root. Raise `GPU_COMMAND_TIMEOUT_SECONDS` on nodes where the driver is slow to respond.

**CPU limits are not applied.** Check that systemd exposes the user slice: `systemctl show user-$(id -u <user>).slice -p CPUQuota`. Nodes without systemd need cgroup v2 mounted and writable.

**Duplicate charges after a retry.** Reports carry a globally unique `report_id`, and the controller deduplicates on it. If you see duplicates, verify the node is not running two agent instances.

**Users cannot log in after enabling the guard.** Set `SSH_GUARD_FAIL_OPEN=1` temporarily, or add the account to `SSH_GUARD_EXCLUDE_USERS`, and confirm the registry lists are syncing.

See [Troubleshooting](troubleshooting.md) for controller-side symptoms.
