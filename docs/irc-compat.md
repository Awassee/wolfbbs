# IRC Compatibility Notes

## Supported commands
- `NICK`
- `USER`
- `PASS`
- `CAP` / `AUTHENTICATE` (SASL PLAIN)
- `JOIN`, `PART`
- `PRIVMSG`, `NOTICE`
- `QUIT`
- `PING`, `PONG`
- `TOPIC`
- `NAMES`, `LIST`, `WHO`, `WHOIS`
- `MODE` (minimal channel mode support)
- `KICK` (ops only)

NICK changes are guarded by collision checks.

## Required numeric replies
- 001-004 welcome block
- 375/372/376 MOTD flow
- 353/366 names listing
- 401, 403, 421, 431, 432, 433, 451, 461, 462

## Authentication mapping
- IRC PASS/NICK should map to existing WolfBBS user handles.
- Unauthenticated sockets cannot JOIN or send PRIVMSG.
- SASL PLAIN is supported via `CAP REQ :sasl` + `AUTHENTICATE`.

## Known limitations
- CTCP and advanced WHOIS fields are not implemented.
- Away messages and channel operators are minimal.

## Security
- Connection limits and per-user flood caps
- Per-IP flood caps and throttling
- IP bans and temporary throttles
- Optional TLS listener (`WOLFBBS_IRC_TLS_LISTEN`, cert/key env vars)
- All moderation actions logged to audit log
