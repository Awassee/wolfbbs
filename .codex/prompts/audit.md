---
description: "Audit a WolfBBS UI surface using the local Impeccable design skills"
argument-hint: "[route or surface]"
---

Use the local Impeccable skills in `.codex/skills`.

Steps:
1. Read `.impeccable.md` for WolfBBS design context.
2. Use the `frontend-design` skill as the design-quality baseline.
3. Use the `audit` skill to evaluate the requested surface. If no argument is provided, audit the shared web shell plus the densest routes: `/start`, `/boards`, `/chat`, `/admin/setup`, and `/admin/config`.
4. Focus on anti-patterns, accessibility, responsive behavior, visual overload, and operator clarity.
5. Return prioritized findings with exact file references and practical fixes.

Target: $ARGUMENTS
