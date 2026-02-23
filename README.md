# WolfBBS

WolfBBS is an SSH-first BBS with a Wildcat-inspired ANSI/TUI flow and companion web/IRC/chat scaffolding.

## Stack
- Go
- SSH (gliderlabs/ssh)
- Web companion + admin surfaces
- Shared chat core with optional Postgres persistence

## Installation

### Quick install

```bash
curl -fsSL https://raw.githubusercontent.com/<owner>/<repo>/main/install.sh | bash -s -- --with-docker --repo-url https://github.com/<owner>/<repo>.git
```

Replace `<owner>/<repo>` with the real Git repository location you host WolfBBS from.

See [`docs/INSTALL.md`](docs/INSTALL.md) for full installer options, supported OS notes, and troubleshooting.

### Install from local clone

```bash
cd /path/to/repo
bash install.sh --with-docker
```

### Quick connect

- SSH: `ssh localhost -p 2222`
- Web companion: `http://localhost:8080/login`
- Web admin: `http://localhost:8080/admin`
- Chat: `http://localhost:8080/chat`
- IRC: connect with any IRC client to `localhost:6667`
- Mail ingest webhook: `http://localhost:8091/ingest`

## Local run

```bash
go run ./cmd/wolfbbs -listen :2222
```

```bash
docker-compose up --build
```

Compose smoke check:

```bash
bash scripts/integration-smoke.sh
```

## Current Runtime Surface

- BBS UI skeleton
  - Welcome screen
  - Login/New User flow
  - Main menu
  - Last Callers and Who's Online screens
  - Small ANSI screen router and input helper
- Screen language
  - `internal/ui` contains ANSI renderer helpers and classic mockups in `docs/screens.md`
- Web companion
  - Login
  - Message boards (list/read/post/reply)
  - Private mail (inbox/outbox/read/compose)
  - Optional external email send via SMTP relay
  - Optional inbound mail ingestion endpoint
  - Settings and admin landing pages
  - Admin user actions at `/admin/users` (read/write unless read-only mode)
- Admin DB-backed panels
  - `/admin/boards`, `/admin/mail`, `/admin/files`, `/admin/gateways`, `/admin/audit`
- Shared chat core
  - `internal/chat` (shared in web/IRC, DB-backed when configured)
  - moderation actions, moderation state, and flood/rate events persisted in DB mode
- Text web gateway at `/gateway` with SSRF deny rules and offline saves
- Doors option in main menu with env-based registration (`/wolfbbs-trivia` sample)
- IRC compatibility endpoint
  - `cmd/wolfbbs-irc`

### Environment Variables
- `WOLFBBS_OFFLINE_DIR` : path for gateway offline cache (default `.wolfbbs/offline`)
- `WOLFBBS_TRIVIA_BINARY` : path to trivia door binary (default `wolfbbs-trivia`)
- `WOLFBBS_DOORS` : semicolon-separated door config entries (`HOTKEY|NAME|COMMAND|arg1,arg2`)
- `WOLFBBS_DOOR_ALLOW_DIR` : optional allow-list path for door binaries
- `WOLFBBS_READ_ONLY` : set to `1` or `true` to block admin mutating actions
- `WOLFBBS_INBOUND_TOKEN` : shared secret for `POST /mail/inbound`
- `WOLFBBS_DATABASE_URL` : PostgreSQL DSN, e.g. `postgres://wolfbbs:wolfbbs@postgres:5432/wolfbbs?sslmode=disable`
- `PGHOST`/`PGPORT`/`PGUSER`/`PGPASSWORD`/`PGDATABASE` can also be used when URL is not set
- When a PostgreSQL DSN is configured, `cmd/wolfbbs` and `cmd/wolfbbs-web` open shared DB repos and apply startup migrations from `migrations/0001_init.sql` if needed.
- PostgreSQL startup defaults are tuned for container startup:
  - `WOLFBBS_DB_CONNECT_RETRIES` (default `10`)
  - `WOLFBBS_DB_CONNECT_DELAY_MS` (default `1000`)

## Docs Added
- `docs/screens.md` screen language and mockups
- `docs/admin.md` admin capabilities and controls
- `docs/chat.md` shared chat architecture
- `docs/irc-compat.md` IRC supported command set / limits
- `docs/web-gateway.md` web gateway safety and pager flow
- `docs/email-gateway.md` email gateway controls and abuse protections

## Definition of Done (MVP)

- [x] PTY-required SSH entry and basic routed screens
- [x] Secure password hashing via bcrypt for local auth store
- [x] Welcome -> Login -> Main menu flow
- [x] Replace in-memory repositories with Postgres adapter when database config is present
- [x] Implement board/mail flows in SSH and web
- [x] Doors registry with env-based plugin registration and optional allow-list directory
- [x] Message/chat moderation controls with persisted state in DB mode
- [x] Shared persistence and moderation-aware chat across SSH/Web/IRC
- [ ] Complete end-to-end production hardening for email/file gateway pipelines
