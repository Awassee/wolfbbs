# WolfBBS Help Guides

This file is the written reference for in-product help surfaces.

## Scope

- SSH ANSI contextual `?` help panels
- Web `/help` hub and inline help links
- Shared key conventions for boards/mail/chat/gateway/doors

## Global Conventions

- `?` opens contextual help for the current SSH screen.
- `Q` and `Esc` return to the previous screen.
- Single-letter commands are case-insensitive.
- Status bar remains visible on all SSH help screens.
- Prompts use classic wording: `Enter selection`, `Press any key to continue`.

## Login Help

- At SSH login handle prompt, enter `?` to open login help.
- `GUEST` enters read-only guided tour mode when enabled.
- Unknown handle + valid password input can trigger account creation flow.
- Optional TOTP 2FA challenge appears after password for 2FA-enabled users.

## Main Menu Help

- `M` Message Boards
- `P` Private Mail
- `F` Files
- `C` Chat
- `G` Gateways
- `D` Doors
- `N` Newscan digest
- `S` Settings
- `A` Sysop/Admin notice
- `L` Last Callers
- `W` Who's Online
- `X` Config Center
- `Y` Status Center
- `/` Quick Jump (opt-in)
  - Sysop quick command: `/app upgrade` (runs configured upgrade hook)
- `Q` Sign-off

## Message Boards Help

- Board list: enter a board ID or `Q` to return.
- Inside board:
  - `N` New post
  - `R` Read messages
  - `S` Search messages (classic list, opt-in)
  - `Q` Exit board
- Reader:
  - `R` Reply
  - `N` / `Enter` / `Right` / `PgDn` Next message
  - `P` / `Left` / `PgUp` Previous message
  - `Q` / `Esc` Exit reader

## Private Mail Help

- `C` Compose
- `R` Read mail by ID
- `Q` Return to main menu
- Recipient can be local handle or external email (policy-controlled).
- Message body entry ends with a line containing only `.`.

## Files Help

- From Files menu:
  - Enter area ID to browse that area
  - `R` recent files across all areas
  - `N` files newer than your last login
  - `S` search filenames
  - `Q` return
- Inside a file area:
  - `S` set/clear filename filter
  - `R` refresh listing
  - `Q` exit area

## Live Chat Help

- `S` Send message
- `J` Join channel
- `O` Show online users
- `R` / `Enter` Refresh
- `Q` / `Esc` Return
- Default channel: `#lobby`

## Gateway Help

- `E` Email gateway (SMTP relay; verified-account policy can apply)
- `W` Text web gateway (timeout, size cap, SSRF protections)
- `R` / `Q` / `Esc` Return

## Guest Tour

- At login prompt, type `GUEST` for read-only guided tour mode.
- Tour highlights:
  - Last callers
  - One-liners from `#lobby`
  - Featured thread
  - Daily download pick prompt

## Smart Newscan / Discover

- `N` in main menu opens `Since your last call` digest.
- Rules are transparent:
  - replies to your posts
  - mentions of your handle
  - per-board new traffic
  - inbox mail since last login
- Optional AI assist line is labeled with `[AI-LABEL]` and disabled by default.

## Doors Help

- Door hotkey launches selected door.
- `!` toggles favorite by door hotkey.
- `T` opens Scores and Trophies.
- `Q` / `R` / `Esc` returns to main menu.
- Turn limits and time-bank rules are per-door.

## Settings Help

- `T` cycle theme
- `A` toggle ANSI
- `P` toggle pager
- `C` toggle 24-hour clock
- `S` save
- `Q` / `Esc` return

## Web Help Hub

- Route: `/help`
- Accessible from login and authenticated pages.
- Contains:
  - terminal key quick guide
  - web route map
  - admin route map
  - setup/config/error panel pointers (`/admin/setup`, `/admin/config`, `/admin/errors`)
  - pointers to docs in `docs/`

## Inline Help Availability Checklist

- SSH Login: yes (`?` at handle prompt)
- SSH Main menu: yes (`?`)
- SSH Boards: yes (`?`)
- SSH Mail: yes (`?`)
- SSH Chat: yes (`?`)
- SSH Gateway: yes (`?`)
- SSH Doors: yes (`?`)
- SSH Settings: yes (`?`)
- Web companion pages: yes (header/footer links to `/help`)
- Admin pages: yes (links to `/help`)
