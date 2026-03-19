# UX/Product Next 10 - Round 6

This tranche defines and ships 10 major UX features in order for the web shell.

## 1) Non-blocking Action Dock default
- Problem: dock could overlay critical controls and block clicks.
- Shipped:
  - Dock now defaults to collapsed.
  - Pinning a route opens dock intentionally.
- Acceptance:
  - Core controls remain clickable on first load.
  - Dock can be opened explicitly when needed.

## 2) Action Dock collision guard
- Problem: opened dock could overlap nav/hero control bands.
- Shipped:
  - Collision detection against key control regions.
  - Auto-adjust behavior: move left first, then collapse.
- Acceptance:
  - Dock no longer traps pointer interaction on critical controls.

## 3) Action Dock side toggle
- Problem: fixed-right docking does not fit every layout state.
- Shipped:
  - Manual side toggle (`Dock left` / `Dock right`).
  - Side persisted in local UI state.
- Acceptance:
  - Dock location can be user-controlled and survives reloads.

## 4) Action Dock live filtering
- Problem: quick-launch links get noisy with pinned + trail growth.
- Shipped:
  - Search box in dock body.
  - Group counters (`shown/total`) per section.
- Acceptance:
  - Operators can filter to target links quickly.

## 5) UI profile presets
- Problem: users repeatedly retune layout/motion/density for context.
- Shipped:
  - `Profile: Balanced/Reader/Operator` cycling control.
  - Presets apply layout/motion/density/accent bundles.
- Acceptance:
  - One-click adaptation for reading-heavy vs ops-heavy workflows.

## 6) Trail back navigation
- Problem: returning to prior route required manual nav hopping.
- Shipped:
  - `Back:` control in hero preferences using session route trail.
- Acceptance:
  - Caller can jump to previous meaningful route in one action.

## 7) Shortcut legend launcher
- Problem: keyboard help existed but lacked explicit high-visibility entry point.
- Shipped:
  - Dedicated `Shortcuts` control in preference strip.
  - Overlay open/close API exposed for UI actions.
- Acceptance:
  - Keyboard affordances are discoverable without memorization.

## 8) Compact controls mode
- Problem: hero control strip becomes dense on high-action pages.
- Shipped:
  - `Controls: Full/Compact` toggle.
  - Compact mode hides secondary controls only.
- Acceptance:
  - Primary controls remain visible while reducing visual load.

## 9) Control priority model
- Problem: all controls looked equally critical, increasing cognitive noise.
- Shipped:
  - Secondary controls marked with priority metadata.
  - Compact mode now uses this priority model.
- Acceptance:
  - Interaction hierarchy is clearer and faster to scan.

## 10) Regression coverage expansion
- Problem: prior tests did not catch dock/control interaction regressions.
- Shipped:
  - Extended Playwright checks for profile/compact/shortcut/dock search/side controls.
  - Explicit path coverage for pin-route + dock visibility.
- Acceptance:
  - Future regressions in these flows surface in CI.

## Validation plan
- `go test ./...`
- `scripts/run-e2e.sh --no-go --no-tui --web-timeout 900`
- `scripts/verify.sh --fast`
- `scripts/verify.sh --smoke`
- `scripts/manual-acceptance.sh --auto --no-smoke --report docs/manual-acceptance-latest.md`
