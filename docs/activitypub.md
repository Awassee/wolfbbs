# ActivityPub (Experimental Baseline)

WolfBBS includes an optional, read-only ActivityPub baseline intended for interoperability experiments.

Defaults:
- Disabled by default.
- No inbox processing or federation write path.

## Enable

- `WOLFBBS_ACTIVITYPUB_ENABLE=true`
- optional `WOLFBBS_ACTIVITYPUB_BASE_URL=https://bbs.example.com`

## Endpoints

- `GET /.well-known/webfinger?resource=acct:<handle>@<host>`
- `GET /ap/users/<handle>`
- `GET /ap/users/<handle>/outbox`

## Behavior

- `webfinger` resolves local handles to actor URLs.
- Actor endpoint exposes a `Person` with inbox/outbox links.
- Outbox exposes recent user-authored board posts as ActivityStreams `Create(Note)` objects.
- Inbox endpoint is intentionally not implemented (`501`).

## Security

- Keep feature disabled unless explicitly needed.
- Treat output as public-readable metadata/content.
- Run behind normal web hardening controls (TLS, reverse proxy, rate limits).

