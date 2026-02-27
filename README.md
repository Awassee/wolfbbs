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

## Install from local clone

```bash
bash install.sh --with-docker
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
```

## Documentation

- `/docs/INSTALL.md`
- `/docs/ACCEPTANCE_SPEC.md`
- `/docs/screens.md`
- `/docs/admin.md`
- `/docs/chat.md`
- `/docs/irc-compat.md`
- `/docs/doors.md`
