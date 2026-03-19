# UX/Product Next 10 (Round 3, Implemented)

This spec defines the next 10 major features delivered in order for the shared web shell in `cmd/wolfbbs-web/main.go`.

## 1) Route Revisit Intelligence
- Problem: users lose continuity when returning to a route.
- Feature: revisit banner showing elapsed time since last visit.
- Acceptance:
  - Stores last visit timestamp per route.
  - Shows contextual elapsed message.
  - Supports dismiss action.

## 2) KPI Delta Indicators
- Problem: KPI cards show absolute numbers but not movement.
- Feature: delta badges on KPI cards versus prior route snapshot.
- Acceptance:
  - Persists prior KPI snapshot by route.
  - Shows up/down/flat indicator.
  - Updates snapshot after each render.

## 3) Inline Glossary Enhancer
- Problem: domain terms are unclear for newer operators/callers.
- Feature: automatic `<abbr>` tooltips for key product terms.
- Acceptance:
  - Enhances `p` and `li` nodes without server-side template changes.
  - Avoids duplicate reprocessing.
  - Preserves original content flow.

## 4) Undoable Preference Actions
- Problem: UI preference tweaks were risky and hard to revert.
- Feature: client-side undo stack for UI control actions.
- Acceptance:
  - Tracks recent preference changes.
  - One-click undo from preference controls.
  - Shows feedback when stack is empty.

## 5) Notification Center with History
- Problem: transient toasts disappear and cannot be reviewed.
- Feature: toast history center with clear function.
- Acceptance:
  - Persists recent toasts locally.
  - Opens/reads/clears historical notifications.
  - Tracks interaction telemetry.

## 6) Workspace Hub + Handoff Export
- Problem: operators lack reusable context bundles for sessions.
- Feature: save/open/delete workspaces and export handoff package.
- Acceptance:
  - Saves workspace with route set.
  - Supports open/delete workflow.
  - Exports handoff markdown + JSON payload.

## 7) Spotlight Search Overlay
- Problem: long pages are hard to scan quickly.
- Feature: global spotlight overlay to highlight in-page matches.
- Acceptance:
  - Ctrl/Cmd+Shift+F shortcut opens spotlight.
  - Highlights matching content nodes.
  - Persists last query.

## 8) Session Checkpoint Hub
- Problem: users cannot quickly restore a known-good local UI state.
- Feature: checkpoint capture/restore/delete for local session state.
- Acceptance:
  - Saves named checkpoints with payload snapshot.
  - Restores preferences/favorites/notes/focus/spotlight state.
  - Supports delete and telemetry hooks.

## 9) Advanced Table Workflow Pack
- Problem: table workflows were limited to filter/sort/export-all.
- Feature: selected-row export, JSON copy, and column visibility toggles.
- Acceptance:
  - Ctrl/Cmd+click row selection.
  - Export selected rows to CSV.
  - Copy visible rows as JSON.
  - Toggle column visibility from toolbar panel.

## 10) Section Flow Controls + Progress
- Problem: long multi-section pages are hard to navigate as a task.
- Feature: section nav progress counter with collapse/expand-all controls.
- Acceptance:
  - Displays viewed sections count.
  - Supports collapse-all and expand-all actions.
  - Uses intersection observer for active-section tracking.

## Validation
- Hook coverage test: `cmd/wolfbbs-web/main_test.go`
- End-to-end checks run through project verification harnesses.
