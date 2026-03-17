# WolfBBS Quickstart

Use this if you want the shortest path from download to a working board.

## 1. Get WolfBBS

Clone the public repo:

```bash
git clone https://github.com/seanheiney/wolfbbs-public.git wolfbbs
cd wolfbbs
```

Or download a release bundle from:

- [GitHub Releases](https://github.com/seanheiney/wolfbbs-public/releases)

## 2. Run the installer

Guided menu mode:

```bash
bash install.sh
```

One-shot install:

```bash
bash install.sh --yes
```

macOS with Homebrew bootstrap allowed:

```bash
bash install.sh --yes --install-brew
```

## 3. Log in

After install, the script prints:

- SSH connect command
- Web admin URL
- Web chat URL
- IRC host/port
- bootstrap SYSOP credentials

Finish setup in:

- `/admin/setup`
- `/admin/config`

## 4. Daily management

```bash
bash install.sh --status
bash install.sh --doctor
bash install.sh --repair
bash install.sh --upgrade
```

## 5. Clean uninstall

Keep data:

```bash
bash install.sh --uninstall
```

Remove data too:

```bash
bash install.sh --uninstall --purge --yes
```
