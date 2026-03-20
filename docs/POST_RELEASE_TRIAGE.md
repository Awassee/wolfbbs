# Post-Release Triage Loop

Use this for the first 72 hours after a public release.

## Daily cadence

1. Check GitHub Actions for new failures on `main`.
2. Review new GitHub issues, especially installer, upgrade, SSH/TUI, and chat regressions.
3. Re-run:

```bash
bash install.sh --status
bash install.sh --doctor
scripts/verify.sh --fast
```

4. If any user-facing install or login issue is reported, reproduce it before taking new feature work.
5. If a regression is confirmed, cut a focused hotfix instead of batching unrelated work.

## Triage buckets

- `release-blocker`
  - install failures
  - upgrade failures
  - broken login/auth
  - broken boards/chat/mail/doors path
- `high-friction`
  - setup confusion
  - poor error messaging
  - mobile or accessibility regressions
- `backlog`
  - cool feature requests
  - quality-of-life improvements
  - nostalgia enhancements that do not block usage

## Evidence to collect before deciding

- exact version/tag
- install method
- platform + arch
- output from:

```bash
bash install.sh --status
bash install.sh --doctor
bash install.sh --debug-bundle
```

- screenshots or terminal transcript
- whether the failure reproduces on a clean reinstall

## Hotfix rule

Ship a hotfix when any of these are true:

- new installs fail on a supported platform
- upgrades strand the board in a broken state
- a core route fails for normal callers
- CI or security checks fail on `main`

## Exit condition

Leave post-release mode only when:

- no release-blocking issues are open
- install/upgrade path is stable for several days
- `main` is green
- the next work is driven by real feedback, not guesswork
