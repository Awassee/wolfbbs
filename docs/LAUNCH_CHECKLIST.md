# WolfBBS Launch Checklist

Use this after install and before you invite real callers.

## Goal

You are ready to announce the board when:

- the sysop can sign in
- the board is not empty
- a non-sysop caller can log in
- chat, doors, and scores all render
- status and health views show no obvious blockers

## 10-Minute Launch Pass

1. Open `/admin/login` and confirm the bootstrap sysop can sign in.
2. Open `/admin/launch` and read the launch verdict.
3. Finish `/admin/setup` in order.
4. Review `/admin/config` and set the real site name and hostname.
5. Run the bootstrap actions in `/admin/setup?step=4`.
6. Create at least one non-sysop account in `/admin/users`.
7. Open `/boards` and confirm the board is not empty.
8. Open `/chat` and send a test line in `#lobby`.
9. Open `/doors` and `/scores`.
10. Test SSH with `ssh <host> -p <port>`.
11. Run `bash install.sh --status` and `bash install.sh --doctor`.

## Go / No-Go Checks

Ship the board only if these are true:

- `/admin/setup` shows a clean readiness baseline or only intentional warnings
- `/status` and `/admin/system` do not show obvious runtime failures
- there is at least one real caller account
- callers can post, chat, and open doors

## Day-One Content Minimum

Do not launch an empty board. At minimum:

- seed default boards
- post one welcome message or bulletin
- set MOTD and announcement text
- enable at least one obvious caller surface: boards, chat, doors

## If Something Fails

Run these in order:

```bash
bash install.sh --status
bash install.sh --doctor
bash install.sh --repair
bash install.sh --logs
```

Then use [TROUBLESHOOTING.md](TROUBLESHOOTING.md).
