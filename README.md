# WolfBBS

<<<<<<< ours
WolfBBS is an SSH-first BBS with a Wildcat-inspired ANSI/TUI flow, web admin/chat surfaces, and IRC bridge support.

## Quick install (Linux)

```bash
git clone https://github.com/seanheiney/wolfbbs.git wolfbbs && cd wolfbbs && bash install.sh --yes
```

## Quick install (macOS)

```bash
git clone https://github.com/seanheiney/wolfbbs.git wolfbbs && cd wolfbbs && bash install.sh --yes --install-brew
```

## Easiest local flow (interactive menu)

```bash
git clone https://github.com/seanheiney/wolfbbs.git wolfbbs && cd wolfbbs && bash install.sh
```

Optional `curl|bash` (requires public raw URL access):

```bash
curl -fsSL "https://raw.githubusercontent.com/seanheiney/wolfbbs/main/install.sh" | bash -s -- --yes
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
=======
WolfBBS is a modern, SSH-first BBS inspired by the classic Wildcat-era ANSI experience.

## Current Status

This commit delivers **Step 1 + Step 2**:
- architecture + data model + screen flow docs
- runnable SSH server skeleton
- login/create-account flow
- initial main menu screen scaffold
- basic Message Boards (list/read/post)
- basic Private Mail (send/inbox/outbox)

## Stack Choice

- **Go** for maintainability, fast SSH session handling, and pragmatic terminal UI control.

## Quickstart

```bash
docker-compose up --build
```

Then connect:

```bash
ssh localhost -p 2222
```

## Local Run (without Docker)

```bash
go run ./cmd/wolfbbs -listen :2222
```

## Project Layout

- `cmd/wolfbbs` - server entrypoint
- `internal/app` - application wiring/lifecycle
- `internal/sshserver` - SSH session handling
- `internal/ui` - ANSI rendering primitives/screens
- `internal/auth` - account + auth logic
- `internal/domain` - core entities
- `internal/repository` - storage abstraction (in-memory MVP)
- `docs` - architecture, screen map, threat model

## Definition of Done (MVP)

- [ ] SSH PTY server with robust key handling and resize support
- [ ] Full ANSI screen framework (windows/forms/popups/status bars)
- [ ] User accounts with secure reset flow and prefs
- [ ] Message boards create/read/reply/new-scan (create/read now in place)
- [ ] Private mail inbox/outbox/reply/forward (inbox/outbox/send now in place)
- [ ] Admin moderation + audit logs
- [ ] Email gateway with anti-abuse controls
- [ ] Text web gateway with SSRF protections + offline save
- [ ] Postgres repository implementation
- [ ] Unit + integration tests for critical flows
- [ ] Dockerized local deployment docs complete
>>>>>>> theirs
