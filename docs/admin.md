# Admin Interface (MVP)

## Scope
This version ships a read/write web-first control panel with role-aware routes. It is intentionally read-focused for this milestone, with mutating actions added behind RBAC checks.

## Roles
- `user`: standard read-only web companion access.
- `moderator`: can manage message moderation queues and abuse flags.
- `admin`: full sysop controls.

## Primary Panels
- Users
  - list/search by handle
  - disable/enable
  - ban/unban
  - password reset (admin-only)
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
  - list/create metadata entries
  - delete stale items
- Gateways
  - SMTP relay settings, allow/deny list, global and per-user caps
  - web gateway SSRF blocklist and timeout policy
- Chat
  - channel list and lock state
  - kick/ban/mute and log visibility
- System
  - motd/announcement editor
  - logs, metrics, health check endpoints
  - auditable admin action log

## Session and Security
- Cookie-based session, `HttpOnly` and `SameSite=Strict`.
- CSRF protection on mutating POST endpoints.
- Optional 2FA (TOTP) for admin accounts.
- Audit trail required for all admin writes with actor, target, action, reason.

## Read-only Mode Toggle
- Runtime toggle keeps all mutating writes disabled.
- In read-only mode, destructive operations return 403 with a short reason.
- MVP web route currently implemented:
  - `/admin` dashboard
  - `/admin/users` for users list + disable/enable/ban/unban/reset actions.
