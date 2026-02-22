# WolfBBS

WolfBBS is an SSH-first BBS with a Wildcat-inspired ANSI/TUI flow and companion web/IRC/chat scaffolding.

## Stack
- Go
- SSH (gliderlabs/ssh)
- Read-only web companion + admin surfaces (in-memory MVP)
- Shared in-memory chat core used by web and IRC stubs

## Quickstart

```bash
# Start SSH, web, IRC, and postgres
	docker-compose up --build
```

- SSH: `ssh localhost -p 2222`
- Web read-only companion: `http://localhost:8080/login`
- IRC endpoint: connect with any IRC client to `localhost:6667`

## Local run

```bash
go run ./cmd/wolfbbs -listen :2222
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
- Web read-only companion
  - Login
  - Message boards list
  - Private mail list
  - Settings and admin landing pages
  - Admin user actions at `/admin/users` (read/write unless read-only mode)
- Shared chat core
  - `internal/chat`
- Text web gateway at `/gateway` with SSRF deny rules and offline saves
- Doors option in main menu (`/wolfbbs-trivia` sample)
- IRC compatibility stub
  - `cmd/wolfbbs-irc`

### Environment Variables
- `WOLFBBS_OFFLINE_DIR` : path for gateway offline cache (default `.wolfbbs/offline`)
- `WOLFBBS_TRIVIA_BINARY` : path to trivia door binary (default `wolfbbs-trivia`)
- `WOLFBBS_READ_ONLY` : set to `1` or `true` to block admin mutating actions
- `WOLFBBS_DATABASE_URL` : PostgreSQL DSN, e.g. `postgres://wolfbbs:wolfbbs@postgres:5432/wolfbbs?sslmode=disable`
- `PGHOST`/`PGPORT`/`PGUSER`/`PGPASSWORD`/`PGDATABASE` can also be used when URL is not set
- When a PostgreSQL DSN is configured, `cmd/wolfbbs` and `cmd/wolfbbs-web` open shared DB repos and apply startup migrations from `migrations/0001_init.sql` if needed.

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
- [ ] Implement full board/mail flows in SSH and web
- [ ] Full doors plugin system
- [ ] Message/chat moderation in admin tools
- [ ] Shared persistence and moderation-aware chat across SSH/Web/IRC
- [ ] Test gateway implementations for file/web/email paths
