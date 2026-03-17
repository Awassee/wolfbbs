# PRD-03: Storage Model, Security Model, Login Servers, Content Servers

## Scope
This slice consolidates platform/runtime infrastructure:
- Storage model evolution (SQLite + FTS compatibility path while keeping Postgres primary in compose)
- User security hardening (PBKDF2 migration path, reset mail, OTP + backup codes)
- Login servers (SSH + Telnet + WebSocket; proxy-aware)
- Content servers (HTTPS web + optional gopher + NNTP/NNTPS)

## Goals
1. Keep existing Postgres default and add optional SQLite/FTS mode for low-footprint installs.
2. Strengthen auth without breaking existing bcrypt accounts.
3. Add extra login transports behind explicit toggles.
4. Add read-oriented content server endpoints safely.

## Functional requirements
- PRD03-001: Repository layer supports both Postgres and SQLite with consistent behavior for users/messages/files.
- PRD03-002: SQLite schema includes FTS indexes for message/file search.
- PRD03-003: Password hash policy supports:
  - existing bcrypt verification
  - PBKDF2 for new accounts (configurable)
  - opportunistic upgrade on successful login
- PRD03-004: Password reset flow supports email token reset with expiration and one-time use.
- PRD03-005: 2FA remains optional; recovery code regeneration and invalidation are audited.
- PRD03-006: Login server abstraction supports SSH (default), Telnet (off), WebSocket/WSS (off by default).
- PRD03-007: Proxy-aware remote-ip extraction for trusted proxy CIDRs.
- PRD03-008: Content server layer supports HTTPS admin/chat UI and optional gopher/NNTP read-only services.

## Acceptance criteria
- A01: Storage tests pass for both Postgres and SQLite backends.
- A02: Hash migration tests prove bcrypt users can login and upgrade to PBKDF2 when enabled.
- A03: Integration tests for SSH and WebSocket login handshake.
- A04: Telnet server disabled by default and requires explicit enable flag.
- A05: Security docs updated with config examples and safe defaults.

## Rollout plan
1. Introduce config schema for transport toggles and auth hash policy.
2. Add SQLite repository implementation + migration files.
3. Add password reset token store and web endpoints.
4. Add WebSocket login server adapter.
5. Add Telnet adapter with clear warning banner.
6. Add optional gopher/NNTP modules disabled by default.

## Current baseline
- Implemented:
  - Password hash policy (`bcrypt` + `pbkdf2-sha256`) with upgrade-on-login support.
  - Password reset token flow, reset endpoints, and reset email delivery for email-form handles.
  - Optional Gopher/NNTP/NNTPS content listeners (off by default).
  - Optional websocket login server (off by default).
  - Optional websocket TLS login server (off by default).
  - Optional telnet login server (off by default).
  - Proxy-aware remote IP extraction for websocket login via trusted CIDRs.
  - SQLite + FTS repository path with backend parity tests.
