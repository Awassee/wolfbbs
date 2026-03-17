# WolfBBS Feature and Function Reference

This document maps product features to concrete runtime surfaces (binaries, routes, menu paths, and operational entrypoints).

## Runtime Binaries

- `cmd/wolfbbs`
  - SSH ANSI BBS entrypoint
  - optional Telnet/WS/WSS login servers via runtime config/env
- `cmd/wolfbbs-web`
  - web companion (boards, mail, settings, discover, gateway)
  - sysop admin panel
  - health/readiness/metrics endpoints
  - optional gopher/nntp/nntps surfaces plus experimental ActivityPub federation ingress/egress baseline
- `cmd/wolfbbs-irc`
  - IRC endpoint bridged to shared chat backend
- `cmd/wolfbbs-mailin`
  - inbound mail adapter forwarder
- `cmd/oputil`
  - sysop CLI (`status`, `users`, `boards`, `network`, `mods`)

## Default Endpoints and Ports

- SSH: `localhost:2222`
- Web: `http://localhost:8080`
- IRC: `localhost:6667`
- IRC TLS (optional): `localhost:6697`
- Mail ingestion adapter: `http://localhost:8091`

## SSH ANSI Surface

### Session Flow

1. Welcome
2. Login (supports account creation and optional `GUEST` tour)
3. Bulletins/Newscan
4. Main Menu
5. Section entry and return

### Main Menu Hotkeys

- `M` Message Boards
- `P` Private Mail
- `F` Files
- `C` Chat
- `G` Gateways
- `D` Doors
- `N` Newscan
- `S` Settings
- `A` Admin note (web sysop panel)
- `L` Last Callers
- `W` Who's Online
- `X` Config Center
- `Y` Status Center
- `/` Quick Jump (opt-in)
- `Q` Quit
- `?` Context help

### Inline Help Coverage

- Login: `?` at handle prompt opens login help.
- Main menu: `?`
- Boards: `?`
- Mail: `?`
- Chat: `?`
- Gateway: `?`
- Doors: `?`
- Settings: `?`

Theme/MCI runtime:
- Optional HJSON theme pack via `WOLFBBS_THEME_FILE`
- Optional settings MCI template via `WOLFBBS_MCI_SETTINGS_FILE`

Reference: `docs/help-guides.md`, `docs/screens.md`.

## Web Companion Surface

## Auth and session routes

- `GET/POST /login`
- `GET/POST /admin/login`
- `GET /logout`
- `GET/POST /reset/request`
- `GET/POST /reset/complete`
- `GET /help` (help hub)

### User routes (authenticated)

- `GET/POST /boards`
- `GET/POST /mail`
- `GET/POST /gateway`
- `GET/POST /settings`
- `GET /status`
- `GET /config`
- `GET /bulletins`
- `GET /directory`
- `GET/POST /feedback`
- `GET /finder`
- `GET /newfiles`
- `GET /radar`
- `GET/POST /clubhouse`
- `GET /chat`
- `POST /chat/send`
- `GET /chat/stream`
- `GET /chat/channels`
- `POST /chat/join`
- `POST /chat/leave`
- `GET /chat/history`
- `GET /chat/online`
- `GET /scores`
- `GET /discover` (when enabled)

### Moderation/sysop routes

- `POST /chat/moderation` (moderator+)
- `GET /admin` (sysop)
- `GET/POST /admin/users`
- `GET/POST /admin/boards`
- `GET/POST /admin/mail`
- `GET/POST /admin/files`
- `GET/POST /admin/gateways`
- `GET/POST /admin/chat`
- `GET/POST /admin/doors`
- `GET/POST /admin/setup`
- `GET/POST /admin/config`
- `GET /admin/errors`
- `GET /admin/system`
- `GET /admin/node-state`
- `GET /admin/audit`

### Public operations routes

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `POST /mail/inbound` (token protected)

### Optional/feature-gated routes

