# Modern Layer (Opt-In)

WolfBBS keeps the ANSI/terminal experience as the default.  
Modern additions are optional and designed to support discovery, safety, and onboarding without replacing the BBS flow.

## Principles

1. Terminal-first remains primary.
2. Modern features are opt-in and bounded.
3. AI output is labeled and rate-limited.
4. Discovery is transparent (no opaque ranking).

## Implemented Features

### 1) Guided Guest Tour (opt-in)

- SSH login supports `GUEST` handle for read-only tour.
- Web has optional `/tour` page with:
  - last caller list
  - one-liners from `#lobby`
  - featured thread
  - file pick hint

### 2) Smart Newscan Digest

- Main menu `N` opens `Since your last call`.
- Digest rules:
  - replies to your posts
  - mentions of your handle
  - per-board new activity
  - private mail since last login
- Capped to a small, fixed list (default `12`, max `30`).

### 3) Web Discover Surface (opt-in)

- `/discover` for authenticated users:
  - digest view
  - deep search (subject/body)
  - saved searches (session-memory for MVP)

### 4) Connect On-Ramp (opt-in)

- `/connect` page provides quick access:
  - SSH/Telnet commands
  - WebSocket command-mode terminal bridge
  - login + guest tour links

### 5) AI Assist (opt-in, labeled)

- Optional digest summary line appears as:
  - `[AI-LABEL] Catch-up: ...`
- Disabled by default.

## Environment Toggles

- `WOLFBBS_GUEST_TOUR_ENABLE` (default `false`)
- `WOLFBBS_SMART_NEWSCAN_ENABLE` (default `true`)
- `WOLFBBS_SMART_NEWSCAN_MAX` (default `12`)
- `WOLFBBS_WEB_ONRAMP_ENABLE` (default `false`)
- `WOLFBBS_DISCOVER_ENABLE` (default `false`)
- `WOLFBBS_WS_TERMINAL_URL` (default `ws://localhost:6080/ws-login`)
- `WOLFBBS_AI_ASSIST_ENABLE` (default `false`)
