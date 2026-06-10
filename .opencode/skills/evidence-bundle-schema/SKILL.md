---
name: evidence-bundle-schema
description: Portable evidence bundle schema (verification.md + outputs.md) and validation rules; never invent evidence.
version: "1.0"
tags:
  vertical: [coding, ops, sre, writing, security, planning, onboarding, benchmarking, personal-context]
  category: verification
  core: true
---

# Evidence Bundle Schema (Portable)

An evidence bundle is the structured receipt that closes the loop: requirements -> plan -> execution -> verification -> evidence.
Without a valid evidence bundle, no work item is "done".

This skill defines an authoritative, portable schema for:
- `.memory-bank/work-items/<WORK_ITEM_ID>/verification.md` (rolling)
- `.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/verification.md` (immutable snapshot)
- `.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/outputs.md` (external actions)

## Storage Locations

Rolling:
```
.memory-bank/work-items/<WORK_ITEM_ID>/verification.md
```

Immutable per-run:
```
.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/verification.md
.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/outputs.md
```

Companions (optional):
```
.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/events.jsonl
.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/checkpoint.yaml
```

## `verification.md` Schema

### YAML Frontmatter (required)

```yaml
---
work_item_id: "ABC-123"
run_id: "2026-02-06T10-00-00Z_01"
status: pass              # pass | fail | blocked
confidence:
  before: 45
  after: 82
  breakdown:
    requirements_clarity: 90
    spec_alignment: 85
    test_coverage: 72
    checks_pass_rate: 100
    plan_completion: 80
    ambiguity_remaining: 65
repo: "org/repo"
pr_url: "https://github.com/org/repo/pull/42"
updated_at: "2026-02-06"
---
```

### Markdown Sections (required; keep headings even if N/A)

1) `## Acceptance Criteria Coverage`
- Table mapping each AC to a concrete verification path and result.
- Result must be `pass`, `fail`, or `unverified`.

```markdown
## Acceptance Criteria Coverage

| # | Criterion | Verification Path | Result | Notes |
|---|-----------|-------------------|--------|-------|
| AC-1 | ... | Test: `tests/...::test_...` | pass | |
| AC-2 | ... | Command: `pytest ...` | pass | |
| AC-3 | ... | Manual check: ... | unverified | why + how to verify |
```

2) `## Checks Executed`
- Only list commands actually run; never invent output.

3) `## Verifier Results`
- List independent verifiers (QA/spec/security/trace) with status and brief notes.

4) `## Changes Summary`
- High-level file list and intent.

5) `## Risks and Assumptions`
- Label assumptions `[A#]` and risks `[R#]`.

6) `## Injected Work`
- List verifier-injected steps and their resolution status.

7) `## Confidence Explanation`
- Explain signal-by-signal why the score moved; include "Why not higher" and "Why not lower".

## `outputs.md` Schema

Frontmatter (required):

```yaml
---
work_item_id: "ABC-123"
run_id: "2026-02-06T10-00-00Z_01"
repo: "org/repo"
pr_url: "https://github.com/org/repo/pull/42"
pr_status: "ready_for_review"
confidence:
  before: 45
  after: 82
updated_at: "2026-02-06"
---
```

Body sections (recommended):
- `## Jira Comments Posted`
- `## PR Comments Posted`
- `## PR Status`
- `## Run Notes`

## Validation Rules

MUST:
- Every acceptance criterion appears in the AC coverage table.
- Every AC has a concrete verification path.
- `Checks Executed` contains only real commands/tests run.
- No secrets/PII (redact as `[REDACTED]`).
- `status: pass` only if all AC results are `pass` and required checks pass.

SHOULD:
- Include short output excerpts (especially first meaningful error line on failures).
- Include trace refs (spec/plan/evidence pointers).

## Non-Negotiable Evidence Policy

Never fabricate:
- test results
- command outputs
- scan results
- approvals

If not verified, mark `unverified` and write the exact verification steps.
