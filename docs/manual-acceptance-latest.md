# WolfBBS Manual Acceptance Report

- Date: 2026-03-18 03:42:33 UTC
- Mode: auto
- Repo: /Users/seanheiney/wolfbbs

| ID | Result | Description | Notes |
|---|---|---|---|
| BBS-003 | PASS | ANSI flow: Welcome -> Login -> Main Menu -> sections | Automated terminal e2e validated welcome/login/main menu flow |
| BBS-004 | PASS | User registration + login | Automated terminal e2e validated account creation + login |
| BBS-005 | PASS | Boards list/read/post/reply | Web e2e validated board list/read/post/reply |
| BBS-006 | PASS | Private mail inbox/outbox/send/read/reply/delete | Terminal integration test validated inbox/outbox/send/read/reply/delete flow |
| BBS-007 | PASS | New scan since last login | Terminal integration test validated newscan digest since previous login |
| BBS-008 | PASS | Paging (More) behavior | Pager unit test validated classic More prompt rendering |
| BBS-009 | PASS | Per-user ANSI on/off toggle | Web e2e validated ANSI preference toggle in settings |
| WA-003 | PASS | Admin users: list/search/disable/ban/reset/audit summary | Web e2e exercised admin user create/search/disable/ban/unban/role changes |
| WA-004 | PASS | Admin boards: create/edit/delete/perms | Web e2e exercised admin board create/edit/delete + ACS fields |
| WA-005 | PASS | Moderation: delete/edit reason, lock, move, queue/reports | Automated moderation test validated delete/lock/move/report queue actions |
| WA-006 | PASS | Gateway settings pages exist and work | Web e2e exercised gateway settings save in /admin/gateways |
| WA-007 | PASS | Chat admin: channels + kick/ban/mute + logs | Web e2e exercised chat channel admin + moderation events |
| WA-008 | PASS | Admin actions recorded in visible audit log | Web e2e verified admin audit log entries are visible |
| WA-009 | PASS | Optional admin TOTP/2FA | Web e2e exercised sysop 2FA enable/disable flow |
| CH-003 | PASS | Realtime web chat between two sessions | Web e2e verified realtime chat between two sessions |
| CH-004 | PASS | Chat history persistence + pagination | Web e2e verified chat history persists after reload |
| CH-005 | PASS | Moderation consistency across web/irc/bbs | Moderation integration validated mute enforcement in shared IRC/chat backend |
| IRC-007 | PASS | Cross-client bridge: IRC<->Web (and BBS when available) | Web e2e verified IRC -> web chat bridge message visibility |
| GW-EMAIL-003 | PASS | Admin can disable outbound email for user | Web e2e exercised admin outbound mail disable/enable controls |
| GW-WEB-001 | PASS | Text web gateway fetch + ANSI render works | Web e2e exercised gateway fetch and readable rendering |
| GW-WEB-004 | PASS | Save for offline reading works | Web e2e exercised save-for-offline in gateway fetch flow |

## Summary

- PASS: 21
- FAIL: 0
- SKIPPED: 0
