# WolfBBS Configuration Reference

This file is the single source of truth for runtime configuration knobs used by WolfBBS binaries.

## Scope

- Core services: `cmd/wolfbbs`, `cmd/wolfbbs-web`, `cmd/wolfbbs-irc`, `cmd/wolfbbs-mailin`
- Shared internals: auth/chat/repository/runtime config loaders
- Installer-generated settings: `install.sh` and `.env`

## Installer Profile and Identity

- `WOLFBBS_SETUP_PROFILE` (default `basic`): installer profile (`basic`, `critical`, `expert`).
- `WOLFBBS_BBS_NAME` (default `WolfBBS`): BBS name shown in install/status summaries and intended runtime identity.
- `WOLFBBS_HOSTNAME` (default detected host, fallback `localhost`): host used by installer connect instructions.

## Core Service and Port Settings

- `WOLFBBS_SSH_PORT` (default `2222`): SSH BBS published port in compose/installer outputs.
- `WOLFBBS_WEB_PORT` (default `8080`): web service published port.
- `WOLFBBS_IRC_PORT` (default `6667`): IRC plaintext published port.
- `WOLFBBS_IRC_TLS_PORT` (default `6697`): IRC TLS published port.
- `WOLFBBS_MAILIN_PORT` (default `8091`): inbound mail adapter published port.

## Database and Storage

- `WOLFBBS_DATABASE_URL`: primary PostgreSQL DSN.
- `DATABASE_URL`: fallback DSN alias.
- `PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, `PGDATABASE`: PG fallback parts.
- `WOLFBBS_SQLITE_PATH`: SQLite path; when set, repository layer resolves to `sqlite://<path>`.
- `WOLFBBS_DB_MAX_CONN` (default code path `10`): maximum DB connections.
- `WOLFBBS_DB_CONNECT_RETRIES` (default `10` in app, `15` in installer env template): connect retry count.
- `WOLFBBS_DB_CONNECT_DELAY_MS` (default `1000` in app, `500` in installer env template): retry delay.

## Auth and Password Policy

- `WOLFBBS_PASSWORD_HASH` (default `bcrypt`): password policy preset (`bcrypt` or `pbkdf2-sha256`).
- `WOLFBBS_PASSWORD_ALGORITHM`: explicit algorithm override; same accepted values.
- `WOLFBBS_PBKDF2_ITERATIONS` (default `210000`): PBKDF2 iteration count.
- `WOLFBBS_PBKDF2_SALT_BYTES` (default `16`): PBKDF2 salt length.
- `WOLFBBS_PASSWORD_UPGRADE_ON_LOGIN` (default `true`): opportunistic hash upgrade after successful login.
- `WOLFBBS_RESET_TTL_MINUTES` (default `30`): password reset token TTL.
- `WOLFBBS_PUBLIC_BASE_URL`: canonical external base URL for password reset links.
- `WOLFBBS_DEV_SHOW_RESET_TOKEN` (default `false`): include token in web response for local/dev.

## Session and Web Security

- `WOLFBBS_SESSION_SECRET`: session secret used by service config/installer.
- `WOLFBBS_SECURE_COOKIE` (default `false`): set `true` behind HTTPS to mark auth cookie `Secure`.
- `WOLFBBS_READ_ONLY` (default `false`): blocks mutating admin actions.
- `WOLFBBS_REQUIRE_VERIFIED_EMAIL` (default `true`): require verified user state before allowing outbound external email.

## Bootstrap Accounts

- `WOLFBBS_BOOTSTRAP_ADMIN_HANDLE`
- `WOLFBBS_BOOTSTRAP_ADMIN_PASSWORD`
- `WOLFBBS_BOOTSTRAP_MODERATOR_HANDLE`
- `WOLFBBS_BOOTSTRAP_MODERATOR_PASSWORD`
- `WOLFBBS_BOOTSTRAP_USER_HANDLE`
- `WOLFBBS_BOOTSTRAP_USER_PASSWORD`

When handle+password pairs are set, startup seeds or updates those users and roles.

## SSH ANSI Runtime, Menu, and Access Controls

