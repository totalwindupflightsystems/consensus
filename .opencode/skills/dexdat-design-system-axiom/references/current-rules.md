# Current Rules

This file is the stable doctrine store for reusable Dexdat design rules.

axiom:trace work_item=dexdat-design-system-01 spec=specs/README.md plan=.memory-bank/work-items/dexdat-design-system-01/plan.md doc=.opencode/skills/dexdat-design-system-axiom/references/current-rules.md evidence=.memory-bank/work-items/dexdat-design-system-01/verification.md commit=

## Purpose

Use this file for rules that should stay stable across multiple products and
surfaces. Keep project-specific choices out of this file unless they are later
promoted into reusable doctrine.

## Rule Buckets To Fill In Later

The designer source material is expected to populate at least these sections:

1. Design philosophy and north star
2. Brand character and emotional tone
3. Visual hierarchy laws
4. Typography system
5. Color system and contrast policy
6. Spacing rhythm and density laws
7. Grid and layout logic
8. Token hierarchy and naming policy
9. Component family rules
10. State model (default, hover, focus, active, disabled, loading, error, success)
11. Motion doctrine (timing families, easing, choreography rules)
12. Responsive and adaptive behavior rules
13. Platform differences (web/mobile/tablet/desktop)
14. Accessibility non-negotiables
15. Empty/loading/error-state doctrine
16. Iconography and illustration rules
17. Copy tone for UI labels and affordances
18. Review rubric and exception policy

## Fixed Doctrine vs Project Choice

For each future section, identify:
- what is globally fixed
- what is tunable per product
- what is experimental and should stay provisional

## Notes For Expansion

- If this file grows too large, split it into themed files under `references/`
  rather than making one monolith.
- Prefer one file per durable doctrine cluster when a section becomes large:
  `typography.md`, `motion.md`, `tokens.md`, `accessibility.md`, etc.
