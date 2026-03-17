# WolfBBS

WolfBBS is an SSH-first BBS with a Wildcat-inspired ANSI/TUI flow, web admin/chat surfaces, and IRC bridge support.

## Quick install (Linux)

```bash
git clone https://github.com/seanheiney/wolfbbs-public.git wolfbbs && cd wolfbbs && bash install.sh --yes
```

## Quick install (macOS)

```bash
git clone https://github.com/seanheiney/wolfbbs-public.git wolfbbs && cd wolfbbs && bash install.sh --yes --install-brew
```

## Easiest local flow (interactive menu)

```bash
git clone https://github.com/seanheiney/wolfbbs-public.git wolfbbs && cd wolfbbs && bash install.sh
```

The canonical public repo is:

- [seanheiney/wolfbbs-public](https://github.com/seanheiney/wolfbbs-public)

## GitHub release bundles

If you want a packaged download instead of cloning source, use the platform tarballs on the GitHub Releases page:

- [GitHub Releases](https://github.com/seanheiney/wolfbbs-public/releases)

After downloading the matching archive for your platform:

```bash
tar -xzf wolfbbs_<version>_<os>_<arch>.tar.gz
cd wolfbbs_<version>_<os>_<arch>
bash install.sh --yes
```

Optional `curl|bash` (requires public raw URL access):

```bash
curl -fsSL "https://raw.githubusercontent.com/seanheiney/wolfbbs-public/main/install.sh" | bash -s -- --yes
```

Easy-button self-heal / manage commands:

```bash
bash install.sh --doctor      # non-mutating diagnostics
bash install.sh --repair      # ensure deps/env, rebuild, verify
bash install.sh --start       # start services
bash install.sh --stop        # stop services
bash install.sh --restart     # restart services
bash install.sh --logs        # tail recent logs
bash install.sh --deps-only   # only install/check prerequisites
bash install.sh --upgrade     # pull latest images and restart
bash install.sh --rapid-upgrade # rebuild/restart local code changes
bash install.sh --uninstall --purge --yes # clean uninstall for test cycles
# optional: in SSH main menu use / then "/app upgrade" after setting WOLFBBS_APP_UPGRADE_COMMAND
```

## Install from local clone

```bash
bash install.sh --with-docker
```

After install, do all board setup/config in the UI:
- `/admin/setup` for basic/critical/expert setup profile
- `/admin/config` for identity, text, safety, and runtime flags

## Preflight doctor (no changes)

```bash
bash install.sh --doctor
```

## Connect

The installer prints connect commands using your configured `WOLFBBS_HOSTNAME`.

- SSH: `ssh localhost -p 2222`
- Web Admin: `http://localhost:8080/admin`
- Web Chat: `http://localhost:8080/chat`
- IRC: `localhost:6667` (TLS: `localhost:6697` if configured)

## Run locally

```bash
docker compose up -d --build
```

## Validate

```bash
go test ./...
go build ./...
scripts/verify.sh --fast
scripts/qa-functional.sh          # targeted admin/settings/chat/irc functional regression suite
scripts/run-e2e.sh
```

If disk is tight or browsers are already installed:

```bash
# skip npm/browser setup when already warm
WOLFBBS_SKIP_NPM_INSTALL=true WOLFBBS_SKIP_BROWSER_INSTALL=true scripts/run-e2e.sh --no-go --no-tui

# or run web checks with a larger preflight threshold override (MB)
WOLFBBS_WEB_E2E_MIN_FREE_MB=800 scripts/run-e2e.sh --no-go --no-tui
```

One-command build/QA runner:

```bash
scripts/build.sh --quick   # fast local check
scripts/build.sh --full    # full smoke + e2e
```

Distribution bundle builder:

```bash
scripts/package-dist.sh
scripts/package-dist.sh --platform linux/amd64 --platform linux/arm64
```

This writes versioned tarballs plus checksums under `dist/`.

If local Node is 25+, install/use Node 24 on macOS:

```bash
brew install node@24
export PATH="$(brew --prefix node@24)/bin:$PATH"
scripts/run-e2e.sh
```

## Manual acceptance checklist

```bash
scripts/manual-acceptance.sh --guided
```

This writes `docs/manual-acceptance-latest.md` with PASS/FAIL/SKIPPED per MANUAL spec ID.

## Documentation

- `/docs/QUICKSTART.md`
- `/docs/INSTALL.md`
- `/docs/ACCEPTANCE_SPEC.md`
- `/docs/manual-acceptance.md`
- `/docs/screens.md`
- `/docs/admin.md`
- `/docs/chat.md`
- `/docs/irc-compat.md`
- `/docs/doors.md`
- `/docs/message-network.md`
- `/docs/mods.md`
