# Security Policy

**English** | [简体中文](SECURITY.zh-CN.md)

GPU Ops handles node control, account mapping, and resource-usage data. Please disclose vulnerabilities responsibly. Do not publish exploitable details in an issue, discussion, or pull request.

## Supported versions

Security fixes target the latest revision of `main`. Until stable releases are published, older commits and private forks are not maintained as supported release lines.

## Private reporting

Use **Security → Advisories → Report a vulnerability** in this GitHub repository. Include:

- the affected version or commit;
- the vulnerability class and impact;
- minimal reproduction steps or a proof of concept;
- a suggested remediation, if available;
- whether the issue has been disclosed elsewhere.

Do not attach production tokens, private keys, database dumps, or personal data. If sensitive material is essential to reproduction, first explain what is needed and wait for a secure transfer method.

## Response targets

- Acknowledge receipt within 7 days.
- Share severity and a remediation plan after initial triage.
- Coordinate disclosure timing before publishing a fix.

These are maintenance targets, not a service-level agreement.

## Deployment responsibility

Security updates do not replace secure deployment. Production operators should follow the [security guide](docs/security.md) and [go-live checklist](docs/go-live-checklist.md), and rotate all related credentials immediately if exposure is suspected.
