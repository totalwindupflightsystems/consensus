---
description: "KISS adversarial review: challenge a plan, spec, or design for overcomplexity. Scores complexity, maps steps to AC, and proposes a simplified plan."
agent: dispatch-axiom
---

Run a KISS (Keep It Simple, Stupid) adversarial review. Dispatches `@kiss-axiom` to challenge the target plan or spec for overcomplexity.

Inputs:
- Target: $TARGET (required; work item ID, plan path, spec path, or "current")
- Acceptance criteria: $AC (optional; comma-separated list. Default: inferred from work item or spec)
- Task size: $TASK_SIZE (optional; trivial|small|medium|large|epic. Default: inferred)
- Scope: $SCOPE (optional; narrow to specific plan steps or spec sections. Default: all)
- Severity threshold: $SEVERITY (optional; critical|high|medium|low. Default: medium)
- Output format: $OUTPUT (optional; markdown|json|summary. Default: markdown)
- Work item: $WORK_ITEM_ID (optional)

Skills (load on demand):
- `kiss-axiom` — Complexity score formula, thresholds, progressive ceremony guidelines, KISS rules, red flags, simplification heuristics.

Do:

1) Load the `kiss-axiom` skill.

2) Resolve $TARGET to a plan or spec:
   - If $TARGET is a file path: read it directly.
   - If $TARGET is a work item ID: read `.memory-bank/work-items/$TARGET/plan.yaml` or `.memory-bank/TODO.md` entries for that work item.
   - If $TARGET is "current": read `.memory-bank/work-items/_current.md` to find the active work item, then read its plan.
   - Extract acceptance criteria from the work item's `work-item.md` if $AC is not provided.

3) Dispatch **@kiss-axiom** with:
   - The plan steps or spec sections from $TARGET
   - The acceptance criteria ($AC or inferred)
   - The task size ($TASK_SIZE or inferred)
   - The constraints from the work item

4) @kiss-axiom returns a YAML complexity review with:
   - `complexity_score` and whether it exceeds the threshold for task size
   - `step_ac_mapping`: every step mapped to an AC (or flagged as "NONE")
   - `red_flags`: overcomplexity signals
   - `simplified_plan`: a leaner plan that still passes all AC
   - `ceremony_recommendation`: what ceremony level is appropriate
   - `ceremony_skips`: what to skip and why
   - `injected_steps`: concrete next steps for @pm-axiom or @specwriter-axiom

5) Return the KISS review report.

Fail-closed: MUST return `status=fail` when complexity score exceeds threshold AND a simplified plan is not yet adopted. MUST return `status=blocked` when AC are missing or untestable.

axiom:trace work_item=DEX-384 spec=specs/77-Adversarial-Review-System.md#REQ-ADV-022 doc=.opencode/commands/axiom-adversary-kiss.md jira_ref=DEX-384
