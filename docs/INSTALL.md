# Install WolfBBS

`install.sh` is the primary installation path and supports local checkout usage and curl-pipe bootstrap.

## Supported Targets

- Linux
  - Ubuntu/Debian (`apt`)
  - Fedora/RHEL/CentOS (`dnf`/`yum`)
  - Arch (`pacman`)
- macOS 12+ (Intel and Apple Silicon)
  - Docker Desktop or Colima runtime

## Install Modes

- Docker-based install: supported and default.
- Native mode: not currently supported.

## Quick Install (copy/paste-safe: git clone + local installer)

Linux:

```bash
git clone https://github.com/seanheiney/New-project.git wolfbbs && cd wolfbbs && bash install.sh --yes
```

macOS:

```bash
git clone https://github.com/seanheiney/New-project.git wolfbbs && cd wolfbbs && bash install.sh --yes --install-brew
```

If the repository is private and HTTPS clone fails, use SSH clone instead:

```bash
git clone git@github.com:seanheiney/New-project.git wolfbbs && cd wolfbbs && bash install.sh --yes
```

## Optional Quick Install (`curl | bash`)

Use this only when the repo's raw GitHub URL is publicly reachable:

```bash
curl -fsSL "https://raw.githubusercontent.com/seanheiney/New-project/main/install.sh" | bash -s -- --yes
```

macOS variant:

```bash
curl -fsSL "https://raw.githubusercontent.com/seanheiney/New-project/main/install.sh" | bash -s -- --yes --install-brew
```

Dry-run preflight:

```bash
curl -fsSL "https://raw.githubusercontent.com/seanheiney/New-project/main/install.sh" | bash -s -- --yes --install-brew --dry-run
```

Install from a fork/custom repository:

```bash
curl -fsSL "https://raw.githubusercontent.com/seanheiney/New-project/main/install.sh" | bash -s -- --yes --repo your-org/your-repo
```

## Install From Local Clone

```bash
cd /path/to/wolfbbs
bash install.sh --with-docker --ssh-port 2222 --web-port 8080 --irc-port 6667
```

## What the Installer Does

1. Detects OS, architecture, package manager.
2. Ensures required tools are present (`curl`, `git`, `openssl`, `sed`, `awk`, `grep`, `nc`).
3. Installs missing base dependencies automatically when possible.
4. Ensures Docker runtime is available.
   - Linux: installs Docker Engine + compose plugin when needed.
   - macOS: supports Docker Desktop or Colima; with `--yes --install-brew`, can bootstrap Colima stack.
5. Resolves compose file.
   - Uses local repo if present.
   - If missing and `--repo`/`--repo-url` set, clones/updates into `--prefix`.
6. Generates `.env` (unless existing and no `--force`), sets `chmod 600`.
7. Runs `docker compose up -d --build`.
8. Verifies health and key ports.
9. Prints connection summary.

## Installer Flags

- `--prefix <dir>`: install directory
  - Linux default: `/opt/wolfbbs`
  - macOS default: `$HOME/.local/share/wolfbbs`
- `--with-docker`: force docker mode (default)
- `--dry-run`: print actions only, no mutations
- `--yes`, `--non-interactive`: disable prompts
- `--install-brew`: allow Homebrew install on macOS if missing
- `--force`: overwrite generated config (`.env`) and allow replacement behavior
- `--ssh-port <port>`: default `2222`
- `--web-port <port>`: default `8080`
- `--irc-port <port>`: default `6667`
- `--irc-tls-port <port>`: default `6697`
- `--mailin-port <port>`: default `8091`
- `--repo <owner/repo|url>`: clone target for standalone bootstrap
- `--repo-url <url>`: alias for `--repo`
- `--status`: show current install status/endpoints
- `--upgrade`: pull/rebuild/restart stack in existing install
- `--uninstall`: stop services and optionally remove data
- `--purge`: with uninstall, remove volumes/data
- `--help`: show flag summary

## Generated and Managed Files

- `<prefix>/.env` (mode `600`)
- `<prefix>/install.log` (or script-dir log before clone)

Important generated values include:

- `WOLFBBS_DATABASE_URL`
- `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
- `WOLFBBS_SESSION_SECRET`
- `WOLFBBS_INBOUND_TOKEN`
- `WOLFBBS_SSH_PORT`, `WOLFBBS_WEB_PORT`, `WOLFBBS_IRC_PORT`, `WOLFBBS_IRC_TLS_PORT`, `WOLFBBS_MAILIN_PORT`
- `WOLFBBS_BOOTSTRAP_ADMIN_HANDLE`, `WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD`

## Post-Install Commands

Status:

```bash
bash install.sh --status
```

Upgrade:

```bash
bash install.sh --upgrade
```

Uninstall (interactive):

```bash
bash install.sh --uninstall
```

Uninstall + purge (non-interactive):

```bash
bash install.sh --uninstall --purge --yes
```

## Verify Running Services

Health checks:

```bash
curl -fsS "http://localhost:8080/healthz"
curl -fsS "http://localhost:8080/readyz"
```

Port checks:

```bash
nc -z localhost 2222
nc -z localhost 6667
nc -z localhost 8091
```

Connect:

- SSH: `ssh localhost -p 2222`
- Web admin: `http://localhost:8080/admin`
- Web chat: `http://localhost:8080/chat`
- Help hub: `http://localhost:8080/help`
- IRC: `localhost:6667`

End-to-end verification (local dev):

```bash
python3 -m pip install pexpect
scripts/run-e2e.sh --no-web
# full suite (requires Node.js 22/24 + npm)
scripts/run-e2e.sh
```

Optional reliability knobs:

```bash
# bound Docker smoke startup duration in verifier
COMPOSE_CMD_TIMEOUT_SECONDS=10 COMPOSE_UP_TIMEOUT_SECONDS=180 scripts/verify.sh --smoke

# allow Playwright run on non-LTS Node when explicitly needed
WOLFBBS_ALLOW_UNSUPPORTED_NODE=true scripts/run-e2e.sh --no-go --no-tui
```

First sysop pass (recommended):

- Sign in to `/admin/login` with bootstrap sysop credentials from `.env`.
- Open `/admin/setup` and run:
  - `Seed Default Boards`
  - `Ensure Mailbot Account`
- Open `/admin/config` and set:
  - MOTD / Announcement
  - runtime flags (`read-only`, on-ramp/tour/discover as desired)
- Use `/admin/system` and `/admin/errors` to verify health and runtime status.

## Troubleshooting

- Docker daemon unavailable
  - macOS Docker Desktop: `open -a Docker`
  - macOS Colima: `colima start`
  - Linux systemd: `sudo systemctl start docker`
- Docker compose/socket hangs or EOF on Colima
  - restart Colima: `colima stop -f && colima start`
  - verify daemon reachability: `docker info`
  - if unix socket forwarding is unhealthy, use ssh transport:
    - `DOCKER_HOST=ssh://lima-colima docker info`
- Docker permission denied on Linux
  - add user to `docker` group or run with sudo-capable user
- Port in use
  - rerun with alternate `--ssh-port`, `--web-port`, `--irc-port`, `--mailin-port`
- Health check failed after up
  - inspect compose status/logs:
    - `docker compose ps`
    - `docker compose logs --tail=200`
- `.env` already exists and you need regeneration
  - rerun with `--force`

## Security Notes

- Review `install.sh` before executing from curl-pipe.
- Installer does **not** modify firewall rules.
- Rotate bootstrap credentials after first login.
- Use HTTPS + `WOLFBBS_SECURE_COOKIE=true` in production.
