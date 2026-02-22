# Unified Chat Service

WolfBBS chat is a shared in-memory service for this milestone, intended to move to Redis/Postgres-backed persistence in a follow-up.

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
- SSH UI: command hooks from `C` menu (placeholder in this milestone)
- Web UI: WebSocket endpoint `/ws/chat`
- IRC endpoint: implemented in `cmd/wolfbbs-irc`

## Message Rules
- Rate limits: per-user and per-IP burst caps
- Moderation hooks before persistence
- Audit each mute/ban/kick with actor and reason
- Optional retention by age configured in service config

## Integration Notes
- Web companion and IRC should read/write through the same chat API.
- Persistence should eventually be backed by `chat_messages` and `chat_presence` tables.
