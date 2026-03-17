ACCEPTANCE SPEC v1.0 (Feature-Complete + Linux + macOS Installer)
Last updated: 2026-02-23
Owner intent: This spec defines “feature complete” and provides concrete checks. Codex must implement the product so it passes all MUST items, and create an automated verifier.

============================================================
0) DEFINITIONS / DEFAULTS
============================================================
D-001 (MUST) Default ports (unless overridden by .env or installer flags):
  - SSH_PORT default 2222
  - WEB_PORT default 8080
  - IRC_PORT default 6667
  - IRC_TLS_PORT default 6697 (if TLS supported)

D-002 (MUST) “Stack” means:
  - SSH BBS service (ANSI UI over SSH)
  - Web service (Admin + Web Chat)
  - IRC endpoint (mIRC-compatible subset)
  - Postgres database
  - Redis optional (if used for pubsub/rate limits)

D-003 (MUST) “Feature complete” means all MUST requirements in sections 1–12 pass in Docker install mode on:
  - Linux (at least one supported distro)
  - macOS 12+ (Monterey or later) on Intel or Apple Silicon

D-004 (MUST) Manual checks are allowed only where explicitly labeled “MANUAL”.
      All other MUST checks must be automatable by scripts/verify.sh.

============================================================
1) REPO STRUCTURE / REQUIRED FILES
============================================================
R-001 (MUST) README exists and includes:
  - Quick install command(s) for Linux and macOS
  - Endpoints summary: SSH, Web Admin, Web Chat, IRC
  Check:
    test -f README.md

R-002 (MUST) Compose file exists at repo root:
  - docker-compose.yml OR compose.yml
  Check:
    test -f docker-compose.yml || test -f compose.yml

R-003 (MUST) .env.example exists with required variables documented
  Check:
    test -f .env.example

R-004 (MUST) docs directory exists with required docs:
  - docs/architecture.md
  - docs/threat-model.md
  - docs/INSTALL.md
  - docs/admin.md
  - docs/chat.md
  - docs/irc-compat.md
  - docs/email-gateway.md (can document inbound as optional)
  - docs/web-gateway.md
  - docs/ACCEPTANCE_SPEC.md (this document)
  Check:
    test -d docs &&
    test -f docs/architecture.md &&
    test -f docs/threat-model.md &&
    test -f docs/INSTALL.md &&
    test -f docs/admin.md &&
    test -f docs/chat.md &&
    test -f docs/irc-compat.md &&
    test -f docs/email-gateway.md &&
    test -f docs/web-gateway.md &&
    test -f docs/ACCEPTANCE_SPEC.md

R-005 (MUST) Verifier script exists:
  - scripts/verify.sh (runs on Linux + macOS)
  Check:
    test -f scripts/verify.sh

R-006 (MUST) Installer support for BOTH Linux and macOS:
  - Either:
      (A) install.sh supports both Linux+macOS
    OR
      (B) install.sh (Linux) AND install-macos.sh (macOS)
  Check:
    test -f install.sh && ( ./install.sh --help | grep -qi macos || test -f install-macos.sh )

============================================================
2) DOCKER COMPOSE CONTRACT
============================================================
C-001 (MUST) docker compose config is valid
  Check:
    docker compose config >/dev/null

C-002 (MUST) Compose defines a Postgres service with a persistent volume
  Check (heuristic):
    docker compose config | grep -qi postgres
    AND
    docker compose config | grep -qi "volumes:"

C-003 (MUST) Compose exposes SSH port (default 2222) from some service
  Check (heuristic):
    docker compose config | grep -Eiq "2222:|SSH_PORT"

C-004 (MUST) Compose exposes WEB port (default 8080) from some service
  Check:
    docker compose config | grep -Eiq "8080:|WEB_PORT"

C-005 (MUST) Compose exposes IRC port (default 6667) from some service
  Check:
    docker compose config | grep -Eiq "6667:|IRC_PORT"

