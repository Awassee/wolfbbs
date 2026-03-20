# Next Feature Wave

This is the next major feature wave after the `v2.0.0` release line stabilizes.

## Priority order

1. Guided upgrade center inside the app
- show current version, target version, rollback hints, and post-upgrade validation

2. Richer operator analytics
- first successful caller session
- caller retention cohorts
- board/channel heat maps
- failed setup and failed upgrade reasons

3. Stronger terminal polish
- resize-safe layouts
- better compose editing
- improved paging and redraw recovery

4. Mail workflows that feel complete
- drafts
- saved replies
- moderation mail triage
- optional digest workflows

5. File area moderation and curation
- staged review flow
- duplicate detection
- curator notes
- better collection publishing

6. Safer internet gateway doors
- clearer outbound trust model
- per-tool controls
- operator-visible audit trail

7. Community loops
- scheduled events with stronger reminders
- seasonal content lanes
- return-visitor hooks

8. Theme and ANSI customization
- more sysop-tunable nostalgia without layout breakage

9. Operator recovery cockpit
- one place for logs, doctor output, backups, and runtime diffs

10. Better public showcase material
- repeatable screenshots
- product demo flows
- operator/tutorial clips

## Gate before starting

Do not begin this wave until:

- install + upgrade CI is green
- terminal and UI regression suites are stable
- post-release triage is quiet
- security audit is green
