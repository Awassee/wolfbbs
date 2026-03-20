# WolfBBS Feature-Complete PRD Suite

This directory is the execution contract for the full WolfBBS feature set requested in the 2026-02-24 planning pass.

Execution model:
- Ship one PRD slice at a time.
- Each slice has explicit acceptance checks.
- No slice is marked complete until build/tests pass and docs are updated.

Current status summary:
- PRD-01: Done (multi-node manager + DB-backed node/caller persistence + web node diagnostics shipped).
- PRD-02: Done (themes/HJSON menus/MCI/ACS baseline is shipped and adopted across the live caller/admin surfaces; future UI work is enhancement-wave work, not unfinished baseline scope).
- PRD-03: Done (sqlite+fts backend path, auth hash policy/reset-token+delivery flow, optional telnet/ws login transports, and optional content/federation servers are implemented).
- PRD-04: Done (message/file/network/doors/mods/ops baseline is shipped across caller + sysop paths; future depth improvements are roadmap extensions, not incomplete baseline scope).

## PRD index
- `PRD-01-session-and-rendering.md`
- `PRD-02-theme-menu-mci-acs.md`
- `PRD-03-storage-auth-servers.md`
- `PRD-04-message-file-doors-mods-ops.md`

## Global delivery rules
- Keep secure-by-default behavior for all network services.
- Preserve current behavior unless feature flags/config toggles explicitly opt in.
- Maintain one-command local run via `docker compose up -d --build`.
- Add tests for each new risk area before marking a PRD slice done.

## Completion gates (all PRDs)
1. `go test ./...` passes.
2. `scripts/verify.sh --fast` passes.
3. `scripts/verify.sh --smoke` passes in a Docker-enabled environment.
4. New config toggles and migration notes are documented.
