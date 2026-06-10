# Axiom Implementation Plans Maintenance

You are maintaining project-level implementation plan markdown files under:
- `.memory-bank/implementation-plans/`

Requirements:
- Read `.opencode/skills/implementation-plan-history/SKILL.md` for how plans relate to work-items.
- Read `.memory-bank/TODO.md`.
- Read `.memory-bank/implementation-plans/_index.md`.
- Read all existing `.memory-bank/implementation-plans/P-*.md` files.

Rules:
- One plan per TODO phase section (Phase 0..N) plus one plan for the "Start Here" bundle.
- Plans are high-signal and stable; avoid churn.
- Each plan must include:
  - Purpose, Scope (in/out)
  - Spec Trace (paths; headings if known)
  - Artifacts to produce (paths)
  - Baby-step tasks with Done Evidence
  - Verification checklist
  - Risks/Open questions only if real

Output:
- Update `.memory-bank/implementation-plans/_index.md` if needed.
- Create/update plan files to match `.memory-bank/TODO.md`.
- Summarize changes in 3-8 bullets.
