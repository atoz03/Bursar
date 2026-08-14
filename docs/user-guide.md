# User Guide

**English** | [简体中文](zh-CN/user-guide.md)

You keep using SSH the way you always have. Bursar handles accounting and enforcement in the background.

## Everyday use

```bash
ssh user@node05
python train.py
```

Nothing about your workflow changes while your balance is healthy.

## Account status

| Status | What it means |
| --- | --- |
| `normal` | Full access |
| `warning` | Low balance; running jobs continue |
| `limited` | New GPU jobs are blocked by the shell hook; CPU may be throttled |
| `blocked` | Overdrawn; GPU access is disabled and CPU is heavily throttled |

If you exceed the monthly overdraft ceiling set by your administrator, all of your processes are terminated.

The node may write status files into your home directory:

| File | Meaning |
| --- | --- |
| `~/.gpu_notice` | A message from the platform |
| `~/.cpu_quota` | A CPU limit is currently applied to you |
| `~/.gpu_blocked` | You are blocked; the shell hook uses this when the controller is unreachable |

## Points

- **General points** are granted monthly according to your category (doctoral, master's, other) or an administrator's special rule.
- **Carryover points** are last month's unused general points, moved forward at the start of the month and capped by the carryover ceiling. That ceiling limits the accumulated pool, not the monthly addition.
- **Node-exclusive points** apply only to a specific node. They do not carry over and do not expire monthly.

Charges are deducted in this order: **node-exclusive → carryover → general**.

## The shell hook

Your administrator may add this to your `~/.bashrc`:

```bash
source /opt/gpu-cluster/check_quota.sh
```

The hook wraps `python`, `python3`, and `nvidia-smi`. Before a command that looks like it will use a GPU — the script or `-c` snippet mentions `torch`, `tensorflow`, `jax`, or `cuda`, or `CUDA_VISIBLE_DEVICES` is set — it checks your balance.

It is deliberately reluctant to block you:

- If the controller answers, its status decides.
- If the controller is unreachable, you are blocked only when `~/.gpu_blocked` exists locally.

## Checking your balance

In the Web UI: **My balance**, which also shows points increments and usage history.

From a node, if your administrator has deployed the helper:

```bash
CONTROLLER_URL=https://gpu-ops.example.org balance-query
```

The balance API requires a credential. On a managed node the helper reads it from `/etc/gpu-ops/query-token`. If you get an authentication error, ask your administrator to deploy that file — it is not something you can create yourself.

## Registering and binding accounts

When the cluster blocks SSH for unregistered accounts, register before you can log in:

1. Open the controller Web UI, for example `https://gpu-ops.example.org/`.
2. Go to **User → Register**.
3. Choose one:
   - **Bind an existing account** — declare which nodes you already have accounts on and the local username on each.
   - **Request a new account** — ask an administrator to create one on a node where you have none.
4. Track the outcome under **My requests** (`pending` / `approved` / `rejected`).

Registration may be restricted to specific email domains and may require email verification.

## Passwords

Registration, reset, and change all require a strong password:

- 12–64 characters
- at least one uppercase letter, one lowercase letter, one digit, and one special character (for example `!@#$%^&*_-+=`)
- no spaces

You can enable TOTP two-factor authentication under your profile. Your administrator may require it.

## FAQ

**I can log in but cannot run GPU jobs.** You are probably `limited` or `blocked`. Check your balance and ask an administrator to top up.

**The controller is unreachable — am I blocked?** Not unless `~/.gpu_blocked` exists on that node. If you are blocked and believe you should not be, contact an administrator.

**I cannot SSH in at all.** The cluster likely blocks unregistered accounts. Submit a bind or new-account request in the Web UI and wait for approval.

**My job was killed.** You were overdrawn beyond the monthly ceiling, or an administrator terminated it. Your usage history shows the charges that led there.

**A CPU limit appeared out of nowhere.** Check `~/.cpu_quota`. Limits are applied automatically at `limited` and `blocked` status.
