# Built-in Mods

WolfBBS includes a lightweight lifecycle manager in `internal/mods` and a built-in baseline pack:

- `onelinerz`
- `rumorz`
- `bbslist`
- `whos_online`

## Lifecycle

Each mod supports:

- `Init(ctx)`
- `Tick(ctx, now)`
- `Shutdown(ctx)`
- `Snapshot()`

The manager:

- registers mods with enabled/disabled state
- starts periodic ticks
- tracks last tick and last error per mod
- provides snapshots for status dashboards

## Runtime visibility

- User discover view (`/discover`): rumor line, one-liner feed, BBS list block.
- User status center (`/status`): mod counts in function status table.
- Sysop WFC dashboard (`/admin/system`): running/total, one-liner count, active rumor.

## Sysop CLI

```bash
oputil mods list
```