C-006 (SHOULD) Compose exposes IRC TLS port (default 6697) from some service
  Check:
    docker compose config | grep -Eiq "6697:|IRC_TLS_PORT"

C-007 (MUST) At least web and db have healthchecks in compose
  Check (heuristic):
    docker compose config | grep -qi "healthcheck"

C-008 (MUST) Stack starts from clean checkout:
  Check:
    docker compose up -d --build

============================================================
3) WEB HEALTH / READINESS ENDPOINTS (REQUIRED FOR VERIFICATION)
============================================================
H-001 (MUST) Web provides GET /healthz returning HTTP 200
  Check:
    curl -fsS "http://localhost:${WEB_PORT:-8080}/healthz" >/dev/null

H-002 (MUST) Web provides GET /readyz returning HTTP 200 once DB reachable
  Check:
    curl -fsS "http://localhost:${WEB_PORT:-8080}/readyz" >/dev/null

H-003 (SHOULD) /metrics exists (Prometheus text format)
  Check:
    curl -fsS "http://localhost:${WEB_PORT:-8080}/metrics" >/dev/null

============================================================
4) SSH BBS CORE (ANSI NOSTALGIA)
============================================================
BBS-001 (MUST) SSH port is open after stack start
  Check:
    nc -z localhost "${SSH_PORT:-2222}"

BBS-002 (MUST) SSH handshake works (host key presented)
  Check:
    ssh-keyscan -p "${SSH_PORT:-2222}" localhost >/dev/null

BBS-003 (MUST, MANUAL) ANSI flow exists and matches docs/screens.md:
  - Welcome -> Login -> Main Menu -> Boards/Mail/Files/Chat/Gateways/Settings/Sysop
  Manual check:
    ssh localhost -p 2222 and verify screen flow.

BBS-004 (MUST, MANUAL) User registration + login works

BBS-005 (MUST, MANUAL) Message boards:
  - list boards
  - list messages
  - read message
  - post new thread
  - reply to thread

BBS-006 (MUST, MANUAL) Private mail:
  - inbox/outbox
  - send to another local user
  - read/reply/forward/delete (forward optional if documented)

BBS-007 (MUST, MANUAL) “New scan since last login” exists

BBS-008 (MUST, MANUAL) Paging (“More”) works on long content

BBS-009 (SHOULD, MANUAL) Per-user ANSI on/off toggle exists

============================================================
5) WEB ADMIN (“SYSOP PANEL”) COMPLETENESS
============================================================
WA-001 (MUST) /admin requires auth
  Check:
    curl -sI "http://localhost:${WEB_PORT:-8080}/admin" | head -n 1 | grep -Eiq "302|401|403"

WA-002 (MUST) RBAC roles exist and enforced (at least: user, moderator, admin)
  Evidence:
    docs/admin.md describes roles + enforcement
  Check (doc presence):
    test -f docs/admin.md

WA-003 (MUST, MANUAL) User management:
  - list/search users
  - disable/ban/unban
  - force password reset (or admin reset)
  - view user audit/activity summary

WA-004 (MUST, MANUAL) Board management:
  - create/edit/delete boards
  - per-board permissions

WA-005 (MUST, MANUAL) Moderation:
  - delete post (with reason)
  - lock thread
  - move thread between boards
  - moderation queue or report system (one of these is required)

WA-006 (MUST, MANUAL) Gateway settings pages exist for:
  - email gateway (smtp relay + limits)
  - web gateway (timeouts/limits/ssrf protection settings)

WA-007 (MUST, MANUAL) Chat admin tools:
  - create/lock channels
  - kick/ban/mute users
  - view chat logs with retention policy

WA-008 (MUST, MANUAL) Admin actions produce an audit log entry visible in UI

WA-009 (SHOULD, MANUAL) Optional 2FA (TOTP) for admin accounts

