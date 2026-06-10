---
name: kiss-axiom
description: KISS (Keep It Simple, Stupid) simplicity enforcer for Axiom. Challenges plans, specs, and designs for overcomplexity. Defines complexity thresholds, progressive ceremony guidelines, and a complexity score formula. Load when reviewing plans, before claiming a plan is ready, or when a task feels bigger than it should be.
version: "1.0"
tags:
  vertical: [coding, planning]
  category: methodology
  core: false
---

# KISS — Keep It Simple, Stupid

Load this skill when a plan, spec, or design might be more complex than the task warrants.

**Relationship to sibling skills:**

| Skill | What it fixes | KISS difference |
|---|---|---|
| `too-much-of-a-good-thing-axiom` | Over-structured **output** (writing, docs, formatting) | KISS fixes over-engineered **plans and execution** |
| `baby-steps-methodology` | Smallest meaningful **change** per step | Baby-steps doesn't prevent scope creep in **planning** |
| `working-backwards-axiom` | Plans from user experience backward | KISS challenges whether the plan is too big for the goal |

axiom:trace work_item=DEX-384 spec=specs/77-Adversarial-Review-System.md#REQ-ADV-022 doc=.opencode/skills/kiss-axiom/SKILL.md jira_ref=DEX-384

---

## Core Principle

The best plan is the one with the fewest steps that still passes all acceptance criteria.

Every step, phase, agent, spec section, and ceremony that doesn't directly serve an acceptance criterion is complexity debt. Complexity debt slows delivery, increases failure surface, and makes verification harder.

---

## Complexity Score Formula

Assign a complexity score to any plan before executing it.

```
complexity_score = (plan_steps × 1)
                 + (phases × 3)
                 + (agents_dispatched × 2)
                 + (new_files_created × 1)
                 + (spec_sections_added × 2)
                 + (adversarial_reviews_required × 3)
```

**Thresholds by task size:**

| Task size | Definition | Max complexity score |
|---|---|---|
| Trivial | < 3 files changed, < 50 lines, no new behavior | 10 |
| Small | 3–10 files, < 200 lines, one new behavior | 25 |
| Medium | 10–30 files, one new subsystem or API | 50 |
| Large | 30+ files, multiple subsystems, migrations | 100 |
| Epic | Multi-sprint, cross-team, architectural change | No cap (but justify every point) |

If `complexity_score > threshold`, the plan MUST be simplified before execution.

### Formula Term Definitions

Count these precisely when scoring a plan:

| Term | What to count |
|---|---|
| `plan_steps` | Each discrete checkbox or numbered action in the plan. A multi-clause step ("implement X, write test, commit") counts as 1. Sub-steps within a single action count as 1 together. |
| `phases` | Top-level phases in the plan (e.g., Phase 1, Phase 2). A single-phase plan scores 3; a 5-phase plan scores 15. |
| `agents_dispatched` | Distinct subagents invoked (e.g., @dev-axiom, @qa-axiom). Count unique agents, not invocations. |
| `new_files_created` | Net new files added to the repo. Edits to existing files score 0. |
| `spec_sections_added` | New `##` or `###` sections added to spec files. Edits to existing sections score 0. |
| `adversarial_reviews_required` | Distinct adversarial review passes planned (e.g., one `/axiom-adversary-assumptions` run = 1). |

### Task Size Inference

When `task_size` is not explicitly provided, infer it from the plan:

| Signal | Inferred size |
|---|---|
| Plan mentions < 3 files, < 50 lines, no new behavior | Trivial |
| Plan mentions 3–10 files or one new behavior | Small |
| Plan mentions a new subsystem, API, or 10–30 files | Medium |
| Plan mentions migrations, 30+ files, or multiple subsystems | Large |
| Plan spans multiple sprints, teams, or architectural changes | Epic |

When signals conflict, use the highest-severity signal. When signals are absent, default to **Small** and note the assumption in output.

---

## Progressive Ceremony Guidelines

Not every task needs the full Axiom ceremony. Match effort to risk.

