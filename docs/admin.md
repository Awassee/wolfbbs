# Admin Interface (MVP)

## Scope
This version ships a read/write web-first control panel with role-aware routes and DB-backed state for boards, file areas, gateway settings, outbound-mail policy, and admin audit entries.

## Roles
- `user`: standard read-only web companion access.
- `moderator`: can manage message moderation queues and abuse flags.
- `sysop`: full system control.
- `admin`: legacy alias that normalizes to `sysop` for backward compatibility.

## Primary Panels
- Users
  - list/search by handle
  - disable/enable
  - ban/unban
  - password reset (sysop-only)
  - role assignment
  - show last login and session/IP history
  - optional audit notes
- Boards
  - create/edit/delete board metadata
  - toggle lock/permissions per board
  - moderate posts: delete/edit reason, lock thread, move post thread
  - report queue and clear actions
- Private Mail
  - metadata-only audit view
  - outbound rate-limit status
  - disable outbound for user
- Files
  - create/delete file areas
  - index files from area paths (SHA-256 + DIZ/NFO description extraction)
  - browse/search indexed files by area/query/tags
  - per-file ratings and saved filters
  - per-user download queue management
  - issue temporary download tickets for `/gateway?download=<token>`
- Gateways
  - SMTP relay settings, allow/deny list, global and per-user caps
  - web gateway SSRF blocklist and timeout policy
- Chat
  - channel list and lock/unlock state
  - create channels
  - kick/ban/mute and log visibility
  - moderation log table
- Doors
  - enable/disable each door
  - configure daily turns, time-bank caps, retention, output/run limits
  - per-door role overrides
  - reset door leaderboards
  - usage stats and door event logs
- System
  - motd/announcement editor
  - runtime feature toggles (read-only, on-ramp, guest tour, discover)
  - ANSI menu runtime editor (HJSON file selection, validation, save)
  - WFC-style dashboard with session/channel/online metrics
  - node diagnostics endpoint for persisted node/caller state
  - setup/install verification panel
  - runtime error log panel
  - logs, metrics, health check endpoints
  - auditable admin action log

## Session and Security
- Cookie-based session, `HttpOnly` and `SameSite=Strict`.
- CSRF protection on mutating POST endpoints.
- Optional 2FA (TOTP) for sysop accounts.
- Audit trail required for all sysop/admin writes with actor, target, action, reason.
- User settings updates (password/2FA/preferences) also require CSRF.

## Read-only Mode Toggle
- Runtime toggle keeps all mutating writes disabled.
- In read-only mode, destructive operations return 403 with a short reason.

## Implemented Routes
- `/admin/login`
- `/reset/request` (public password reset token request)
- `/reset/complete` (public token redemption endpoint)
- `/admin`
- `/admin/users` (search + enable/disable + ban/unban + reset + role + verify/unverify)
- `/admin/boards` (create/delete boards)
- `/admin/mail` (per-user outbound email policy)
- `/admin/files` (file areas + indexing + tagged file search + ratings + filters + queue + ticket issuance)
- `/admin/gateways` (SMTP/web gateway limit settings)
- `/admin/chat` (channel list)
- `/admin/setup` (bootstrap + install checks)
- `/admin/config` (runtime settings + site text)
- `/admin/config` also includes menu editor controls for HJSON menu files
- `/admin/errors` (runtime web error log)
- `/admin/doors` (door policy + stats + logs + leaderboard reset)
- `/admin/system` (system summary)
- `/admin/node-state` (JSON diagnostics: persisted node sessions + caller history)
- `/admin/audit` (persisted audit trail)
- `/scores` (global and per-door leaderboard views)
- `/settings` (self-service theme/ANSI/paging/time-format + password + 2FA)

## Sysop CLI (`oputil`)
- Binary: `cmd/oputil`
- Commands:
  - `oputil status`
  - `oputil users list`
  - `oputil users set-role --handle <name> --role <user|moderator|sysop>`
  - `oputil boards list`
  - `oputil boards create --name <title> [--description <text>]`
  - `oputil boards delete --id <id>`
  - `oputil network status`
  - `oputil network export --format <ftn|bso|qwk> --board <id> [--out <path>]`
  - `oputil network import --in <packet.json> [--board <id>] [--author <id>]`
  - `oputil network queue-netmail --from <uid> --to <handle> --subject <s> --body <b>`
  - `oputil network import-queue [--board <id>] [--author <id>]`
  - `oputil mods list`
- Uses the same DB and auth/repository model as SSH/Web services.
