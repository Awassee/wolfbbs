# WolfBBS Requested Feature Matrix

Status legend:
- `done`: implemented and tested in repository
- `in_progress`: actively being delivered in current sequence
- `planned`: PRD committed, implementation pending

| Requested feature | PRD | Status | Notes |
|---|---|---|---|
| Multi-node session manager + node list/state | PRD-01 | done | SSH session manager + DB-backed node/caller state + web diagnostics shipped |
| Terminal rendering stack (CP437/UTF-8/ANSI/SyncTERM/SAUCE) | PRD-01, PRD-02 | done | output profiles + CP437/UTF-8 handling + SAUCE parser/stripper + ANSI Art Gallery rendering shipped |
| Theme system + user-selectable themes | PRD-02 | done | built-in named themes + env-loaded HJSON theme packs + SSH/web user theme selection shipped |
| Menu system (HJSON) + menu-module plugin model | PRD-02 | done | parser/validator + module registry + runtime routing are wired in SSH with ACS-aware visibility |
| MCI + view framework | PRD-02 | done | control/view framework shipped with HJSON file loader and SSH settings template override support |
| ACS rule engine across menus/messages/files/admin | PRD-02 | done | ACS now enforced in SSH + web boards/mail/files and optional `/admin/*` policy gate |
| Storage model (SQLite + FTS) | PRD-03 | done | sqlite backend path + schema + FTS tables/triggers + backend parity tests shipped |
| Login servers: Telnet + SSH + WebSocket | PRD-03 | done | SSH remains default; optional telnet/ws/wss login servers shipped behind explicit config toggles with handshake coverage |
| Content servers: HTTPS + gopher + NNTP/NNTPS | PRD-03 | done | HTTPS exists; optional read-only gopher + nntp + nntps listeners shipped behind explicit env flags |
| User security: PBKDF2/reset email/optional 2FA | PRD-03 | done | 2FA + recovery codes, PBKDF2 policy + bcrypt upgrade path, reset-token flow, and reset email delivery for email-form handles are shipped |
| Message base: conferences/areas + pointers/newscan | PRD-04 | done | conference-aware board filters and area-level newscan summaries shipped across SSH + web discover |
| Message network support: FTN/BSO, netmail, QWK | PRD-04 | done | spool import/export + queue + external sync hooks + board/netmail routing maps shipped |
| File base: tags/search/ratings/upload processor/dedupe/temp links | PRD-04 | done | tagged search/ratings/filters, SHA-256 dedupe index, upload metadata extraction, queue, and temporary ticketed downloads shipped in SSH+web |
| Download managers: legacy queue + web queue + batch zip | PRD-04 | done | SSH legacy queue manager + web/admin queue + ticket lifecycle + gateway batch zip streaming shipped |
| Doors: local dropfiles + BBSLink + DoorParty + telnet bridge | PRD-04 | done | local runtime + optional dropfiles + DoorParty/BBSLink/Telnet Bridge command adapters shipped with policy checks and audit |
| Built-in mods (onelinerz/rumorz/bbs list/who's online/etc.) | PRD-04 | done | lifecycle manager + built-ins + status surfacing shipped |
| Sysop ops: WFC dashboard, oputil CLI, Bunyan logs + monitoring | PRD-04 | done | `/admin/system`, `cmd/oputil`, health/metrics, and Bunyan-compatible structured logging are shipped |