### Trivial tasks (score ≤ 10)

Skip:
- Meta-plan
- Adversarial reviews (assumption-buster, devils-advocate, redteam)
- Multi-phase rollouts
- Separate spec sections (add a note to an existing spec instead)
- Runbooks (unless the change touches an existing alert)

Do:
- One-step implementation
- Inline verification (grep/test command)
- Single commit with trace marker

### Small tasks (score ≤ 25)

Skip:
- Multi-phase rollouts (one phase is fine)
- Full adversarial review suite (one targeted review if risk warrants)
- Separate runbook (update existing runbook instead)

Do:
- Spec stub or note in existing spec
- Plan with 3–7 steps
- Unit + smoke test
- Trace marker in code and spec

### Medium tasks (score ≤ 50)

Use standard Axiom ceremony:
- Spec update with AC
- Plan with phases and gates
- QA + spec-verifier
- One adversarial review if risk warrants
- Docs/runbook update if operator-facing

### Large / Epic tasks (score > 50)

Use full ceremony. Every added step must justify its complexity score cost.

---

## KISS Rules

### Rule 1: Cut scope to AC

If a requirement isn't in the acceptance criteria, it doesn't belong in the plan. Before adding any step, ask: *which AC does this serve?* If the answer is "none," cut it.

### Rule 2: Prefer extending over creating

Before creating a new file, agent, spec, or table, check whether an existing one can be extended. New files cost complexity points. Extensions are free.

### Rule 3: YAGNI

Don't build for hypothetical future needs unless explicitly asked. "We might need this later" is not an acceptance criterion.

### Rule 4: Match effort to risk

Low-risk changes get lightweight verification. High-risk changes get full ceremony. Don't apply mission-critical gates to a 3-line config change.

### Rule 5: Smallest viable change

The plan that passes AC with the fewest steps is correct. A longer plan is not more thorough — it's more expensive.

### Rule 6: One phase unless you need two

Multi-phase rollouts exist for risky, reversible changes. If the change is small and rollback is trivial, one phase is enough.

---

## Red Flags (plan is too complex)

- Plan has more steps than the task has acceptance criteria
- Every step has the same ceremony regardless of risk
- Adversarial reviews are scheduled for a 3-file change
- A new spec file is created for a feature that could be a section in an existing spec
- The rollback plan is more work than the change itself
- "Phase 1" is just "write the spec" for a trivial task
- More than 3 agents are dispatched for a small task
- The plan includes steps that don't map to any AC

---

## Simplification Heuristics

When the complexity score exceeds the threshold:

1. **Map every step to an AC.** Remove steps with no AC mapping.
2. **Collapse phases.** If phases can be merged without increasing risk, merge them.
3. **Reduce agent dispatch.** Use the minimum set of agents that covers the risk surface.
4. **Extend, don't create.** Replace "create new spec" with "add section to existing spec."
5. **Defer non-AC work.** Move "nice to have" steps to a follow-up work item.
6. **Inline verification.** Replace a full QA phase with a targeted test command when the change is small.

---

## Complexity Audit Checklist

Run this before approving any plan:

- [ ] Every plan step maps to at least one AC
- [ ] Complexity score is within threshold for task size
- [ ] No new files created when an existing file could be extended
- [ ] No adversarial reviews scheduled for trivial/small tasks unless risk warrants
- [ ] Multi-phase rollout is justified by rollback risk, not habit
- [ ] Agent dispatch list is the minimum needed to cover the risk surface
- [ ] "Nice to have" work is deferred, not included in this plan

---

## Integration with `too-much-of-a-good-thing-axiom`

KISS and TMGT are complementary:

- **KISS** fires at **planning time** — before the plan is executed.
- **TMGT** fires at **output time** — after an artifact is produced.

If a plan passes KISS but the resulting spec or doc feels over-structured, load `too-much-of-a-good-thing-axiom` to fix the output.

---

## One-Line Reminder

If the plan is bigger than the problem, the plan is the problem.
