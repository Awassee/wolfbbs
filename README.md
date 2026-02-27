# WolfBBS

WolfBBS is an SSH-first BBS with a Wildcat-inspired ANSI/TUI flow, web admin/chat surfaces, and IRC bridge support.

## Quick install (Linux)

```bash
git clone https://github.com/seanheiney/New-project.git wolfbbs && cd wolfbbs && bash install.sh --yes
```

## Quick install (macOS)

```bash
git clone https://github.com/seanheiney/New-project.git wolfbbs && cd wolfbbs && bash install.sh --yes --install-brew
```

Optional `curl|bash` (requires public raw URL access):

```bash
curl -fsSL "https://raw.githubusercontent.com/seanheiney/New-project/main/install.sh" | bash -s -- --yes
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
```

## Install from local clone

```bash
bash install.sh --with-docker
```

## Preflight doctor (no changes)

```bash
bash install.sh --doctor
```

## Connect

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
scripts/run-e2e.sh
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