============================================================
6) UNIFIED CHAT (BBS + WEB + IRC ALL SHARE ONE BACKEND)
============================================================
CH-001 (MUST) Default channel exists (e.g. #lobby)
  Evidence:
    docs/chat.md documents default channel(s)

CH-002 (MUST) Web chat UI exists at /chat
  Check:
    curl -sI "http://localhost:${WEB_PORT:-8080}/chat" | head -n 1 | grep -Eiq "200|302"

CH-003 (MUST, MANUAL) Realtime web chat works (WebSocket or SSE):
  - Two browser sessions exchange messages instantly in #lobby

CH-004 (MUST, MANUAL) Message history persists across reload and supports pagination

CH-005 (MUST, MANUAL) Moderation actions apply consistently across clients:
  - mute/ban/kick reflected for BBS users and IRC users

CH-006 (MUST) Rate limiting / anti-flood exists at the backend layer
  Evidence:
    docs/chat.md documents rate limits + enforcement points

============================================================
7) IRC ENDPOINT (mIRC-COMPATIBLE SUBSET)
============================================================
IRC-001 (MUST) IRC TCP port open
  Check:
    nc -z localhost "${IRC_PORT:-6667}"

IRC-002 (SHOULD) IRC TLS port open (if supported)
  Check:
    nc -z localhost "${IRC_TLS_PORT:-6697}" || true

IRC-003 (MUST) Authentication required (no anonymous joins)
  Evidence:
    docs/irc-compat.md documents auth method (PASS or SASL)
  Check (automated via scripted test):
    scripts/test_irc_login.sh (or .py/.js) must fail unauth JOIN and pass auth JOIN

IRC-004 (MUST) Must support these commands:
  - NICK, USER, PASS (or SASL PLAIN), JOIN, PART, PRIVMSG, NOTICE, QUIT, PING/PONG
  Evidence:
    docs/irc-compat.md lists supported commands

IRC-005 (MUST) Must support basic: TOPIC, NAMES, LIST, WHO, WHOIS
  Evidence:
    docs/irc-compat.md lists supported commands

IRC-006 (MUST) Must implement core numeric replies so mIRC behaves normally:
  - 001-004 (welcome)
  - 375/372/376 (MOTD)
  - 353/366 (NAMES)
  - common errors: 401, 403, 421, 431, 432, 433, 451, 461, 462
  Check:
    scripted IRC test asserts it receives at least: 001 and MOTD end (376) after auth

IRC-007 (MUST, MANUAL) Cross-client bridging:
  - message from IRC appears in Web chat in same channel
  - message from Web chat appears in IRC
  - same for BBS chat (if BBS chat UI exists)

IRC-008 (MUST) Anti-flood enforced on IRC endpoint
  Check:
    scripted test sends burst; expect throttle/kick or numeric error per docs

============================================================
8) GATEWAYS
============================================================
GW-EMAIL-001 (MUST) Outbound email uses configured SMTP relay (no custom MTA)
  Evidence:
    docs/email-gateway.md shows SMTP_* env vars and behavior

GW-EMAIL-002 (MUST) Outbound protections:
  - verified-account gate OR explicit admin permission
  - per-user rate limit
  - max recipients
  - max message size
  Evidence:
    docs/email-gateway.md

GW-EMAIL-003 (MUST, MANUAL) Admin can disable outbound for a user

GW-EMAIL-004 (SHOULD) Inbound email path documented (implementation optional)
  Check:
    grep -qi "inbound" docs/email-gateway.md

GW-WEB-001 (MUST, MANUAL) Text web gateway fetches URL and renders readable text in BBS UI

GW-WEB-002 (MUST) Web gateway enforces timeout + max response size
  Evidence:
    docs/web-gateway.md lists default limits and config variables

GW-WEB-003 (MUST) SSRF protection blocks localhost/RFC1918/link-local by default
  Evidence:
    docs/web-gateway.md lists blocked ranges and denies by default
  Check (if HTTP API exists for gateway; otherwise MANUAL in BBS):
    Attempt fetch http://127.0.0.1 should be blocked.

