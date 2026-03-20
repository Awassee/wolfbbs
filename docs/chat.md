# Unified Chat Service

WolfBBS chat is a shared service used by SSH/web/IRC and supports persistence when
`WOLFBBS_DATABASE_URL` (or `DATABASE_URL`) is configured.

In DB mode the service writes messages and presence into:
- `chat_channels`
- `chat_messages`
- `chat_presence`
- `chat_moderation_state`
- `chat_moderation_actions`
- `chat_rate_events`

Cross-process realtime fanout:
- Uses Postgres `LISTEN/NOTIFY` on channel `wolfbbs_chat_events` when DB mode is enabled.
- Falls back to periodic history polling for clients if notify subscription is unavailable.

In-memory mode is used automatically when DB configuration is not present.

## Objects
- Channels (default `#lobby`)
- Direct messages between users
- Presence: online/away/idle
- Moderation events: kick, ban, mute

## Required Fields
- `id` numeric (stable for history)
- `channel` or `dmTarget`
- `fromUserID`
- `text`
- `createdAt`
- `messageType` (channel/dm/system)

## Access Paths
- SSH UI: `C` / `Chat Rooms` from the main menu opens the live room desk with send, join/open, leave, roster, and switch-by-slot actions.
- Web UI: `/chat` renders a multi-column chat client with joined rooms, active-room discovery, per-channel drafts, unread markers, and a compact mode backed by SSE `/chat/stream`.
- IRC endpoint: implemented in `cmd/wolfbbs-irc`

## Caller Experience
- Default room is `#lobby`, but callers can join or open additional rooms without losing their current set.
- Web chat keeps drafts per room so switching between rooms does not drop unfinished text.
- The right-side room desk shows topic, latest traffic, membership state, and roster signal.
- The terminal chat desk mirrors the same room model with numbered room slots and plain-language prompts intended for non-technical callers.
- Locked rooms stay visible in every client and become read-only for non-moderators.

## Message Rules
- Rate limits: per-user and per-IP burst caps
  - enforced in `internal/chat/service.go` for shared backend writes
  - enforced again at IRC edge in `cmd/wolfbbs-irc/main.go` for connection-level flood control
- Moderation hooks before persistence
- Audit each mute/ban/kick with actor and reason
- Optional retention by age configured in service config

## Integration Notes
- Web companion and IRC should read/write through the same chat API.
- Persistence is backed by the shared `chat_messages` and `chat_presence` tables when DB mode is enabled.
