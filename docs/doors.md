# Doors & Plugins

WolfBBS ships a manifest-driven Doors framework with both native and external door execution paths.  
All included doors are original writing/art and are inspired by classic BBS gameplay loops without copying copyrighted assets.

## Framework Summary

- Door types:
  - `native`: rendered and executed in WolfBBS ANSI flows.
  - `external`: launched as subprocesses with PTY-attached stdin/stdout.
- Manifest location:
  - `doors/<door_id>/door.json`
- Runtime context for every door:
  - `user_id`, `username`, `role`, `created_at`
  - `node_id`, `session_id`
  - terminal data (`cols`, `rows`, ANSI flag, color depth)
  - timezone + current timestamp
  - stable per-door storage namespace
- Persistent storage:
  - `door_configs`
  - `door_user_state`
  - `door_global_state`
  - `door_event_log`
  - `door_turn_bank`
  - `door_achievements`
  - `door_scores`
  - `door_user_meta`

## Manifest Format

Each `door.json` includes:

- `id`, `display_name`, `short_description`, `category`, `version`
- `entry_type` (`native` or `external`)
- `entrypoint` (native module token or executable command)
- `hotkey` (single-letter)
- `concurrency_mode` (`exclusive`, `per_user`, `shared`)
- `requires`:
  - `ansi`, `min_cols`, `min_rows`
- `capabilities`:
  - `needs_network`, `needs_filesystem_write`, `needs_sound`
- `data_retention`:
  - `messages_days`, `logs_days`
- `admin_defaults`:
  - `enabled`, `daily_turns`, `time_bank_max`, `reset_hour`
  - `max_run_seconds`, `max_output_rate`
- `required_role` (default `user`, supports `verified_user`, `moderator`, `sysop`; `admin` accepted as alias)
- `help_text`, `rules_text`
- `args` (for external command args)

## Default Door Catalog (25)

### Games (16)
- `space-trader-wars`
- `dragon-tavern-legends`
- `barren-realms-commander`
- `solar-realms-dominion`
- `yankee-trader-syndicate`
- `assassins-guild`
- `arrowbridge-quest`
- `exitilus-realms`
- `falcon-relic-wars`
- `global-war-command`
- `pit-arena`
- `overkill-ops`
- `siege-engines`
- `fishing-derby`
- `word-duel-arena`
- `casino-royale-suite`

### Utilities / Plugins (9)
- `door-hub`
- `doorparty-connector`
- `bbslink-connector`
- `telnet-bridge`
- `voting-booth`
- `bulletin-news-center`
- `filebase-pro`
- `ansi-art-gallery`
- `tournaments-achievements-center`
- `oracle-door` (disabled by default; contained AI helper door)

## ANSI UX Integration

- Main menu entry: `(D) Doors`.
- Door hub shows:
  - categories
  - favorites
  - recently played
  - turns remaining per door
- Per-door commands:
  - `(P)lay`
  - `(H)elp`
  - `(R)ules`
  - `(S)cores`
  - `(A)chievements`
  - `(Q)uit`
- Global ANSI screen: `Door Scores & Trophies`.

## Native Door Pattern Templates

WolfBBS now uses a reusable native pattern layer:

- Full end-to-end templates (stateful gameplay + persistence + scores + achievements):
  - `space-trader-wars`
  - `dragon-tavern-legends`
  - `voting-booth`
  - `filebase-pro`
- Fan-out templates for remaining native doors:
  - strategy/rpg/arcade/utility profiles are handled by a shared pattern runner with per-door labels and command verbs.
  - each template-backed door still gets user state persistence, score submission, and achievement hooks.

## Connector Adapters

- `doorparty-connector`, `bbslink-connector`, and `telnet-bridge` are native adapters that launch configured bridge commands.
- Runtime config source:
  - `connectors.doorparty.*`, `connectors.bbslink.*`, and `connectors.telnet_bridge.*` in `wolfbbs.hjson`
  - env overrides:
    - `WOLFBBS_DOORPARTY_ENABLE`, `WOLFBBS_DOORPARTY_COMMAND`, `WOLFBBS_DOORPARTY_ARGS`
    - `WOLFBBS_BBSLINK_ENABLE`, `WOLFBBS_BBSLINK_COMMAND`, `WOLFBBS_BBSLINK_ARGS`
    - `WOLFBBS_TELNET_BRIDGE_ENABLE`, `WOLFBBS_TELNET_BRIDGE_COMMAND`, `WOLFBBS_TELNET_BRIDGE_ARGS`
- Policy controls enforced at launch:
  - door `enabled`
  - role requirement / override
  - network allow/deny from door policy
  - output rate and runtime limits
- Audited door events:
  - `connector_launch`, `connector_stop`, `connector_error`, `connector_denied`

## Web Integration

- Scores:
  - `/scores` global leaderboard and trophies view.
  - `/scores?door=<door_id>` filtered view.
- Admin:
  - `/admin/doors`
    - enable/disable
    - daily turns, time-bank, reset hour
    - output/run limits
    - role override
    - network/filesystem toggles
    - usage stats (DAU/MAU/plays)
    - leaderboard reset
    - door event logs

## Safety Model (External Doors)

- Per-run timeout (`max_run_seconds`).
- Output throttle (`max_output_rate`).
- Optional network deny-by-default (Linux best-effort `unshare -n` when available).
- Per-door writable working directory (`WOLFBBS_DOOR_DATA_DIR`).
- Optional command allow-list root via `WOLFBBS_DOOR_ALLOW_DIR`.
- Optional legacy dropfile generation (off by default):
  - set `WOLFBBS_DOOR_DROPFILES=door32,doorsys,dorinfo`
  - generated in each run directory: `DOOR32.SYS`, `DOOR.SYS`, `DORINFOx.DEF`
  - helper env hints exported for wrappers: `WOLFBBS_DOOR32_SYS`, `WOLFBBS_DOOR_SYS`, `WOLFBBS_DORINFO_DEF`
- Start/stop/error events are written to `door_event_log`.

## Achievements and Scores API

Registry API exposes:

- `AwardAchievement(doorID, userID, code)`
- `SubmitScore(doorID, userID, scoreType, value, metadataJSON)`
- `ListScores(doorID, limit)`
- `ListAchievements(userID, doorID, limit)`
- `ResetDoorScores(doorID)`

## Daily Turns + Time Bank

- If a door has `daily_turns > 0`, each launch consumes one turn.
- Unused turns carry to a time bank up to `time_bank_max`.
- Reset schedule is based on server local day boundary.
- Doors with `daily_turns = 0` are turn-unlimited.

## Registering a New Door

1. Create `doors/<id>/door.json`.
2. Choose a unique single-letter `hotkey`.
3. Set `entry_type` and `entrypoint`.
4. Configure `admin_defaults` and capability flags.
5. Add tests:
   - unit test for rules/state transitions.
   - integration/smoke test for launch + clean exit.

## Testing Contract

- Door framework tests:
  - `internal/doors/doors_test.go`
    - manifest catalog loading
    - turns/time-bank behavior
    - native launch smoke over catalog
    - external launch smoke
- Run with:
  - `go test ./internal/doors -v`
AI containment note:
- `oracle-door` keeps AI interaction isolated as a door instead of injecting AI into all message areas.
- responses are explicitly tagged `[AI-LABEL]`.
- per-user prompt limits apply (default `10/day`, configurable via `WOLFBBS_ORACLE_DAILY_PROMPTS`).