- `GET /connect` (modern on-ramp)
- `GET /tour` (guest tour)
- `GET /.well-known/webfinger`
- `GET /ap/users/<handle>`
- `GET /ap/users/<handle>/outbox`

## Chat and IRC Functional Surface

### Unified chat backend features

- channels and membership
- persisted history
- online presence
- moderation actions (kick/mute/ban)
- per-user and per-IP anti-flood

### IRC command coverage

Documented in `docs/irc-compat.md`:

- `PASS`, `NICK`, `USER`, `JOIN`, `PART`, `PRIVMSG`, `NOTICE`, `QUIT`
- `PING/PONG`
- `TOPIC`, `NAMES`, `LIST`, `WHO`, `WHOIS`
- numeric replies: welcome, MOTD, names, core errors

## Message Boards and Mail

- Boards list, board message index, reader, new post, reply/quote
- SSH boards include conference filter toggle (`C`) for area-focused browsing
- Mail inbox/outbox, read by ID, compose local/external (policy-gated)
- Newscan/digest integration for message/mail activity

## Message Network Baseline

- Packet spool service in `internal/network`:
  - FTN/BSO/QWK board packet export/import
  - netmail queue + import processing
  - inbound/outbound/processed spool tracking
  - board/netmail routing helpers (`WOLFBBS_NET_BOARD_ROUTES`, `WOLFBBS_NET_HANDLE_ROUTES`)
- Sysop CLI:
  - `oputil network status`
  - `oputil network export --format <ftn|bso|qwk> --board <id>`
  - `oputil network import --in <packet.json>`
  - `oputil network sync-in` / `oputil network sync-out` for external tosser hooks
  - `oputil network queue-netmail --from <uid> --to <handle> --subject <s> --body <b>`
  - `oputil network import-queue`

## File Base and Download Queue

- File tags, ratings, saved filters, SHA-256 dedupe index
- SSH files menu includes indexed search (`I`) and legacy queue manager (`D`)
- Queue manager supports remove, one-time ticket issue, and batch zip handoff via gateway

## Gateways

### Text web gateway

- fetch URL with timeout/size/content-type restrictions
- SSRF blocking by default
- ANSI/web reader output
- optional offline save

Reference: `docs/web-gateway.md`.

### Email gateway

- outbound SMTP relay from BBS/web compose
- verified-account and policy controls
- rate, recipient, and payload limits
- optional inbound ingest to mailbox

Reference: `docs/email-gateway.md`.

## Doors and Plugin Surface

- manifest-driven doors (`doors/*/door.json`)
- native and external door types
- turn bank, score, achievements, event logs
- connector adapters (DoorParty, BBSLink, Telnet bridge)
- sysop controls at `/admin/doors`

Reference: `docs/doors.md`.

## Built-in Mods

- Lifecycle manager: `internal/mods/manager.go`
- Built-ins:
  - `onelinerz`
  - `rumorz`
  - `bbslist`
  - `whos_online`
- Surfaced in web:
  - discover feed (`/discover`)
  - status center (`/status`)
  - sysop system dashboard (`/admin/system`)

## Content Servers and Federation

- Optional gopher, nntp, nntps read-only exports
- Optional experimental ActivityPub webfinger/actor/outbox plus bounded inbox ingress

Reference: `docs/content-servers.md`, `docs/activitypub.md`.

## Sysop Operations and Observability

- admin audit log (`/admin/audit`)
- WFC-style dashboard (`/admin/system`)
- health/readiness/metrics endpoints
- CLI ops (`cmd/oputil`)
- optional ACS policy gate for admin routes (`WOLFBBS_ACS_ADMIN`)

## Related Documentation Index

- `docs/INSTALL.md`
- `docs/config-reference.md`
- `docs/help-guides.md`
- `docs/screens.md`
- `docs/admin.md`
- `docs/chat.md`
- `docs/irc-compat.md`
- `docs/architecture.md`
- `docs/threat-model.md`