GW-WEB-004 (MUST, MANUAL) Save for offline reading exists; content persists

============================================================
9) SECURITY + OPS
============================================================
SEC-001 (MUST) Password hashing uses a modern algorithm (argon2/bcrypt/scrypt)
  Evidence:
    docs/threat-model.md states algorithm; code uses it.

SEC-002 (MUST) Web session cookies are httpOnly and secure in production; CSRF protection if cookie auth
  Evidence:
    docs/threat-model.md and docs/admin.md

SEC-003 (MUST) Structured logs exist; admin actions audited
  Evidence:
    docs/threat-model.md and runtime logs

SEC-004 (MUST) No default hardcoded admin credentials
  Evidence:
    docs/INSTALL.md explains first-run admin creation or env-provided bootstrap token

SEC-005 (MUST) .env permissions set to 600 after installer run
  Check:
    (after install) stat -c "%a" .env 2>/dev/null | grep -qx "600" || stat -f "%Lp" .env | grep -qx "600"

SEC-006 (MUST) Installer does NOT silently modify firewall rules
  Evidence:
    docs/INSTALL.md explicitly states it will not open firewall ports

SEC-007 (SHOULD) Login rate limiting exists (web + ssh)
  Evidence:
    docs/threat-model.md

============================================================
10) INSTALLERS (LINUX + macOS REQUIRED)
============================================================
INS-001 (MUST) --help exists and lists flags
  Check:
    ./install.sh --help >/dev/null

INS-002 (MUST) Non-interactive mode:
  - supports --yes or --non-interactive
  Check:
    ./install.sh --dry-run --yes >/dev/null

INS-003 (MUST) Dry run:
  - prints intended actions
  - makes no changes
  Check:
    ./install.sh --dry-run --yes | grep -Eiq "would|plan|dry"

INS-004 (MUST) Idempotent:
  - running twice does not fail; does not overwrite config unless --force
  Check:
    ./install.sh --dry-run --yes && ./install.sh --dry-run --yes

INS-005 (MUST) Required flags implemented:
  - --prefix <dir>
  - --ssh-port <port>
  - --web-port <port>
  - --irc-port <port>
  - --upgrade
  - --status
  - --uninstall
  - --force
  Check:
    ./install.sh --help | grep -Eiq "--prefix" &&
    ./install.sh --help | grep -Eiq "--ssh-port" &&
    ./install.sh --help | grep -Eiq "--web-port" &&
    ./install.sh --help | grep -Eiq "--irc-port" &&
    ./install.sh --help | grep -Eiq "--upgrade" &&
    ./install.sh --help | grep -Eiq "--status" &&
    ./install.sh --help | grep -Eiq "--uninstall" &&
    ./install.sh --help | grep -Eiq "--force"

INS-006 (MUST) Secrets generated and .env created with chmod 600
  Check (after real install):
    test -f .env && (stat -c "%a" .env 2>/dev/null | grep -qx "600" || stat -f "%Lp" .env | grep -qx "600")

INS-007 (MUST) Default install mode uses Docker Compose:
  - installs/validates docker and compose plugin or prints explicit instructions and exits non-zero
  Evidence:
    docs/INSTALL.md

INS-008 (MUST) Post-install verification:
  - checks /healthz and required ports, fails loudly if unhealthy
  Evidence:
    install logs show checks; or rerunnable verify command exists

INS-009 (MUST) Prints “How to connect” summary:
  - ssh command
  - admin URL
  - chat URL
  - IRC host/ports
  Check (heuristic after install):
    install output contains "ssh" AND "/admin" AND "/chat" AND "IRC"

---------------------------
Linux-specific
---------------------------
INS-LNX-001 (MUST) Detect distro + package manager (apt/dnf/pacman)
  Evidence:
    install logs show detection

