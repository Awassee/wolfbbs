# End-User Coverage Audit

Release baseline: `v2.0.2+working`.

This audit is the product-manager view of a simple question: can a normal caller find and use the core WolfBBS loops from both the terminal and the web companion without needing sysop knowledge?

Automation:
- Run `python3 scripts/check-end-user-coverage.py`
- This checks route-level parity in [function-registry.json](function-registry.json) and confirms the terminal main menu still exposes the expected plain-English labels.

## Core caller loops

| Loop | Web path | Terminal path | Current state |
|---|---|---|---|
| Read public discussion | `/boards` | `Read Boards` | covered |
| Send private follow-up | `/mail` | `Private Mail` | covered |
| Live social chat | `/chat` | `Chat Rooms` | covered |
| Browse doors and scores | `/doors`, `/scores` | `Games & Doors` | covered |
| Catch up on the day | `/today`, `/attention` | `What's New`, `My Activity`, quick-jump deck | covered |
| File discovery and packets | `/collections`, `/offline` | `Files & Downloads`, `Offline Packets` | covered |
| Personal organization | `/bookmarks`, `/circles`, `/settings` | `My Settings` with bookmarks/circles/export tools | covered |
| Tour and discovery | `/showcase`, `/start` | `Showcase Tour`, `Find a Feature` | covered |

## Terminal menu contract

The terminal main menu should keep these user-facing labels visible and stable:

- `Read Boards`
- `Chat Rooms`
- `Files & Downloads`
- `Games & Doors`
- `Internet Tools`
- `What's New`
- `My Activity`
- `Offline Packets`
- `Find a Feature`

These labels are intentionally plain-English. They are meant to read like AOL/CompuServe tasks, not operator jargon.

## Intentional exceptions

Not every web surface needs a terminal twin.

Examples that remain intentionally web-first or operator-first:
- browser connection helper and copy-ready connect page chrome
- image-heavy docs/showcase presentation details
- sysop-only admin routes where terminal parity exists as an operator console, not as caller-facing UX

## Exit standard

This audit is healthy when:
- every core caller loop above is marked `user_tui=true` and `user_web=true`
- the terminal main menu still exposes the plain-English labels above
- browser and terminal end-to-end journeys pass in the same tranche
