# UI/UX Pass: 20 Major Improvements (Round 2)

This pass defines and implements 20 additional major UX improvements in order, focused on accessibility, operator productivity, data workflows, and keyboard-first control.

## 1) Live Accessibility Announcer
- Adds ARIA live region for toast/action announcements.
- Acceptance: screen-reader friendly status updates.

## 2) Focus UI Mode
- Toggle to hide nonessential guidance rails for deep work.
- Acceptance: persistent on/off mode.

## 3) Palette Query History
- Stores and reuses command palette search history.
- Acceptance: history chips shown in palette header.

## 4) Network Status Chip
- Real-time online/offline state in hero.
- Acceptance: updates on online/offline events.

## 5) Latency Probe Chip
- Periodic `/healthz` RTT display in hero.
- Acceptance: updates every 60s, handles failures.

## 6) Auto-Refresh Control
- Optional periodic refresh for ops/data pages.
- Acceptance: persisted toggle, safe interval guard.

## 7) Section Batch Controls
- Expand-all / collapse-all from section nav.
- Acceptance: applies to all collapsible route sections.

## 8) Section Progress Tracker
- Shows how many sections were viewed this session.
- Acceptance: intersection-observer based progress count.

## 9) Guide Strip Minimize
- Hide/show primer guidance panel per route.
- Acceptance: persisted minimize state.

## 10) Shortcut Legend Overlay
- Global keyboard shortcut cheat sheet overlay.
- Acceptance: opened by Ctrl/Cmd+Shift+/ and closed with Esc.

## 11) Table Column Visibility Panel
- Show/hide columns without backend changes.
- Acceptance: checkbox controls in table toolbar.

## 12) Table JSON Copy
- Copy currently visible table rows as JSON.
- Acceptance: clipboard copy success/failure feedback.

## 13) Text Counters
- Adds live character counters to text inputs/areas.
- Acceptance: supports max length and free-form modes.

## 14) Required-Field Submit Guard
- Client-side required/invalid summary before busy state.
- Acceptance: focuses first invalid field and reports reason.

## 15) Telemetry Hero Chip
- Visible count of UX interactions in current browser profile.
- Acceptance: updates on telemetry event writes.

## 16) Quick Diagnostics Export Button
- One-click UI diagnostics JSON export button.
- Acceptance: exports prefs/timer/favorites/notes/telemetry.

## 17) Telemetry Event Bus Sync
- Dispatches and listens for telemetry update events.
- Acceptance: same-tab telemetry visual updates without reload.

## 18) Macro Route Jumps (Alt+1..6)
- Direct keyboard route jumps for core caller flow.
- Acceptance: ignored while typing in form controls.

## 19) Macro Help Trigger (Alt+0 / F1)
- Dedicated keyboard help quick access.
- Acceptance: modal open with route-agnostic help content.

## 20) Accessible Toast Announcements
- Toasts now also announce via live region.
- Acceptance: visible + assistive feedback parity.
