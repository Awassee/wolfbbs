# Threat Model (MVP)

## Assets
- Credentials, sessions, and MFA secrets.
- Message board and private mail content.
- Admin moderation actions and audit trail.
- Chat history and presence signals.
- Gateway outputs (email/web fetch).

## Risks
- Brute-force login and session flooding.
- Privilege abuse in admin operations.
- Unauthorized access to shared chat and IRC relay.
- Email outbound abuse and SSRF via web gateway fetch.
- Inbound mail spoofing or replay against webhook ingest endpoint.
- IRC credential sniffing without TLS and brute-force auth attempts.
- Command injection via malformed escape and terminal sequences.

## Controls in Place
- bcrypt hashing.
- PTY required for SSH terminal mode.
- Logged menu/auth events.
- Web cookie flags (`HttpOnly`, `SameSite=Strict`) in web companion.
- Shared chat service applies basic per-nick rate limiting and presence tracking.

## Planned Controls
- Connection/IP/command-rate throttles for SSH and IRC.
- 2FA enforcement for admin paths.
- Postgres-backed storage and transaction boundaries.
- Gateway kill-switches, allowlists, and immutable audit logs.
- Inbound token authentication and sender-domain allowlists for `/mail/inbound`.
- Optional IRC TLS listener with cert/key config and SASL PLAIN support.
