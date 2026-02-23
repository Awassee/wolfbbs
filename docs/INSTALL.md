# Install WolfBBS

`install.sh` is the preferred install path. It is designed for fresh hosts and can also be run from a local checkout.

## Supported targets

- Linux (Ubuntu/Debian via `apt`, Fedora/RHEL via `dnf` or `yum`, Arch via `pacman`)
- macOS (Darwin) using Docker Desktop (Homebrew-assisted install supported)

`install.sh` currently supports Docker-based install as the production path. Native install is reserved for a future release.

## Quick commands

One-liner:

```bash
curl -fsSL https://raw.githubusercontent.com/<owner>/<repo>/main/install.sh | bash -s -- --with-docker --repo-url https://github.com/<owner>/<repo>.git
```

Replace `<owner>/<repo>` with the real Git repository location.

If you are running from the repository checkout, no `--repo-url` is needed.

From local clone:

```bash
cd /path/to/wolfbbs
bash install.sh --with-docker --ssh-port 2222 --web-port 8080 --irc-port 6667
```

Status:

```bash
bash install.sh --status
```

Upgrade:

```bash
bash install.sh --upgrade
```

Uninstall:

```bash
bash install.sh --uninstall
```

## Flags

- `--prefix <dir>`: install directory (default `/opt/wolfbbs`)
- `--with-docker`: force docker mode (default)
- `--dry-run`: print actions only
- `--yes` / `--non-interactive`: no prompts
- `--force`: overwrite existing `.env`
- `--ssh-port <port>`: default `2222`
- `--web-port <port>`: default `8080`
- `--irc-port <port>`: default `6667`
- `--irc-tls-port <port>`: default `6697`
- `--mailin-port <port>`: default `8091`
- `--uninstall`: stop services
- `--purge`: with `--uninstall`, remove docker volumes and data
- `--upgrade`: pull images and restart
- `--status`: show current install and endpoints
- `--repo-url <url>`: clone from git URL when not run inside a checkout

## Non-interactive mode and dry-run

By default, `install.sh` prompts for potentially disruptive actions. Use `--yes` for automation.

`--dry-run` can be used with all actions to verify commands and generated config before changing the host.

## What it configures

- `.env` values (generated secrets and ports)
  - `WOLFBBS_DATABASE_URL`
  - `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
  - `WOLFBBS_SSH_PORT`, `WOLFBBS_WEB_PORT`, `WOLFBBS_IRC_PORT`
  - app secrets and startup retry values
- Brings up:
  - `postgres`
  - `bbs`
  - `web`
  - `irc`
  - `mailin` webhook adapter
- Verifies reachable services after startup

## macOS notes

The script defaults to Docker mode and will:

- recommend Docker Desktop if Docker is missing
- use Homebrew if present and interactive

It does **not** modify firewall rules.

## Troubleshooting

- Docker not installed
  - Install Docker Desktop manually and rerun.
- Port already in use
  - Choose alternate ports with `--ssh-port`, `--web-port`, `--irc-port`.
- Permissions denied on Docker socket
  - Use `sudo` for install or add your user to the docker group if preferred.
- Health check failing
  - Verify `docker compose ps`, then check host/container logs.

## Security notes

- Review `install.sh` before running `curl | bash`.
- Use strong secrets (installer-generated values are written to `.env` with mode `600`).
- Rotate generated credentials before production.
