# Login Servers (SSH, Telnet, WebSocket)

WolfBBS primary access remains SSH. Optional login transports are available for compatibility and testing.

Defaults:
- SSH: enabled by `cmd/wolfbbs -listen :2222`
- Telnet: disabled by default
- WebSocket login: disabled by default
- WebSocket TLS login: disabled by default

## Telnet (optional)

Enable:
- `WOLFBBS_TELNET_ENABLE=true`
- `WOLFBBS_TELNET_LISTEN=0.0.0.0:2323`

Behavior:
- Command-mode login/session flow (non-ANSI full-screen).
- Shares auth and node/session state with SSH.
- Designed as compatibility transport; SSH remains recommended.

## WebSocket login (optional)

Enable:
- `WOLFBBS_WS_ENABLE=true`
- `WOLFBBS_WS_LISTEN=0.0.0.0:6080`
- `WOLFBBS_WS_PATH=/ws-login`

Behavior:
- Text-frame command-mode login/session flow.
- Shares auth and node/session state with SSH.
- Health endpoint available on the WS listener at `/healthz`.

## WebSocket TLS login (optional)

Enable:
- `WOLFBBS_WSS_ENABLE=true`
- `WOLFBBS_WSS_LISTEN=0.0.0.0:6443`
- `WOLFBBS_WSS_PATH=/ws-login`
- `WOLFBBS_WSS_CERT=/path/to/fullchain.pem`
- `WOLFBBS_WSS_KEY=/path/to/privkey.pem`

Behavior:
- Same command-mode flow as WS login.
- TLS terminates in-process using configured certificate/key.

## Proxy-aware client IP

When WS login is behind trusted reverse proxies:
- Set `WOLFBBS_TRUSTED_PROXIES` to a comma-separated CIDR list.
- Example: `WOLFBBS_TRUSTED_PROXIES=10.0.0.0/8,192.168.0.0/16`

If the direct peer IP is in a trusted CIDR:
- `X-Forwarded-For` is used (first valid IP).
- fallback to `X-Real-IP` if needed.

If not trusted:
- direct remote socket IP is used.

## Security notes

- Telnet and WS are opt-in and off by default.
- Prefer SSH for interactive BBS use and encrypted terminal transport.
- Prefer WSS over WS on untrusted networks.
- Keep trusted proxy CIDRs strict; do not trust broad internet ranges.
