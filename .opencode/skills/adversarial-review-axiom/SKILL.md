---
name: adversarial-review-axiom
description: >
  Portable adversarial review system for Axiom. Defines the /axiom-adversary command family,
  agent dispatch tables, finding format, severity classification, memory bank integration, and
  consolidated report format. Fully self-contained — works in installed repos without specs/.
version: "1.0"
synopsis: |
  Defines how adversarial reviews work in Axiom: 5 commands dispatching 10 specialized agents
  across 4 categories (security, assumptions, resilience, compliance). Findings accumulate in
  .memory-bank/findings/ as a library of institutional knowledge. Source spec: specs/77-Adversarial-Review-System.md.
when-to-use: |
  Load when running /axiom-adversary or any sub-command, when any agent needs to produce
  adversarial findings, when declaring a work item complete (adversarial check before PASS),
  or when a spec/plan feels settled and needs challenging.
tags:
  vertical: [security, coding, planning]
  category: security
  core: false
---

# Adversarial Review System (Axiom — Portable)

> **"The job of adversarial review is to falsify claims, not validate them."**
>
> **"A finding that isn't written down doesn't exist."**
>
> **"Every adversarial finding that accumulates in the library makes the next build better."**

Source spec (Axiom repo only): `specs/77-Adversarial-Review-System.md`

---

## 1. Command Family

| Command | Agents Dispatched | Focus | When to Use |
|---------|-------------------|-------|-------------|
| `/axiom-adversary` | All 10 agents | Comprehensive review | Pre-release, milestone gates, periodic sweeps |
| `/axiom-adversary-security` | security-review, whitehat, redteam, security-engineer | Security vulnerabilities | Auth/secrets/PII/network changes |
| `/axiom-adversary-assumptions` | assumption-buster, devils-advocate | Assumptions and ambiguities | Before finalizing specs/plans |
| `/axiom-adversary-resilience` | chaos-engineer, sre-ops | Resilience and reliability | Ops impact, new services, runbook coverage |
| `/axiom-adversary-compliance` | privacy-compliance, accessibility-review, finops-cost | Compliance and cost | Data handling, UI surfaces, cost-sensitive changes |

### Common Parameters (all commands)

| Parameter | Default | Description |
|-----------|---------|-------------|
| `--scope` | all | Limit review to specific files/directories/components |
| `--severity-threshold` | medium | Minimum severity to report (critical, high, medium, low) |
| `--output` | markdown | Report format: markdown, json, summary |
| `--timeout` | 5m | Per-agent timeout |
| `--work-item` | auto-detect | Associate findings with a work item |

### Standalone vs /axiom-verify

| Aspect | `/axiom-adversary-*` | `/axiom-verify` Phase 3 |
|--------|------------------------|---------------------------|
| Invocation | Manual/on-demand | Automatic during verification |
| Scope | User-specified | Work item scope |
| Agents | Full set per command (up to 10) | Limited set (assumption-buster, devils-advocate, ralph-wiggum) |
| Output | Findings report written to memory bank | Verification report with findings section |
| Persistence | Always writes to `.memory-bank/findings/` | Writes to work item verification.md |

Use `/axiom-verify` for routine step verification. Use `/axiom-adversary` for deeper, targeted, or periodic reviews.

---

## 2. Agent Dispatch Tables

### Security Category

| Agent | Role | Dispatch |
|-------|------|----------|
| `@security-review-axiom` | Threat model, secrets hygiene, vulnerability class detection, risk gates | Always (unconditional) |
| `@whitehat-axiom` | Authorized penetration validation, exploitability checks, retest after fixes | Always (unconditional) |
| `@redteam-axiom` | Adversarial falsification of claims, attack matrix, exploitable paths | Always (unconditional) |
| `@security-engineer-axiom` | Security architecture review, defense-in-depth validation, concrete mitigations | Always (unconditional) |

### Assumptions Category

| Agent | Role | Dispatch |
|-------|------|----------|
| `@assumption-buster-axiom` | Surface undocumented prerequisites, ambiguous specs, non-verifiable acceptance criteria | Always |
| `@devils-advocate-axiom` | Challenge specs/plans/designs, force explicit tradeoffs, simplest-thing-that-works pressure test | Always |
| `@strategy-falsifier-axiom` | Pre-implementation hypothesis challenge (Gate 3) — selected hypothesis, ≥2 alternatives, falsification criteria, blast radius, existing-fix check | For non-mechanical bug fixes and new features; dispatched before implementation begins |

