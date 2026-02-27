# PRD-01: Multi-Node Session Manager + Terminal Rendering Stack

## Scope
This slice implements:
- Multi-node session manager with node allocation, online tracking, and caller history.
- Terminal rendering baseline: ANSI on/off, CP437/UTF-8/ASCII output profile selection, width-aware framing.
- Nostalgia UX consistency in status bars and online/caller displays.

## Goals
1. Every SSH session has a stable node id.
2. `Who's Online` and `Last Callers` are live views, not static text.
3. Rendering is deterministic across ANSI/ASCII and CP437/UTF-8 environments.
4. No regression in login/boards/mail/chat flows.

## Non-goals
- Full SAUCE pipeline for art packs (covered by PRD-02 extension work).
- Telnet/WebSocket login transports (covered by PRD-03).

## Functional requirements
- PRD01-001: Session manager allocates/reuses node ids with bounded max nodes.
- PRD01-002: Session state includes node id, username, area, login time, last activity, and idle time.
- PRD01-003: Last callers list persists recent completed sessions in memory with bounded length.
- PRD01-004: Top status bar always renders dynamic node label.
- PRD01-005: Session area updates as users enter major sections (welcome/login/main/boards/mail/chat/gateway/doors).
- PRD01-006: ANSI renderer maps to ASCII-safe borders when ANSI is disabled.
- PRD01-007: Encoding profile selection supports UTF-8, CP437, and ASCII fallbacks.

## Acceptance criteria
- A01: `go test ./internal/session ./internal/sshserver ./internal/term ./internal/ui` passes.
- A02: SSH integration test still reaches main menu and can post in boards.
- A03: In live SSH, `L`/`W` screens show manager-backed rows including node/user/time/area/idle or duration.
- A04: Door launches receive the current node id in `WOLFBBS_NODE`.

## Test plan
- Unit: `internal/session/manager_test.go` covers lifecycle + snapshots.
- Unit: terminal profile + output transform tests cover encoding and ANSI fallbacks.
- Integration: SSH login and board-post flow tests remain green.

## Implementation status
- Complete in current pass:
  - `internal/session/manager.go` + tests.
  - SSH server wired to manager lifecycle (`Start/SetUser/SetArea/Touch/End`).
  - Dynamic node labels across top bars.
  - Manager-backed `Last Callers` and `Who's Online` rendering.
  - `WOLFBBS_NODE` exported from allocated node id.

## Remaining follow-up
- Completed in this pass:
  - PRD01-F01: Node and caller state persisted to DB for cross-process online visibility.
  - PRD01-F02: Web sysop diagnostics now expose persisted node state (`/admin/node-state`) and dashboard tables.
