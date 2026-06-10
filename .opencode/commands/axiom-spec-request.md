---
description: Turn a feature request into spec updates, then run spec verification.
agent: dispatch-axiom
---

Convert a feature request into spec updates, then verify spec consistency.

Inputs
- Feature request text: `$ARGUMENTS` (required)

Skills (load on demand):
- `axiom-copilot` — If the user is new, load this to explain what specs are, why they come before code, and how the spec-request flow works.
- `spec-kickoff-axiom` — For iterative spec refinement with multi-agent review.
- `traceability-doctrine` — For understanding trace markers and required links.
- `spec-writing-axiom` — Style guide for technical specs and behavior contracts.

Do (fail-closed)
1) Create or reuse a work item id (default: `feature-request-01` unless the user provides one).
2) Call `@specwriter-axiom` to draft/update specs under `specs/`:
   - requirements/invariants
   - open decisions (if needed)
   - trace markers `axiom:trace work_item=<ID> ...`
3) Call `@spec-verifier-axiom` to verify the updated specs:
   - report contradictions or missing propagations
4) If verifier returns FAIL/BLOCKED, inject corrective steps and do not claim success.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`).
- Use:
  - `<command>/axiom-spec-request</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence
  - `<evidence>` include:
    - `<work_item_id>`
    - `<specs_touched>` (semicolon-separated paths)
    - `<verifier_status>` (PASS|FAIL|BLOCKED)
  - `<diagnostics>` for warnings/errors

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating which specs were updated and whether verification passed.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL spec files created/modified (full paths, semicolon-separated)
- `evidence.work_item_id`: the work item ID used
- `evidence.specs_touched`: list of spec file paths that were created or updated
- `evidence.verifier_status`: PASS|FAIL|BLOCKED
- `related_commands`: suggested follow-up commands
  - "To create a work item plan for this feature, run: `/axiom-plan --work-item <id>`"
  - "To kick off full implementation, run: `/axiom-kickoff <same-request>`"
  - "To run a contradiction check on the updated specs, run: `/axiom-contradict specs/<updated-spec>.md`"

### Cross-References
- "Spec style guide is in: `.opencode/skills/spec-writing-axiom/SKILL.md`"
- "Spec inventory is at: `specs/README.md`"
- "To view the updated spec, read the path returned in `evidence.specs_touched`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
