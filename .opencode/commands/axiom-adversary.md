---
description: "Run all adversarial agents against a work item, spec, plan, or code surface. Returns a consolidated adversarial report."
agent: dispatch-axiom
---

Run a full adversarial review against the specified target. Dispatches all adversarial agent categories in parallel and returns a consolidated findings report written to `.memory-bank/findings/`.

Inputs:
- Target: $TARGET (required; work item ID, spec path, plan path, or "current" for active work item)
- Category: $CATEGORY (optional; "full" = all categories, "security", "assumptions", "resilience", "compliance", "kiss". Default: "full")
- Scope: $SCOPE (optional; file paths or directories to limit the review surface. Default: derived from $TARGET)
- Severity threshold: $SEVERITY (optional; minimum severity to report: "critical", "high", "medium", "low". Default: "medium")
- Output format: $OUTPUT (optional; "markdown", "json", "summary". Default: "markdown")
- Timeout: $TIMEOUT (optional; seconds per agent before timeout. Default: 300)
- Work item: $WORK_ITEM_ID (optional; work item to write findings to. Default: derived from $TARGET)

Target resolution:
- When $TARGET is "current": read `.memory-bank/work-items/_current.md` to identify the active work item ID. If the file does not exist or is empty, fail with: "No active work item found. Specify $TARGET explicitly."
- When $TARGET is a file path: use as the scope for all agents.
- When $TARGET is a work item ID: load the work item's plan and changed files as scope.

Skills (load on demand):
- `adversarial-review-axiom` — Full adversarial review methodology, finding format, severity classification, and memory bank integration rules.
- `evidence-bundle-schema` — For structuring findings as evidence.
- `axiom-confidence-scoring` — For scoring confidence based on adversarial findings.

Spec contract: `specs/77-Adversarial-Review-System.md`

Do:

**If $CATEGORY is "full" (default)**: Dispatch all 5 adversarial categories in parallel (steps 1-5 below).

**If $CATEGORY is a specific category**: Dispatch only that category.

1) **Security adversarial review** (when category includes "security"):
   Dispatch in parallel:
   - **@security-review-axiom** — Threat model, secrets hygiene, risk gates. Return threat model + risk score.
   - **@whitehat-axiom** — Authorized penetration validation, exploitability checks. Return exploitability findings.
   - **@redteam-axiom** — Adversarial falsification of claims, attack matrix. Return attack surface + falsified claims.
   - **@security-engineer-axiom** — Concrete mitigation patches for any findings. Return mitigations.

2) **Assumption adversarial review** (when category includes "assumptions"):
   Dispatch in parallel:
   - **@assumption-buster-axiom** — Surface undocumented prerequisites, ambiguous specs, non-verifiable work. Return findings with severity.
   - **@devils-advocate-axiom** — Challenge the implementation: simplest thing that works? Where will it break? Implicit tradeoffs? Return challenge pack.
   - **@strategy-falsifier-axiom** — Pre-implementation hypothesis challenge. Only dispatch if $TARGET is a work item in planning state (not a completed work item or post-hoc review). If target is post-implementation, skip with note: "Strategy falsification is a pre-implementation gate; skipped for post-hoc review."

3) **Resilience adversarial review** (when category includes "resilience"):
   Dispatch in parallel:
   - **@chaos-engineer-axiom** — Fault injection patterns, resilience testing, runbook validation. Return chaos findings + RTO/RPO gaps.
   - **@sre-ops-axiom** — SLO/SLI gaps, alert coverage, runbook linkage. Return observability gaps. (OPTIONAL per spec 22; if unavailable, log WARN and produce partial report noting gap.)

4) **Compliance adversarial review** (when category includes "compliance"):
   Dispatch in parallel:
   - **@privacy-compliance-axiom** — PII detection, retention policy, consent flow. Return privacy verdict (PASS|WARN|FAIL).
   - **@accessibility-review-axiom** — WCAG audit, a11y gaps (for UI surfaces only; skip if no UI files in $SCOPE). Return a11y verdict.
   - **@finops-cost-axiom** — Cost-risk detection, cardinality guardrails. Return cost risk findings.

5) **KISS simplicity review** (when category includes "kiss"):
   Dispatch:
   - **@kiss-axiom** — Score plan/spec complexity, map steps to AC, enforce progressive ceremony, propose simplified plan. Return complexity review.

6) **Merge and classify all findings**:
   - Collect findings from all dispatched agents.
   - Deduplicate per REQ-ADV-018: findings with identical scope AND category are candidates. Merged finding lists all contributing agents. Severity = max of all duplicates.
   - Classify by severity: CRITICAL, HIGH, MEDIUM, LOW.
   - Apply severity threshold filter ($SEVERITY).

7) **Write findings to memory bank**:
   - Write individual findings to `.memory-bank/findings/<category>/<work-item-id>-<YYYYMMDD>-<slug>.md`
   - Format per `adversarial-review-axiom` skill §4 (finding format).
   - Update `.memory-bank/findings/_index.md` with new entry.
   - If $WORK_ITEM_ID is set, also write to `.memory-bank/work-items/$WORK_ITEM_ID/adversarial-review.md`.

8) **Emit structured summary**:
   - Total findings by severity.
   - Top 3 most critical findings.
   - Recommended next steps.
   - "Run `/axiom-adversary-security` for deeper security analysis" if security findings exist.

Fail-closed rules:
- MUST return `status=fail` if any CRITICAL finding is found.
- MUST return `status=warn` if any HIGH finding is found (but no CRITICAL).
- MUST write findings to memory bank before returning.
- MUST NOT return `status=ok` if findings exist above the severity threshold.
- If no findings at severity threshold or above: return `status=ok`.

## Output Contract (XML envelope)

The final message MUST contain a `<axiom>` XML envelope with these tags:

```xml
<axiom>
  <status>ok|warn|fail</status>
  <confidence>0-100</confidence>
  <summary>One sentence: total findings by severity and top category.</summary>
  <detailed_summary>
    Findings by category, top 3 critical findings, recommended next steps.
  </detailed_summary>
  <evidence>
    <files_changed>path1;path2;path3</files_changed>
    <findings_paths>finding1.md;finding2.md</findings_paths>
    <findings_by_severity>
      <critical>N</critical>
      <high>N</high>
      <medium>N</medium>
      <low>N</low>
    </findings_by_severity>
    <categories_reviewed>security;assumptions;resilience;compliance;kiss</categories_reviewed>
  </evidence>
  <related_commands>
    /axiom-adversary-security --target TARGET
    /axiom-adversary-assumptions --target TARGET
    /axiom-report-issue title=... type=bug severity=critical
    /axiom-verify --work-item WORK_ITEM_ID
  </related_commands>
  <memory_updates>
    .memory-bank/findings/_index.md updated;
    .memory-bank/findings/CATEGORY/FINDING.md created
  </memory_updates>
</axiom>
```

### Cross-References
- "Finding format is defined in: `.opencode/skills/adversarial-review-axiom/SKILL.md`"
- "Spec: `specs/77-Adversarial-Review-System.md`"
- "Sub-commands: `/axiom-adversary-security`, `/axiom-adversary-assumptions`, `/axiom-adversary-resilience`, `/axiom-adversary-compliance`, `/axiom-adversary-kiss`"

See: `.opencode/skills/adversarial-review-axiom/SKILL.md` (portable — works in installed repos)

axiom:trace work_item=adversarial-review-system spec=specs/77-Adversarial-Review-System.md
