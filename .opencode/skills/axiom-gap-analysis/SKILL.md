---
name: axiom-gap-analysis
description: Orchestrate multi-agent gap analysis and progress reporting with evidence-first, fail-closed outputs.
version: "1.0"
tags:
  vertical: [planning]
  category: planning
  core: false
---

# Axiom Gap Analysis

Use this skill when you need a deterministic, verifier-ready status of what is implemented, what is verified, what is contract-aligned, and what remains blocked.

Preferred invocation:
- Run the slash command `/axiom-gap-analysis`.

Useful skill-design ideas adapted from Claude-style skills (OpenCode-compatible):
- Keep frontmatter short and stable for reliable discovery (`name`, `description`, `version`).
- Put execution rules in Markdown sections so the caller can run the same workflow repeatedly.

## Purpose and when to use

Use this skill for any work item where you need a merged, evidence-backed progress report across implementation, QA, spec alignment, and trace integrity.

Trigger examples:
- Before marking a work item `ready-for-review`.
- After a verification failure to identify exact remediation gaps.
- During handoff when multiple agents changed scope and you need one truth-set.

## Code Analysis as a Gap Data Source

`axiom analyze` (see `code-analysis-axiom` skill) can feed directly into gap analysis progress reporting:

- Run `axiom analyze --score` to get a health score and per-category issue counts (dead code, complexity, lint).
- Include the score in the `confidence.signals` section of the gap report as a `code_quality` signal.
- Dead-code findings map to `implementation` gaps (unreachable code that may indicate incomplete wiring).
- High-complexity findings map to `qa` gaps (functions that are hard to test and likely under-covered).
- A declining health score across reruns is a signal that remediation is introducing new problems.

## Required inputs

Provide these inputs explicitly:
- `WORK_ITEM_ID`: stable id (Jira key preferred).
- `SCOPE_PATHS`: repo paths in scope (files, folders, or glob patterns).
- `TIME_WINDOW`: start/end (or run-id range) for evidence considered current.
- `CONSTRAINTS`: no-breaking-change, no-destructive-commands, required approvals, and any restricted paths.
- `PLAN_REF` (recommended): `phase/task/step` cursor or plan segment.
- `SPEC_REFS` (recommended): authoritative spec files/anchors.
- `RERUN_BUDGET` (required, default `2` if omitted by caller): max fan-in reruns before escalation.

Fail closed if any required input is missing after normalization.

Input packet template:

```yaml
WORK_ITEM_ID: "ABC-123"
SCOPE_PATHS: ["src/**", "tests/**", ".memory-bank/work-items/ABC-123/**"]
TIME_WINDOW: "2026-02-07T00:00:00Z..2026-02-07T23:59:59Z"
CONSTRAINTS:
  no_breaking_changes: true
  allow_destructive_commands: false
  restricted_paths: []
PLAN_REF: "phase-2/task-3/step-3.2"
  SPEC_REFS: ["specs/<repo-spec>.md#anchor"]
RERUN_BUDGET: 2
```

## Canonical multi-agent call graph

Naming note:
- Invocation text may use `@agent-handle` style.
- Canonical agent identity should match the installed agent handles in `.opencode/agents/`.

Agents used by this skill:
- `@memory-bank-axiom`
- `@dev-axiom`
- `@qa-axiom`
- `@spec-verifier-axiom`
- `@trace-auditor-axiom`

Execution graph:

```mermaid
flowchart TD
  A[Input normalization + governance fence] --> B[@memory-bank-axiom truth scan]
  B --> C[@dev-axiom implemented-state report]
  B --> D[@qa-axiom verification-state report]
  B --> E[@spec-verifier-axiom contract alignment check]
  B --> F[@trace-auditor-axiom trace integrity audit]
  C --> G[Fan-in merge: gap matrix + severity]
  D --> G
  E --> G
  F --> G
  G --> H{Any critical/unverified gaps?}
  H -- Yes --> I[Inject remediation steps + rerun targeted agents]
  I --> G
  H -- No --> J[Final report + confidence + next actions]
```

## Parallelization and rerun loop

Run in three passes:

1) Truth baseline (serial):
- Call `@memory-bank-axiom` first to establish canonical plan/progress/evidence pointers.
- Require memory-bank root load order: `.memory-bank/_prompt.md` then `.memory-bank/_index.md`, then follow links only.

2) Verification fan-out (parallel):
- Run `@dev-axiom`, `@qa-axiom`, `@spec-verifier-axiom`, and `@trace-auditor-axiom` in parallel against the same input packet.
- If `@trace-auditor-axiom` is unavailable in the environment, keep running and record a `high` trace gap with `status: unverified`.

