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
- `area` is last context (`Welcome`, `Login`, `Main Menu`, `Internet Tools`, etc.).

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
│                           /\_/\                                          │
│                          / o o \                                         │
│                         (   "   )                                        │
│                          \~(*)~/                                         │
│                           // \\                                           │
├──────────────────────────────────────────────────────────────────────────┤
│ wolfbbs (c) 2026                                                        │
│ Node-ready ANSI board with classic flow and modern plumbing.            │
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
┌════════════════════════ WolfBBS Main Menu ═══════════════════════┐
│ Pick the job you want to do. Help is always one key away.       │
│                                                                  │
│ == Start Here ==                                                 │
│ [N] What's New  [M] Read Boards  [C] Chat Rooms  [P] Private Mail│
│                                                                  │
│ == Talk + Read ==                                                │
│ Public messages, personal mail, live rooms, downloads, games,   │
│ and internet tools.                                             │
│ [M] Read Boards      [P] Private Mail    [C] Chat Rooms          │
│ [F] Files & Downloads [D] Games & Doors [G] Internet Tools       │
│                                                                  │
│ == Track + Return ==                                             │
│ Catch up, see people, save packets, and revisit highlights.      │
│ [N] What's New       [R] My Activity     [L] Recent Callers      │
│ [W] Who's Here Now   [O] Offline Packets [V] Showcase Tour       │
│                                                                  │
│ == Personal + System ==                                          │
│ Adjust your experience, inspect board info, or sign off.         │
│ [S] My Settings      [X] Board Info      [Y] System Status       │
│ [/] Find a Feature   [A] Sysop Center    [Q] Sign Off            │
│                                                                  │
│ == Popular Places ==                                             │
│ collections  bookmarks  circles  events  challenges  digest-prefs│
└──────────────────────────────────────────────────────────────────┘

Enter selection:
```

### Quick jump deck
```text
┌──────────────────────── Quick Jump Deck ────────────────────────┐
│ Use Find a Feature when you know the job but not the menu key.  │
│                                                                  │
│ == Talk + Read ==                                                │
│ boards/messages   read public message boards                     │
│ mail/private      check private mail                             │
│ chat/rooms        jump into live chat rooms                      │
│ files/downloads   browse file areas and tickets                  │
│ doors/games       play games and utilities                       │
│ gateway/internet  web, email, feeds, JSON, AI                    │
│                                                                  │
│ == Follow-up + Utility ==                                        │
│ collections       curated file bundles                           │
│ offline/packets   save packets or import replies                 │
│ bookmarks         personal quick links                           │
│ circles           caller groups and tags                         │
│ settings          theme, pager, clock, exports                   │
│ showcase/tour     guided feature tour                            │
│ statusz/config    board or runtime snapshots                     │
│                                                                  │
│ == Pulse + Community ==                                          │
│ pulse/activity    streaks, missions, next actions                │
│ events/recaps     scheduled community events                     │
│ challenges        seasonal challenge board                       │
│ spotlights        feature highlights                             │
│ digest-prefs      weekly digest choices                          │
│                                                                  │
│ Prompt shown below: Feature or place                             │
│ Press Enter on blank input to cancel.                            │
└──────────────────────────────────────────────────────────────────┘
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

### Internet tools
```text
┌──────────────────── Internet Tools ─────────────────────┐
│ [W]eb browser   - read a web page and save it for later│
│ [E]mail gateway - send outside email through SMTP      │
│ [F]eed reader   - compact RSS/Atom headlines           │
│ [S]ummarizer    - quick bullets from an article URL    │
│ [J]SON explorer - readable JSON API responses          │
│ [X] My activity - streaks, events, challenges          │
│ [A]I assistant  - ask the configured AI helper         │
│ [R]eturn [Q]uit [?]Help                                │
└────────────────────────────────────────────────────┘
```

### Games & Doors
```text
┌────────────────────────── Games & Doors ────────────────────────┐
│ [R]eturn [Q]uit [!]Favorite Toggle [?]Help [T]Trophies          │
│ Favorites: SPACE-TRADER-WARS, PIT-ARENA                         │
│ Recent: DRAGON-TAVERN-LEGENDS, FISHING-DERBY                    │
│                                                                  │
│ HK  Category   Door Name                          Turns  Flags   │
│ ────────────────────────────────────────────────────────────────  │
│ A   STRATEGY   Space Trader Wars                  34     *       │
│ B   RPG        Dragon Tavern Legends              39             │
│ K   ARCADE     The Pit Arena                      18     *       │
│ X   UTILITY    FileBase Pro                        -             │
└──────────────────────────────────────────────────────────────────┘
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

### Context Help (new `?` panels)
```text
┌──────────────────── Help: Main Menu ─────────────────────┐
│ Main Menu Key Guide                                      │
│                                                           │
│ If you are new here:                                      │
│ N Start with what's new since your last visit             │
│ M Read boards        C Join chat rooms                    │
│ P Check private mail Q Sign off when done                 │
│                                                           │
│ Talk + Read                                               │
│ M Read Boards           P Private Mail                    │
│ C Chat Rooms            F Files & Downloads               │
│ D Games & Doors         G Internet Tools                  │
│                                                           │
│ Track + Return                                            │
│ N What's New            R My Activity                     │
│ L Recent Callers        W Who's Here Now                  │
│ O Offline Packets       V Showcase Tour                   │
│                                                           │
│ Personal + System                                         │
│ S My Settings           X Board Info                      │
│ Y System Status         A Sysop/Admin                     │
│ / Find a Feature prompt Q Quit to sign-off                │
│                                                           │
│ Press any key to return.                                  │
└───────────────────────────────────────────────────────────┘
```

### Live chat desk
```text
┌───────────────────────── Live Chat ──────────────────────────────┐
│ == Chat Command Bar ==                                           │
│ [1-9] Switch room  [S] Send  [J] Join/open  [L] Leave current   │
│ [O] Online roster  [R] Refresh  [?] Help  [Q] Return            │
│                                                                   │
│ Current room: #lobby   Open rooms: 2   Online here: 4   Mode: open│
│ Room guide: Main lobby for general chat, greetings, and quick    │
│ social check-ins.                                                │
│                                                                   │
│ == Open Rooms ==                                                  │
│ 1) #lobby     current | 4 live     sysop: welcome back           │
│ 2) #ansi      joined  | 2 live     caller: ansi art tonight      │
│ 3) #trade     watch   | 18:42      hatch: fsx upload open        │
│                                                                   │
│ == Transcript ==                                                  │
│ 18:41   sysop      welcome back to the lobby                     │
│ 18:42   caller     swapping to #ansi for screenshots             │
│                                                                   │
│ Use J to open another room without losing the ones you joined.   │
└───────────────────────────────────────────────────────────────────┘
Selection:
```

### My Settings
```text
┌────────────────────────── My Settings ───────────────────────────┐
│ WolfBBS personal settings                                        │
│                                                                   │
│ Color + ANSI            On                                         │
│ Output mode             Auto detect                                │
│ Pause on long screens   On                                         │
│ Theme list              Midnight ANSI                              │
│ Clock style             24-hour                                    │
│                                                                   │
│ U Output mode  B Bookmarks   O Circles   X Profile export         │
│ E Attention export  T Theme  A Color  P Pager  C Clock           │
│ S Save My Settings      Q Return                                 │
└───────────────────────────────────────────────────────────────────┘
```

### Welcome back notice
```text
┌────────────────────────── Welcome Back ───────────────────────────┐
│ It looks like your last session ended before you signed off.     │
│                                                                   │
│ Last place: Chat Rooms                                            │
│ Disconnected: 2026-03-20 21:15                                    │
│ Visit length: 12m                                                 │
│                                                                   │
│ If the terminal looked strange before disconnecting, open         │
│ My Settings and switch Output mode to Plain text safe mode.       │
│                                                                   │
│ Press any key to return.                                          │
└───────────────────────────────────────────────────────────────────┘
```

### Guest Tour
```text
┌────────────────────── Guest Tour ──────────────────────┐
│ Read-only guided tour. No posting in this mode.        │
│                                                         │
│ Last callers:                                           │
│ - Node 01 byteforge   02-27 10:14                      │
│ - Node 03 shells      02-27 10:08                      │
│                                                         │
│ One-liners (#lobby):                                    │
│ - [10:15] sysop: Welcome to WolfBBS.                   │
│                                                         │
│ Featured thread: General / Build status and node logs  │
│ Today's download pick: FileBase Pro door (X).          │
│                                                         │
│ Press any key to return to login.                       │
└─────────────────────────────────────────────────────────┘
```

```text
┌────────────────── Help: Message Boards ─────────────┐
│ Boards Navigation                                    │
│                                                      │
│ Board list: enter ID or Q to return                 │
│ Inside board: N New, R Read, Q Quit                 │
│ Reader: R Reply, N/P Next/Prev, Q Exit              │
│                                                      │
│ Paging: Space/Enter continue, Q/Esc abort            │
│                                                      │
│ Press any key to return.                             │
└──────────────────────────────────────────────────────┘
```

## 3) Keybindings and Input Notes

- `↑ ↓ ← →` -> `UP`, `DOWN`, `LEFT`, `RIGHT` (menu/list movement)
- `PgUp / PgDn` -> page movement in readers/pagers
- `Home / End` -> cursor/end-of-line support in future text inputs
- `Enter` -> accept selection / confirm
- `Esc` / `Q` -> back/cancel
- `Ctrl+C` -> abort flow
- `?` -> help
- `!` -> toggle a favorite in Games & Doors
- `T` -> scores/trophies screen in Games & Doors
- `SPACE` -> pager continue

Notes:
- Non-printable control bytes should be ignored unless explicitly mapped.
- Keep reads forgiving with immediate redraw and small buffers for sync-safe clients.
