# WolfBBS Datasheet

WolfBBS is a self-hosted, SSH-first BBS platform that combines classic ANSI caller flow with a modern web companion, IRC bridge, sysop control plane, and turnkey installer lifecycle.

## At A Glance

| Attribute | Value |
| --- | --- |
| Product category | Self-hosted BBS / community platform |
| Release target | `v2.1.7` (current public release tag) |
| Primary UX | SSH ANSI/TUI caller experience |
| Secondary UX | Web companion for callers and operators with a grouped, lower-noise shell |
| Additional protocol | IRC bridge with shared chat layer |
| Deployment model | Docker Compose (default supported path) |
| Install options | One-line bootstrap, repo installer, release tarball |
| Typical first-run path | `/admin/setup` -> `/admin/launch` -> SSH + `/boards` + `/chat` + `/doors` |

## Highlighted Feature Matrix

| Feature area | Highlighted features | Practical value |
| --- | --- | --- |
| Caller experience | ANSI menu flow, threaded boards, private mail, doors, score surfaces | Delivers the nostalgic board feel users expect |
| Daily retention loop | `/today`, `/attention`, `/events`, `/challenges`, `/clubhouse` | Gives callers a clear reason to return each day |
| Live social layer | Web chat + IRC bridge + presence signals | Keeps conversation active across client preferences |
| Operator launch workflow | Setup wizard, launch center, config center, status center | Reduces first-run ambiguity and misconfiguration |
| Community operations | Users/roles, board admin, chat moderation, bulletin and event management | Supports real moderation and content cadence |
| Reliability and lifecycle | `install.sh` doctor/repair/upgrade/uninstall + packaged releases | Makes operation viable for non-developer sysops |
| Distribution and onboarding | Bootstrap installer, release artifacts, docs hub, showcase pages | Lowers adoption friction for new operators |
| Security controls | CSRF-protected mutating routes, strict cookie profile, optional TOTP, audit visibility | Improves default safety for self-hosted communities |

## Screenshot-Backed Surfaces

| Surface | Screenshot | Why it matters |
| --- | --- | --- |
| Connect hub | [connect.png](assets/screenshots/connect.png) | Single place for SSH/web/IRC onboarding |
| Message boards | [boards.png](assets/screenshots/boards.png) | Core long-form community interaction |
| Live chat | [chat.png](assets/screenshots/chat.png) | Real-time social loop (shared with IRC) |
| Door cockpit | [doors.png](assets/screenshots/doors.png) | Replay loop, score chase, and game depth |
| Today Brief | [today.png](assets/screenshots/today.png) | Daily dashboard for callers/operators |
| Admin setup | [admin-setup.png](assets/screenshots/admin-setup.png) | Guided launch-readiness workflow |
| Admin config | [admin-config.png](assets/screenshots/admin-config.png) | Runtime feature and service controls |
| Mobile connect | [connect-mobile.png](assets/screenshots/connect-mobile.png) | Usable onboarding on phones/tablets |

Full gallery: [SHOWCASE.md](SHOWCASE.md)

## Role-Based Value

| Role | Core routes | Value outcome |
| --- | --- | --- |
| Sysop | `/admin/setup`, `/admin/launch`, `/admin/ops`, `/admin/config` | Faster, safer launch and routine operations |
| Moderator | `/chat`, `/boards`, `/admin/chat`, `/admin/users` | Direct moderation and user support workflow |
| Caller | SSH, `/today`, `/boards`, `/chat`, `/doors`, `/scores` | Engaging classic flow with modern convenience |
| Visitor | `/connect`, `/tour`, `/help` | Understand the product quickly before signup |

## Technical Profile

### Runtime binaries

| Binary | Purpose |
| --- | --- |
| `wolfbbs` | SSH ANSI BBS runtime |
| `wolfbbs-web` | Web caller + admin surfaces |
| `wolfbbs-irc` | IRC bridge service |
| `wolfbbs-mailin` | Inbound mail adapter |
| `wolfbbs-trivia` | Bundled trivia door |
| `oputil` | Sysop/operator CLI utility |

### Protocol and default port map

| Protocol/surface | Default |
| --- | --- |
| SSH caller service | `2222` |
| Web companion/admin | `8080` |
| IRC | `6667` |
| IRC TLS (optional) | `6697` |
| Mail ingestion adapter | `8091` |

### Deployment assumptions

- Docker and Docker Compose available on host
- outbound network access for container pulls and bootstrap dependencies
- host ports available for configured SSH/web/IRC endpoints

Supported host targets:
- Linux: Debian/Ubuntu, Fedora/RHEL/CentOS-class, Arch
- macOS: Intel + Apple Silicon (Docker Desktop or Colima path)

## Installation and Lifecycle Summary

Install paths:
- One-line bootstrap from GitHub (`bootstrap.sh`)
- Guided installer from cloned repo (`install.sh`)
- Packaged release tarball + local `install.sh`

Common operator commands:
- `bash install.sh --status`
- `bash install.sh --doctor`
- `bash install.sh --repair`
- `bash install.sh --upgrade`
- `bash install.sh --rapid-upgrade`
- `bash install.sh --uninstall --purge --yes`

## Security and Operations Notes

| Control | Current behavior |
| --- | --- |
| Session cookies | `HttpOnly`, strict same-site profile |
| CSRF | Enabled for mutating web routes |
| 2FA | Optional TOTP for operator accounts |
| Auditability | Admin writes and operational events are auditable |
| Artifact integrity | Release bundles include checksums/manifests |

## Product Boundaries

- Native non-Docker deployment is not the primary supported product path.
- ActivityPub support is present but not a complete federation implementation.
- Product scope is focused on self-hosted BBS/community operation, not broad social-network parity.

## Canonical Resources

- Repository: [Awassee/wolfbbs](https://github.com/Awassee/wolfbbs)
- Releases: [GitHub Releases](https://github.com/Awassee/wolfbbs/releases)
- Start here: [START_HERE.md](START_HERE.md)
- Quickstart: [QUICKSTART.md](QUICKSTART.md)
- Install guide: [INSTALL.md](INSTALL.md)
- Product guide: [PRODUCT_GUIDE.md](PRODUCT_GUIDE.md)
- Showcase: [SHOWCASE.md](SHOWCASE.md)
- Documentation hub: [README.md](README.md)
