# IRC Compatibility Notes

## Supported commands
- `NICK`
- `USER`
- `PASS`
- `JOIN`, `PART`
- `PRIVMSG`, `NOTICE`
- `QUIT`
- `PING`, `PONG`
- `TOPIC`
- `NAMES`, `LIST`, `WHO`, `WHOIS`
- `MODE` (minimal channel mode support)
- `KICK` (ops only)

## Required numeric replies
- 001-004 welcome block
- 375/372/376 MOTD flow
- 353/366 names listing
- 421, 431, 432, 433, 441, 451, 461, 462 and 403

## Authentication mapping
- IRC PASS/NICK should map to existing WolfBBS user handles.
- Unauthenticated sockets cannot JOIN or send PRIVMSG.

## Known limitations
- No SASL PLAIN in this milestone; PASS maps to existing password flow.
- CTCP and advanced WHOIS fields are not implemented.
- Away messages and channel operators are minimal.

## Security
- Connection limits and per-user flood caps
- IP bans and temporary throttles
- All moderation actions logged to audit log
