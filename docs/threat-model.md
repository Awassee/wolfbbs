# Threat Model (MVP)

## Assets
- Credentials, sessions, and MFA secrets.
- Message board and private mail content.
- Sysop moderation actions and audit trail.
- Chat history and presence signals.
- Gateway outputs (email/web fetch).
- Door runtime state and door event logs.

## Risks
- Brute-force login and session flooding.
- Privilege abuse in sysop operations.
- Unauthorized access to shared chat and IRC relay.
- Email outbound abuse and SSRF via web gateway fetch.
- Inbound mail spoofing or replay against webhook ingest endpoint.
- IRC credential sniffing without TLS and brute-force auth attempts.
- ActivityPub endpoint scraping/abuse when experimental federation is enabled.
- Command injection via malformed escape and terminal sequences.
- External door sandbox escape or resource exhaustion.

## Controls in Place
- Password hash policy: `bcrypt` and `pbkdf2-sha256` with optional upgrade-on-login path.
- PTY required for SSH terminal mode.
- Logged menu/auth events.
- Web cookie flags (`HttpOnly`, `SameSite=Strict`) in web companion.
- Web login and password-reset request throttles are enforced per client address.
- Password reset tokens are one-time, hashed at rest, and expiry-bound.
- Password reset request responses are non-enumerating; SMTP delivery is attempted only when configured and logs failures server-side.
- Shared chat service applies basic per-nick rate limiting and presence tracking.
- IRC edge enforces connection, per-IP, and per-user flood throttles.
- Sysop/admin (alias) and moderation actions are recorded through audit repositories.
- External doors run with timeout + output-rate caps and optional network deny by default.
- Per-door writable directories isolate external door filesystem access.
- Inbound mail webhooks require shared-secret token auth; default dev tokens are limited to local callers only.
- Websocket login rejects foreign browser origins.
- JSON webhook/chat endpoints are size-bounded and strict-decoded.

## Planned Controls
- Connection/IP/command-rate throttles for SSH flows beyond the current login throttles.
- 2FA enforcement for sysop paths.
- Postgres-backed storage and transaction boundaries.
- Gateway kill-switches, allowlists, and immutable audit logs.
- Optional IRC TLS listener with cert/key config and SASL PLAIN support.
- ActivityPub remains disabled by default. When enabled, actor/outbox are public and inbox ingress is limited to bounded accepted activity logging without signature validation or remote delivery.
