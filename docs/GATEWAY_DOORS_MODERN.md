# Gateway Doors Modernization Spec

## Goal
Ship six practical "modern internet gateway doors" inside WolfBBS with smooth caller UX and operator controls.

## Scope
1. Text Web Browser Door (`/gateway?view=browser`)
2. Email Gateway Door (`/gateway?view=email`)
3. Generative AI Door (`/gateway?view=ai`)
4. RSS/Atom Feed Reader Door (`/gateway?view=rss`)
5. Article Summarizer Door (`/gateway?view=summarize`)
6. JSON API Explorer Door (`/gateway?view=json`)

## UX Contract
- `/gateway` is now a hub page with cards for all six doors.
- Every door has:
  - clear description
  - dedicated form
  - direct links to adjacent doors
  - back-link to its own door home after execution
- Door Cockpit (`/doors`) links to gateway doors for discoverability.

## Safety Contract
- All web-facing fetch doors use existing gateway URL safety controls:
  - SSRF host/IP blocking
  - redirect limit
  - timeout and body-size limits from gateway settings
- All POST interactions remain CSRF-protected.
- FileBase actions remain policy-gated by file ACS checks.

## Operator Contract
- `/admin/gateways` now controls:
  - SMTP relay + web fetch safety limits
  - AI gateway settings (enabled, base URL, model, API key, timeout, max tokens, system prompt)
- Sensitive fields preserve existing stored values when left blank (SMTP pass / AI API key).
- Page shows diagnostics for relay and AI readiness.

## Functional Contract
- External email send path uses runtime gateway settings (not env-only bootstrap defaults).
- Password reset mail delivery also uses runtime gateway settings.
- AI door calls an OpenAI-compatible `chat/completions` endpoint and labels output.

## Test Plan
- Unit tests (`internal/gateway`):
  - AI client request/response behavior
  - feed parsing
  - summarizer fallback behavior
  - unsafe URL rejection for URL-based tools
- Web tests (`cmd/wolfbbs-web/main_test.go`):
  - gateway hub exposes all six doors
  - AI config persists from admin gateways and is visible in AI door
  - admin gateway SMTP settings are honored by outbound mail validation path

## Out of Scope
- Full interactive ANSI terminal streaming for gateway doors in this change.
- Non-OpenAI protocol AI providers.
- Authenticated API explorer sessions (only anonymous GET JSON fetch in this tranche).
