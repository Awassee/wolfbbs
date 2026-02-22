# WolfBBS Screen Language

This document defines the ANSI screen conventions for the SSH UI.  
All layouts are original and intentionally inspired by late-80s/90s Wildcat-era board flow without reusing any existing dumps.

## 1) Screen Style Guide

### Target terminal
- Primary target: `80x25` layout.
- If terminal is wider, the full 80-column UI is centered and the rest is left as margin.
- If terminal is narrower, clipping/degradation is preferred over wrapping UI geometry.
- Session loop should always start from:
  - `top status bar`
  - `content frame`
  - `footer prompt` when waiting for input

### Status bar format
Format:  
`WolfBBS | Node 01 | User: <handle> | <YYYY-MM-DD HH:MM> | <area>`

Behavior:
- Always visible on interactive screens.
- Uses high-contrast colors (`StatusBg + StatusFg` in UI theme).
- `area` is last context (`Welcome`, `Login`, `Main Menu`, `Gateways`, etc.).

### Box drawing and visual motifs
Preferred palette: Unicode CP437-style line glyphs.

Default top/bottom border characters:
- `┌ ┐ └ ┘`
- Horizontal: `─`
- Vertical: `│`
- Junctions: `├ ┤ ┬ ┴`

ASCII-safe fallback:
- `+` `+` `+` `+`
- Horizontal: `-`
- Vertical: `|`
- Junctions: `+`

CP437 mapping (for classic feel; we emit Unicode glyphs in ANSI output by default):
- `0xDA` → `┌`
- `0xBF` → `┐`
- `0xC0` → `└`
- `0xD9` → `┘`
- `0xC4` → `─`
- `0xB3` → `│`
- `0xC3` → `├`
- `0xB4` → `┤`
- `0xC2` → `┬`
- `0xC1` → `┴`

### Theme strategy
- Themes are map-style via `Theme` values in `internal/ui`.
- Keep color usage minimal:
  - `body`: cyan (or amber/green classically)
  - `accent`: yellow
  - `warnings/errors`: red/magenta
  - `status`: white on blue
- Theme names can be added later (`retro-amber`, `teal`, `monochrome`).

### Paging behavior
- Long output is split into pages and ends in footer with:
  - `-- More --`
- Controls:
  - `SPACE` or `ENTER`: next page
  - `Q` / `ESC`: abort pager
  - `Ctrl-C`: abort current flow
- Footer prompt should always be centered and visibly distinct from content.

### Hotkey conventions
- Primary actions: single uppercase hotkeys.
- Escape conventions:
  - `Esc` or `Q` -> return/exit screen
  - `?` -> contextual help
- Common phrases:
  - “Enter selection”
  - “Press any key to continue”
  - “(Q)uit to previous menu”
- Keep prompts dense, no multi-key chords for core navigation.

### Screen language primitives
`internal/ui` currently provides:
- `ClearScreen()`
- `MoveCursor(row,col)`
- `DrawBox(width,height,title,content,borderSet,fg,bg)`
- `CenterText(width,text)` and footer helper
- `Color(fg,bg,body)`
- ANSI color constants (`Fg*`, `Bg*`)

## 2) ASCII Mockups

### Welcome / splash
```text
┌──────────────────────────────────────────────────────────────────────────┐
│   ██╗    ██╗██╗    ██╗███████╗██████╗ ██████╗ ███████╗               │
│   ██║    ██║██║    ██║██╔════╝██╔══██╗██╔══██╗██╔════╝               │
│   ██║ █╗ ██║██║ █╗██║███████╗██████╔╝██████╔╝███████╗               │
│   ██║███╗██║██║███╗██║╚════██║██╔═══╝ ██╔══██╗╚════██║               │
│   ╚███╔███╔╝╚███╔███╔╝███████║██║     ██████╔╝███████║               │
│    ╚══╝╚══╝  ╚══╝╚══╝ ╚══════╝╚═╝     ╚═════╝ ╚══════╝               │
├──────────────────────────────────────────────────────────────────────────┤
│ WolfBBS  -  modern ANSI terminal board. Keep it fast, keep it clean.      │
├──────────────────────────────────────────────────────────────────────────┤
│ Press ESC to quit, any other key to continue.                          │
└──────────────────────────────────────────────────────────────────────────┘
```