**Note on @strategy-falsifier-axiom**: For bug-fix work items, this agent SHOULD be dispatched before implementation begins (Gate 3 in the bug-fix gate order — see `specs/20-Meta-Planning.md#gate-order`). It is distinct from @devils-advocate-axiom (which challenges specs/plans broadly) — @strategy-falsifier-axiom specifically challenges the fix approach with a structured 5-element output. The `strategy-falsification-axiom` skill provides the same output inline without subagent dispatch.

### Resilience Category

| Agent | Role | Dispatch |
|-------|------|----------|
| `@chaos-engineer-axiom` | Fault injection patterns, resilience testing, runbook validation under failure, RTO/RPO verification | Always |
| `@sre-ops-axiom` | SLO/SLI validation, error budget analysis, deploy safety review, alert coverage | OPTIONAL — if unavailable, log WARN and produce partial report noting the gap |

### Compliance Category

| Agent | Role | Dispatch |
|-------|------|----------|
| `@privacy-compliance-axiom` | PII detection, data retention policy, consent flow validation, GDPR/CCPA/HIPAA controls | Always |
| `@accessibility-review-axiom` | WCAG 2.1 AA audit, keyboard navigation, screen reader compatibility | When UI surfaces are in scope |
| `@finops-cost-axiom` | Cost-risk detection, cardinality guardrails, scaling cost projections | Always |

### Parallel Dispatch Rules

- Agents within the same category run in parallel
- Agents from different categories run in parallel
- The master command dispatches all 4 categories simultaneously
- Per-agent timeout: 5 minutes (configurable via `--timeout`)

---

## 3. Severity Classification

| Level | Definition | Response Time | Exit Code |
|-------|------------|---------------|-----------|
| **CRITICAL** | Immediate security vulnerability, data breach risk, system unusable | Fix before any PASS claim | 2 |
| **HIGH** | Significant risk requiring prompt attention, blocks specific workflow | Fix before next milestone | 1 |
| **MEDIUM** | Moderate risk or quality issue, workaround exists | Track; fix in next iteration | 0 |
| **LOW** | Minor issue or improvement opportunity | Backlog | 0 |

Exit code 3 = execution error (timeout, agent failure).

---

## 4. Finding Format

Every finding MUST use this format when written to `.memory-bank/findings/`. This format extends the base finding template from `.memory-bank/findings/_prompt.md` with adversarial-specific fields.

```yaml
---
mb:
  type: finding
  title: "<concise finding title>"
  created: YYYY-MM-DD
  updated: YYYY-MM-DD
  tags: [finding, adversarial, <category>]
  severity: critical|high|medium|low
  status: open|addressed|wont-fix
  agent: "<agent-name that produced this finding>"
  category: security|assumptions|resilience|compliance
  scope: "<files/components/specs affected>"
  links:
    up: "../_index.md"
    related: []
  source:
    type: adversarial-agent
    ref: "work_item=<ID> agent=<agent-name>"
  git:
    commit: ""
    paths: []
    blame: ""
---

# Finding: [Title]

## Summary
<1-2 sentence summary of what was found and why it matters>

## Details
- **Trigger**: What caused this finding to surface.
- **Impact**: What breaks or degrades if not addressed.
- **Root cause**: Why this happens.

## Evidence
<specific evidence: file paths, code snippets, test results>

## Prevention / Fix
<actionable remediation steps; link to updated spec or _prompt.md if a rule was changed>

## Links
- [Up: Findings Index](../_index.md)
- [Related: ...]

## Traceability
- **Source**: adversarial agent / work item / QA sweep
- **Git**: commit / paths (leave blank if unavailable)
```

### Finding File Naming

`<work-item-id>-<YYYYMMDD>-<slug>.md`

Example: `DEX-285-20260405-missing-auth-check.md`

---

## 5. Memory Bank Integration

### Where Findings Live

```
.memory-bank/findings/
├── _index.md          ← inventory of all findings
├── _prompt.md         ← rules for writing findings (base template)
├── security/          ← security adversarial findings
├── assumptions/       ← assumption-busting findings
├── resilience/        ← resilience/chaos findings
├── compliance/        ← privacy/a11y/cost findings
├── quality/           ← code/test quality findings
├── anti-patterns/     ← recurring anti-patterns discovered
├── process/           ← process improvement findings
└── agent-reflections/ ← agent self-improvement notes
```

### Rules for Writing Findings

