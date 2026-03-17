# WolfBBS Troubleshooting

Use this when the installer ran but the product still feels wrong.

## First Commands

Run these before changing random settings:

```bash
bash install.sh --status
bash install.sh --doctor
bash install.sh --repair
bash install.sh --logs
```

Useful generated files:

- `<prefix>/FIRST_STEPS.txt`
- `<prefix>/SERVICE_STATUS.txt`

## Symptom Routing

### I am not sure what to do next after install

- Open `FIRST_STEPS.txt`
- Open `/admin/launch`
- Read [START_HERE.md](START_HERE.md)
- Finish `/admin/setup`

### Web UI is up but the board feels empty

- Open `/admin/setup?step=4`
- seed default boards
- ensure the mailbot account
- create a real caller in `/admin/users`

### Chat works strangely or IRC users complain

- test `/chat`
- check `/admin/chat`
- review `/status` and `/admin/system`
- verify the expected IRC port is exposed

### SSH works but caller experience still feels incomplete

- walk the ANSI flow yourself
- confirm boards, mail, chat, files, and doors from the main menu
- compare against [LAUNCH_CHECKLIST.md](LAUNCH_CHECKLIST.md)

### Installer or status output shows warnings

- warnings are not always blockers
- fix blockers first
- if warnings are intentional, document them before launch

### Docker or Compose looks broken

- make sure Docker is installed
- make sure the daemon is reachable for the current user
- rerun `bash install.sh --doctor`
- if needed, rerun the bootstrap installer

## Common Cases

### `git` is missing

That is supported on the bootstrap path. Use:

```bash
curl -fsSL https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh | bash
```

The installer downloads a GitHub archive when `git` is unavailable or broken.

### I changed ports or proxies and now I do not trust the install

Run:

```bash
bash install.sh --doctor
bash install.sh --status
```

Then verify `/admin/setup`, `/status`, and `/admin/system`.

### I want a clean rebuild without re-learning the product

Run:

```bash
bash install.sh --repair
```

Then use `FIRST_STEPS.txt` again.
