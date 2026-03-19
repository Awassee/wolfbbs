# Email Gateway

## Outbound
- Composed in BBS mail composer and sent via SMTP relay:
  - `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `FROM_DOMAIN`
  - or prefixed aliases: `WOLFBBS_SMTP_HOST`, `WOLFBBS_SMTP_PORT`, `WOLFBBS_SMTP_USER`, `WOLFBBS_SMTP_PASS`, `WOLFBBS_FROM_DOMAIN`
- Hard safety:
  - per-user send rate cap
  - max recipients per message
  - max message size
  - verified-account flag must be true before sending external recipients
  - global outbound disable toggle for incident response
- Runtime source of truth:
  - `/admin/gateways` values are applied at send time for outbound relay and reset-email delivery
  - if SMTP password is left blank in admin form, the existing stored secret is retained
- Audit log fields:
  - actor id, recipient count, subject, status, error, relay response, timestamp

## Password Reset Delivery
- `/reset/request` issues one-time reset tokens with expiration.
- If SMTP is configured and the account handle is an email-form handle, WolfBBS sends a reset link via email.
- Reset URL base:
  - `WOLFBBS_PUBLIC_BASE_URL` when set, otherwise the configured WolfBBS hostname/base URL.
- Delivery failures are logged server-side while API responses remain non-enumerating.
- Web reset requests are rate-limited per client address.

## Inbound (preferred optional)
- HTTP ingestion route is available:
  - `POST /mail/inbound`
  - Requires `X-Inbound-Token` matching `WOLFBBS_INBOUND_TOKEN`
  - Default dev token is accepted only from loopback/LAN callers; public exposure requires a custom token
  - JSON payload: `from`, `to`, `subject`, `body`, `raw_headers`
- Companion daemon (`cmd/wolfbbs-mailin`):
  - Receives inbound JSON on `/ingest`
  - Requires one of:
    - `X-Inbound-Token`
    - `Authorization: Bearer <token>`
    - `?token=<token>`
  - Applies sender-domain allowlist (`WOLFBBS_MAILIN_ALLOW_DOMAINS`)
  - Forwards to `/mail/inbound` with token auth
- Recipient mapping:
  - `user@example.com` -> `user`
  - `user+wolfbbs@example.com` -> `user`
- Inbound messages are stored as private mail using `mailbot` as sender.
- Anti-abuse:
  - shared-secret token required
  - plus-address mapping support
  - store raw headers/body preview for operator review

## Admin Controls
- Global allowlist/denylist
- Per-user allow/disable outbound
- per-minute and per-day throttles
