# Start Here

This is the shortest practical path from install to a usable WolfBBS board.

Release baseline: `v2.0.2` (current public release).

## Goal

You are done when:

- the sysop can sign into `/admin`
- SSH login works
- `/boards`, `/chat`, `/doors`, and `/scores` all work
- at least one non-sysop account exists

## Best path for most people

1. Run the bootstrap installer.
2. Sign in as the bootstrap sysop.
3. Open `/admin/launch`.
4. Finish `/admin/setup`.
5. Review `/admin/config`.
6. Seed default boards.
7. Create a real caller account in `/admin/users`.
8. Test SSH, web, and chat.

The installer writes `<prefix>/FIRST_STEPS.txt` with the exact URLs and commands for this flow.

## Install

Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh | bash
```

This install path only requires `curl` and `bash` up front. The installer figures out the rest.

macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh | bash -s -- --install-brew
```

## First 15 minutes

### 1. Finish sysop setup

- open `/admin/setup`
- set board name and hostname
- review safety settings
- seed default boards
- ensure the mailbot account

### 2. Review runtime behavior

- open `/admin/config`
- confirm any optional features you want enabled
- leave advanced toggles alone unless you know why you need them

### 3. Validate the public product

- SSH into the board
- open `/boards`
- open `/chat`
- open `/doors`
- open `/scores`

### 4. Create a real user

- go to `/admin/users`
- add one non-sysop account
- test login as that user

### 5. Do the launch pass

- run `bash install.sh --status`
- run `bash install.sh --doctor`
- run `bash install.sh --port-audit` if ports look wrong
- walk [LAUNCH_CHECKLIST.md](LAUNCH_CHECKLIST.md)

## If something looks wrong

```bash
bash install.sh --status
bash install.sh --doctor
bash install.sh --port-audit
bash install.sh --debug-bundle
bash install.sh --repair
```

## Read next

- [First 30 Minutes As Sysop](FIRST_30_MINUTES.md)
- [Quickstart](QUICKSTART.md)
- [Launch Checklist](LAUNCH_CHECKLIST.md)
- [Operator Playbook](OPERATOR_PLAYBOOK.md)
- [Running A Community](RUNNING_A_COMMUNITY.md)
- [Install Guide](INSTALL.md)
- [Troubleshooting](TROUBLESHOOTING.md)
- [Operations Guide](OPERATIONS.md)
- [Product Guide](PRODUCT_GUIDE.md)
