# UX/UI Audit

## Baseline (Step 0)

Build/test baseline on this pass:

- `go test ./...` passed
- `go build ./...` passed
- `scripts/verify.sh --fast` passed all MUST checks (one SHOULD warning for local shellcheck availability)

Entrypoints discovered:

- Terminal UI server (SSH): `go run ./cmd/wolfbbs -listen :2222`
- Web user + admin UI server: `go run ./cmd/wolfbbs-web -listen :8080`
- IRC endpoint: `go run ./cmd/wolfbbs-irc -listen :6667`
- Sysop CLI: `go run ./cmd/oputil <command>`

## Function Inventory Artifacts (Step 1)

- Machine-readable registry: `docs/function-registry.json`
- Coverage matrix: `docs/UI_COVERAGE_MATRIX.md`
- Coverage validator test: `internal/app/function_registry_test.go`

Validator scope:

- Web route discovery from `cmd/wolfbbs-web/main.go`
- SSH state-machine discovery from `internal/sshserver/server.go`
- Sysop CLI discovery from `cmd/oputil/main.go`
- Door module discovery from `doors/*/door.json`

## User Journey Audit

### User TUI

- Onboarding/login/logout: present, plus login help (`?`) and guest tour.
- Boards/mail/chat/gateway/doors/settings: present and reachable from main menu.
- Last callers/who online: present.
- Closed:
  - Status Center and Config Center are first-class screens from main menu (`Y`, `X`) and quick jump (`/`).
  - Status rows now include backend readiness, session/node data, feature flags, and login transport state.
  - Contextual help (`?`) and Back/Quit handling remain consistent across boards/mail/chat/gateway/doors/settings.

### User Web

- Login/logout + reset: present.
- Boards/mail/chat/gateway/settings/discover: present.
- Closed:
  - Added `/status` and `/config` user-facing hubs.
  - `/status` now exposes function state rows for boards/chat/doors/gateway/transports/connectors/content-servers and opt-in features.
  - `/config` now groups user preferences + runtime feature + transport/service config visibility.

### Admin TUI

- Sysop guidance currently redirects to web admin for full controls.
- Closed:
  - Sysop status/config telemetry is visible in terminal Status/Config Centers.
  - `A` remains a direct bridge to full SYSOP web control panel.

### Admin Web

- Users/boards/mail/files/gateways/chat/doors/system/audit: present.
- Added this pass:
  - `/admin/setup` bootstrap/install verification panel
  - `/admin/config` persisted runtime config center
  - `/admin/errors` runtime error center
  - Chat channel lock/unlock + moderation log
  - User create + enhanced user management status data
  - Board metadata edit (name/description)
  - Runtime status includes transport/content/connector checks in addition to app-level checks

## Gaps Closed In This Pass

- Added PRD-04 message-network baseline (`internal/network`) with sysop CLI import/export/queue operations.
- Added built-in mods lifecycle manager (`internal/mods`) with onelinerz/rumorz/bbslist/who's-online modules.
- Surfaced mods/network state in user status center, discover view, config center, and sysop WFC dashboard.
- Added manual acceptance harness `scripts/manual-acceptance.sh` + report output workflow.
- Hardened web e2e runner to auto-select macOS Node 24/22 toolchains when local Node is 25+.
- Added function registry and coverage matrix with automated completeness checks.
- Added sysop setup/config/error centers and linked them from admin navigation.
- Added persistent system settings backend (`system_settings`) for runtime config.
- Added channel lock management and enforcement in chat flows.
- Added richer status metrics in WFC dashboard (errors, locks, MOTD/announcement).
- Added missing management actions (user creation, board updates).
- Added user-facing Status/Config Centers in web and terminal.
- Added opt-in quick jump + classic search visibility in status/config surfaces.
- Added Playwright e2e harness for user/admin flows with keyboard and accessibility checks.
- Added terminal pexpect e2e harness for registration/login/menu traversal and sysop entrypoint checks.

## Run Locally (One Command)

```bash
docker compose up -d --build
```

## Run E2E (One Command)

```bash
scripts/run-e2e.sh
```

## Captured Terminal Sample (from pexpect flow)

```text
Press any key to continue
Handle:
Password:
Create account? (Y/N):
Created. Any key to continue.
Enter selection:
Status Center
Quick jump state: ON
Enter selection:
Use web admin at /admin for full sysop controls.
```
