# Security Policy

## Supported Versions

Security fixes are provided for the latest public release branch unless a
separate support window is announced.

## Reporting a Vulnerability

Please report vulnerabilities privately to the project maintainers before public
disclosure. Include:

- affected version or commit
- reproduction steps
- impact assessment
- logs or proof of concept when safe to share

Do not include real API keys, OAuth tokens, cookies, private account data, or
production credentials in reports.

## Security Baseline

The project should prioritize:

- authentication and session protection
- least-privilege admin access
- API key redaction in logs and UI
- rate limiting and abuse throttling
- SSRF and request-body limits
- secure default config examples
- signed release artifacts and checksums
- dependency vulnerability review before release
- explicit review before enabling remote management-panel downloads or
  auto-updates

## Not A Goal

The open source edition does not attempt to prevent users from modifying,
building, forking, or self-hosting the code. License compliance and secure
service operation are the goals, not anti-tamper restrictions.