INS-LNX-002 (MUST) Installs prereqs or clearly instructs (curl, git, openssl, docker)
  Evidence:
    docs/INSTALL.md + installer output

INS-LNX-003 (MUST) Supports curl|bash bootstrap
  Evidence:
    docs/INSTALL.md provides one-liner; installer supports running outside repo by cloning.

---------------------------
macOS-specific (REQUIRED)
---------------------------
INS-MAC-001 (MUST) Detect macOS (Darwin path)
  Check:
    (On macOS) ./install.sh --dry-run --yes does not error on OS detection

INS-MAC-002 (MUST) macOS entrypoint exists:
  - install.sh supports macOS OR install-macos.sh exists
  Check:
    ./install.sh --help | grep -qi macos || test -f install-macos.sh

INS-MAC-003 (MUST) Homebrew handling:
  - detects brew
  - optionally installs brew only with explicit consent (or with --yes --install-brew)
  Evidence:
    docs/INSTALL.md and installer flags

INS-MAC-004 (MUST) Detects Xcode Command Line Tools and instructs installation if missing
  Evidence:
    installer checks xcode-select -p

INS-MAC-005 (MUST) Detects Docker availability:
  - supports Docker Desktop OR Colima (documented)
  - if missing, prints clear instructions and exits non-zero
  Evidence:
    docs/INSTALL.md

INS-MAC-006 (MUST) Avoid GNU-only tool assumptions:
  - works with BSD sed/grep
  Check:
    shellcheck passes and verify runs on macOS

INS-MAC-007 (MUST) Default prefix on macOS does not require root:
  - default to something under $HOME (e.g. $HOME/.local/share/<app>)
  Evidence:
    docs/INSTALL.md and installer --help text

============================================================
11) AUTOMATED VERIFICATION HARNESS (REQUIRED)
============================================================
VER-001 (MUST) scripts/verify.sh exists, executable, and runs on Linux + macOS
  Check:
    test -x scripts/verify.sh || chmod +x scripts/verify.sh

VER-002 (MUST) verify prints PASS/FAIL per requirement ID and exits non-zero if any MUST fails
  Check:
    scripts/verify.sh --fast ; echo $?
    Output includes "PASS R-001" style lines

VER-003 (MUST) verify supports at least two modes:
  - --fast  : no docker up; only static checks (files, help flags, shellcheck, compose config if docker present)
  - --smoke : docker compose up -d --build then checks health + ports
  Check:
    scripts/verify.sh --help shows these

VER-004 (MUST) verify can run an IRC scripted test (can be python/go/node):
  - connect
  - authenticate
  - join #lobby
  - send a message
  - assert a numeric welcome and NAMES reply
  Check:
    scripts/verify.sh --smoke includes this step OR documents how to run scripts/test_irc.*

============================================================
12) CI REQUIREMENTS (REQUIRED)
============================================================
CI-001 (MUST) CI runs bash syntax + shellcheck on installer(s)
  Evidence:
    .github/workflows/* exists and contains bash -n + shellcheck steps

CI-002 (MUST) CI runs scripts/verify.sh --fast
  Evidence:
    workflow includes it

CI-003 (SHOULD) CI runs smoke test (docker compose up + /healthz) if runner allows
  Evidence:
    optional job exists

============================================================
IMPLEMENTATION INSTRUCTIONS FOR CODEX
============================================================
- Create docs/ACCEPTANCE_SPEC.md with this content verbatim.
- Implement scripts/verify.sh to automate all non-MANUAL MUST checks above.
- For MANUAL checks, verify.sh should print "MANUAL <ID> - SKIPPED" and not fail.
- verify.sh must be portable across Linux and macOS (BSD/GNU differences handled).
- Add CI that runs shellcheck + verify.sh --fast on pull requests.
- Ensure installer(s) and the application satisfy every MUST requirement in this spec.
