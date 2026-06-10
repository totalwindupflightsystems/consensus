---
description: "Compliance adversarial review: privacy, accessibility, and cost-risk checks."
agent: dispatch-axiom
---

Run a compliance-focused adversarial review. Dispatches privacy, accessibility (for UI surfaces), and cost-risk agents.

Inputs:
- Target: $TARGET (required; work item ID, spec path, or "current")
- Scope: $SCOPE (optional; narrow to specific surfaces. Default: all)
- Surface: $SURFACE (optional; "ui" to include accessibility review. Default: auto-detect)
- Severity threshold: $SEVERITY (optional; minimum severity to report. Default: "medium")
- Output format: $OUTPUT (optional; "markdown", "json", "summary". Default: "markdown")
- Work item: $WORK_ITEM_ID (optional)

Skills (load on demand):
- `adversarial-review-axiom` — Finding format, severity classification, memory bank integration rules.
- `privacy-compliance-axiom` — PII detection, retention policy, consent flow validation.

Do:

1) Dispatch in parallel:
   - **@privacy-compliance-axiom** — PII detection, data retention policy, consent flow validation, GDPR/CCPA/HIPAA controls. Return privacy verdict (PASS|WARN|FAIL|BLOCKED) with score 0-100.
   - **@finops-cost-axiom** — Cost-risk detection, cardinality guardrails, scaling cost projections. Return cost risk findings.

2) If $SURFACE includes "ui" OR target touches UI surfaces:
   - **@accessibility-review-axiom** — WCAG 2.1 AA audit, keyboard navigation, screen reader compatibility. Return a11y verdict.

3) Merge findings, deduplicate, classify by severity. Apply $SEVERITY threshold.

4) Write findings to `.memory-bank/findings/compliance/` per adversarial-review-axiom skill §5.
   - Individual finding files: `.memory-bank/findings/compliance/<work-item-id>-<YYYYMMDD>-<slug>.md`
   - Update `.memory-bank/findings/_index.md` with new entries.
   - If $WORK_ITEM_ID is set, also write to `.memory-bank/work-items/$WORK_ITEM_ID/adversarial-review.md`.

5) Return consolidated compliance report.

Fail-closed: MUST return `status=fail` for CRITICAL compliance gaps (PII exposure, WCAG critical violations).

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating privacy verdict, a11y verdict (if applicable), and cost risk level.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Individual finding files in `.memory-bank/findings/compliance/`
  - `.memory-bank/findings/_index.md` (if updated)
  - `.memory-bank/work-items/$WORK_ITEM_ID/adversarial-review.md` (if $WORK_ITEM_ID set)
- `evidence.findings_paths`: list of individual finding file paths (for downstream agents to read)
- `evidence.privacy_verdict`: PASS|WARN|FAIL|BLOCKED with score 0-100
- `evidence.a11y_verdict`: PASS|WARN|FAIL|BLOCKED (or "not_applicable" if no UI surface)
- `evidence.cost_risk_level`: none|low|medium|high|critical
- `evidence.findings_count_by_severity`: `{ critical: N, high: N, medium: N, low: N }`
- `related_commands`: suggested follow-up commands
  - "To run a full adversarial review (all categories), run: `/axiom-adversary --target <same-target>`"
  - "To file critical compliance findings as Jira tickets, run: `/axiom-report-issue title=... type=bug severity=critical`"

### Cross-References
- "Finding format is defined in: `.opencode/skills/adversarial-review-axiom/SKILL.md`"
- "Spec: `specs/77-Adversarial-Review-System.md#REQ-ADV-005`"
- "For security-specific adversarial review, run: `/axiom-adversary-security --target <same-target>`"

axiom:trace spec=specs/77-Adversarial-Review-System.md#REQ-ADV-005 work_item=command-quality-01
