# WolfBBS Showcase

Use this page for a quick product walk-through when introducing WolfBBS on GitHub, social posts, or release notes.

Launch links:
- [Install Guide](INSTALL.md)
- [Start Here](START_HERE.md)
- [Feature Datasheet](DATASHEET.md)
- [Documentation Hub](README.md)
- [Release v2.0.2 notes](releases/v2.0.2.md)
- [GitHub Releases](https://github.com/Awassee/wolfbbs/releases)

## Release v2.0.2 Snapshot

This showcase tracks the `v2.0.2` distribution target: SSH-first caller flow, full admin parity, modern gateway surfaces, turnkey install/upgrade lifecycle, and the calmer 2.0 web shell with grouped controls and compacted nav rails ready for public GitHub delivery.

## Product Description

WolfBBS is a self-hosted community platform that blends a classic ANSI BBS experience with modern browser and operations tooling.

- `Classic caller feel`: SSH-first menus, message boards, private mail, doors, scores, and newscan
- `Modern access`: browser companion, admin console, account recovery, and live chat with a lower-noise shared shell
- `Cross-client community`: IRC bridge with shared channel state
- `Operator-ready`: setup wizard, config center, diagnostics, upgrades, and repair workflows

## Gallery Wall

<table>
  <tr>
    <td width="50%"><a href="assets/screenshots/connect.png"><img src="assets/screenshots/connect.png" alt="Connect hub"></a></td>
    <td width="50%"><a href="assets/screenshots/boards.png"><img src="assets/screenshots/boards.png" alt="Message boards"></a></td>
  </tr>
  <tr>
    <td width="50%"><a href="assets/screenshots/chat.png"><img src="assets/screenshots/chat.png" alt="Live chat"></a></td>
    <td width="50%"><a href="assets/screenshots/doors.png"><img src="assets/screenshots/doors.png" alt="Doors"></a></td>
  </tr>
  <tr>
    <td width="50%"><a href="assets/screenshots/today.png"><img src="assets/screenshots/today.png" alt="Today Brief"></a></td>
    <td width="50%"><a href="assets/screenshots/admin-setup.png"><img src="assets/screenshots/admin-setup.png" alt="Admin setup"></a></td>
  </tr>
</table>

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
