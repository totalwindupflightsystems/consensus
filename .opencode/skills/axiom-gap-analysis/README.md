# axiom-gap-analysis

OpenCode skill for multi-agent gap analysis and progress reporting with fail-closed evidence rules.

## Quick use

- Suggested slash command: `/axiom-gap-analysis`
- Required inputs: `WORK_ITEM_ID`, `SCOPE_PATHS`, `TIME_WINDOW`, `CONSTRAINTS`
- Rerun control: `RERUN_BUDGET` is enforced (defaults to `2` when omitted).
- Naming: use `@agent` handles in invocation text; use installed agent handles from `.opencode/agents/` in structured fields.

Example prompt packet:

```text
Run /axiom-gap-analysis
WORK_ITEM_ID=spec-conformance-recovery
SCOPE_PATHS=.axiom/**,.memory-bank/work-items/spec-conformance-recovery/**
TIME_WINDOW=2026-02-07T00:00:00Z..2026-02-07T23:59:59Z
CONSTRAINTS=no_breaking_changes=true;allow_destructive_commands=false
PLAN_REF=phase-3/task-3-1
SPEC_REFS=specs/<repo-spec>.md#anchor
```

## Expected output

- Final merged report with `gaps[]` entries containing severity, evidence, owner, and next step.
- Gap statuses use strict semantics: `open|resolved|unverified|deferred`.
- Confidence block aligned to `.opencode/skills/axiom-confidence-scoring/SKILL.md`.
- Deferred credential-gated items called out explicitly as `status=deferred`.
- PASS is fail-closed: no unresolved critical/high required gaps, no unverified required gates, and no rerun stop-rule violation.
- Trace line in generated artifacts:
  - `axiom:trace work_item=<ID> spec=<REF> plan=<phase/task/step> ...`

## Tracking record

- Work item record: `.memory-bank/work-items/skill-gap-analysis-01/`
- Primary plan step for this skill bootstrap: `phase-1/task-1-1/step-1-1-1`

axiom:trace work_item=skill-gap-analysis-01 spec=specs/<repo-spec>.md#anchor plan=phase-1/task-1-1/step-1-1-1 test= doc=.opencode/skills/axiom-gap-analysis/README.md prompt=.opencode/skills/axiom-gap-analysis/SKILL.md evidence=.memory-bank/work-items/skill-gap-analysis-01/verification.md commit=
