---
description: "Resilience adversarial review: fault injection, chaos experiments, SLO gaps, and runbook validation."
agent: dispatch-axiom
---

Run a resilience-focused adversarial review. Dispatches `@chaos-engineer-axiom` and `@sre-ops-axiom` in parallel.

Inputs:
- Target: $TARGET (required; work item ID, service name, or "current")
- Scope: $SCOPE (optional; narrow to specific services/components. Default: all)
- Severity threshold: $SEVERITY (optional; minimum severity to report. Default: "medium")
- Output format: $OUTPUT (optional; "markdown", "json", "summary". Default: "markdown")
- Work item: $WORK_ITEM_ID (optional)

Skills (load on demand):
- `adversarial-review-axiom` — Finding format, severity classification, memory bank integration rules.
- `sre-ops-axiom` — SLO/SLI definitions, deploy safety, observability setup.
- `chaos-engineer-axiom` — Fault injection patterns, resilience testing methodology.

Do:

1) Dispatch in parallel:
   - **@chaos-engineer-axiom** — Fault injection patterns, resilience testing, runbook validation under failure conditions, RTO/RPO verification. Return chaos findings + resilience verdict (PASS|WARN|FAIL|BLOCKED).
   - **@sre-ops-axiom** (OPTIONAL) — SLO/SLI gaps, alert coverage, runbook linkage, observability gaps. Return SRE findings. If unavailable, log WARN and produce partial report noting the gap.

2) Merge findings, deduplicate, classify by severity. Apply $SEVERITY threshold.

3) Write findings to `.memory-bank/findings/resilience/` per adversarial-review-axiom skill §5.
   - Individual finding files: `.memory-bank/findings/resilience/<work-item-id>-<YYYYMMDD>-<slug>.md`
   - Update `.memory-bank/findings/_index.md` with new entries.
   - If $WORK_ITEM_ID is set, also write to `.memory-bank/work-items/$WORK_ITEM_ID/adversarial-review.md`.

4) Return consolidated resilience report.

Fail-closed: MUST return `status=fail` for CRITICAL resilience gaps (no runbook for critical failure mode, RTO not achievable).

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating resilience verdict and top gap.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Individual finding files in `.memory-bank/findings/resilience/`
  - `.memory-bank/findings/_index.md` (if updated)
  - `.memory-bank/work-items/$WORK_ITEM_ID/adversarial-review.md` (if $WORK_ITEM_ID set)
- `evidence.findings_paths`: list of individual finding file paths (for downstream agents to read)
- `evidence.resilience_verdict`: PASS|WARN|FAIL|BLOCKED
- `evidence.sre_findings_available`: true|false (false if @sre-ops-axiom was unavailable)
- `evidence.findings_count_by_severity`: `{ critical: N, high: N, medium: N, low: N }`
- `related_commands`: suggested follow-up commands
  - "To run a full adversarial review (all categories), run: `/axiom-adversary --target <same-target>`"
  - "To create runbooks for critical failure modes, call: `@docs-runbooks-axiom`"
  - "To file critical resilience findings as Jira tickets, run: `/axiom-report-issue title=... type=bug severity=critical`"

### Cross-References
- "Finding format is defined in: `.opencode/skills/adversarial-review-axiom/SKILL.md`"
- "Spec: `specs/77-Adversarial-Review-System.md#REQ-ADV-004`"
- "For security-specific adversarial review, run: `/axiom-adversary-security --target <same-target>`"

axiom:trace spec=specs/77-Adversarial-Review-System.md#REQ-ADV-004 work_item=command-quality-01
