# Security Policy

## Supported versions

WolfBBS is maintained on a rolling `main` branch plus the current public release.

- `main`: supported for active development and security fixes
- current public release tag: supported for the current stable release line
- older tags: best effort only unless explicitly called out in release notes

If you are reporting a vulnerability, please include:

- WolfBBS version or git SHA
- install method
- platform and architecture
- reproduction steps
- impact
- whether the issue affects default installs or only custom configuration

## Reporting a vulnerability

Please do not open a public issue with exploit details for a live security problem.

Preferred path:

1. Use GitHub private vulnerability reporting or a GitHub security advisory on the public repository.
2. If private reporting is not available in your GitHub view, open a minimal issue asking for a private contact path and do not include sensitive exploit details.

For non-sensitive hardening ideas or low-risk findings, a normal GitHub issue is fine.

## Response goals

Best-effort targets:

- initial triage acknowledgement: within 3 business days
- reproduction / severity assessment: within 7 business days
- fix or mitigation plan for confirmed issues: as quickly as practical based on severity

## Scope

This policy covers:

- installer and upgrade flows
- SSH / terminal BBS surface
- web UI and admin surfaces
- IRC gateway
- mail ingest and outbound relay controls
- gateway integrations and auth/session handling

Third-party dependencies keep their own security processes, but WolfBBS will patch or pin them when they create risk for supported releases.
