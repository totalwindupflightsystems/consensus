---
mb:
  type: note
  title: "Meta-Planning: onboarding-01"
  created: YYYY-MM-DD
  updated: YYYY-MM-DD
  tags: [meta-planning, onboarding]
  links:
    up: "../_index.md"
    related: ["plan.md", "plan.yaml", "verification.md"]
  source:
    type: manual
    ref: "onboarding"
  git:
    commit: ""
    paths: []
---

# Meta-Planning: onboarding-01

## Summary

Bootstrap this repository so Axiom can plan and execute work safely.

## Scope

- **In scope**: create or validate base scaffolding, generate TODO + implementation plans, generate a runnable Ralph loop scaffold.
- **Out of scope**: implementing product features.

## Acceptance Criteria

1. `.memory-bank/TODO.md` exists and is updated from repo specs/docs.
2. `.memory-bank/implementation-plans/` exists and has an index.
3. A work item exists for ongoing work with plans and verification (`.memory-bank/work-items/onboarding-01/`).
4. A Ralph loop scaffold exists (`PROMPT.md` + runner script) and points at `.memory-bank/work-items/_current.md`.

axiom:trace work_item=onboarding-01 spec=specs/_prompt.md plan=.memory-bank/work-items/onboarding-01/plan.md evidence=.memory-bank/work-items/onboarding-01/verification.md commit=

## Sources

- `.opencode/commands/axiom-bootstrap.md`
- `.opencode/skills/ralph-wiggum-loop/SKILL.md`
- `.opencode/skills/meta-plan-axiom/SKILL.md`
