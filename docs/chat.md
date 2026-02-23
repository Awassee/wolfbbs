# Unified Chat Service

WolfBBS chat is a shared service used by SSH/web/IRC and supports persistence when
`WOLFBBS_DATABASE_URL` (or `DATABASE_URL`) is configured.

In DB mode the service writes messages and presence into:
- `chat_channels`
- `chat_messages`
- `chat_presence`

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
- SSH UI: command hooks from `C` menu (planned for next pass)
- Web UI: SSE endpoint `/chat/stream` with channel history and live messages
- IRC endpoint: implemented in `cmd/wolfbbs-irc`

## Message Rules
- Rate limits: per-user and per-IP burst caps
- Moderation hooks before persistence
- Audit each mute/ban/kick with actor and reason
- Optional retention by age configured in service config

## Integration Notes
- Web companion and IRC should read/write through the same chat API.
- Persistence should eventually be backed by `chat_messages` and `chat_presence` tables.