3) Fan-in + rerun loop:
- Merge outputs into one gap matrix.
- If any item is `critical` or `unverified`, inject targeted remediation.
- Rerun only affected agent(s) and then rerun fan-in.
- Enforce rerun budget strictly: stop when `reruns_used >= RERUN_BUDGET`.
- Track state hash across reruns (gap ids + status + severity + owner). Stop and escalate when two consecutive reruns produce no state change.
- Stop when all `critical` gaps are resolved or explicitly deferred with owner + due condition.

Rerun stop rules:
- Stop immediately on governance conflict.
- Stop when two consecutive reruns produce no state change (escalate).
- Stop when rerun budget is exhausted (`reruns_used >= RERUN_BUDGET`).
- Stop when required evidence cannot be produced in this environment.

## Gap status semantics (required)

Use exactly these status values for each `gaps[]` item:
- `open`: issue is confirmed and unresolved; remediation step is known.
- `resolved`: issue has a verifying evidence record proving closure.
- `unverified`: issue may exist or may be fixed, but required evidence is missing or invalid.
- `deferred`: issue is intentionally postponed due to a declared blocker (for example, credential-gated or approval-gated).

Status transition rules:
- `open -> resolved` requires new validating evidence and a verification method in the gap entry.
- `open -> deferred` requires `deferred_reason`, owner, and rerun condition.
- Any status -> `unverified` when evidence is missing, malformed, stale, or contradictory.
- `resolved` without valid evidence is invalid; coerce to `unverified`.

## Agent responsibilities (deterministic)

- `@memory-bank-axiom`
  - Validate memory-bank truth for scope/time window.
  - Output canonical pointers for plan, verification, run snapshots, and TODO/progress state.

- `@dev-axiom`
  - Report implemented behavior boundaries from changed files.
  - Confirm presence/absence of test updates tied to each behavior change.

- `@qa-axiom`
  - Validate executed checks and failures from raw evidence.
  - Mark every acceptance criterion as pass/fail/unverified with evidence pointers.
  - **Assess test value**: For each test added/modified, verify it produces real value per `specs/48-Test-Quality-Gates.md#REQ-TQ-011`. Flag green theater patterns (fake executor bypass, raw HTTP instead of adapter, coverage padding, weak assertions, missing negative tests).

- `@spec-verifier-axiom`
  - Validate code/test/docs against referenced specs.
  - Flag contract drift, missing spec updates, and unresolved contradictions.
  - **Verify spec-to-test coverage**: For each spec requirement in scope, confirm at least one test verifies it. Flag uncovered spec requirements as gaps.

- `@trace-auditor-axiom`
  - Validate `axiom:trace` completeness and link integrity.
  - Flag missing, stale, or broken trace references.

## Required output schema (final report)

Produce a machine-readable section (JSON or YAML) with this shape:

```yaml
report:
  work_item_id: "<ID>"
  scope:
    paths: ["<path-or-glob>"]
    time_window: "<start..end|run-range>"
  status: "pass|fail|blocked"
  rerun:
    budget: 2
    used: 0
    stop_reason: "none|budget_exhausted|no_state_change|governance_conflict|required_evidence_unavailable"
  confidence:
    score: 0
    band: "low|medium|high"
    signals:
      requirements_clarity: { value: 0, weight: 20, present: true, source: "" }
      spec_alignment: { value: 0, weight: 20, present: true, source: "" }
      test_coverage: { value: 0, weight: 20, present: true, source: "" }
      checks_pass_rate: { value: 0, weight: 25, present: true, source: "" }
      plan_completion: { value: 0, weight: 10, present: true, source: "" }
      ambiguity_remaining: { value: 0, weight: 5, present: true, source: "" }
  gaps:
    - id: "gap-001"
      category: "implementation|qa|spec|trace|ops|security|evidence"
      severity: "critical|high|medium|low"
      statement: "what is missing or inconsistent"
      evidence:
        - kind: "command_output|file|log|manual"
          ref: "<command string|repo path|log id|manual note id>"
          result: "pass|fail|info|not-run"
          excerpt: "<short raw excerpt or pointer>"
      owner: "dev-axiom|qa-axiom|spec-verifier-axiom|trace-auditor-axiom|memory-bank-axiom"
      invocation_handle: "@dev-axiom|@qa-axiom|@spec-verifier-axiom|@trace-auditor-axiom|@memory-bank-axiom"
      next_step: "smallest verifiable action"
      verification: "how to prove resolved"
      status: "open|resolved|deferred|unverified"
      deferred_reason: "credential-gated|environment|approval|none"
      spec_refs: ["specs/<file>.md#anchor"]
      plan_refs: ["phase-<n>/task-<n>/step-<n>"]
      test_refs: ["tests/<path>::<test_name>"]
      doc_refs: ["docs/<path>.md"]
      prompt_refs: [".opencode/<path>"]
```