### Login
```text
┌─────────────────── WolfBBS Login ────────────────────┐
│ Node: 01                                            │
│ Terminal: 80x25                                     │
└──────────────────────────────────────────────────────┘

Handle: _____________________________
Password: __________________________
```

### New user registration
```text
┌─────────────── New User Registration ────────────────┐
│ Enter desired handle (3-20 chars, unique)            │
│ Password (min 8 chars):                             │
│ Re-type password:                                    │
└──────────────────────────────────────────────────────┘

[N]ew account [Q]uit
```

### Bulletin list
```text
┌───────────────────── Bulletins ─────────────────────┐
│ 1) System announcements                            │
│ 2) Sysop notes                                     │
│ 3) Feature updates                                 │
│                                                   │
│ Enter selection:                                   │
└────────────────────────────────────────────────────┘
```

### Main menu
```text
┌════════════════════ WolfBBS Main Menu ════════════┐
│ [M]essage Boards   [P]rivate Mail   [F]iles     │
│ [C]hat             [G]ateways       [S]ettings  │
│ [D]oors            [A]dmin          [L]ast Callers│
│ [W]ho's Online     [Q]uit                        │
└───────────────────────────────────────────────────┘

Enter selection:
```

### Message board list
```text
┌──────────────────── Message Boards ───────────────────┐
│  1) General                                   120   │
│  2) Tech & Systems                             78   │
│  3) Local Topics                               46   │
│                                                  │
│ [S]elect  [N]ext  [P]revious  [Q]uit            │
└────────────────────────────────────────────────────┘
```

### Message reader
```text
┌────────────────── Message Reader ───────────────────┐
│ 17) Re: New node script for board automation        │
│ From: sysop        Time: 2026-02-22 09:01:12       │
├────────────────────────────────────────────────────┤
│> We need a minimal parser for ANSI state output...    │
│> with robust arrow handling and pager behavior.      │
├────────────────────────────────────────────────────┤
│ (R)eply  (N)ext  (P)rev  (Q)uit  (?)-help       │
└────────────────────────────────────────────────────┘
-- More --
```

### Post editor (compose / reply + quote)
```text
┌───────────── Post Editor (Compose / Reply) ──────────────┐
│ Subject: Re: New node script for board automation        │
│ Recipient: #general                                     │
├───────────────────────────────────────────────────────────┤
│ > We need a minimal parser for ANSI state output.        │
│ > and it should support page navigation.                │
│                                                        │
│                                                        │
│ [Ctrl+Q] quote   [Ctrl+S] send   [Esc] cancel         │
└───────────────────────────────────────────────────────────┘
```

### Gateway menu
```text
┌──────────────────── Gateway Menu ────────────────────┐
│ [E]mail Gateway - send BBS mail to external SMTP   │
│ [W]eb Gateway   - read and trim HTML pages         │
│ [O]ffline Save  - stash pages for later          │
│ [B]ack                                            │
└────────────────────────────────────────────────────┘
```

### Last Callers
```text
┌─────────────── Last Callers / Traffic ───────────────┐
│ Node User        Login Time    Area        Idle      │
│ 01  riker       10:12         Main        00:14:22  │
│ 07  byteforge   09:44         Chat        00:02:17  │
│ 12  shells      09:33         Boards      00:43:05  │
└──────────────────────────────────────────────────────┘
```

### Who’s Online
```text
┌──────────────── Who’s Online / Presence ───────────────┐
│ Node User        Login Time   Area           Idle    │
│ 01   riker       12:03       Main Lobby      00:01 │
│ 07   byteforge   12:15       Messages        00:00 │
└───────────────────────────────────────────────────────┘
```

## 3) Keybindings and Input Notes

- `↑ ↓ ← →` -> `UP`, `DOWN`, `LEFT`, `RIGHT` (menu/list movement)
- `PgUp / PgDn` -> page movement in readers/pagers
- `Home / End` -> cursor/end-of-line support in future text inputs
- `Enter` -> accept selection / confirm
- `Esc` / `Q` -> back/cancel
- `Ctrl+C` -> abort flow
- `?` -> help
- `SPACE` -> pager continue

Notes:
- Non-printable control bytes should be ignored unless explicitly mapped.
- Keep reads forgiving with immediate redraw and small buffers for sync-safe clients.
