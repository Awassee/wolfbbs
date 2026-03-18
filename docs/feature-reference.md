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

- `GET /today`
- `GET/POST /first-call`
- `GET/POST /boards`
- `GET/POST /mail`
- `GET/POST /bookmarks`
- `GET/POST /gateway`
- `GET/POST /settings`
- `GET /profile/export`
- `GET/POST /circles`
- `GET /status`
- `GET /config`
- `GET /bulletins`
- `GET /directory`
- `GET /attention`
- `GET /attention/export`
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
- `GET /handles/suggest`
- `GET /scores`
- `GET /discover` (when enabled)
- `GET /events`

### Moderation/sysop routes

- `POST /chat/moderation` (moderator+)
- `GET /admin` (sysop)
- `GET/POST /admin/ops`
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
- `GET/POST /admin/events`
- `GET/POST /admin/bulletins`

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
- Board reader supports thread lifecycle states: `active`, `slow`, `archived`, `frozen`
- Board reader supports structured poll threads with per-caller voting
- Board reader supports post editing plus revision history for authors and moderators
- Board detail includes welcome kits, seed prompts, starter-thread guidance, and topic stewards
- Caller-managed board subscription tiers in web boards view:
  - `watch` for high-priority tracking
  - `digest` for lower-noise daily brief inclusion
  - `mute` to suppress board resurfacing in focused views
- Attention Center at `/attention` with per-user read/unread and dismiss/restore state
- Attention Center also supports per-item snooze/restore so callers can defer follow-up without losing it
- Attention Center also surfaces live caller pages with mail handoff links
- Attention state is exportable as per-caller JSON via `/attention/export`
- Read-Later Queue at `/bookmarks` for persistent saved board posts and mail
- Daily brief at `/today` for watch-tier boards, digest-tier boards, upcoming events, and direct follow-up
- Daily brief also includes a 14-day caller activity heatmap for posts, mail, and completed calls
- Daily Digest at `/digest` with per-user opt-in preferences from `/settings`
- Digest preferences now include per-route cadence controls for attention, bulletin wire, and event reminders
- Daily Digest now groups digest-tier board packs by conference
- `/settings` includes role-based attention presets for guest-style, caller, moderator, and sysop rule packs
- `/settings` includes caller-managed profile card controls for status line, bio, and verified contact preferences
- `/profile/export` downloads per-caller JSON for profile card, aliases, circles, digest rules, and home route
- Weekly digest can be delivered by internal private mail from the `mailbot` service account
- Guided onboarding at `/first-call` to create a first post, lobby line, private mail, and saved home route
- Guest `/start` and caller `/today` surfaces now carry persistent quick-start checklists
- `/settings` includes saved home-route preference for `/today`, `/digest`, `/boards`, `/chat`, or `/doors`
- Private mail supports urgency levels (`normal`, `urgent`, `low`) rendered in inbox, outbox, and message reader views
- SSH boards include conference filter toggle (`C`) for area-focused browsing
- Board detail views support per-board quiet hours so lower-priority boards drop out of attention loops during configured windows
- Mail inbox/outbox, read by ID, compose local/external (policy-gated)
- Mail compose adds presence-aware handoff guidance when the local recipient is online now
- `/admin/mail` includes a shared moderator inbox with assignment state for staff-facing mail
- `/admin/mail` includes internal mail-merge delivery for targeted sysop outreach with `{{handle}}` personalization
- Newscan/digest integration for message/mail activity
- Web composer supports preview, focus mode, fullscreen mode, quote context, signature insert, draft restore, and keyboard shortcuts
- Web composer and chat input now include handle assist for mentions and local-recipient lookup

## Community Calendar

- Public calendar at `/events`
- Tournament Center at `/tournaments` for competitive door nights, standings, and bracket preview
- Sysop event scheduling at `/admin/events`
- Event metadata: category, start/end, location, host, audience, link, description
- Event recurrence: daily, weekly, and monthly series with repeat-until support
- Caller RSVP state is captured per event occurrence and can be cleared or updated from `/events`
- Caller profiles can send presence-aware event invites that create a `maybe` RSVP and optional live page
- Today brief integration so scheduled events appear in the daily caller loop

## Bulletins, Directory, and Caller Presence

- Public Bulletins Center at `/bulletins`
- Sysop Bulletin Scheduler at `/admin/bulletins`
- Timed announcements publish from the scheduler into the bulletins wire and caller-facing bulletin cards
- Timed announcements support caller acknowledgement tracking, with aggregate ack counts visible in admin bulletins
- Bulletin Center includes a cross-board “Best of Week” editorial lane
- Caller Directory at `/directory` includes live node, idle time, origin, and verified/role filtering
- Caller profiles support status line, bio card, verified contact preferences, per-contact aliases, circles, and exportable privacy state
- Favorite Callers with quick actions and dedicated filter lane
- Recurring Correspondents surface on caller profiles to show repeat communication patterns
- Relationship Timeline surfaces repeat mail history between the viewer and a profile target
- Moderator/sysop views also show a shared incident timeline for prior escalations and staff notes
- Live paging from caller profiles into the recipient's Attention Center
- Moderator/sysop staff notes on caller profiles for continuity across support and moderation work
- Moderator staff notes can be escalated into the shared Ops Center queue for follow-through

## Operator Triage and Ops

- Ops Center at `/admin/ops` now surfaces unresolved pages and staff escalations alongside errors, audits, and live sessions
- Ops Center includes explicit resolve actions for live-page backlog and staff follow-through items
- `/admin/boards` includes moderation macros, per-thread lifecycle controls, board welcome-kit editing, board staff notes, topic stewards, best-of-week curation, and legacy archive import

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
- Browser-native upload intake in `/admin/files` with metadata extraction and moderation hold queue
- Caller-facing FileBase preview pages show descriptions, tags, related uploads, and direct queue/ticket actions
- Held uploads stay out of caller-facing file surfaces until approved
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
- action-oriented Ops Center at `/admin/ops`
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
