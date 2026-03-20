# Install WolfBBS

`install.sh` is the primary installation path and supports local checkout usage and curl-pipe bootstrap.

If you want the shortest route, start with [QUICKSTART.md](QUICKSTART.md). If you want the product overview first, read [PRODUCT_GUIDE.md](PRODUCT_GUIDE.md) and [DATASHEET.md](DATASHEET.md).

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

## Recommended Consumer Path

For most users, the right path is:

1. run the bootstrap command
2. let the guided installer complete
3. finish setup in `/admin/setup`
4. tune behavior in `/admin/config`
5. connect over SSH and web to verify the caller experience

## What The Installer Handles For You

The normal installer path is intentionally consumer-oriented. It:

- detects the host platform
- installs supported prerequisites when needed
- prepares Docker runtime support
- creates a managed runtime layout under the install prefix
- writes the `.env`
- starts services
- prints exact first-login and verification steps

You should not need to download dependencies manually or remember a long flag list for a normal install.

## Quick Install (turnkey paste-and-go)

Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh | bash
```

macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh | bash -s -- --install-brew
```

Interactive menu mode (no flags):

```bash
git clone https://github.com/Awassee/wolfbbs.git wolfbbs && cd wolfbbs && bash install.sh
```

The guided menu path is the recommended consumer install flow. It opens the Installer Command Center first, then a guided install plan where you can:

- accept recommended defaults
- tune install directory, ports, and source repo
- open advanced install options (profile/name/hostname/toggles)
- open Troubleshooting Center (doctor/status/logs/port audit/debug bundle/repair)

What the bootstrap does:

1. Downloads the current `install.sh` to a temp location.
2. Runs the installer with any flags you pass after `bash -s --`.
3. Installs base dependencies and Docker runtime when supported.
4. Fetches or updates a managed WolfBBS checkout under `<prefix>/app`.
   - uses `git` when it is available
   - falls back to a GitHub archive download when `git` is not installed
5. Writes runtime config into `<prefix>/.env`.
6. Starts the stack and prints first-login steps.

What you have at the end:

- running WolfBBS services
- a bootstrap sysop account
- printed connection URLs and ports
- a managed install prefix with runtime config and upgrade commands
- `<prefix>/FIRST_STEPS.txt` with the exact first-login flow
- `<prefix>/SERVICE_STATUS.txt` after `bash install.sh --status`

## Files That Matter After Install

- `<prefix>/.env`: runtime configuration and bootstrap sysop credentials
- `<prefix>/FIRST_STEPS.txt`: exact launch workflow and URLs
- `<prefix>/SERVICE_STATUS.txt`: last status snapshot from `bash install.sh --status`
- `<prefix>/app/`: managed WolfBBS checkout when using the standalone installer path
- `<prefix>/install.log`: installer log

## What To Expect On First Login

The installer prints a first-login block. Use it in this order:

1. sign in to `/admin`
2. open `/admin/launch`
3. finish `/admin/setup`
4. review `/admin/config`
5. create at least one non-sysop user
6. schedule one event in `/admin/events` and confirm `/events` + `/events/recaps`
7. define one active season in `/admin/challenges` and confirm `/challenges` + `/clubhouse`
8. review `/admin/upgrade-safety` and `/admin/backups`
9. verify SSH, web chat, boards, and doors
10. run `bash install.sh --status`
11. if anything feels wrong, run `bash install.sh --doctor`

## Install From GitHub Release Bundle

If you prefer a packaged download instead of cloning the repo, download the archive matching your platform from:

