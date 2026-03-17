# WolfBBS Quickstart

Use this if you want the shortest path from zero to a working board.

## Outcome

At the end of this quickstart you will have:

- a running WolfBBS install
- a bootstrap sysop account
- web admin access
- SSH caller access
- optional IRC access ready to test

## 1. Install WolfBBS

Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh | bash
```

macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/Awassee/wolfbbs/main/bootstrap.sh | bash -s -- --install-brew
```

Alternative paths:

- [GitHub Releases](https://github.com/Awassee/wolfbbs/releases)
- [Public repo](https://github.com/Awassee/wolfbbs)

If you prefer to clone first:

```bash
git clone https://github.com/Awassee/wolfbbs.git wolfbbs
cd wolfbbs
bash install.sh
```

If you are unsure which path to use:

- use the bootstrap installer for the fastest working system
- use the release tarball for a cleaner handoff or packaged install
- use a clone when you expect to inspect or change code

## 2. Let the installer finish

The installer:

1. checks your platform
2. installs supported dependencies when needed
3. prepares Docker runtime
4. clones or updates the app
5. writes the runtime `.env`
6. starts the stack
7. prints connection details and bootstrap credentials

Non-interactive install:

```bash
bash install.sh --yes
```

## 3. Complete first-run setup

After install, open the printed admin URL and do this in order:

1. Visit `/admin/setup`
2. Visit `/admin/config`
3. Seed default boards and confirm the mailbot bootstrap action
4. Review the board name, hostname, and runtime flags
5. Create any extra users or moderators in `/admin/users`

The installer prints:

- SSH connect command
- web admin URL
- web chat URL
- IRC host and port
- bootstrap sysop credentials

## 4. Test the product like a real operator

Use these checks immediately after setup:

1. Connect over SSH and verify the ANSI menu flow.
2. Open `/boards` and create a starter post.
3. Open `/chat` and send a message in `#lobby`.
4. Open `/doors` and launch a built-in door.
5. Open `/scores` to confirm score surfaces render.
6. Run `bash install.sh --doctor` and make sure the report is understandable.

## 5. Decide what you hand to real users

Before inviting callers, make sure you know which surfaces are public:

- SSH is the main nostalgic caller path
- `/chat` and IRC share the same live conversation layer
- `/boards`, `/bulletins`, `/directory`, and `/doors` are the core browser surfaces
- `/admin/*` is for sysops only

## 6. Daily management

```bash
bash install.sh --status
bash install.sh --doctor
bash install.sh --repair
bash install.sh --upgrade
```

Fast local rebuild while iterating:

```bash
bash install.sh --rapid-upgrade
```

## 7. Uninstall

Keep data:

```bash
bash install.sh --uninstall
```

Remove data too:

```bash
bash install.sh --uninstall --purge --yes
```

## 8. Read next

- [Start Here](START_HERE.md)
- [Install Guide](INSTALL.md)
- [Operations Guide](OPERATIONS.md)
- [Product Guide](PRODUCT_GUIDE.md)
- [Datasheet](DATASHEET.md)
