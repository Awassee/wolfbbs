# Doors Plugin System

## Philosophy
A door is an external executable attached to a menu entry that receives a PTY stream and runs as a child process in a constrained context.

## Contract
- Executable path with optional arguments.
- Inherited stdin/stdout/stderr are the session PTY streams.
- Environment variables:
  - `WOLFBBS_HANDLE`
  - `WOLFBBS_USER_ID`
  - `WOLFBBS_NODE`
  - `WOLFBBS_AREAS`

## Safety Defaults
- Only run doors from an allow-list directory.
- Optional CPU/memory/time caps (wrapper process).
- Kill door on client disconnect or timeout.
- Write exit status to audit log.

## Registering a Door
- config block
  - `name`
  - `command`
  - `args`
  - `hotkey`
  - `auth_required`
- Doors are displayed in the Main Menu with a single hotkey.
- Env registration:
  - `WOLFBBS_DOORS="T|Trivia|/opt/wolfbbs/doors/trivia|;B|Bash|/bin/bash|"`
  - Format: `HOTKEY|NAME|COMMAND|arg1,arg2` (entries separated by `;`)
- Optional allow-list:
  - `WOLFBBS_DOOR_ALLOW_DIR=/opt/wolfbbs/doors`
  - When set, door binaries must resolve under this directory.

## Sample Door
- Trivia game:
  - command: `./doors/trivia`
  - prompts multiple-choice via terminal
  - tracks score in `/var/lib/wolfbbs/doors/<handle>/trivia.json`

## Security
- Keep doors read-only by default.
- Allowlist every binary and deny environment mutations.
- Never pass raw shell strings; invoke explicit command + args.
