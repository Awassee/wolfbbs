# UI Coverage Matrix

Auto-generated from `docs/function-registry.json`.

| ID | Type | User TUI | User Web | Admin TUI | Admin Web | Config UI | Status UI |
|---|---|---:|---:|---:|---:|---|---|
| `door:ansi-art-gallery` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:arrowbridge-quest` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:assassins-guild` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:barren-realms-commander` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:bbslink-connector` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:bulletin-news-center` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:casino-royale-suite` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:door-hub` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:doorparty-connector` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:dragon-tavern-legends` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:exitilus-realms` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:falcon-relic-wars` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:filebase-pro` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:fishing-derby` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:global-war-command` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:oracle-door` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:overkill-ops` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:pit-arena` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:siege-engines` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:solar-realms-dominion` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:space-trader-wars` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:telnet-bridge` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:tournaments-achievements-center` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:voting-booth` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:word-duel-arena` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `door:yankee-trader-syndicate` | `door_module` | Y | N | Y | Y | `/admin/doors` | `/admin/doors + /scores + /admin/system` |
| `oputil:boards.create` | `admin_cli` | N | N | Y | N | `N/A (CLI command)` | `oputil status + /admin/system` |
| `oputil:boards.delete` | `admin_cli` | N | N | Y | N | `N/A (CLI command)` | `oputil status + /admin/system` |
| `oputil:boards.list` | `admin_cli` | N | N | Y | N | `N/A (CLI command)` | `oputil status + /admin/system` |
| `oputil:status` | `admin_cli` | N | N | Y | N | `N/A (CLI command)` | `oputil status + /admin/system` |
| `oputil:users.list` | `admin_cli` | N | N | Y | N | `N/A (CLI command)` | `oputil status + /admin/system` |
| `oputil:users.set-role` | `admin_cli` | N | N | Y | N | `N/A (CLI command)` | `oputil status + /admin/system` |
| `tui:stateBulletins` | `tui_state` | Y | N | N | N | `system.config_center (SSH) + /admin/config` | `system.status_center (SSH) + /admin/system` |
| `tui:stateConfigCenter` | `tui_state` | Y | N | Y | N | `system.config_center (SSH) + /admin/config` | `system.status_center (SSH) + /admin/system` |
| `tui:stateDoors` | `tui_state` | Y | N | Y | N | `system.config_center (SSH) + /admin/config` | `system.status_center (SSH) + /admin/system` |
| `tui:stateExit` | `tui_state` | Y | N | N | N | `system.config_center (SSH) + /admin/config` | `system.status_center (SSH) + /admin/system` |
| `tui:stateGateway` | `tui_state` | Y | N | Y | N | `system.config_center (SSH) + /admin/config` | `system.status_center (SSH) + /admin/system` |
| `tui:stateGuestTour` | `tui_state` | Y | N | N | N | `system.config_center (SSH) + /admin/config` | `system.status_center (SSH) + /admin/system` |
| `tui:stateLastCallers` | `tui_state` | Y | N | N | N | `system.config_center (SSH) + /admin/config` | `system.status_center (SSH) + /admin/system` |
| `tui:stateLogin` | `tui_state` | Y | N | N | N | `system.config_center (SSH) + /admin/config` | `system.status_center (SSH) + /admin/system` |
| `tui:stateMainMenu` | `tui_state` | Y | N | Y | N | `system.config_center (SSH) + /admin/config` | `system.status_center (SSH) + /admin/system` |
| `tui:stateStatusCenter` | `tui_state` | Y | N | Y | N | `system.config_center (SSH) + /admin/config` | `system.status_center (SSH) + /admin/system` |
| `tui:stateWelcome` | `tui_state` | Y | N | N | N | `system.config_center (SSH) + /admin/config` | `system.status_center (SSH) + /admin/system` |
| `tui:stateWhoOnline` | `tui_state` | Y | N | N | N | `system.config_center (SSH) + /admin/config` | `system.status_center (SSH) + /admin/system` |
| `web:/` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/.well-known/webfinger` | `web_route` | N | N | N | N | `/admin/config` | `/status` |
| `web:/admin` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/analytics` | `web_route` | N | N | Y | Y | `/admin/analytics` | `/admin/analytics + /status` |
| `web:/admin/audit` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/backups` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/boards` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/bulletins` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/challenges` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/chat` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/config` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/doors` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/errors` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/events` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/files` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/gateways` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/launch` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/login` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/mail` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/mentorship` | `web_route` | N | N | Y | Y | `/admin/mentorship` | `/admin/mentorship + /admin/audit` |
| `web:/admin/missions` | `web_route` | N | N | Y | Y | `/admin/missions` | `/admin/missions + /admin/release` |
| `web:/admin/mod-center` | `web_route` | N | N | Y | Y | `/admin/mod-center` | `/admin/mod-center + /status` |
| `web:/admin/node-state` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/ops` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/plugins` | `web_route` | N | N | Y | Y | `/admin/plugins` | `/admin/plugins + /admin/release` |
| `web:/admin/plugins/starter` | `web_route` | N | N | Y | Y | `/admin/plugins` | `/admin/plugins + /admin/release` |
| `web:/admin/release` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/setup` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/system` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/themes` | `web_route` | N | N | Y | Y | `/admin/themes` | `/admin/themes + /status` |
| `web:/admin/upgrade-safety` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/users` | `web_route` | N | N | Y | Y | `/admin/config` | `/admin/system` |
| `web:/admin/webhooks` | `web_route` | N | N | Y | Y | `/admin/webhooks` | `/admin/webhooks + /admin/errors` |
| `web:/ap/users/` | `web_route` | N | N | N | N | `/admin/config` | `/status` |
| `web:/attention` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/attention/export` | `web_route` | Y | Y | Y | Y | `/settings` | `/status` |
| `web:/boards` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/bookmarks` | `web_route` | Y | Y | Y | N | `/admin/config` | `/status` |
| `web:/bulletins` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/challenges` | `web_route` | Y | Y | Y | Y | `/admin/challenges` | `/status` |
| `web:/chat` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/chat/channels` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/chat/history` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/chat/join` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/chat/leave` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/chat/moderation` | `web_route` | Y | N | Y | Y | `/admin/config` | `/status` |
| `web:/chat/online` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/chat/send` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/chat/stream` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/circles` | `web_route` | Y | Y | Y | Y | `/settings` | `/status` |
| `web:/clubhouse` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/collections` | `web_route` | Y | Y | Y | Y | `/admin/files` | `/status` |
| `web:/config` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/connect` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/digest` | `web_route` | Y | Y | N | N | `/settings` | `/status` |
| `web:/digest/preferences` | `web_route` | Y | Y | Y | Y | `/settings` | `/digest + /status` |
| `web:/directory` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/discover` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/doors` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/doors/comeback` | `web_route` | Y | Y | Y | Y | `/doors` | `/doors/comeback + /streaks` |
| `web:/events` | `web_route` | Y | Y | Y | Y | `/admin/events` | `/status` |
| `web:/events/recaps` | `web_route` | Y | Y | Y | Y | `/admin/events` | `/status` |
| `web:/feedback` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/finder` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/first-call` | `web_route` | Y | Y | N | N | `/admin/setup` | `/status` |
| `web:/gateway` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/handles/suggest` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/healthz` | `web_route` | N | N | N | N | `N/A (operational endpoint)` | `/admin/system` |
| `web:/help` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/login` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/logout` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/mail` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/mail/inbound` | `web_route` | N | N | N | N | `/admin/gateways` | `/admin/system` |
| `web:/mentorship` | `web_route` | Y | Y | Y | Y | `/admin/mentorship` | `/mentorship + /mail` |
| `web:/metrics` | `web_route` | N | N | N | N | `N/A (operational endpoint)` | `/admin/system` |
| `web:/milestones` | `web_route` | Y | Y | Y | Y | `/missions` | `/milestones + /status` |
| `web:/missions` | `web_route` | Y | Y | Y | Y | `/admin/missions` | `/missions + /status` |
| `web:/newfiles` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/next` | `web_route` | Y | Y | Y | Y | `/admin/analytics` | `/next + /status` |
| `web:/offline` | `web_route` | Y | Y | Y | Y | `/admin/files + /settings` | `/status` |
| `web:/profile/export` | `web_route` | Y | Y | Y | Y | `/settings` | `/status` |
| `web:/radar` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/readyz` | `web_route` | N | N | N | N | `N/A (operational endpoint)` | `/admin/system` |
| `web:/reset/complete` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/reset/request` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/resume` | `web_route` | Y | Y | Y | Y | `/today` | `/resume + /status` |
| `web:/scores` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/settings` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/showcase` | `web_route` | Y | Y | Y | N | `/admin/config` | `/status` |
| `web:/spotlights` | `web_route` | Y | Y | Y | Y | `/admin/analytics` | `/spotlights + /status` |
| `web:/start` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/status` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/statusz` | `web_route` | Y | Y | Y | N | `/admin/config` | `/status` |
| `web:/streaks` | `web_route` | Y | Y | Y | Y | `/admin/analytics` | `/streaks + /status` |
| `web:/time-lane` | `web_route` | Y | Y | Y | Y | `/settings` | `/time-lane + /status` |
| `web:/today` | `web_route` | Y | Y | N | N | `/admin/events` | `/status` |
| `web:/topx` | `web_route` | Y | Y | N | N | `/admin/analytics` | `/topx + /status` |
| `web:/tour` | `web_route` | Y | Y | N | N | `/admin/config` | `/status` |
| `web:/tournaments` | `web_route` | Y | Y | Y | Y | `/admin/events` | `/status` |
