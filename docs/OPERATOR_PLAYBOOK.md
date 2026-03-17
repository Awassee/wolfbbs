# WolfBBS Operator Playbook

Use this when you want the shortest path from "the stack is up" to "this feels like a real product."

## First Session

1. Open `/admin/launch`
2. Open `/admin/setup`
3. Open `/admin/config`
4. Create at least one non-sysop caller in `/admin/users`
5. Walk `/boards`, `/chat`, `/doors`, `/scores`, and SSH

## When To Use Which Screen

- `/admin/launch`: operator home base, launch readiness, and next actions
- `/admin/setup`: identity, safety, and bootstrap actions
- `/admin/config`: runtime exposure, feature flags, and service settings
- `/admin/system`: runtime and transport details
- `/status`: quick caller-facing confidence check

## Launch-Day Rhythm

### Before announcing

- verify launch readiness in `/admin/launch`
- seed boards and starter content
- confirm chat and doors
- make sure at least one real caller account works

### Right after announcing

- watch `/admin/system`
- watch `/admin/audit`
- keep `/chat` open
- verify that caller-origin surfaces are updating

## Recovery Order

Use this order when the board feels wrong:

```bash
bash install.sh --status
bash install.sh --doctor
bash install.sh --repair
bash install.sh --logs
```

Then compare what you see against:

- [LAUNCH_CHECKLIST.md](LAUNCH_CHECKLIST.md)
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## Anti-Patterns

- do not change random runtime flags before finishing `/admin/setup`
- do not invite users to an empty board
- do not trust only one surface; walk boards, chat, doors, and SSH
- do not treat warnings as harmless without understanding them
