# Email Gateway

## Outbound
- Composed in BBS mail composer and sent via SMTP relay:
  - `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `FROM_DOMAIN`
- Hard safety:
  - per-user send rate cap
  - max recipients per message
  - max message size
  - verified-account flag must be true before sending external recipients
  - global outbound disable toggle for incident response
- Audit log fields:
  - actor id, recipient count, subject, status, error, relay response, timestamp

## Inbound (preferred optional)
- Dedicated SMTP ingestion endpoint is recommended as a separate small service.
- Anti-abuse:
  - allowlist domains and/or plus-address mapping
  - DKIM/SPF checks by host policy
  - store raw headers + body preview for operator review

## Admin Controls
- Global allowlist/denylist
- Per-user allow/disable outbound
- per-minute and per-day throttles
