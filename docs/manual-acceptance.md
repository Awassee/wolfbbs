# Manual Acceptance Pass

Use this when you want to close the spec items marked `MANUAL` in `/Users/seanheiney/wolfbbs/docs/ACCEPTANCE_SPEC.md`.

## Runner

Interactive (recommended):

```bash
/Users/seanheiney/wolfbbs/scripts/manual-acceptance.sh --guided
```

Non-interactive checklist export:

```bash
/Users/seanheiney/wolfbbs/scripts/manual-acceptance.sh --non-interactive --no-smoke
```

Custom report path:

```bash
/Users/seanheiney/wolfbbs/scripts/manual-acceptance.sh --guided --report docs/manual-acceptance-2026-02-27.md
```

## What it does

1. Runs `scripts/verify.sh --smoke --keep-stack` by default (can be skipped with `--no-smoke`).
2. Walks each MANUAL acceptance ID.
3. Captures PASS/FAIL/SKIPPED per item.
4. Writes a markdown report for audit trail.

## Covered IDs

- `BBS-003`..`BBS-009`
- `WA-003`..`WA-009`
- `CH-003`..`CH-005`
- `IRC-007`
- `GW-EMAIL-003`
- `GW-WEB-001`
- `GW-WEB-004`

## Exit behavior

- Exit `0`: no manual failures recorded.
- Exit `1`: one or more manual checks marked FAIL.

