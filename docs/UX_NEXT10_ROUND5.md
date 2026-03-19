# UX/Product Next 10 - Round 5 (156-165)

This spec defines the next 10 major features implemented in order from `docs/ROADMAP_151_200.md` items 156-165.

## 156) Smart Re-entry to last unfinished workflow
- Problem: callers lose momentum between sessions.
- Routes: `/resume`.
- Acceptance:
  - Renders prioritized unfinished workflow cards.
  - Supports one-click home-route updates from re-entry cards.
  - Includes lane links into primary call surfaces.

## 157) Door comeback prompts after streak breaks
- Problem: casual callers drop off after streak gaps.
- Routes: `/doors/comeback`.
- Acceptance:
  - Shows comeback state using caller streak metrics.
  - Presents recommended doors and direct action links.
  - Keeps messaging specific to streak-restoration behavior.

## 158) New user mentorship pairing
- Problem: first-run users need guided onboarding.
- Routes: `/admin/mentorship`, `/mentorship`.
- Acceptance:
  - Staff can create/update/delete mentor-mentee pairs.
  - Mentees see assigned mentor details.
  - Mentees can send direct mentorship check-ins via internal mail.

## 159) Profile milestone celebrations
- Problem: progress lacks visible social reward hooks.
- Routes: `/milestones`.
- Acceptance:
  - Computes milestone completion from boards/chat/doors/streak activity.
  - Allows celebrating completed milestones.
  - Persists celebration state per caller.

## 160) Time-of-day tailored landing states
- Problem: landing experience feels static across dayparts.
- Routes: `/time-lane`.
- Acceptance:
  - Selects lane guidance from current local daypart.
  - Provides primary and secondary route recommendations.
  - Supports one-click home-route update.

## 161) Moderator assignment queue for reports
- Problem: report ownership is ambiguous and duplicated.
- Routes: `/admin/mod-center`.
- Acceptance:
  - Lists reports with assignee, status, priority, and note.
  - Persists assignment changes.
  - Supports report resolution action.

## 162) Report SLA timers and breach warnings
- Problem: moderation follow-through lacks explicit SLOs.
- Routes: `/admin/mod-center`.
- Acceptance:
  - Computes due timestamps from assignment controls.
  - Renders SLA state (`ok`, `warning`, `breach`) per report.
  - Shows aggregate warning/breach counts in KPIs.

## 163) Caller risk summary on staff-visible profiles
- Problem: moderators need fast risk context on profile view.
- Routes: `/directory` (staff profile card).
- Acceptance:
  - Shows risk score and level for selected caller.
  - Explains contributing risk signals.
  - Links directly to moderation queue.

## 164) Moderator canned responses with audit trails
- Problem: repeated moderator responses are inconsistent and slow.
- Routes: `/admin/mod-center`.
- Acceptance:
  - Supports create/update/delete canned templates.
  - Sends canned templates to target caller mailboxes.
  - Tracks usage and records moderation actions.

## 165) Case threads for multi-step incidents
- Problem: multi-step incidents fragment across tools.
- Routes: `/admin/mod-center`.
- Acceptance:
  - Supports case create/update/close lifecycle.
  - Captures timeline updates (notes/actor/timestamps).
  - Links cases to target handles and optional report IDs.

## Validation
- `go test ./...`
- `scripts/run-e2e.sh --no-go --no-tui --web-timeout 900`
- `scripts/verify.sh --fast`
- `scripts/verify.sh --smoke`
- `scripts/manual-acceptance.sh --auto --no-smoke --report docs/manual-acceptance-latest.md`
