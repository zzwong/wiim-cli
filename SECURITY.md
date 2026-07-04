# Security Policy

## Supported Versions

Only the latest release is supported with security updates.
Older versions should be upgraded promptly.

## Reporting a Vulnerability

Please report security vulnerabilities via **GitHub Security Advisories** on this repository:

https://github.com/zzwong/wiim-cli/security/advisories

1. Go to the link above and click **"Report a vulnerability"**.
2. Provide a clear description of the issue, including steps to reproduce and any relevant
   configuration or environment details.
3. If possible, include a suggested fix or mitigation.

For non-critical security questions, you may also open a regular GitHub issue.

## Response Expectations

This project is maintained by a single developer on a best-effort basis. You can expect:

- An initial acknowledgment within **5 business days** of the report.
- A fix or mitigation plan as soon as reasonably possible, depending on severity and
  availability.
- Disclosure coordinated with the reporter before public announcement.

## Security Model

See [docs/security.md](docs/security.md) for a detailed description of the security
architecture, including credential storage, OAuth token handling, LAN file serving,
device TLS, and known limitations.
