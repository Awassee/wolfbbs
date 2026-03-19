# UI/UX Pass: 20 Major Improvements (Round 3)

This pass defines and implements 20 additional major UX improvements in order for the shared modern web shell in `cmd/wolfbbs-web/main.go`.

## 1) Layout Width Modes
- Adds `standard`, `wide`, and `focus` layout modes.
- Acceptance: width mode is toggleable from hero controls and persisted.

## 2) Accent Tone Modes
- Adds `blue`, `teal`, and `amber` accent palettes.
- Acceptance: accent mode cycles in hero controls and updates CSS variables.

## 3) Motion Reduction Toggle
- Adds runtime motion preference (`full` vs `reduced`).
- Acceptance: reduced mode disables animations/transitions via body class.

## 4) Session Duration Hero Chip
- Shows live elapsed session time.
- Acceptance: chip updates every second and survives route changes in-session.

## 5) Session Trail Hero Chip
- Shows count of route hops in current session.
- Acceptance: hop count and path trail are persisted and reflected in hero chip.

## 6) Route Pinning Controls
- Adds pin/unpin current route action.
- Acceptance: pinned routes persist locally and are available to quick-launch surfaces.

## 7) Route URL Copy Action
- Adds one-click route copy action in hero controls.
- Acceptance: copies full route URL and shows toast feedback.

## 8) Context Help Drawer
- Adds route-aware help overlay with focused action links.
- Acceptance: openable from hero controls and renders route-specific guidance.

## 9) Quick Launch Dock
- Adds fixed dock with route actions, pinned routes, and recent hops.
- Acceptance: supports refresh, collapse, and persisted collapse state.

## 10) Section Pin Rail
- Adds per-section pin buttons on H2 headings.
- Acceptance: pinned section links render in a dedicated quick-return rail.

## 11) Section Completion Toggles
- Adds “mark done” toggles on sections.
- Acceptance: completion state persists per route and updates heading state.

## 12) Section Reading Time Indicators
- Adds estimated minutes per section to section navigation links.
- Acceptance: each section nav item shows a `Xm` estimate.

## 13) Copy-All Section Links
- Adds one-click copy of all section anchor URLs.
- Acceptance: copies newline-delimited section links to clipboard.

## 14) Keyboard Section Navigation
- Adds `Alt+J` / `Alt+K` section navigation.
- Acceptance: jumps to next/previous section and updates hash/scroll state.

## 15) Sticky Table Header Toggle
- Adds per-table sticky-header control.
- Acceptance: toggles sticky table head class and persists table state.

## 16) Compact Table Row Toggle
- Adds per-table compact-row rendering control.
- Acceptance: toggles compact density class and persists table state.

## 17) Row Inspector Panel
- Adds click-to-inspect panel for table rows.
- Acceptance: clicking a row renders key/value breakdown from visible columns.

## 18) Form Completion Meter
- Adds required-field progress meter for forms with required inputs.
- Acceptance: meter updates live as required fields are completed.

## 19) Dirty-Form Title Indicator
- Adds tab-title unsaved indicator (`●`) when form state is dirty.
- Acceptance: title badge appears on edits and clears on submit/clean state.

## 20) Bug Report Capture Overlay
- Adds route-aware bug capture with copy/download workflow.
- Acceptance: capture includes route/UI/telemetry/notification context and supports JSON export.

## Validation
- `go test ./...`
- `scripts/run-e2e.sh --no-go --no-tui --web-timeout 900`
- `scripts/verify.sh --fast`
- `scripts/verify.sh --smoke`
- `scripts/manual-acceptance.sh --auto --no-smoke --report docs/manual-acceptance-latest.md`