- `WOLFBBS_TERM_ENCODING`: terminal encoding hint (`utf-8`, `cp437`, etc.).
- `WOLFBBS_THEME_FILE`: optional HJSON theme pack file loaded by SSH/web services.
- `WOLFBBS_MCI_SETTINGS_FILE`: optional HJSON MCI template used by the SSH settings screen.
- `WOLFBBS_MENU_ENABLE` (default `false`): load HJSON menu.
- `WOLFBBS_MENU_FILE` (default `menus/main.hjson` when enabled): menu file path.
- `WOLFBBS_MENU_ROOT` (default `menus`): menu editor root directory for `/admin/config`.
- `WOLFBBS_ACS_STRICT` (default `false`): deny on ACS parse errors.
- `WOLFBBS_ACS_BOARDS_READ`: ACS expression for board list/read access.
- `WOLFBBS_ACS_BOARDS_POST`: ACS expression for new/reply posting access.
- `WOLFBBS_ACS_MAIL_READ`: ACS expression for mailbox access.
- `WOLFBBS_ACS_MAIL_SEND`: ACS expression for mail compose/send.
- `WOLFBBS_ACS_FILES_READ`: ACS expression for files menu access.
- `WOLFBBS_ACS_ADMIN`: optional ACS expression for `/admin/*` access in addition to role checks.
- `WOLFBBS_APP_UPGRADE_COMMAND`: optional sysop-triggered command for SSH quick jump `/app upgrade`.
- `WOLFBBS_APP_UPGRADE_WORKDIR`: optional working directory for the upgrade command.
- `WOLFBBS_APP_UPGRADE_TIMEOUT_SECONDS` (default `900`, min `15`, max `3600`): timeout for `/app upgrade`.

Sysop menu editing:
- Use `/admin/config` to edit HJSON menus with parse validation and safe-path checks.
- Menu runtime settings are persisted as `menu.enabled` and `menu.file` system settings.

## Discoverability and Modern On-Ramp Flags

- `WOLFBBS_GUEST_TOUR_ENABLE` (default `false`): enable `GUEST` login path + `/tour` page.
- `WOLFBBS_SMART_NEWSCAN_ENABLE` (default `true`): digest-based since-last-call path.
- `WOLFBBS_SMART_NEWSCAN_MAX` (default `12`, capped at `30`): digest item cap.
- `WOLFBBS_DISCOVER_ENABLE` (default `false`): enable `/discover` web feature.
- `WOLFBBS_WEB_ONRAMP_ENABLE` (default `false`): redirect signed-out root to `/connect`.
- `WOLFBBS_WS_TERMINAL_URL` (fallback aliases `WOLFBBS_WS_URL`, `WOLFBBS_WSS_URL`): URL shown/used by connect page JS terminal.
- `WOLFBBS_AI_ASSIST_ENABLE` (default `false`): include labeled AI catch-up line in digest outputs.

## File/Web Gateway and Offline Reading

- `WOLFBBS_OFFLINE_DIR` (default `.wolfbbs/offline` in app, `/app/.wolfbbs/offline` in compose env): offline gateway save root.

Web gateway fetch hard limits are currently code defaults in `gateway.DefaultFetchConfig`:

- timeout `10s`
- max bytes `2 MiB`
- allowlist content-types: `text/html`, `text/plain`
- SSRF denylist: localhost, RFC1918, link-local, etc.

## Email Gateway

Supported prefixed and unprefixed aliases are both accepted:

- `WOLFBBS_SMTP_HOST` / `SMTP_HOST`
- `WOLFBBS_SMTP_PORT` / `SMTP_PORT` (default `587`)
- `WOLFBBS_SMTP_USER` / `SMTP_USER`
- `WOLFBBS_SMTP_PASS` / `SMTP_PASS`
- `WOLFBBS_FROM_DOMAIN` / `FROM_DOMAIN`

Safety limits:

- `WOLFBBS_MAIL_MAX_RECIPIENTS` (default `3`)
- `WOLFBBS_MAIL_MAX_BYTES` (default `65536`)
- `WOLFBBS_MAIL_RATE_PER_HOUR` (default `20`)

Inbound handling:

