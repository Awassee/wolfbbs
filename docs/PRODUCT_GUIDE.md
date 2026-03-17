# WolfBBS Product Guide

This guide explains what WolfBBS is, what it includes, and how to run it like a product instead of a code project.

## Product Positioning

WolfBBS is a self-hosted community system that combines:

- a classic ANSI BBS caller experience
- a modern web companion for setup, admin, and browser access
- shared live chat across web and IRC
- file areas, doors, scoreboards, and classic operator views

It is designed for operators who want nostalgia in the user experience but modern expectations in install, recovery, and daily management.

## Core Value

WolfBBS solves four problems at once:

1. `Classic feel without classic pain`
   - SSH-first ANSI menus, message bases, newscan, files, and doors
   - modern bootstrap, package, and repair workflows
2. `One community, multiple ways in`
   - callers can use SSH, the web companion, or IRC
   - operators can manage the board from the browser instead of editing config by hand
3. `Real operator controls`
   - setup wizard, config center, audit logs, user administration, runtime diagnostics
4. `Ready to distribute`
   - release tarballs, checksums, bootstrap installer, smoke verification, and product docs
5. `Ready to operate`
   - first-step brief, status snapshot, launch checklist, and troubleshooting path

## Product Surfaces

### SSH ANSI BBS

Best for:

- callers who want the real BBS experience
- sysops who want Wildcat-style navigation and operator status views

Includes:

- welcome screen and login
- guest tour
- bulletins and newscan
- message boards
- private mail
- file areas and queue
- live chat
- doors and scoreboards
- who’s online and last callers
- settings and quick jump

### Web Companion

Best for:

- browser-first users
- sysops doing setup, moderation, and day-to-day management

Includes:

- login and password reset
- boards, mail, bulletins, files, finder, new files, directory
- live chat and channels
- doors, scores, radar, clubhouse, and discovery surfaces
- admin setup, config, audit, errors, and system dashboards

### IRC Bridge

Best for:

- existing IRC communities
- users who prefer IRC clients over the in-product chat UI

Includes:

- shared channel state with web chat
- core IRC command coverage
- moderation integration
- operator visibility and away handling

## Feature Groups And Why They Matter

### Community And Conversation

- `Message boards`
  - threaded discussion and reply flow
  - gives the board long-form memory and structured conversation
- `Private mail`
  - direct messages between callers and operator feedback flows
  - supports moderation, onboarding, and community trust
- `Chat and IRC bridge`
  - live conversation across browser and IRC clients
  - lowers friction for regular engagement
- `Clubhouse and one-liners`
  - lighter-weight social interaction
  - keeps the board feeling active between longer posts

### Nostalgia And Stickiness

- `ANSI menus and caller flow`
  - preserves the emotional feel of classic BBS use
- `Doors, scores, and trophies`
  - gives the board repeat-visit energy
- `Last callers and who’s online`
  - builds presence and social proof
- `Bulletins and system wire`
  - gives the operator a voice and helps frame the culture of the board

### Operator Control

- `Setup wizard`
  - first-run configuration without reading internal implementation details
- `Config center`
  - identity, text, runtime services, feature flags, and safety settings in one place
- `Admin panels`
  - users, boards, files, gateways, chat, doors, audit, and system state
- `Doctor, repair, upgrade`
  - allows product-style maintenance instead of ad hoc shell work

### Packaging And Distribution

- `Paste-and-run installer`
  - fastest path from zero to running system
- `Release bundles`
  - platform tarballs with docs, scripts, binaries, and checksums
- `Verification scripts`
  - supports repeatable install and smoke validation

## First Day With WolfBBS

Use this order for a clean launch:

1. Install with the bootstrap command or release bundle.
2. Sign in as the bootstrap sysop.
3. Complete `/admin/setup`.
4. Review `/admin/config`.
5. Visit `/boards`, `/chat`, `/doors`, `/scores`, and `/directory`.
6. Connect via SSH and verify the ANSI flow.
7. Create at least one non-sysop user.
8. Post a welcome bulletin or board message.
9. Open IRC if you want external client access.
10. Run `bash install.sh --doctor` and save the output for operations.

## Recommended Operator Workflow

### Initial launch

- set board identity and hostname
- review feature flags and public surfaces
- create moderators if needed
- seed starter content so the board is not empty on day one

### Weekly rhythm

- check `/admin/system` and `/admin/audit`
- review new users and moderation surfaces
- refresh bulletins and featured threads
- review door scores and file uploads
- run `bash install.sh --upgrade` or `--rapid-upgrade`

### Recovery path

When something looks broken:

```bash
bash install.sh --doctor
bash install.sh --repair
bash install.sh --logs
```

## Deployment Models

### Hobby board

- single host
- low to moderate concurrency
- SSH + web companion enabled
- optional IRC

### Community hub

- web and IRC both active
- moderators plus sysop
- message boards, live chat, and files as primary features

### Retro arcade board

- doors and scoreboards emphasized
- SSH/TUI promoted as the preferred caller path
- clubhouse and bulletins used for community momentum

## What WolfBBS Is Not

- not a general-purpose SaaS forum platform
- not a native non-Docker production target
- not a full ActivityPub federation product yet
- not a mail server replacement

## Documentation Map

- [Start Here](START_HERE.md)
- [Launch Checklist](LAUNCH_CHECKLIST.md)
- [Quickstart](QUICKSTART.md)
- [Install Guide](INSTALL.md)
- [Troubleshooting](TROUBLESHOOTING.md)
- [Operations Guide](OPERATIONS.md)
- [Datasheet](DATASHEET.md)
- [Feature Reference](feature-reference.md)
- [Admin Guide](admin.md)
- [Chat Guide](chat.md)
- [Doors Guide](doors.md)
