# Start Here

This is the shortest practical path from install to a usable WolfBBS board.

## Goal

You are done when:

- the sysop can sign into `/admin`
- SSH login works
- `/boards`, `/chat`, `/doors`, and `/scores` all work
- at least one non-sysop account exists

## Best path for most people

1. Run the bootstrap installer.
2. Sign in as the bootstrap sysop.
3. Finish `/admin/setup`.
4. Review `/admin/config`.
5. Seed default boards.
6. Create a real caller account in `/admin/users`.
7. Test SSH, web, and chat.

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

## If something looks wrong

```bash
bash install.sh --status
bash install.sh --doctor
bash install.sh --repair
```

## Read next

- [Quickstart](QUICKSTART.md)
- [Install Guide](INSTALL.md)
- [Operations Guide](OPERATIONS.md)
- [Product Guide](PRODUCT_GUIDE.md)
