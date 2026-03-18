# WolfBBS 50-Item Product Roadmap

This document is the structured backlog for the next major product cycles.

## Delivery rule
- Work ships in coherent tranches, not as a 50-feature big bang.
- Every tranche must end with server tests, browser validation, verifier pass, and a packaged release.

## Current tranche shipped in this release
1. Community calendar: public `/events` plus sysop `/admin/events` so the board has concrete scheduled activity.
Value: gives callers a reason to return on a specific day and time.
Status: shipped in v1.1.11.
2. Watched boards: caller-controlled board tracking from `/boards`.
Value: narrows the board experience to what a caller actually cares about.
Status: shipped in v1.1.11.
3. Today brief: authenticated `/today` daily loop joining queue, watched boards, and events.
Value: makes the first post-login decision obvious.
Status: shipped in v1.1.11.

## Theme 1: Onboarding and daily-use clarity
4. Guided first caller session that creates one post, one chat message, and one mail.
Value: reduces first-session confusion.
Status: shipped in v1.1.13.
5. Sysop go-live wizard with explicit readiness checkpoints and rollback steps.
Value: reduces fragile launches.
Status: shipped in v1.1.13.
6. Role-aware empty states across boards, chat, files, and doors.
Value: explains why a page looks empty instead of looking broken.
Status: shipped in v1.1.13.
7. Persistent quick-start checklist for guests and newly registered callers.
Value: improves activation rate.
Status: shipped in v1.1.13.
8. Saved home route preference (`/today`, `/boards`, `/chat`, `/doors`).
Value: makes repeat visits faster.
Status: shipped in v1.1.13.
9. Daily digest email or web summary opt-in.
Value: supports return behavior outside active sessions.
Status: planned.
10. Mobile-first connect guidance for SSH, web terminal, and IRC.
Value: lowers friction for non-desktop callers.
Status: planned.

## Theme 2: Communication and social depth
11. Persistent notification model with read/unread state, not just generated queues.
Value: enables reliable follow-up behavior.
Status: shipped in v1.1.12.
12. Board subscription tiers: watch, mute, digest-only.
Value: gives users more control than a binary watch list.
Status: shipped in v1.1.12.
13. Mention autocomplete in boards, mail, and chat.
Value: reduces failed mentions and typing friction.
Status: planned.
14. Caller presence cards showing current surface, idle, and entry source.
Value: makes the board feel alive.
Status: planned.
15. Rich private mail folders and labels.
Value: improves mailbox triage.
Status: planned.
16. Thread bookmarking and later-read queue.
Value: supports deep boards without overload.
Status: planned.
17. Scheduled announcements and bulletin publishing.
Value: lets sysops plan communication instead of posting ad hoc.
Status: planned.
18. Node-to-node paging and attention requests.
Value: restores a classic BBS interaction pattern in a modern way.
Status: planned.
19. Cross-surface social graph: recent correspondents, recurring collaborators, favorite callers.
Value: strengthens community stickiness.
Status: later.
20. Community badges tied to helpful behavior, not only game scores.
Value: broadens recognition beyond doors.
Status: later.

## Theme 3: Boards and knowledge systems
21. Full-screen composer with preview, quote tools, templates, and keyboard map.
Value: makes posting feel intentional instead of form-based.
Status: shipped in v1.1.12.
22. Thread summaries and high-signal reply highlighting.
Value: speeds catch-up in long discussions.
Status: planned.
23. Moderator review queue with duplicate/spam triage helpers.
Value: reduces moderation overhead.
Status: planned.
24. Board-level posting templates and pinned welcome messages.
Value: improves content quality and consistency.
Status: planned.
25. Per-board statistics: active users, median reply time, hot tags.
Value: helps callers decide where to participate.
Status: planned.
26. Message tagging and topic maps.
Value: improves later retrieval and organization.
Status: planned.
27. Archive mode for old boards with read-only searchability.
Value: preserves history without cluttering active flows.
Status: planned.
28. Cross-board digest generation by interest profile.
Value: turns high volume into a manageable scan.
Status: later.
29. Shared reading lists curated by moderators.
Value: supports onboarding and recurring themes.
Status: later.
30. Import/export tools for board content snapshots.
Value: improves portability and backup use cases.
Status: later.

## Theme 4: Files, downloads, and offline use
31. Modernized upload flow with metadata extraction and moderation hold queue.
Value: makes the filebase usable as a living system.
Status: next.
32. Rich file preview pages with tags, ratings, and related uploads.
Value: improves discoverability.
Status: planned.
33. Resumable download tokens and clearer download desk UX.
Value: reduces failed transfers.
Status: planned.
34. Offline packet generation for selected boards and mail.
Value: supports classic asynchronous use.
Status: planned.
35. New-files personalization by tags, boards, and interests.
Value: improves return value for heavy callers.
Status: planned.
36. File request board and fulfillment workflow.
Value: turns files into a collaborative feature.
Status: later.
37. Virus-scan and policy hooks for uploads.
Value: reduces security risk.
Status: later.
38. Curated file collections and rotating featured packs.
Value: adds editorial product value.
Status: later.
39. Download analytics for sysops.
Value: helps prioritize content.
Status: later.
40. QWK-style offline reader package for files and boards.
Value: serves nostalgia and low-connectivity use.
Status: later.

## Theme 5: Doors, events, and retention loops
41. Tournament engine with brackets, standings, and reward surfaces.
Value: makes competitive doors feel like a product system.
Status: next.
42. Event recurrence and series management in `/admin/events`.
Value: reduces manual calendar maintenance.
Status: shipped in v1.1.12.
43. Door parties and scheduled multiplayer sessions.
Value: turns doors into community events.
Status: planned.
44. Seasonal ladders and resets.
Value: creates recurring reasons to come back.
Status: planned.
45. Personal streaks, milestones, and comeback prompts.
Value: improves retention without spamming users.
Status: planned.
46. Team or club systems around boards and doors.
Value: creates longer-term group identity.
Status: later.
47. Community challenges that blend posting, chat, and doors.
Value: links isolated features together.
Status: later.
48. Featured weekly route or event spotlight on home surfaces.
Value: improves product rhythm.
Status: later.
49. Door API/plugin framework for third-party contributions.
Value: scales feature growth beyond core code.
Status: later.
50. Release train dashboard covering roadmap progress, test coverage, and launch readiness.
Value: keeps product improvement tied to operational quality.
Status: later.

## Next recommended tranche
- Tournament scaffolding on top of the recurring events system.
- File upload modernization.
- Daily digest email or web summary opt-in.
- Mobile-first connect guidance for SSH, web terminal, and IRC.
