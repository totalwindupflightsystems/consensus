---
description: "Assumption-busting adversarial review: surface undocumented prerequisites, ambiguous specs, and challenge implicit tradeoffs."
agent: dispatch-axiom
---

Run an assumption-busting adversarial review. Dispatches `@assumption-buster-axiom` and `@devils-advocate-axiom` in parallel.

Inputs:
- Target: $TARGET (required; work item ID, spec path, plan path, or "current")
- Scope: $SCOPE (optional; narrow to specific specs/plans. Default: all relevant specs)
- Severity threshold: $SEVERITY (optional; minimum severity to report. Default: "medium")
- Output format: $OUTPUT (optional; "markdown", "json", "summary". Default: "markdown")
- Work item: $WORK_ITEM_ID (optional)

Skills (load on demand):
- `adversarial-review-axiom` — Finding format, severity classification, memory bank integration rules.

Do:

1) Dispatch in parallel:
   - **@assumption-buster-axiom** — Surface undocumented prerequisites, ambiguous specs, non-verifiable acceptance criteria. Return findings with severity and recommended fixes.
   - **@devils-advocate-axiom** — Challenge the implementation: simplest thing that works? Where will it break? Implicit tradeoffs? Return challenge pack with pressure tests and risks.

2) Merge findings, deduplicate, classify by severity. Apply $SEVERITY threshold.

3) Write findings to `.memory-bank/findings/assumptions/` per adversarial-review-axiom skill §5.
   - Individual finding files: `.memory-bank/findings/assumptions/<work-item-id>-<YYYYMMDD>-<slug>.md`
   - Update `.memory-bank/findings/_index.md` with new entries.
   - If $WORK_ITEM_ID is set, also write to `.memory-bank/work-items/$WORK_ITEM_ID/adversarial-review.md`.

4) Return consolidated assumption-busting report.

Fail-closed: MUST return `status=fail` for CRITICAL findings (non-verifiable AC, missing prerequisites that block implementation).

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many assumption findings were found and at what severity.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Individual finding files in `.memory-bank/findings/assumptions/`
  - `.memory-bank/findings/_index.md` (if updated)
  - `.memory-bank/work-items/$WORK_ITEM_ID/adversarial-review.md` (if $WORK_ITEM_ID set)
- `evidence.findings_paths`: list of individual finding file paths (for downstream agents to read)
- `evidence.findings_count_by_severity`: `{ critical: N, high: N, medium: N, low: N }`
- `related_commands`: suggested follow-up commands
  - "To run a full adversarial review (all categories), run: `/axiom-adversary --target <same-target>`"
  - "To file critical findings as Jira tickets, run: `/axiom-report-issue title=... type=bug severity=critical`"
  - "To verify the spec after fixing assumptions, run: `/axiom-verify --work-item <id>`"

### Cross-References
- "Finding format is defined in: `.opencode/skills/adversarial-review-axiom/SKILL.md`"
- "Spec: `specs/77-Adversarial-Review-System.md#REQ-ADV-003`"
- "For security-specific adversarial review, run: `/axiom-adversary-security --target <same-target>`"

axiom:trace spec=specs/77-Adversarial-Review-System.md#REQ-ADV-003 work_item=command-quality-01
