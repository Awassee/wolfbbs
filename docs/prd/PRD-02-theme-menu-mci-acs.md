# PRD-02: Theme System, HJSON Menus, MCI/View Framework, ACS Rules

## Scope
This slice covers four tightly related UX/runtime systems:
- Theme system (art + palette config + user-selectable theme)
- Menu system driven by HJSON
- MCI/View control layer (lightbars, toggles, inputs)
- ACS rule engine used uniformly across menu entries and content operations

## Goals
1. Remove hardcoded screen flows where possible.
2. Allow adding/changing menus without recompiling the binary.
3. Introduce consistent access control decisions (`allow/deny`) from one engine.
4. Keep 80x25 ANSI nostalgia behavior and hotkey latency.

## Functional requirements
- PRD02-001: Named themes with fallback resolution and deterministic defaults.
- PRD02-002: User preference selects theme and applies to SSH screens at login.
- PRD02-003: HJSON menu file format supports:
  - screen id/title
  - entries with hotkey, label, action, target, and optional ACS expression
  - optional help/footer text
- PRD02-004: Menu module plugin model supports built-in handlers + external module bindings.
- PRD02-005: MCI parser supports minimum controls:
  - label/text
  - input
  - toggle
  - lightbar list
  - action button
- PRD02-006: ACS engine expression model supports:
  - role checks (`user/moderator/sysop`, with `admin` alias accepted)
  - verified account check
  - board/channel-specific permissions
  - boolean operators (`and`, `or`, `not`)
- PRD02-007: ACS evaluation is enforced in:
  - menu entry visibility
  - board read/write
  - file area access
  - admin route guards

## Acceptance criteria
- A01: Unit tests for theme selection/fallback and ACS expression evaluation.
- A02: At least one major screen (main menu) loads from HJSON and functions with hotkeys.
- A03: At least one lightbar/toggle/input form is rendered by MCI view layer in SSH.
- A04: Unauthorized entries are hidden/blocked consistently in SSH and web admin.

## Current implementation baseline
- Implemented in this pass:
  - Named theme registry and fallback in `internal/ui/themes.go`.
  - SSH session applies user-selected theme after login.
  - HJSON menu foundation in `internal/menu`:
    - schema parser/validator
    - hotkey lookup helper
    - module registry execution API
    - sample menu at `menus/main.hjson`
  - ACS evaluator in `internal/acs`:
    - expression parsing for `and/or/not`, role, verified, and attribute checks
    - wired into SSH menu visibility and board/mail/files access gates
  - MCI view foundation in `internal/mci`:
    - control schema + HJSON parsing + render helpers
    - integrated into SSH `(S)ettings` flow with persisted preference updates
- Post-baseline enhancement track:
  - Continue pushing more secondary SSH/web surfaces through the shared runtime structures where it improves maintainability.
  - Keep tightening ACS coverage and test depth as new caller/sysop surfaces are added.
  - Expand richer lightbar/input usage where it materially improves usability rather than just increasing framework surface area.

## Rollout
- Default remains current hardcoded menus until `menus.enabled=true` config toggle is set.
- ACS starts in permissive mode for existing entities; strict mode behind `acs.strict=true`.
