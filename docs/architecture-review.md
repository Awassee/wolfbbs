# WolfBBS Architecture Review (2026-02-27)

## Scope

This review focuses on operational reliability, test determinism, and service-boundary maintainability across:

- SSH BBS (`cmd/wolfbbs`, `internal/sshserver`, `internal/ui`)
- Web/Admin (`cmd/wolfbbs-web`)
- IRC (`cmd/wolfbbs-irc`)
- Shared domain/repository/chat/door modules (`internal/*`)
- Installer + verification surfaces (`install.sh`, `scripts/verify.sh`, `scripts/run-e2e.sh`, compose)

## Current Strengths

- Good package boundaries: transport entrypoints in `cmd/*`, reusable domain logic in `internal/*`.
- Shared chat backend used by web and IRC (reduces behavior drift).
- Security baseline in place (hash policy, CSRF/cookie controls, audit routes/docs).
- Broad test surface: unit tests, terminal e2e, scripted IRC checks, Playwright suite present.
- Acceptance verifier exists and encodes spec IDs for fast/static + smoke paths.

## Main Architectural Risks Found

1. Runtime orchestration could block indefinitely in unhealthy Docker/Colima states.
2. Web e2e orchestration had no bounded execution; Playwright could hang with no actionable failure.
3. Compose file had obsolete `version` key producing avoidable noise/warnings.
4. Local environment drift (Node major, Docker transport behavior) could cause non-deterministic results.

## Optimizations Implemented In This Pass

### 1) Compose contract cleanup

- Removed obsolete compose schema header from `docker-compose.yml` (`version: '3.9'`).

### 2) Deterministic e2e runner behavior

Updated `scripts/run-e2e.sh`:

- Added bounded command execution (`run_with_timeout`) for:
  - `npm --prefix e2e/web install`
  - `npm --prefix e2e/web run install:browsers`
  - `npm --prefix e2e/web test`
- Added explicit Node compatibility gate:
  - default fail-fast on Node `>=25` with clear remediation.
  - override flag: `--allow-unsupported-node`
  - env override: `WOLFBBS_ALLOW_UNSUPPORTED_NODE=true`
- Added `--web-timeout <seconds>` and `WOLFBBS_WEB_E2E_TIMEOUT_SECONDS`.

### 3) Verifier resilience for Docker/Colima instability

Updated `scripts/verify.sh`:

- Added bounded command execution helper (`run_with_timeout`).
- Added `run_compose` wrapper to normalize `docker compose` vs `docker-compose`.
- Added bounded `compose config` checks in both `--fast` and `--smoke`.
- Added bounded `compose up -d --build` in smoke mode.
- Hardened Docker host probing:
  - try default daemon with timeout
  - try Colima unix socket with timeout
  - fallback to `ssh://lima-colima` if available (bounded check)
- Added timeout env knobs:
  - `COMPOSE_CMD_TIMEOUT_SECONDS` (default `30`)
  - `COMPOSE_UP_TIMEOUT_SECONDS` (default `900`)

## Validation Summary (Post-Changes)

- `go test ./...` passed.
- `go test ./... -race` passed.
- `scripts/run-e2e.sh --no-web` passed.
- `scripts/verify.sh --fast` passed all MUST checks.
- `scripts/verify.sh --smoke` now fails fast (no hang) when Docker host is unhealthy.

## Recommended Next Architecture Work

### P0 (operational safety)

1. Add explicit web health/readiness checks for SSH and IRC binaries (mirroring web `/healthz` behavior via lightweight probes).
2. Standardize structured logging fields across all `cmd/*` entrypoints (service, node/session/user, request ID, area).
3. Add startup diagnostics endpoint/page in web admin summarizing:
   - DB connectivity
   - IRC listener
   - gateway config validity
   - queue/backlog pressure

### P1 (maintainability)

1. Extract repeated bootstrap/env parsing in `cmd/*` into `internal/app` boot helpers.
2. Unify config validation and defaults into a single typed config layer used by SSH/Web/IRC/mailin.
3. Add smoke fixtures for deterministic seeded data to reduce test flakiness.

### P2 (scale/perf)

1. Add explicit indexing review + migration checks for chat/message/file query hot paths.
2. Add retention jobs with bounded batch windows for chat logs, door events, and audit records.
3. Add p95/p99 timing metrics around menu rendering, board reads, and chat fanout.

