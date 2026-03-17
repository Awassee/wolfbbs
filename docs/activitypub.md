# ActivityPub (Experimental Baseline)

WolfBBS includes an optional ActivityPub baseline intended for interoperability experiments.

Defaults:
- Disabled by default.
- No remote delivery, signature verification, or follower-state sync.

## Enable

- `WOLFBBS_ACTIVITYPUB_ENABLE=true`
- optional `WOLFBBS_ACTIVITYPUB_BASE_URL=https://bbs.example.com`

## Endpoints

- `GET /.well-known/webfinger?resource=acct:<handle>@<host>`
- `GET /ap/users/<handle>`
- `GET /ap/users/<handle>/outbox`
- `POST /ap/users/<handle>/inbox`

## Behavior

- `webfinger` resolves local handles to actor URLs.
- Actor endpoint exposes a `Person` with inbox/outbox links.
- Outbox exposes recent user-authored board posts as ActivityStreams `Create(Note)` objects.
- Inbox accepts a bounded allowlist of inbound activities (`Follow`, `Undo`, `Create`, `Accept`, `Like`, `Announce`, `Update`, `Delete`) and returns `202 Accepted`.
- Accepted inbound activities are logged to the existing admin audit trail for visibility, but do not trigger remote delivery or automatic follower management.

## Security

- Keep feature disabled unless explicitly needed.
- Treat output as public-readable metadata/content.
- Treat inbox processing as experimental ingress only; WolfBBS does not currently verify signatures or maintain a federated social graph.
- Run behind normal web hardening controls (TLS, reverse proxy, rate limits).