- `WOLFBBS_INBOUND_TOKEN`: required shared token for inbound endpoint/adaptor.
- `WOLFBBS_MAILIN_ALLOW_DOMAINS`: comma-separated sender allowlist.
- `WOLFBBS_MAILIN_FORWARD_URL`: URL used by `cmd/wolfbbs-mailin` when relaying to web.

## Doors and Plugins

Door registration and runtime:

- `WOLFBBS_TRIVIA_BINARY` (default `wolfbbs-trivia`): sample external door command.
- `WOLFBBS_DOORS`: semicolon-separated door entries (`HOTKEY|NAME|COMMAND|arg1,arg2`).
- `WOLFBBS_DOOR_ALLOW_DIR`: optional allowlist root for external door commands.
- `WOLFBBS_DOOR_DATA_ROOT` (default `.wolfbbs/doors`): per-door writable data root.
- `WOLFBBS_DOOR_DROPFILES`: comma-separated legacy dropfile types (`door32`, `doorsys`, `dorinfo`).
- `WOLFBBS_ORACLE_DAILY_PROMPTS` (default `10`): daily prompt cap for oracle door.

Bridge connectors:

- `WOLFBBS_DOORPARTY_ENABLE`, `WOLFBBS_DOORPARTY_COMMAND`, `WOLFBBS_DOORPARTY_ARGS`
- `WOLFBBS_BBSLINK_ENABLE`, `WOLFBBS_BBSLINK_COMMAND`, `WOLFBBS_BBSLINK_ARGS`
- `WOLFBBS_TELNET_BRIDGE_ENABLE`, `WOLFBBS_TELNET_BRIDGE_COMMAND`, `WOLFBBS_TELNET_BRIDGE_ARGS`

### Door Environment Exports (runtime-generated)

These are generated by the door runtime and passed into external/connector processes:

- `WOLFBBS_MODE` (`door` or `connector`)
- `WOLFBBS_CONNECTOR`
- `WOLFBBS_DOOR_ID`
- `WOLFBBS_DOOR_DATA_DIR`
- `WOLFBBS_DOOR_NETWORK`
- `WOLFBBS_DOOR32_SYS`
- `WOLFBBS_DOOR_SYS`
- `WOLFBBS_DORINFO_DEF`
- `WOLFBBS_USER_ID`
- `WOLFBBS_HANDLE`
- `WOLFBBS_ROLE`
- `WOLFBBS_USER_CREATED_AT`
- `WOLFBBS_NODE`
- `WOLFBBS_SESSION_ID`
- `WOLFBBS_AREA`
- `WOLFBBS_TERM_COLS`
- `WOLFBBS_TERM_ROWS`
- `WOLFBBS_ANSI`
- `WOLFBBS_ENCODING`
- `WOLFBBS_COLOR_DEPTH`
- `WOLFBBS_TZ`

## Login Servers (Telnet / WS / WSS)

- `WOLFBBS_TELNET_ENABLE` (default `false`)
- `WOLFBBS_TELNET_LISTEN` (default `:2323`)
- `WOLFBBS_WS_ENABLE` (default `false`)
- `WOLFBBS_WS_LISTEN` (default `:6080`)
- `WOLFBBS_WS_PATH` (default `/ws-login`)
- `WOLFBBS_WSS_ENABLE` (default `false`)
- `WOLFBBS_WSS_LISTEN` (default `:6443`)
- `WOLFBBS_WSS_PATH` (default `/ws-login`)
- `WOLFBBS_WSS_CERT`
- `WOLFBBS_WSS_KEY`
- `WOLFBBS_TRUSTED_PROXIES`: comma-separated trusted CIDRs for forwarded IP parsing.

## Message Network Hooks (FTN/BSO/QWK External Bridge)

- `WOLFBBS_NET_SPOOL_DIR` (default `.wolfbbs/network/spool`): packet spool root used by import/export processing and external hooks.
- `WOLFBBS_NET_IMPORT_CMD`: external tosser import command executed by `oputil network sync-in`.
- `WOLFBBS_NET_EXPORT_CMD`: external tosser export command executed by `oputil network sync-out`.
- `WOLFBBS_NET_BOARD_ROUTES`: optional board routing map (`key=id`) for inbound packets (supports `board`, `conference`, `conference/board` keys).
- `WOLFBBS_NET_HANDLE_ROUTES`: optional netmail handle alias map (`remote=local`) for inbound handle resolution.

