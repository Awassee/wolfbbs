# Operations Guide

This is the day-two operator runbook for WolfBBS after install and first launch.

## Daily checks

Run these first:

```bash
bash install.sh --status
bash install.sh --doctor
```

Review these pages:

- `/admin/system`
- `/admin/audit`
- `/admin/ops`
- `/status`

Generated references:

- `<prefix>/FIRST_STEPS.txt`
- `<prefix>/SERVICE_STATUS.txt`

## Common operator jobs

### Update the board safely

Published update:

```bash
bash install.sh --upgrade
```

Local rebuild from the current checkout:

```bash
bash install.sh --rapid-upgrade
```

### Recover from a bad state

```bash
bash install.sh --status
bash install.sh --doctor
bash install.sh --repair
bash install.sh --logs
```

### Stop or restart services

```bash
bash install.sh --stop
bash install.sh --start
bash install.sh --restart
```

## First things to check when users report problems

### "I cannot log in"

- check `/admin/system`
- check `/admin/errors`
- run `bash install.sh --doctor`

### "The web app feels broken"

- check `/healthz`
- check `/readyz`
- check `bash install.sh --logs`

### "SSH works but the browser does not"

- verify the configured web port in `bash install.sh --status`
- verify reverse proxy or hostname settings in `/admin/config`

### "Chat is weird"

- test `/chat`
- if IRC is enabled, verify the IRC bridge separately
- check `/admin/chat`

## Weekly operator rhythm

- review `/admin/audit`
- review `/admin/users`
- refresh bulletin or board starter content
- verify doors and scores still feel alive
- run upgrade or rapid-upgrade when needed

## Release discipline

Before packaging a release, prefer the one-command QA path:

```bash
scripts/release.sh --version vX.Y.Z --full-qa
```

That path now runs smoke verification, browser and terminal functional checks, manual acceptance auto mode, and the security audit before packaging.

When you want the same readiness bar without tagging a release, use:

```bash
scripts/feature-complete.sh
```

That wrapper runs the full feature-complete gate in one command:

- `go test ./...`
- `scripts/verify.sh --fast`
- `scripts/qa-functional.sh --with-web-e2e --with-manual-auto`
- `scripts/verify.sh --smoke`
- `scripts/security-audit.sh`

After a public release, switch to the triage loop in [POST_RELEASE_TRIAGE.md](POST_RELEASE_TRIAGE.md) before starting the next feature tranche.

## Useful references

- [Start Here](START_HERE.md)
- [Install Guide](INSTALL.md)
- [Product Guide](PRODUCT_GUIDE.md)
- [Feature Reference](feature-reference.md)
- [Post-Release Triage](POST_RELEASE_TRIAGE.md)
- [Next Feature Wave](NEXT_FEATURE_WAVE.md)
