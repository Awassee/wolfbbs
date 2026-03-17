# WolfBBS

WolfBBS is a self-hosted, SSH-first bulletin board system with a Wildcat-style ANSI experience, modern web companion, IRC bridge, doors, file areas, and turnkey installation.

The canonical public repo is [Awassee/wolfbbs](https://github.com/Awassee/wolfbbs).

## Why WolfBBS

WolfBBS is built for operators who want the feel of a classic board without the usual setup pain.

- `Retro caller experience`: ANSI/TUI menus, message boards, private mail, doors, newscan, and classic operator views.
- `Modern access layer`: web companion, admin console, live chat, IRC bridge, password reset, health checks, and packaging.
- `Low-friction operations`: paste-and-run bootstrap, guided installer menu, upgrade and repair commands, Docker-first deployment.
- `Community-ready`: callers, moderators, sysops, guest tours, directory, bulletins, one-liners, scores, and public-facing discovery surfaces.

## What You Get

| Capability | What it does | Why it matters |
| --- | --- | --- |
| ANSI BBS | SSH-first caller experience with message boards, files, chat, doors, newscan, and classic menus | Delivers the nostalgic interaction model people actually want |
| Web companion | Browser-based boards, mail, chat, admin, setup, status, and discovery routes | Makes the board usable for modern users and operators |
| IRC bridge | Shared channel state between web chat and IRC clients | Lets existing IRC users join the same community without a custom client |
| Sysop control center | Setup wizard, config center, audit views, diagnostics, and door/file/chat administration | Reduces day-two operational burden |
| File base and doors | Uploads, indexing, queue management, scores, trophies, and integrated games | Gives the board depth beyond message threads |
| Packaging and lifecycle | Bootstrap installer, release tarballs, repair, doctor, upgrade, uninstall | Makes the product practical to deploy and maintain |

## Best Fit

WolfBBS is a good fit if you want to:

- host a hobbyist retro board with modern onboarding
- run an internal community hub with SSH, web, and IRC access
- launch a retro-gaming or door-game focused community
- experiment with BBS-style interaction without building a stack from scratch

## Quick install (Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh | bash
```

## Quick install (macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh | bash -s -- --install-brew
```

## Install In Minutes

Interactive local flow:

```bash
git clone https://github.com/Awassee/wolfbbs.git wolfbbs
cd wolfbbs
bash install.sh
```

If you want packaged downloads instead of cloning source, use [GitHub Releases](https://github.com/Awassee/wolfbbs/releases):

```bash
tar -xzf wolfbbs_<version>_<os>_<arch>.tar.gz
cd wolfbbs_<version>_<os>_<arch>
bash install.sh --yes
```

## First Launch Checklist

After install, WolfBBS prints the connection summary and bootstrap sysop credentials. The recommended first-run flow is:

1. Open `/admin/setup` to complete identity, safety, and bootstrap checks.
2. Open `/admin/config` to tune site text, runtime flags, services, and operator preferences.
3. Connect over SSH and verify the caller-facing ANSI flow.
4. Open `/chat`, `/boards`, `/doors`, and `/scores` to confirm the public experience.
5. Create additional users or moderators from `/admin/users`.

Default local endpoints:

- SSH: `ssh localhost -p 2222`
- Web admin: `http://localhost:8080/admin`
- Web chat: `http://localhost:8080/chat`
- IRC: `localhost:6667`
- IRC TLS: `localhost:6697` when enabled

## Daily Operator Commands

```bash
bash install.sh --status
bash install.sh --doctor
bash install.sh --repair
bash install.sh --start
bash install.sh --stop
bash install.sh --restart
bash install.sh --logs
bash install.sh --upgrade
bash install.sh --rapid-upgrade
bash install.sh --uninstall --purge --yes
```

Optional in-BBS upgrade hook:

```bash
export WOLFBBS_APP_UPGRADE_COMMAND="bash install.sh --rapid-upgrade --yes"
export WOLFBBS_APP_UPGRADE_WORKDIR="/path/to/wolfbbs"
```

Then in the SSH main menu press `/` and enter `/app upgrade`.

## Product Guides

- [Quickstart](docs/QUICKSTART.md): fastest path from download to first login
- [Install Guide](docs/INSTALL.md): install modes, flags, lifecycle, and packaging
- [Product Guide](docs/PRODUCT_GUIDE.md): what WolfBBS includes and how to use it
- [Datasheet](docs/DATASHEET.md): deployment summary, capabilities, ports, and operator facts
- [Feature Reference](docs/feature-reference.md): route, binary, and surface inventory

## Core Product Areas

- `Callers`: ANSI login, guest tour, boards, private mail, bulletins, who’s online, last callers, files, doors, chat
- `Community`: IRC bridge, one-liners, clubhouse, directory, discovery queue, scoreboards, file picks
- `Operators`: admin setup wizard, config center, users, boards, files, doors, gateways, audit, health, diagnostics
- `Distribution`: release bundles, bootstrap installer, upgrade flows, smoke verification, packaging checksums

## Build, Test, And Package

```bash
go test ./...
go build ./...
scripts/verify.sh --fast
scripts/qa-functional.sh
scripts/run-e2e.sh
scripts/package-dist.sh
```

Warm-environment shortcuts:

```bash
WOLFBBS_SKIP_NPM_INSTALL=true WOLFBBS_SKIP_BROWSER_INSTALL=true scripts/run-e2e.sh --no-go --no-tui
WOLFBBS_WEB_E2E_MIN_FREE_MB=800 scripts/run-e2e.sh --no-go --no-tui
```

One-command QA runner:

```bash
scripts/build.sh --quick
scripts/build.sh --full
```

If local Node is 25+, use Node 24 on macOS:

```bash
brew install node@24
export PATH="$(brew --prefix node@24)/bin:$PATH"
scripts/run-e2e.sh
```

## Manual Acceptance

```bash
scripts/manual-acceptance.sh --guided
```

This writes `docs/manual-acceptance-latest.md` with PASS, FAIL, and SKIPPED results by manual spec ID.

## Technical Documentation

- `docs/ACCEPTANCE_SPEC.md`
- `docs/manual-acceptance.md`
- `docs/screens.md`
- `docs/admin.md`
- `docs/chat.md`
- `docs/irc-compat.md`
- `docs/doors.md`
- `docs/message-network.md`
- `docs/mods.md`
