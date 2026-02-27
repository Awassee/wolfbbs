# PRD-04: Message/File/Network Depth, Doors, Built-in Mods, Sysop Ops

## Scope
This slice finishes core BBS depth:
- Message base and message networking
- File base and download workflows
- Doors/connectors
- Built-in mods/utilities
- Sysop operations and observability

## Goals
1. Reach classic BBS completeness while preserving secure defaults.
2. Keep all operations auditable across SSH/Web/IRC.
3. Provide a maintainable extension path for doors and mods.

## Functional requirements

### Message base + networks
- PRD04-001: Conferences/areas with ACS read/write control.
- PRD04-002: Per-user pointers and reliable newscan behavior.
- PRD04-003: Optional FTN/BSO import/export boundaries (scanner/tosser separation).
- PRD04-004: Netmail and QWK packet import/export baseline.

### File base + downloads
- PRD04-005: File tags, ratings, saved filters, browse/search.
- PRD04-006: Upload processor extracts FILE_ID.DIZ/NFO metadata.
- PRD04-007: SHA-256 dedupe on ingest.
- PRD04-008: Temporary signed web links for downloads.
- PRD04-009: Batch download manager supports queue + zip streaming.

### Doors
- PRD04-010: Local doors support dropfiles and stdio/socket runtime.
- PRD04-011: Connector adapters for BBSLink, DoorParty, and telnet bridge.
- PRD04-012: Door launch policy controls (per-user/per-node/per-day limits).

### Built-in mods
- PRD04-013: Built-ins include onelinerz, rumorz, BBS list, who’s online, and bulletin/news modules.
- PRD04-014: Mod lifecycle hooks (init/tick/shutdown) with structured logging.

### Sysop ops
- PRD04-015: WFC dashboard for live node/channel/message health.
- PRD04-016: `oputil` CLI for admin automation (users, boards, doors, gateways, stats).
- PRD04-017: Structured logs + metrics + alert-ready health endpoints.

## Acceptance criteria
- A01: Message and file repository tests include pointer/newscan, tags/ratings/search, dedupe.
- A02: At least one integration test for FTN/QWK import-export pipeline shape.
- A03: Door integration tests for local/dropfile and connector launch paths.
- A04: WFC dashboard and `oputil` commands documented and smoke-tested.
- A05: End-to-end manual pass confirms SSH + Web + IRC + Doors coexist without regressions.

## Milestone order inside PRD-04
1. Message pointers/newscan + ACS area controls.
2. File indexing/tagging/ratings + dedupe.
3. Download queues + temporary links.
4. Door connector adapters.
5. Built-in mods pack.
6. WFC and `oputil` operations tooling.

## Current baseline
- Implemented:
  - Board conference/read/write ACS controls with per-user message pointers and pointer-driven newscan updates in SSH + Web.
  - Local doors runtime with optional legacy dropfiles (`DOOR32.SYS`, `DOOR.SYS`, `DORINFOx.DEF`).
  - Connector adapters for DoorParty, BBSLink, and Telnet Bridge with policy/audit controls.
  - SSH files area UX (`F`): area list, browse, recent files, new-files-since-last-call, and filename search.
  - Indexed file repository APIs: tagged search, per-file ratings, saved filters, download queue, and temporary download ticket lifecycle (in-memory/sqlite/postgres).
  - Web admin files panel now includes area indexing, indexed-file search, ratings, saved filters, queue management, and download ticket issuance.
  - Web gateway file browser (`/gateway?view=files`) now supports tagged search, ratings, queue actions, and one-time ticketed downloads.
  - Message networking baseline in `internal/network`: FTN/BSO/QWK packet export/import plus netmail queue/import processing with spool directories.
  - Sysop CLI networking controls in `cmd/oputil` (`network status/export/import/sync-in/sync-out/queue-netmail/import-queue`).
  - Built-in mods lifecycle manager in `internal/mods` with onelinerz, rumorz, bbslist, and whos_online modules.
  - Mods and network status surfaced in `/status`, `/discover`, `/config`, and `/admin/system`.
  - Web admin doors panel with config overrides, usage stats, score reset, and event logs.
  - WFC-style system dashboard at `/admin/system` including session/channel/online metrics.
  - `cmd/oputil` CLI baseline for status, user role updates, and board management.
  - SSH files queue parity (`D`) with queue remove, one-time ticket issue, and batch handoff hints.
  - ANSI Art Gallery native door reads `.ans/.asc/.txt`, parses SAUCE metadata, and renders through terminal output profiles.
  - Bunyan-compatible structured logging pipeline for app/web/irc/mailin/oputil via `internal/logging`.
  - Message network routing depth: board/conference route keys and netmail handle alias/domain routing for inbound packet import.
- Pending:
  - None for PRD-04 baseline scope.
