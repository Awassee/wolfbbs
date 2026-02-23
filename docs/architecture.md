# WolfBBS Architecture

## Stack Choice

Go was selected for this implementation because it gives fast, concurrency-friendly SSH handling and direct terminal control while keeping an opinionated yet practical admin and service surface.

## Components

- `cmd/wolfbbs`: SSH + ANSI session entrypoint.
- `cmd/wolfbbs-web`: read-only web companion + admin shell.
- `cmd/wolfbbs-irc`: IRC endpoint implementation with shared chat integration.
- `internal/ui`: ANSI rendering primitives and screen templates.
- `internal/app`: lifecycle and wiring.
- `internal/sshserver`: PTY session loop + screen router.
- `internal/auth`: account service, optional TOTP helpers, bcrypt credentials.
- `internal/domain`: domain entities.
- `internal/repository`: repository abstractions and in-memory adapters.
- `internal/chat`: shared chat service with optional Postgres persistence used by web and IRC.

## Data Model (MVP)

- `users`
  - `id`, `handle` (unique), `password_hash`, `role`
  - `verified`, `theme`, `paging_enabled`, `ansi_enabled`
  - optional `totp_secret` and `recovery_codes`
- `boards`
  - `id`, `name`, `description`, metadata
- `messages`
  - board-scoped message threads and body/headers
- `private_mail`
  - sender/recipient/message fields plus external routing metadata
- `chat_channels`, `chat_messages`, `chat_presence`
  - persisted when DB-backed mode is enabled

## Security Baseline

- bcrypt password hashing in auth service.
- Per-session PTY gate for SSH terminal UI.
- Session cookies for web companion (HttpOnly/SameSite).
- Structured logging with user/action tags.
- Planned controls: throttles, verified account flags, gateway allow-lists/deny-lists.

## Flow

1. SSH connect.
2. Welcome screen.
3. Login flow (existing account or create new account).
4. Main menu with classic hotkeys.
5. `Last Callers` and `Who's Online` screens.

## Web + IRC Notes

- Web reads message-board/mail data in read-only mode.
- IRC endpoint supports standard minimal IRC commands and channel behavior.
- Both web and IRC use the same `internal/chat` core to keep events shareable.
