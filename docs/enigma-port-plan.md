# ENiGMA½ Port Plan (Pattern-First)

This document tracks ENiGMA½-inspired technical patterns to port into WolfBBS in small, reviewable slices.

Scope note:
- We are porting architecture and implementation patterns, not importing ENiGMA wholesale.
- No ENiGMA source file is copied directly in this pass.
- If code is copied in future passes, BSD-2 notices must be preserved and third-party attributions updated.

## Feature / Tech Matrix

| ENiGMA½ feature | ENiGMA½ source location | WolfBBS current state | Proposed WolfBBS approach | Priority / risk |
|---|---|---|---|---|
| Multi-login server abstraction (telnet/ssh/ws/wss) | docs: `docs/_docs/servers/loginservers/*.md`; code: `core/servers/login/telnet.js`, `core/servers/login/ssh.js`, `core/servers/login/websocket.js` | SSH + Web + IRC are separate binaries without a shared transport profile type | Add shared transport/session profile model in `internal/term` + server wrappers; keep services separate for now | P0, low |
| Terminal capability detection (ANSI + UTF-8/CP437) | docs index advertises CP437/UTF-8; code: `core/ansi_term.js` | ANSI rendering exists but terminal encoding profile was implicit | Add explicit terminal profile detection + output profile transform for ANSI and ASCII fallback | P0, low |
| Theme/menu ergonomics + MCI-like layout | docs: `docs/index.md`, `docs/_docs/art/mci.md`; templates: `misc/menu_templates/*.hjson` | Boxed-screen renderer + hardcoded layouts | Introduce declarative screen fragments incrementally; start with reusable panel composition and role-aware menu components | P1, medium |
| HJSON-first config ergonomics | config template: `misc/config_template.in.hjson`; defaults: `core/config_default.js` | Env-driven config + limited docs | Add optional file config layer (env override precedence) and validation pass | P1, medium |
| Security: strong password + 2FA + recovery | docs: `docs/_docs/configuration/security.md`; code: `core/user.js`, `core/user_2fa_otp*.js` | bcrypt + TOTP + recovery codes exist | Add stronger session hardening, CSRF consistency checks, PBKDF2 policy, and bcrypt->PBKDF2 upgrade path | P0, low |
| Mods/modules evented extensibility | docs index + mods docs; code: `core/system_events.js`, module loaders in `core` | Doors/plugin manifests exist; limited global hooks | Add lightweight event bus and plugin hook points around auth/chat/message lifecycle | P1, medium |
| Local door runner + dropfile compatibility | docs: `docs/_docs/modding/local-doors.md`; code: `core/abracadabra.js`, `core/dropfile.js`, `core/door.js` | Native/external door runtime + sandbox controls implemented | Add optional dropfile generation (`DOOR.SYS`, `DOOR32`, `DORINFO`) behind config flags | P0, medium |
| Door server connectors (BBSLink / DoorParty) | docs: `docs/_docs/modding/door-servers.md`; code: `core/bbs_link.js`, `core/door_party.js` | Connectors exist as manifests with template flow | Implement real connector adapters + audit and policy controls | P1, medium |
| Message/file base depth (index/search/metadata) | menu templates + file modules (`core/file_base_*`) | Basic board/mail and FileBase Pro metadata pattern | Expand file indexing/search + richer moderation queue + newscan state per area | P1, medium |
| Optional federation/content servers (ActivityPub, NNTP/Gopher patterns) | docs/activitypub + server docs | Optional read-only Gopher/NNTP/NNTPS listeners and ActivityPub actor/outbox/webfinger baseline are available behind env flags | Keep off by default and treat as experimental interfaces | P2, high |

## Incremental PR Plan

### PR1 (foundation, this pass)
- Add terminal/session profile foundation (`internal/term`).
- Wire SSH output profile handling for ANSI and ASCII-safe fallback.
- Normalize per-session settings use (ANSI + time format in top bar, door env encoding hints).
- Tighten web settings CSRF enforcement and preferences update path.

Acceptance:
- `go test ./...` passes.
- Existing behavior preserved for default users.
- ANSI-off users can still navigate screens with readable output.

Status:
- Completed.

### PR2 (terminal/encoding correctness)
- Add CP437-aware render options and SAUCE metadata parser for art/file views.
- Add focused tests for encoding transforms and SAUCE metadata extraction.

Acceptance:
- CP437/UTF-8 selection is deterministic and documented.
- ANSI art display remains stable across common terminal emulators.