- [GitHub Releases](https://github.com/Awassee/wolfbbs/releases)

Then unpack and run the installer from the bundle root:

```bash
tar -xzf wolfbbs_<version>_<os>_<arch>.tar.gz
cd wolfbbs_<version>_<os>_<arch>
bash install.sh --yes
```

If HTTPS clone is blocked in your environment, use SSH clone instead:

```bash
git clone git@github.com:Awassee/wolfbbs.git wolfbbs && cd wolfbbs && bash install.sh --yes
```

Build clean distro artifacts for `v1.1.24`:

```bash
scripts/package-dist.sh --clean --version v1.1.24 \
  --platform linux/amd64 --platform linux/arm64 \
  --platform darwin/amd64 --platform darwin/arm64
```

## Optional Quick Install (`curl | bash`)

Use this only when the repo's raw GitHub URL is publicly reachable:

```bash
curl -fsSL "https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh" | bash
```

macOS variant:

```bash
curl -fsSL "https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh" | bash -s -- --install-brew
```

Dry-run preflight:

```bash
curl -fsSL "https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh" | bash -s -- --yes --install-brew --dry-run
```

## Fastest Recovery Order

When the product is up but not trustworthy, use this order:

```bash
bash install.sh --status
bash install.sh --doctor
bash install.sh --port-audit
bash install.sh --debug-bundle
bash install.sh --repair
bash install.sh --logs
```

Then compare the result to [LAUNCH_CHECKLIST.md](LAUNCH_CHECKLIST.md) and [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

Install from a fork/custom repository:

```bash
curl -fsSL "https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh" | bash -s -- --yes --repo your-org/your-repo
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

## First 10 Minutes After Install

Use this sequence:

1. sign in as the bootstrap sysop
2. finish `/admin/setup`
3. review `/admin/config`
4. open `/admin/events` and add one event, then verify `/events` and `/events/recaps`
5. open `/admin/challenges` and define one season + one goal, then verify `/challenges` and `/clubhouse`
6. open `/admin/upgrade-safety` and `/admin/backups`
7. open `/boards`, `/chat`, `/doors`, and `/scores`
8. connect via SSH and verify the ANSI menus
9. run `bash install.sh --doctor`

## Which Command Should I Run?

| Situation | Command |
| --- | --- |
| first install from a clone | `bash install.sh` |
| first install without cloning first | bootstrap command from `README.md` |
| show endpoints and current state | `bash install.sh --status` |
| check health without changing anything | `bash install.sh --doctor` |
| audit listener ownership on core ports | `bash install.sh --port-audit` |
| capture a support-ready diagnostics file | `bash install.sh --debug-bundle` |
| repair a broken install | `bash install.sh --repair` |
| pull latest shipped images | `bash install.sh --upgrade` |
| rebuild local source changes quickly | `bash install.sh --rapid-upgrade` |
| uninstall services but keep files | `bash install.sh --uninstall --purge --yes` |
| fully reset install for retesting | `bash install.sh --clean-uninstall --yes` |

## Release v1.1.24 Surface Set

These operator surfaces are part of the `v1.1.24` distro baseline:

- `/admin/events`: schedule events and post event recaps.
- `/events/recaps`: public recap feed with attendance outcomes.
- `/admin/challenges`: configure seasonal scoring and shared goals.
- `/challenges`: caller-facing seasonal leaderboard.
- `/admin/upgrade-safety`: pre-upgrade risk/trust dashboard.
- `/admin/backups`: backup artifact browser with validation states.
- `/admin/release`: release cockpit linking roadmap, QA, docs, and artifacts.

## What the Installer Does

1. Detects OS, architecture, package manager.
2. Ensures required tools are present (`curl`, `tar`, `openssl`, `sed`, `awk`, `grep`, `nc`).
3. Installs missing base dependencies automatically when possible.
4. Ensures Docker runtime is available.
   - Linux: installs Docker Engine + compose plugin when needed.
   - macOS: supports Docker Desktop or Colima; with `--yes --install-brew`, can bootstrap Colima stack.
5. Resolves compose file.
   - Uses local repo if present.
   - If missing and `--repo`/`--repo-url` set, fetches/updates into `--prefix`.
   - GitHub repos can be downloaded without `git` by using archive fallback.
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
- `--port-audit`: inspect configured ports (SSH/Web/IRC/Mail) and show listener ownership
- `--debug-bundle`: write a deep diagnostics report to `<prefix>/WOLFBBS_DIAGNOSTICS_<timestamp>.txt`
- `--start`: start existing WolfBBS services
- `--stop`: stop existing WolfBBS services
- `--restart`: restart existing WolfBBS services
- `--logs`: show recent service logs
- `--repair`: self-heal install (ensure deps/env, rebuild, verify)
- `--deps-only`: install/check prerequisites and Docker runtime only
- `--upgrade`: pull/rebuild/restart stack in existing install
- `--rapid-upgrade`: rebuild/restart from local source (no image pull) for fast iteration
- `--uninstall`: stop services and optionally remove data
- `--clean-uninstall`: stop services, purge volumes, and remove install directory (git checkout requires `--force`)
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
- `WOLFBBS_INSTALL_PREFIX`, `WOLFBBS_INSTALL_WORKDIR`, `WOLFBBS_DOCKER_SOCKET`
- `WOLFBBS_APP_UPGRADE_COMMAND`, `WOLFBBS_APP_UPGRADE_WORKDIR`, `WOLFBBS_APP_UPGRADE_TIMEOUT_SECONDS`

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

Current product-facing docs included in release bundles:

- `docs/START_HERE.md`
- `docs/QUICKSTART.md`
- `docs/INSTALL.md`
- `docs/OPERATIONS.md`
- `docs/PRODUCT_GUIDE.md`
- `docs/DATASHEET.md`
- `docs/feature-reference.md`

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

In-BBS quick upgrade (sysop):

```bash
bash install.sh --repair
```

`--repair` backfills the in-app upgrade env wiring on older installs. Then in SSH main menu press `/` and enter `/app upgrade`.

Uninstall (interactive):

```bash
bash install.sh --uninstall
```

Uninstall + purge (non-interactive):

```bash
bash install.sh --uninstall --purge --yes
```

Clean uninstall (non-interactive, removes install directory):

```bash
bash install.sh --clean-uninstall --yes
```

Note: if install prefix is a git checkout, directory deletion is blocked unless you also pass `--force`.

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
