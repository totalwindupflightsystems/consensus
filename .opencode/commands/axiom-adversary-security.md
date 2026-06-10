---
description: "Security-focused adversarial review: threat model, exploitability, attack surface, and mitigations."
agent: dispatch-axiom
---

Run a security-focused adversarial review. Dispatches all 4 security agents in parallel per spec 77 REQ-ADV-002.

Inputs:
- Target: $TARGET (required; work item ID, spec path, or "current")
- Scope: $SCOPE (optional; narrow to specific files/components. Default: all security surfaces)
- Severity threshold: $SEVERITY (optional; minimum severity to report. Default: "medium")
- Output format: $OUTPUT (optional; "markdown", "json", "summary". Default: "markdown")
- Work item: $WORK_ITEM_ID (optional)

Skills (load on demand):
- `adversarial-review-axiom` — Finding format, severity classification, memory bank integration rules.
- `security-review-axiom` — Threat model methodology and security gate checklist.

Spec contract: `specs/77-Adversarial-Review-System.md#REQ-ADV-002`

Do:

1) Dispatch ALL 4 security agents in parallel (unconditional per REQ-ADV-002):
   - **@security-review-axiom** — Threat model, secrets hygiene, risk gates. Return threat model + risk score (0-100).
   - **@whitehat-axiom** — Authorized penetration validation, exploitability checks. Return exploitability findings.
   - **@redteam-axiom** — Adversarial falsification of claims, attack matrix. Return attack surface + falsified claims.
   - **@security-engineer-axiom** — Security architecture review, defense-in-depth validation, concrete mitigation patches. Return mitigations.

2) Merge findings, deduplicate, classify by severity. Apply $SEVERITY threshold.

3) Write findings to `.memory-bank/findings/security/` per spec 77 Finding Categories table.
   - Individual finding files: `.memory-bank/findings/security/<work-item-id>-<YYYYMMDD>-<slug>.md`
   - Update `.memory-bank/findings/_index.md` with new entries.
   - If $WORK_ITEM_ID is set, also write to `.memory-bank/work-items/$WORK_ITEM_ID/adversarial-review.md`.

4) Return consolidated security report.

Fail-closed: MUST return `status=fail` for CRITICAL findings. MUST return `status=warn` for HIGH findings.

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating security risk score and top finding.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Individual finding files in `.memory-bank/findings/security/`
  - `.memory-bank/findings/_index.md` (if updated)
  - `.memory-bank/work-items/$WORK_ITEM_ID/adversarial-review.md` (if $WORK_ITEM_ID set)
- `evidence.findings_paths`: list of individual finding file paths (for downstream agents to read)
- `evidence.risk_score`: 0-100 (from @security-review-axiom)
- `evidence.findings_count_by_severity`: `{ critical: N, high: N, medium: N, low: N }`
- `evidence.mitigations_available`: true|false (whether @security-engineer-axiom produced patches)
- `related_commands`: suggested follow-up commands
  - "To run a full adversarial review (all categories), run: `/axiom-adversary --target <same-target>`"
  - "To implement security mitigations, call: `@security-engineer-axiom` with the findings paths"
  - "To file critical security findings as Jira tickets, run: `/axiom-report-issue title=... type=security severity=critical`"
  - "To validate mitigations after fixing, run: `/axiom-adversary-security --target <same-target>`"

### Cross-References
- "Finding format is defined in: `.opencode/skills/adversarial-review-axiom/SKILL.md`"
- "Spec: `specs/77-Adversarial-Review-System.md#REQ-ADV-002`"
- "For compliance-specific adversarial review, run: `/axiom-adversary-compliance --target <same-target>`"

See: `specs/77-Adversarial-Review-System.md`

axiom:trace spec=specs/77-Adversarial-Review-System.md#REQ-ADV-002 work_item=command-quality-01
