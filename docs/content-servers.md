# Content Servers (Optional)

WolfBBS can expose read-only message board content over classic protocols.

Defaults:
- Disabled by default.
- HTTP/HTTPS web companion remains the primary content endpoint.

## Gopher

Enable with:
- `WOLFBBS_GOPHER_LISTEN=0.0.0.0:7070`
- `WOLFBBS_CONTENT_HOST=bbs.example.com` (optional; used in gopher menu links)

Behavior:
- Selector `/` lists message boards.
- Selector `/board/<id>` lists messages for a board.
- Selector `/msg/<id>` returns plain text message content.
- Read-only; no posting or authentication.

## NNTP

Enable with:
- `WOLFBBS_NNTP_LISTEN=0.0.0.0:1190`

Behavior:
- Group mapping: `wolfbbs.board.<board_id>`
- Read-only command subset:
  - `CAPABILITIES`
  - `MODE READER`
  - `LIST`
  - `GROUP <group>`
  - `XOVER <range>`
  - `ARTICLE <n|message-id>`
  - `HEAD <n|message-id>`
  - `BODY <n|message-id>`
  - `HELP`
  - `QUIT`
- Read-only; no posting/auth commands.

## NNTPS (TLS)

Enable with:
- `WOLFBBS_NNTPS_LISTEN=0.0.0.0:1563`
- `WOLFBBS_NNTPS_CERT=/path/to/fullchain.pem`
- `WOLFBBS_NNTPS_KEY=/path/to/privkey.pem`

Behavior:
- Same read-only command subset as NNTP.
- TLS 1.2+ enforced.
- Disabled by default.

## ActivityPub (Experimental)

Enable with:
- `WOLFBBS_ACTIVITYPUB_ENABLE=true`
- optional `WOLFBBS_ACTIVITYPUB_BASE_URL=https://bbs.example.com`

Routes:
- `GET /.well-known/webfinger?resource=acct:<handle>@<host>`
- `GET /ap/users/<handle>`
- `GET /ap/users/<handle>/outbox`
- `POST /ap/users/<handle>/inbox`

Notes:
- Off by default.
- Public actor/outbox baseline for federation compatibility.
- Inbox accepts a bounded allowlist of inbound activities and logs them to admin audit.
- No remote delivery, signature verification, or follower-state sync in the current baseline.

## Security Notes

- Keep these endpoints behind trusted network boundaries when possible.
- Treat as public-readable unless you add external network controls.
- NNTPS built-in support requires explicit cert/key paths.
- ActivityPub is experimental and should be exposed only with explicit operator intent.
- Feature is intentionally opt-in and off by default.
