# WolfBBS Datasheet

## Product Summary

WolfBBS is a self-hosted bulletin board system that combines a classic ANSI caller experience with a modern web companion, IRC bridge, admin control plane, doors, and packaged deployment.

## Product Type

- `Category`: self-hosted community platform / BBS
- `Primary access`: SSH ANSI/TUI
- `Secondary access`: web companion
- `Additional access`: IRC bridge
- `Deployment model`: Docker Compose

## Key Benefits

| Benefit | Detail |
| --- | --- |
| Fast to deploy | Paste-and-run bootstrap installer or release tarball install |
| Nostalgic by design | Classic menu flow, message boards, private mail, doors, bulletins, operator screens |
| Modern enough to operate | Web admin, diagnostics, password reset, health endpoints, packaging, upgrade tooling |
| Multi-surface community | SSH, web, and IRC share the same board and chat system |
| Operator-friendly | Setup wizard, config center, audit, repair, lifecycle commands |

## Major Capabilities

### Caller Features

- ANSI login and main menu
- guest tour
- message boards and replies
- private mail
- bulletins
- file areas and download queue
- live chat
- door launching
- who’s online
- last callers
- scoreboards and trophies
- personal settings

### Sysop And Moderator Features

- setup wizard
- config center
- users and roles
- board management
- file area management
- gateway controls
- chat moderation
- door administration
- system dashboard
- audit view
- runtime diagnostics

### Web Companion Features

- browser login
- password reset
- boards, mail, bulletins, directory, finder, new files
- clubhouse, radar, scores, discover, help
- chat with lobby and channels
- admin panels for all major subsystems

## Included Runtime Components

| Binary | Purpose |
| --- | --- |
| `wolfbbs` | SSH ANSI BBS service |
| `wolfbbs-web` | web companion and admin surface |
| `wolfbbs-irc` | IRC bridge service |
| `wolfbbs-mailin` | inbound mail adapter |
| `wolfbbs-trivia` | bundled trivia door |
| `oputil` | sysop CLI |

## Protocols And Default Ports

| Surface | Default |
| --- | --- |
| SSH | `2222` |
| Web | `8080` |
| IRC | `6667` |
| IRC TLS | `6697` |
| Mail ingestion adapter | `8091` |

## Installation And Lifecycle

### Supported installation paths

- bootstrap installer from GitHub
- guided installer from cloned repo
- packaged release tarballs

### Operator lifecycle commands

- `bash install.sh --status`
- `bash install.sh --doctor`
- `bash install.sh --repair`
- `bash install.sh --upgrade`
- `bash install.sh --rapid-upgrade`
- `bash install.sh --uninstall`

## Package Contents

Release bundles include:

- platform binaries
- `install.sh`
- `bootstrap.sh`
- `docker-compose.yml`
- `.env.example`
- product docs
- validation and build helpers
- checksums and release manifest

## Deployment Requirements

### Runtime assumptions

- Docker and Docker Compose support
- outbound network access for dependency bootstrap and container pulls
- available ports for SSH, web, IRC, and supporting services

### Supported host targets

- Linux
  - Debian or Ubuntu
  - Fedora, RHEL, CentOS class systems
  - Arch
- macOS
  - Intel and Apple Silicon
  - Docker Desktop or Colima path

## Security And Operations Notes

- session cookies are `HttpOnly` with strict same-site behavior
- CSRF protection is enabled on mutating web routes
- admin writes are auditable
- optional TOTP is available for sysop accounts
- release bundles are checksummed

## Current Boundaries

- native non-Docker deployment is not the supported product path
- ActivityPub support is limited and not a full federation implementation
- product scope is centered on self-hosted BBS/community operation, not general social networking

## Canonical Resources

- Repo: [Awassee/wolfbbs](https://github.com/Awassee/wolfbbs)
- Releases: [GitHub Releases](https://github.com/Awassee/wolfbbs/releases)
- Start here: [START_HERE.md](START_HERE.md)
- Quickstart: [QUICKSTART.md](QUICKSTART.md)
- Install guide: [INSTALL.md](INSTALL.md)
- Operations guide: [OPERATIONS.md](OPERATIONS.md)
- Product guide: [PRODUCT_GUIDE.md](PRODUCT_GUIDE.md)
