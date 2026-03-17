# Operations Guide

This is the day-two guide for running WolfBBS after it is installed.

## Daily checks

Use these commands first:

```bash
bash install.sh --status
bash install.sh --doctor
```

Review these pages:

- `/admin/system`
- `/admin/audit`
- `/status`

## Common operator jobs

### Update the board safely

Shipped update:

```bash
bash install.sh --upgrade
```

Local code rebuild:

```bash
bash install.sh --rapid-upgrade
```

### Recover from a bad state

```bash
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

### “I cannot log in”

- check `/admin/system`
- check `/admin/errors`
- run `bash install.sh --doctor`

### “The web app feels broken”

- check `/healthz`
- check `/readyz`
- check `bash install.sh --logs`

### “SSH works but the browser does not”

- verify the configured web port in `bash install.sh --status`
- verify reverse proxy or hostname settings in `/admin/config`

### “Chat is weird”

- test `/chat`
- if IRC is enabled, verify the IRC bridge separately
- check `/admin/chat`

## Weekly operator rhythm

- review `/admin/audit`
- review `/admin/users`
- refresh bulletin or board starter content
- verify doors and scores still feel alive
- run upgrade or rapid-upgrade when needed

## Useful references

- [Start Here](START_HERE.md)
- [Install Guide](INSTALL.md)
- [Product Guide](PRODUCT_GUIDE.md)
- [Feature Reference](feature-reference.md)
