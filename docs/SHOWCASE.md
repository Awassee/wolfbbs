# WolfBBS Showcase

Use this page for a quick product walk-through when introducing WolfBBS on GitHub, social posts, or release notes.

## Product Description

WolfBBS is a self-hosted community platform that blends a classic ANSI BBS experience with modern browser and operations tooling.

- `Classic caller feel`: SSH-first menus, message boards, private mail, doors, scores, and newscan
- `Modern access`: browser companion, admin console, account recovery, and live chat
- `Cross-client community`: IRC bridge with shared channel state
- `Operator-ready`: setup wizard, config center, diagnostics, upgrades, and repair workflows

## Screenshot Tour

### Connect hub

![WolfBBS Connect hub](assets/screenshots/connect.png)

Highlights:
- copy-ready connection commands
- browser terminal entrypoint
- first-call checklist for new users

### Message boards

![WolfBBS Message Boards](assets/screenshots/boards.png)

Highlights:
- board filters and caller cockpit
- watch/digest style follow modes
- fast path into threads and posting

### Live chat

![WolfBBS Live Chat](assets/screenshots/chat.png)

Highlights:
- real-time room timeline
- shared moderation model
- IRC bridge compatibility

### Doors and score surfaces

![WolfBBS Doors](assets/screenshots/doors.png)

Highlights:
- curated door directory
- score and challenge-oriented discovery
- replay-friendly game hub

### Today Brief

![WolfBBS Today Brief](assets/screenshots/today.png)

Highlights:
- daily operator/caller summary
- watch and digest board rollups
- quick glance for return users

### Admin setup and config

![WolfBBS Admin Setup](assets/screenshots/admin-setup.png)
![WolfBBS Admin Config](assets/screenshots/admin-config.png)

Highlights:
- launch-time identity and safety setup
- runtime feature controls
- operations-oriented settings layout

### Mobile connect page

![WolfBBS Connect mobile](assets/screenshots/connect-mobile.png)

Highlights:
- compact handheld layout
- same connect instructions and workflows

## Re-generate Screenshots

From repo root:

```bash
scripts/capture-doc-screenshots.sh
```

This starts a local Playwright-driven web instance and refreshes `docs/assets/screenshots/*.png`.
