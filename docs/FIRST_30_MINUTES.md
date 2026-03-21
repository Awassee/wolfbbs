# First 30 Minutes As Sysop

Release baseline: `v2.1.6`.

This is the fastest operator runbook after a clean install.

## 0-5 minutes: establish control

1. Open `/admin/login`.
2. Sign in with the bootstrap sysop from `<prefix>/.env` or `<prefix>/FIRST_STEPS.txt`.
3. Open `/admin/setup`.
4. Set the board name and public hostname.
5. Seed default boards.
6. Ensure the mailbot account.

## 5-10 minutes: prove the product is alive

1. Open `/admin/launch`.
2. Open `/admin/config`.
3. Check `/status`.
4. Run:

```bash
bash install.sh --status
bash install.sh --doctor
```

## 10-20 minutes: validate the caller journey

1. Create one non-sysop account in `/admin/users`.
2. Sign in as that user.
3. Complete `/first-call`.
4. Verify `/boards`, `/chat`, `/doors`, and `/scores`.
5. Test SSH login with the same non-sysop account.

## 20-30 minutes: create the first repeat loop

1. Schedule one event in `/admin/events`.
2. Set one challenge in `/admin/challenges`.
3. Post one bulletin in `/admin/bulletins`.
4. Ask for one real feedback note through `/feedback`.

## Exit criteria

You are in a healthy state when:

- sysop and non-sysop logins both work
- web and SSH caller flows both work
- boards, chat, mail, and doors all have at least one real interaction
- installer `--status` and `--doctor` are readable and unsurprising

## If anything feels wrong

```bash
bash install.sh --status
bash install.sh --port-audit
bash install.sh --debug-bundle
bash install.sh --repair
```
