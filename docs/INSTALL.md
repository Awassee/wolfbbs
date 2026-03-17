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

## Quick Install (turnkey paste-and-go)

Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/seanheiney/wolfbbs-public/main/bootstrap.sh | bash
```

macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/seanheiney/wolfbbs-public/main/bootstrap.sh | bash -s -- --install-brew
```

Interactive menu mode (no flags):

```bash
git clone https://github.com/seanheiney/wolfbbs-public.git wolfbbs && cd wolfbbs && bash install.sh
```

The guided menu path is the recommended consumer install flow. It opens a menu first, then an easy-install screen where you can accept defaults or edit install directory, ports, and source repo without remembering flags.

What the bootstrap does:

1. Downloads the current `install.sh` to a temp location.
2. Runs the installer with any flags you pass after `bash -s --`.
3. Installs base dependencies and Docker runtime when supported.
4. Clones or updates a managed WolfBBS checkout under `<prefix>/app`.
5. Writes runtime config into `<prefix>/.env`.
6. Starts the stack and prints first-login steps.

## Install From GitHub Release Bundle

If you prefer a packaged download instead of cloning the repo, download the archive matching your platform from:

- [GitHub Releases](https://github.com/seanheiney/wolfbbs-public/releases)

Then unpack and run the installer from the bundle root:

```bash
tar -xzf wolfbbs_<version>_<os>_<arch>.tar.gz
cd wolfbbs_<version>_<os>_<arch>
bash install.sh --yes
```

If HTTPS clone is blocked in your environment, use SSH clone instead:

```bash
git clone git@github.com:seanheiney/wolfbbs-public.git wolfbbs && cd wolfbbs && bash install.sh --yes
```

## Optional Quick Install (`curl | bash`)

Use this only when the repo's raw GitHub URL is publicly reachable:

```bash
curl -fsSL "https://raw.githubusercontent.com/seanheiney/wolfbbs-public/main/bootstrap.sh" | bash
```

macOS variant:

```bash
curl -fsSL "https://raw.githubusercontent.com/seanheiney/wolfbbs-public/main/bootstrap.sh" | bash -s -- --install-brew
```

Dry-run preflight:

```bash
curl -fsSL "https://raw.githubusercontent.com/seanheiney/wolfbbs-public/main/bootstrap.sh" | bash -s -- --yes --install-brew --dry-run
```

Install from a fork/custom repository:

```bash
curl -fsSL "https://raw.githubusercontent.com/seanheiney/wolfbbs-public/main/bootstrap.sh" | bash -s -- --yes --repo your-org/your-repo
```

## Install From Local Clone

```bash
cd /path/to/wolfbbs
bash install.sh --with-docker --ssh-port 2222 --web-port 8080 --irc-port 6667
```

UI-first setup (recommended): complete identity/profile/runtime config in the product after install:

```bash
http://localhost:8080/admin/setup
http://localhost:8080/admin/config
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

When run without flags in an interactive terminal, `install.sh` opens an action menu (install, upgrade, repair, status, uninstall, etc.).

- `--prefix <dir>`: install directory
  - Linux default: `/opt/wolfbbs`
  - macOS default: `$HOME/.local/share/wolfbbs`
- `--with-docker`: force docker mode (default)
- `--dry-run`: print actions only, no mutations
- `--yes`, `--non-interactive`: disable prompts
- `--install-brew`: allow Homebrew install on macOS if missing
- `--force`: overwrite generated config (`.env`) and allow replacement behavior
- `--bbs-name <name>`: advanced automation override for BBS display name (prefer `/admin/setup`)
- `--hostname <name>`: advanced automation override for hostname (prefer `/admin/setup`)
- `--setup-profile <name>`: advanced automation baseline (`basic`, `critical`, or `expert`; prefer `/admin/setup`)
- `--ssh-port <port>`: default `2222`
- `--web-port <port>`: default `8080`
- `--irc-port <port>`: default `6667`
- `--irc-tls-port <port>`: default `6697`
- `--mailin-port <port>`: default `8091`
- `--repo <owner/repo|url>`: clone target for standalone bootstrap
- `--repo-url <url>`: alias for `--repo`
- `--status`: show current install status/endpoints
- `--doctor`: run non-mutating diagnostics (preflight + current install health)
- `--start`: start existing WolfBBS services
- `--stop`: stop existing WolfBBS services
- `--restart`: restart existing WolfBBS services
- `--logs`: show recent service logs
- `--repair`: self-heal install (ensure deps/env, rebuild, verify)
- `--deps-only`: install/check prerequisites and Docker runtime only
- `--upgrade`: pull/rebuild/restart stack in existing install
- `--rapid-upgrade`: rebuild/restart from local source (no image pull) for fast iteration
- `--uninstall`: stop services and optionally remove data
- `--purge`: with uninstall, remove volumes/data
- `--help`: show flag summary

## Generated and Managed Files

- `<prefix>/.env` (mode `600`)
- `<prefix>/app/` managed source checkout for bootstrap installs
- `<prefix>/install.log` (or script-dir log before clone)

Important generated values include:

- `WOLFBBS_DATABASE_URL`
- `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
- `WOLFBBS_SESSION_SECRET`
- `WOLFBBS_INBOUND_TOKEN`
- `WOLFBBS_BBS_NAME`, `WOLFBBS_HOSTNAME`, `WOLFBBS_SETUP_PROFILE` (automation overrides; prefer UI setup)
- `WOLFBBS_SSH_PORT`, `WOLFBBS_WEB_PORT`, `WOLFBBS_IRC_PORT`, `WOLFBBS_IRC_TLS_PORT`, `WOLFBBS_MAILIN_PORT`
- `WOLFBBS_BOOTSTRAP_ADMIN_HANDLE`, `WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD`

## Post-Install Commands

Status:

```bash
bash install.sh --status
```

Doctor (safe, non-mutating diagnostics):

```bash
bash install.sh --doctor
```

Repair (recommended when something is broken):

```bash
bash install.sh --repair
```

Service lifecycle management:

```bash
bash install.sh --start
bash install.sh --stop
bash install.sh --restart
bash install.sh --logs
```

## Distribution Packaging

Build versioned tarball bundles with binaries, installer, compose assets, docs, and checksums:

```bash
scripts/package-dist.sh
scripts/package-dist.sh --platform linux/amd64 --platform linux/arm64
```

Artifacts are written to `dist/<version>/`.

Dependencies/runtime bootstrap only:

```bash
bash install.sh --deps-only
```

Upgrade:

```bash
bash install.sh --upgrade
```

Rapid local upgrade (while iterating on code):

```bash
bash install.sh --rapid-upgrade
```

Optional in-BBS quick upgrade hook (sysop):

```bash
export WOLFBBS_APP_UPGRADE_COMMAND="bash install.sh --rapid-upgrade --yes"
export WOLFBBS_APP_UPGRADE_WORKDIR="/path/to/wolfbbs"
```

Then in SSH main menu press `/` and enter `/app upgrade`.

Uninstall (interactive):

```bash
bash install.sh --uninstall
```

Uninstall + purge (non-interactive):

```bash
bash install.sh --uninstall --purge --yes
```

Note: if install prefix is a git checkout, the installer now keeps that directory and only removes running services/volumes.

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

Single-command build + QA:

```bash
scripts/build.sh --quick
scripts/build.sh --full
# focused functional regression matrix (admin/users/settings/chat/irc)
scripts/qa-functional.sh --with-web-e2e
```

macOS Node 25 fallback (preferred for Playwright stability):

```bash
brew install node@24
export PATH="$(brew --prefix node@24)/bin:$PATH"
scripts/run-e2e.sh
```

Explicit toolchain path override:

```bash
WOLFBBS_NODE_BIN="$(brew --prefix node@24)/bin/node" \
WOLFBBS_NPM_BIN="$(brew --prefix node@24)/bin/npm" \
scripts/run-e2e.sh
```

Optional reliability knobs:

```bash
# bound Docker smoke startup duration in verifier
COMPOSE_CMD_TIMEOUT_SECONDS=10 COMPOSE_UP_TIMEOUT_SECONDS=180 scripts/verify.sh --smoke

# allow Playwright run on non-LTS Node when explicitly needed
WOLFBBS_ALLOW_UNSUPPORTED_NODE=true scripts/run-e2e.sh --no-go --no-tui

# skip browser install only when Chromium cache is already present
WOLFBBS_SKIP_BROWSER_INSTALL=true scripts/run-e2e.sh --no-go --no-tui

# skip npm dependency install when node_modules + lock hash are unchanged
WOLFBBS_SKIP_NPM_INSTALL=true scripts/run-e2e.sh --no-go --no-tui

# lower/raise web e2e disk preflight threshold in MB (default 1200)
WOLFBBS_WEB_E2E_MIN_FREE_MB=800 scripts/run-e2e.sh --no-go --no-tui
```

Manual acceptance checklist and report:

```bash
scripts/manual-acceptance.sh --guided
```

Installer setup wizard:

- Interactive installs now focus on bootstrap `sysop` credentials.
- Site identity, setup profile, and runtime configuration are done in `/admin/setup` and `/admin/config`.
- After install, the script prints a "First Login Wizard" block with exact URLs and commands for `/admin/login`, `/admin/setup`, SSH, chat, and runtime status.
- Non-interactive installs (`--yes`) skip prompts and auto-generate bootstrap credentials, then print where to retrieve them.

Setup profiles:

- `basic`: identity + ports + bootstrap users with safe defaults.
- `critical`: includes security-critical prompts (`secure cookie`, external-email verification gate).
- `expert`: includes critical prompts plus runtime tuning prompts (`menu enable`, terminal encoding).

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
  - then run: `bash install.sh --repair`
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
