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

## Choose Your Path

| If you want to... | Use this | Why |
| --- | --- | --- |
| get WolfBBS running as fast as possible | bootstrap installer | installs dependencies, pulls the app, and starts the stack |
| inspect or modify the code locally | clone the repo | best for operators who also want a working tree |
| download a packaged bundle | GitHub Release tarball | best for controlled installs and offline handoff |

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

## Which Surface Should You Use?

| Role | Best starting point | What it is for |
| --- | --- | --- |
| sysop | `/admin/setup` | identity, safety baseline, bootstrap actions, first health checks |
| moderator | `/chat`, `/boards`, `/admin/chat` | live moderation and day-to-day community visibility |
| caller | SSH, `/boards`, `/chat`, `/doors` | the actual board experience |
| visitor | `/connect`, `/tour`, `/help` | orientation before committing to an account |

## First Launch Checklist

After install, WolfBBS prints the connection summary and bootstrap sysop credentials. The recommended first-run flow is:

1. Open `/admin/setup` to complete identity, safety, and bootstrap checks.
2. Open `/admin/config` to tune site text, runtime flags, services, and operator preferences.
3. Seed default boards and confirm the mailbot bootstrap action.
4. Create at least one non-sysop user or moderator from `/admin/users`.
5. Connect over SSH and verify the caller-facing ANSI flow.
6. Open `/boards`, `/chat`, `/doors`, and `/scores` to confirm the public experience.

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

- [Start Here](docs/START_HERE.md): fastest route from install to a usable board
- [Quickstart](docs/QUICKSTART.md): fastest path from download to first login
- [Install Guide](docs/INSTALL.md): install modes, flags, lifecycle, and packaging
- [Operations Guide](docs/OPERATIONS.md): daily operator tasks, recovery, and upgrades
- [Product Guide](docs/PRODUCT_GUIDE.md): what WolfBBS includes and how to use it
- [Datasheet](docs/DATASHEET.md): deployment summary, capabilities, ports, and operator facts
- [Feature Reference](docs/feature-reference.md): route, binary, and surface inventory

## Core Product Areas

- `Callers`: ANSI login, guest tour, boards, private mail, bulletins, who’s online, last callers, files, doors, chat
- `Community`: IRC bridge, one-liners, clubhouse, directory, discovery queue, scoreboards, file picks
- `Operators`: admin setup wizard, config center, users, boards, files, doors, gateways, audit, health, diagnostics
- `Distribution`: release bundles, bootstrap installer, upgrade flows, smoke verification, packaging checksums

## What Success Looks Like In 15 Minutes

You should be able to say yes to all of these:

1. I can sign into `/admin`.
2. `/admin/setup` and `/admin/config` reflect my board name and host.
3. SSH login works and the ANSI menu feels right.
4. `/chat` works and mirrors to IRC if IRC is enabled.
5. `/boards` has seeded or starter content.
6. `/doors` and `/scores` render without dead ends.
7. `bash install.sh --doctor` returns a usable health report.

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

- `docs/START_HERE.md`
- `docs/ACCEPTANCE_SPEC.md`
- `docs/OPERATIONS.md`
- `docs/manual-acceptance.md`
- `docs/screens.md`
- `docs/admin.md`
- `docs/chat.md`
- `docs/irc-compat.md`
- `docs/doors.md`
- `docs/message-network.md`
- `docs/mods.md`
