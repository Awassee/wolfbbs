# Running A Real Community

WolfBBS is most effective when it has a predictable operator cadence instead of occasional reactive cleanup.

## Weekly cadence

1. Review `/admin/analytics`.
2. Review `/admin/ops`.
3. Read `/feedback` mail and answer anything actionable.
4. Refresh `/events`, `/bulletins`, and `/challenges`.
5. Walk the board once as a normal caller over SSH.

## What keeps callers returning

- one visible event on the calendar
- one active challenge or mission
- one fresh bulletin or announcement
- one reason to use chat right now
- one clear door or file highlight

## What to watch

- low weekly returners
- no first-call completions
- no feedback reaching the sysop inbox
- empty chat history
- stale bulletins and empty upcoming events

## Healthy board habits

- create at least one non-sysop moderator account
- keep sysop work separate from normal caller testing
- use `/first-call` after major changes
- keep `/admin/users` resets deliberate and short-lived
- run `bash install.sh --doctor` before changing ports, proxies, or transport settings

## Release rhythm

Before an upgrade:

1. Run `bash install.sh --status`
2. Run `bash install.sh --doctor`
3. Run the automated verifier
4. Validate SSH, web chat, IRC, and one door

After an upgrade:

1. Re-run `/first-call`
2. Send one chat line from web and IRC
3. Confirm one non-sysop can log in and post
4. Check `/admin/analytics` for obvious regressions

## Suggested team split

- `sysop`: release, setup, runtime, backups
- `moderator`: chat, reports, caller safety
- `community host`: events, bulletins, challenges, outreach

Small boards can combine these roles, but the checklist still helps.
