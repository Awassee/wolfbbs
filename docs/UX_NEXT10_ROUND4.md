# UX/Product Next 10 (Round 4, Implemented)

This round defines and delivers the next 10 major features in order for the shared web shell in `cmd/wolfbbs-web/main.go`.

## 1) Cross-Tab Sync Bus
- Problem: operators running multiple tabs see stale state and conflicting UI context.
- Feature: `BroadcastChannel` + storage event sync for UI state updates.
- Acceptance:
  - Sync channel initialized with loop prevention.
  - Key state updates refresh across tabs without manual reload.

## 2) Continuous Form Draft Autosave
- Problem: typed input can be lost before submit.
- Feature: autosave drafts for non-sensitive form fields while typing.
- Acceptance:
  - Draft snapshots persist per route/form.
  - Restore banner appears when a draft exists.
  - Password/hidden/file fields are excluded.

## 3) Draft Center Overlay
- Problem: saved drafts are hard to discover/manage.
- Feature: global draft center with restore/delete/export.
- Acceptance:
  - Lists route-scoped drafts.
  - Restores target form values on demand.
  - Supports JSON export.

## 4) KPI Watch Center
- Problem: KPI cards show values but no alert thresholds.
- Feature: per-KPI threshold watches with trigger notifications.
- Acceptance:
  - Set/clear threshold per KPI slot.
  - Trigger toast when threshold is reached.
  - State persists per route.

## 5) Incident Console
- Problem: lightweight ops incidents are not captured in-session.
- Feature: local incident register with severity/state workflow.
- Acceptance:
  - Create incidents with severity + note.
  - Resolve/reopen/delete incidents.
  - Export incident history.

## 6) Playbook Runner
- Problem: operator runbooks are inconsistent between routes.
- Feature: route-aware playbook checklist runner.
- Acceptance:
  - Route-specific default playbooks.
  - Step completion persistence.
  - Complete-all/reset controls.

## 7) Reminder Scheduler
- Problem: important follow-up actions are easy to forget mid-session.
- Feature: local reminder scheduler with due notifications.
- Acceptance:
  - Add reminder with relative due time.
  - Due reminders generate toast alerts.
  - Done/reopen/delete lifecycle.

## 8) Release Gate Checklist
- Problem: release readiness is often inferred rather than explicit.
- Feature: formal release gate checklist with confidence score.
- Acceptance:
  - Toggleable gate checks with progress summary.
  - Export current gate state.
  - Reset workflow.

## 9) Feedback Pulse Capture
- Problem: route-level UX feedback is rarely captured in context.
- Feature: in-app feedback pulse (rating + note) and export.
- Acceptance:
  - 1-5 rating plus freeform note.
  - Route and timestamp captured.
  - Export feedback entries.

## 10) Advanced Table Selection Toolkit
- Problem: table actions required repetitive manual row targeting.
- Feature: visible/select/invert/clear and selected-JSON copy tools.
- Acceptance:
  - Select visible rows in one click.
  - Invert and clear selection controls.
  - Copy selected rows as JSON.

## Validation
- Hook coverage test: `cmd/wolfbbs-web/main_test.go`
- Full QA pass re-run through unit/integration/e2e/manual scripts.
