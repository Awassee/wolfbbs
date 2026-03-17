# WolfBBS Architecture

## Stack Choice

Go was selected for this implementation because it gives fast, concurrency-friendly SSH handling and direct terminal control while keeping an opinionated yet practical admin and service surface.

## Components

- `cmd/wolfbbs`: SSH + ANSI session entrypoint plus optional telnet/websocket login transports.
- `cmd/wolfbbs-web`: web companion + admin shell.
- `cmd/wolfbbs-irc`: IRC endpoint implementation with shared chat integration.
- `cmd/wolfbbs-mailin`: inbound email webhook adapter that forwards into web inbox ingestion.
- optional content servers in web process:
  - Gopher (read-only board/message browsing)
  - NNTP/NNTPS (read-only board/message browsing)
  - ActivityPub webfinger/actor/outbox plus bounded inbox ingress (experimental, opt-in)
- `internal/ui`: ANSI rendering primitives and screen templates.
- `internal/term`: terminal capability/encoding profile detection.
- `internal/acs`: access control expression evaluator for menus and content actions.
- `internal/mci`: MCI-style view/control schema and rendering helpers.
- `internal/discovery`: transparent digest/newscan and discover feed builders.
- `internal/menu`: HJSON-driven menu schema loader + menu-module registry.
- `internal/session`: node/session manager for online list, last callers, and idle tracking.
- `internal/app`: lifecycle and wiring.
- `internal/sshserver`: PTY session loop + screen router.
- `internal/loginserver`: optional command-mode telnet/websocket/websocket-tls login servers sharing auth + node/session state.
- `internal/netutil`: trusted-proxy client IP resolution helpers.
- `internal/auth`: account service, optional TOTP helpers, password hash policy (`bcrypt`/`pbkdf2-sha256`), reset-token flow.
- `internal/domain`: domain entities.
- `internal/repository`: repository abstractions and in-memory adapters.
- `internal/chat`: shared chat service with optional Postgres persistence used by web and IRC.
- `internal/doors`: manifest-driven door framework (native + external).
- `internal/content`: optional read-only Gopher/NNTP server implementations.

## Data Model (MVP)

- `users`
  - `id`, `handle` (unique), `password_hash`, `role`
  - `verified`, `theme`, `paging_enabled`, `ansi_enabled`
  - optional `totp_secret` and `recovery_codes`
- `boards`
  - `id`, `name`, `description`, metadata
- `messages`
  - board-scoped message threads and body/headers
  - includes `parent_id` and `thread_id` fields for replies/thread grouping
- `private_mail`
  - sender/recipient/message fields plus external routing metadata
- `chat_channels`, `chat_messages`, `chat_presence`
- `chat_moderation_state`, `chat_moderation_actions`, `chat_rate_events`
  - persisted when DB-backed mode is enabled
- `door_configs`
  - per-door enablement, turn rules, run limits, role override, capability toggles
- `door_user_state`, `door_global_state`
  - persistent per-user and shared door state
- `door_turn_bank`
  - daily turns and carryover time-bank tracking
- `door_scores`, `door_achievements`
  - cross-door leaderboard and trophy records
- `door_user_meta`
  - favorites, recent play, play counts
- `door_event_log`
  - start/stop/error and admin-observable runtime events
- `password_reset_tokens`
  - hashed one-time reset tokens with expiry and consumed state

## Security Baseline

- Password hash policy with bcrypt and pbkdf2-sha256, including opportunistic bcrypt->PBKDF2 upgrade on successful login.
- Per-session PTY gate for SSH terminal UI.
- Session cookies for web companion (HttpOnly/SameSite).
- Password reset tokens are hashed at rest and expire.
- Structured logging with user/action tags.
- Throttles, verified account flags, gateway allow-lists/deny-lists, and inbound token auth for email ingestion.

## Flow

1. SSH connect.
2. Welcome screen.
3. Login flow (existing account or create new account).
4. Main menu with classic hotkeys.
5. `N` opens “Since your last call” digest (replies/mentions/board traffic/mail).
5. `Last Callers` and `Who's Online` screens backed by live session manager state.
6. Live chat via `(C)` uses shared chat backend (same channels as web/IRC).

Web on-ramp notes:
- optional `/connect` page can present a quick WebSocket command-mode bridge and SSH/Telnet hints.
- optional `/tour` provides read-only guided content for first-time visitors.

Theme notes:
- named themes currently include `retro-amber`, `ice-blue`, and `emerald`.
- user preference is applied at login and used across SSH top bars and panels.
- SSH settings now render via an MCI-style view model and persist preferences through auth service.

Terminal metadata notes:
- SAUCE metadata parser baseline exists in `internal/term/sauce.go` for ANSI art/file workflows.

## Web + IRC Notes

- Web supports board/mail read+write flows with shared DB state.
- IRC endpoint supports standard minimal IRC commands and channel behavior.
- IRC supports optional TLS listener and SASL PLAIN authentication.
- Both web and IRC use the same `internal/chat` core and Postgres notify fanout for cross-process events.