### PR3 (login server abstraction expansion)
- Introduce transport abstraction and optional telnet/ws login adapters (disabled by default).
- Keep SSH as default and secure path.

Acceptance:
- SSH behavior unchanged by default.
- Optional transports gated by explicit config flags and security warnings.

Status:
- Completed.

### PR4 (MCI-like templating layer)
- Add lightweight declarative screen/view schema for lists, prompts, lightbar-style selection.

Acceptance:
- At least boards + doors UI use declarative templates.
- Hotkey behavior and latency remain equivalent.

### PR5 (doors compatibility uplift)
- Add optional dropfile support + connector adapters (DoorParty/BBSLink) with audit controls.

Acceptance:
- External door launches support configured dropfile types.
- Connector sessions are logged and policy-limited.

### PR6+ (message/file/network depth)
- File indexing/search improvements.
- Scanner/tosser-friendly message network boundaries.
- Optional federation/content server experiments behind disabled-by-default flags.

## Current Status

- PR1 foundation is now implemented:
  - `internal/term` profile detection added.
  - SSH renderer now applies output profile transforms and user time-format preference.
  - Web settings now enforce CSRF and support explicit preference updates.
- P0 security/doors follow-up is implemented:
  - Password hash policy supports `bcrypt` and `pbkdf2-sha256`.
  - Opportunistic bcrypt->PBKDF2 upgrade on successful login is available.
  - Password reset token repository + web reset endpoints (`/reset/request`, `/reset/complete`) are available.
  - External door runtime can generate optional legacy dropfiles (`DOOR32.SYS`, `DOOR.SYS`, `DORINFOx.DEF`) via `WOLFBBS_DOOR_DROPFILES`.
- P2 content-server baseline is implemented:
  - Optional read-only Gopher listener via `WOLFBBS_GOPHER_LISTEN`.
  - Optional read-only NNTP listener via `WOLFBBS_NNTP_LISTEN`.
  - Optional NNTPS listener via `WOLFBBS_NNTPS_LISTEN` + cert/key envs.
  - Optional ActivityPub read-only endpoints (`webfinger`, actor, outbox) via `WOLFBBS_ACTIVITYPUB_ENABLE`.
  - All are disabled by default and documented in `docs/content-servers.md`.
  - This closes the current P2 item for optional content/federation baseline patterns.
- P1 follow-ups implemented in this pass:
  - Canonical top role is `sysop` (with legacy `admin` alias preserved via role normalization).
  - Lightweight event bus/hooks are wired into auth/chat/SSH/web lifecycle points.
  - HJSON runtime config supports connector command settings with env precedence.
  - `doorparty-connector` and `bbslink-connector` now execute configured bridge commands with door-policy checks and door-event audit entries.
- PR3 login transport expansion is now in place:
  - Optional telnet login server behind `WOLFBBS_TELNET_ENABLE` / `WOLFBBS_TELNET_LISTEN`.
  - Optional websocket login server behind `WOLFBBS_WS_ENABLE` / `WOLFBBS_WS_LISTEN` / `WOLFBBS_WS_PATH`.
  - Optional websocket TLS login server behind `WOLFBBS_WSS_ENABLE` / `WOLFBBS_WSS_LISTEN` / `WOLFBBS_WSS_PATH` / cert+key.
  - Shared node/session manager is used across SSH/telnet/ws login paths.
  - Trusted-proxy CIDR support for websocket remote IP extraction via `WOLFBBS_TRUSTED_PROXIES`.
- Additional completion updates in this pass:
  - SQLite + FTS storage path is implemented and auto-selected via `WOLFBBS_SQLITE_PATH` / `sqlite://` DSN.
  - Password reset now includes SMTP delivery for email-form handles (`/reset/request`) with `WOLFBBS_PUBLIC_BASE_URL` support.
  - Door connectors include `telnet-bridge` with runtime config/env overrides and connector integration tests.
  - Sysop ops baseline now includes WFC-style dashboard at `/admin/system` and `cmd/oputil` admin CLI.
  - Multi-node persistence follow-up is now complete:
    - SSH sessions upsert/delete `node_sessions` and append `caller_history` on signoff.
    - Web sysop diagnostics include `/admin/node-state` JSON and dashboard tables from persisted state.
  - SSH `Files` main-menu path is now fully implemented (area list, browse, recent, newscan-style, search, contextual help).

## Linked execution specs

- `docs/prd/README.md`
- `docs/prd/FEATURE_MATRIX.md`