Runtime-only env exported by WolfBBS when executing external hook commands:

- `WOLFBBS_NET_MODE`: `import` or `export`.
- `WOLFBBS_NET_SPOOL_DIR`: resolved spool directory path.

## IRC Server Settings

- `WOLFBBS_IRC_TLS_LISTEN`: optional TLS bind.
- `WOLFBBS_IRC_TLS_CERT`
- `WOLFBBS_IRC_TLS_KEY`
- `WOLFBBS_IRC_MAX_CONN` (default `400`)
- `WOLFBBS_IRC_MAX_CONN_PER_IP` (default `6`)
- `WOLFBBS_IRC_MSG_WINDOW_MS` (default `1500`)
- `WOLFBBS_IRC_MSG_BURST` (default `8`)
- `WOLFBBS_IRC_IP_MSG_WINDOW_MS` (default `2000`)
- `WOLFBBS_IRC_IP_MSG_BURST` (default `20`)
- `WOLFBBS_IRC_POLL_MS` (default `1000`)
- `WOLFBBS_IRC_REQUIRE_PASS` (default `true`)
- `WOLFBBS_IRC_ALLOW_ALIAS` (default `false`)
- `WOLFBBS_IRC_BANNED_IPS`: comma-separated IP/CIDR denylist.

## Shared Chat Service Settings

- `WOLFBBS_CHAT_HISTORY_LIMIT` (default `200`)
- `WOLFBBS_CHAT_RATE_BURST` (default `8`)
- `WOLFBBS_CHAT_RATE_WINDOW_MS` (default `1500`)
- `WOLFBBS_CHAT_RETENTION_HOURS` (default `168`)
- `WOLFBBS_CHAT_POLL_INTERVAL_MS` (default `1000`)

## Logging and Observability

- `WOLFBBS_LOG_FORMAT` (default `bunyan`): `bunyan`, `json`, or `text`.
- `WOLFBBS_LOG_LEVEL` (default `info`): `debug`, `info`, `warn`, `error`.

`bunyan` format emits Bunyan-compatible JSON logs and wraps stdlib logs in that same shape for web, IRC, mail-in, and `oputil`.

## Content Servers and Federation

- `WOLFBBS_CONTENT_HOST` (default `localhost`): host used in gopher links.
- `WOLFBBS_GOPHER_LISTEN`: optional gopher listener address.
- `WOLFBBS_NNTP_LISTEN`: optional NNTP listener address.
- `WOLFBBS_NNTPS_LISTEN`: optional NNTPS listener address.
- `WOLFBBS_NNTPS_CERT`
- `WOLFBBS_NNTPS_KEY`
- `WOLFBBS_ACTIVITYPUB_ENABLE` (default `false`): enable experimental ActivityPub webfinger/actor/outbox/inbox endpoints.
- `WOLFBBS_ACTIVITYPUB_BASE_URL`: canonical ActivityPub base URL.

## Runtime Config Loader

- `WOLFBBS_CONFIG_FILE`: optional runtime HJSON config file path used by `internal/config`.

## Sysop-Persisted Runtime Settings

The web sysop panel stores these keys in DB-backed `system_settings`:

- `site.motd`
- `site.announcement`
- `site.read_only`
- `site.web_onramp_enable`
- `site.guest_tour_enable`
- `site.discover_enable`
- `chat.locked_channels` (comma-separated channels)

## Installer and Bootstrap Control

- `WOLFBBS_REPO_URL`: fallback repo URL consumed by `install.sh --repo-url`.

## Notes on Defaults

- Source-of-truth defaults come from code when env values are unset.
- `.env.example` documents recommended deployment defaults, which may intentionally differ from internal fallback defaults.
- When both prefixed and unprefixed SMTP vars are set, prefixed aliases are checked with the same precedence path as code (`envFirst`).