1. **Write every finding** — even LOW severity. Findings that aren't written don't exist.
2. **One finding per file** — don't bundle multiple findings in one file.
3. **Link to work item** — every finding MUST reference the work item where it was discovered.
4. **Update the index** — after writing a finding, update `.memory-bank/findings/_index.md`.
5. **Mark status** — findings start as `open`. Update to `acknowledged` when triaged, `addressed` when fixed.
6. **Cross-reference** — if a finding relates to a spec, link to the spec. If it relates to a previous finding, link to that too.
7. **Deduplicate** — when multiple agents surface the same issue, merge into one finding listing all contributing agents. Severity = max of all duplicates.

### Accumulation Over Time

Findings accumulate into a library. Over time, patterns emerge:
- Same anti-pattern in multiple work items → create an anti-pattern entry in `anti-patterns/`
- Same assumption wrong repeatedly → update the spec or skill
- Same agent making the same mistake → update the agent's prompt or skill

This is how Axiom self-improves: adversarial findings → library → spec/skill updates → fewer findings next time.

---

## 6. Consolidated Report Format

When `/axiom-adversary` runs, it produces a consolidated report:

```markdown
# Adversarial Review: <target>

**Date**: <ISO 8601>
**Target**: <work item / spec / plan>
**Scope**: <full|security|assumptions|resilience|compliance>
**Agents dispatched**: <count> (<list>)
**Duration**: <wall time>

## Summary

| Severity | Count |
|---|---|
| CRITICAL | N |
| HIGH | N |
| MEDIUM | N |
| LOW | N |

**Exit code**: <0|1|2|3>

## Critical Findings

<numbered list of CRITICAL findings with brief description and recommended fix>

## High Findings

<numbered list of HIGH findings>

## Recommended Next Steps

1. <most important action>
2. <second most important action>
3. <third most important action>

## Full Findings

Written to: `.memory-bank/findings/<category>/`
```

---

## 7. Fail-Closed Rules

- MUST return `status=fail` if any CRITICAL finding is found (exit code 2)
- MUST return `status=warn` if any HIGH finding is found but no CRITICAL (exit code 1)
- MUST write findings to memory bank before returning
- MUST NOT return `status=ok` if findings exist above the severity threshold
- MUST update `.memory-bank/findings/_index.md` with new entries
- If an agent times out, produce a partial finding with status `timeout` (exit code 3)
- If an OPTIONAL agent is unavailable, log WARN and continue with partial report

---

## 8. Integration Points

### With /axiom-verify

`/axiom-verify` Phase 3 dispatches a subset of adversarial agents (assumption-buster, devils-advocate, ralph-wiggum, qa-axiom, and optionally frontend-dev). The `/axiom-adversary` family provides deeper coverage with all 10 agents and persistent findings.

### With /axiom-report-issue

When adversarial findings surface platform bugs (not user code bugs), agents SHOULD use `/axiom-report-issue` to file the finding as a Jira ticket in the incoming feedback epic. This creates a feedback loop: adversarial review → finding → Jira ticket → work item → fix → fewer findings.

### With Working Backwards

Load `working-backwards-axiom` alongside this skill when reviewing plans. The working-backwards skill prevents wiring gaps at planning time; this skill detects them after implementation.

### With Runtime Completeness Gate

Load `runtime-completeness-gate-axiom` alongside this skill when reviewing implementations. The gate catches nil executors, unregistered routes, and verification theater that adversarial review should also flag.

---

## 9. Skills That Adversarial Commands Should Load

| Command | Skills to Load |
|---------|---------------|
| `/axiom-adversary` | `adversarial-review-axiom` (this), `evidence-bundle-schema`, `axiom-confidence-scoring` |
| `/axiom-adversary-security` | `adversarial-review-axiom`, `security-review-axiom` |
| `/axiom-adversary-assumptions` | `adversarial-review-axiom` |
| `/axiom-adversary-resilience` | `adversarial-review-axiom`, `sre-ops-axiom`, `chaos-engineer-axiom` |
| `/axiom-adversary-compliance` | `adversarial-review-axiom`, `privacy-compliance-axiom` |

---

## 10. Checklist for Running an Adversarial Review

Before running:
- [ ] Target is identified (work item, spec, plan, or code surface)
- [ ] Scope is defined (specific files/components or "full")
- [ ] Severity threshold is set (default: medium)

During:
- [ ] All dispatched agents return findings or timeout
- [ ] Findings are deduplicated (same root cause → one finding)
- [ ] Findings are classified by severity

After:
- [ ] Findings written to `.memory-bank/findings/<category>/`
- [ ] `.memory-bank/findings/_index.md` updated
- [ ] Consolidated report produced
- [ ] Exit code reflects highest severity finding
- [ ] If work item specified, findings linked from work item folder
- [ ] Platform bugs filed via `/axiom-report-issue` if applicable