Minimum requirements:
- Every gap must include `severity`, `evidence`, `owner`, and `next_step`.
- `evidence` entries must include `kind`, `ref`, and `result`; empty evidence arrays are invalid.
- `resolved` gaps must contain at least one `evidence.result=pass` entry.
- `verification` must be non-empty for `open`, `resolved`, and `deferred` gaps.
- Add `spec_refs` and `plan_refs` whenever the gap maps to contract or plan scope.
- If evidence is absent, set `status: unverified`.
- Severity defaults to `critical` when evidence is missing for a required gate.

PASS criteria (fail-closed):
- `report.status=pass` is allowed only when: (1) no `critical` gaps remain, (2) no required gate is `unverified`, (3) no unresolved `open` gap with `severity in {critical, high}`, and (4) rerun stop reason is `none`.
- If any PASS criterion is unmet, output `fail` (or `blocked` when execution is prevented by governance/environment constraints).

## Fail-closed rules

- No evidence => unverified.
- Unverified required gate => overall status cannot be `pass`.
- Missing spec reference for behavior change => `high` or `critical` gap.
- Missing trace marker near changed behavior boundary => `high` gap.
- Claimed test pass without raw command output => `critical` evidence gap.
- Budget exhausted with unresolved required gaps => overall `fail`.
- No-state-change stop with unresolved required gaps => overall `blocked` with escalation note.

## Baby-steps execution requirement

Follow `.opencode/skills/baby-steps-methodology/SKILL.md` during remediation and reruns:
- One meaningful remediation change per iteration.
- Validate and capture evidence after each iteration.
- Do not declare PASS until all fail-closed PASS criteria are satisfied.

## Operator self-check checklist (before declaring PASS)

- Required inputs were normalized, including effective `RERUN_BUDGET`.
- Agent naming is consistent: `@...` in narrative, canonical bare names in structured fields.
- Every gap has valid status semantics (`open|resolved|unverified|deferred`) and allowed transitions.
- Every required gate has valid evidence; no invented or unverifiable pass claims.
- `spec_refs`/`plan_refs` are present for contract/plan-linked gaps.
- Rerun accounting is complete (`budget`, `used`, `stop_reason`) and respects budget/state-change stop rules.
- Final `report.status` satisfies all fail-closed PASS criteria.

## Credential-gated deferred items

For checks requiring unavailable credentials or external systems:
- Mark gap `status: deferred` and `deferred_reason: credential-gated`.
- Include the exact credential/system needed (redacted names only).
- Include the exact rerun command and expected verifier owner.
- Do not count deferred checks as passed.

## Confidence scoring guidance (spec-aligned)

Follow `.opencode/skills/axiom-confidence-scoring/SKILL.md`:
- Use only canonical six v1 signals.
- Compute weighted average from present signals only.
- Exclude absent signals from numerator and denominator.
- If all signals are absent, score is `0` and status is `blocked`.
- Apply bands: `low < 40`, `medium 40-69`, `high >= 70`.
- Confidence never overrides failed required gates.

## Artifact trace marker requirement

Any artifact generated or updated by this workflow (reports, remediation notes, run snapshots) must include a one-line marker near the behavior/result boundary:

`axiom:trace work_item=<ID> spec=<REF> plan=<phase/task/step> test=<REF?> doc=<REF?> prompt=<REF?> evidence=<REF?> commit=<REF?>`

This marker must remain single-line and grep-friendly.

## Minimal execution checklist

- Validate required inputs.
- Run memory-bank truth scan first.
- Run four verification agents in parallel.
- Merge findings into output schema.
- Apply fail-closed status rules.
- If gaps remain, inject targeted rerun steps.
- Emit final report with confidence and explicit deferred items.

axiom:trace work_item=skill-gap-analysis-01 spec=specs/<repo-spec>.md#anchor plan=phase-1/task-1-1/step-1-1-1 test= doc=.opencode/skills/axiom-gap-analysis/SKILL.md prompt=.opencode/skills/axiom-gap-analysis/SKILL.md evidence=.memory-bank/work-items/skill-gap-analysis-01/verification.md commit=
