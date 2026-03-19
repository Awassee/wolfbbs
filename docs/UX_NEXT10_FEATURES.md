# UX/Product Next 10 (Implemented)

This spec defines the next 10 major features prioritized for caller/sysop productivity and trust.
All ten are implemented in the shared web UX shell (`cmd/wolfbbs-web/main.go`) so they apply across routes.

## 1) Route Goal Coach
- Problem: users land on pages without a concrete completion path.
- Feature: route-specific checklist panel with progress and reset.
- Acceptance:
  - Shows 3-4 tasks for mapped routes.
  - Progress persists locally per route.
  - Can reset checklist state.

## 2) Saved Table Views
- Problem: operators repeatedly reapply table filters/sorts.
- Feature: save/restore per-table filter and sort state.
- Acceptance:
  - Each table supports save and restore view actions.
  - Restored state reapplies search + sort.
  - Works per route/table index.

## 3) Quick Notes Workspace
- Problem: no place to park transient operational notes.
- Feature: floating notes drawer with copy/download/clear.
- Acceptance:
  - Notes persist locally.
  - Supports copy and JSON/text export.
  - Opens from global UI button.

## 4) Route Readiness Scorecard
- Problem: users cannot quickly gauge page actionability.
- Feature: page-level scorecard (actions/data/guidance).
- Acceptance:
  - Computes score from visible route signals.
  - Renders near hero area.
  - Provides compact interpretation text.

## 5) Smart Empty-State Actions
- Problem: empty pages often dead-end users.
- Feature: auto-attach “next move” links to detected empty states.
- Acceptance:
  - Detects common “No ... yet” states.
  - Adds route-relevant action chips.
  - Does not alter existing server content.

## 6) Keyboard Macro Layer
- Problem: power users need faster keyboard movement.
- Feature: Alt+1..6 macro navigation and help overlay.
- Acceptance:
  - Shortcut opens common routes.
  - Dedicated shortcut help panel exists.
  - Does not trigger while typing in inputs.

## 7) Focus Session Timer
- Problem: long admin sessions drift without pacing.
- Feature: configurable focus timer with visual countdown.
- Acceptance:
  - Start/pause/reset controls.
  - Remaining time visible in UI.
  - Local state persistence.

## 8) Compose Template Injector
- Problem: repetitive message/event drafting overhead.
- Feature: contextual templates insert for compose textareas.
- Acceptance:
  - Template chips appear on eligible forms.
  - Inserts structured starter content.
  - Supports route context hints.

## 9) Form Replay Safety Net
- Problem: accidental submit/navigation loses typed intent.
- Feature: restore last submitted payload per form key.
- Acceptance:
  - Stores previous submitted fields.
  - Offers one-click restore banner.
  - Skips password/hidden fields.

## 10) UX Telemetry Tracker + Export
- Problem: no objective view of feature usage in browser UX.
- Feature: local interaction telemetry and export action.
- Acceptance:
  - Tracks key UX interactions (palette, shortcuts, actions).
  - Exposes export/download action.
  - Data remains local to browser storage.
