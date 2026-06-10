---
name: axiom-implementation-plans
description: Maintain `.memory-bank/implementation-plans/` as phase-level plans aligned to TODO.
version: "1.2"
tags:
  vertical: [planning]
  category: planning
  core: false
---

Use this skill when you need to create/update the phase-level implementation plans under `.memory-bank/implementation-plans/`.

Preferred mechanism:
- Run the slash command `/axiom-implementation-plans`.

What "good" looks like:
- One plan per TODO phase + one for "Start Here".
- High-signal, low-churn plans with explicit Spec Trace and Done Evidence.

## Jira Ticket Tracking in Implementation Plans (REQUIRED when Jira-sourced)

When work items in the implementation plan originate from or are mirrored to Jira tickets, the plan MUST integrate Jira awareness so that execution agents know how to interact with Jira throughout the lifecycle.

### Required Jira Context in Plans

1. **Jira key in plan header**: Each phase-level plan that maps to a Jira ticket MUST include the Jira key in its header or frontmatter (e.g., `jira_ref: PROJ-123`).

2. **Per-step Jira expectations**: Each plan step that produces user-visible progress SHOULD note the expected Jira action:
   ```markdown
   ### Step 3: Implement rate limiter middleware
   - Spec ref: specs/auth.md#rate-limiting
   - Jira: Post progress comment after implementation; transition to "AI Handoff (Implement)" if not already
   - Done evidence: Unit tests pass; rate limiter active on /login endpoint
   ```

3. **Jira status transitions in plan**: The plan SHOULD include explicit Jira status transition points:

   | Plan Phase | Expected Jira Status | Transition Trigger |
   |---|---|---|
   | Planning complete | AI Handoff (Implement) | First implementation step starts |
   | Implementation complete | AI Handoff (Verify) | Verification chain starts |
   | Verification complete | Human Review | All gates pass + PR created |
   | Blocked | Blocked | Escalation triggered |

4. **Evidence posting to Jira**: The plan's verification steps SHOULD include posting evidence summaries to Jira as comments (per `specs/05-Jira-Integration.md#comment-format`).

5. **`jira_ref` in trace markers**: All `axiom:trace` markers in the plan MUST include `jira_ref=<KEY>` when the work item is Jira-sourced.

### Non-Jira Work Items

When work items are NOT Jira-sourced:
- Omit Jira-specific fields and transition notes
- All other plan rules still apply
- If the work item is later linked to a Jira ticket, update the plan with the Jira key

References:
- `specs/05-Jira-Integration.md` (ticket-as-work-unit, comment format, status transitions)
- `specs/10-Lifecycle-State-Machine.md` (Jira status mapping, transition triggers)
- `specs/20-Meta-Planning.md#jira-ticket-tracking-and-management` (meta-plan Jira contract)
- `specs/21-Traceability-Doctrine.md#external-reference-fields` (jira_ref in trace markers)

## Test Strategy in Implementation Plans (REQUIRED)

Per `specs/48-Test-Quality-Gates.md#REQ-TQ-011`, every implementation plan that includes test steps MUST contain a **Test Strategy** section that ensures tests produce real value, not green theater.

### Required Test Strategy Content

For each phase that adds or modifies tests, include:

1. **Spec-to-test mapping table**: Which spec requirement each test verifies.
   ```
   | Test | Spec Requirement | What It Proves |
   |------|-----------------|----------------|
   | test_crash_recovery | specs/24 §checkpoint-resume | System resumes from checkpoint after crash |
   ```

2. **Real code path declaration**: For each test, state the actual product function/endpoint it calls. If a test uses a fake/stub, explain why and what the stub replaces.

3. **Value justification**: For each test, state what regression it would catch. Tests that cannot catch a real regression should not be written.

4. **Anti-pattern review**: Confirm no planned tests match these green theater patterns:
   - Fake executor bypass (returns "ok" without real execution)
   - Raw HTTP instead of adapter (tests mock server, not product code)
   - Coverage padding (hits lines without asserting behavior)
   - Source inspection (reads source text instead of calling functions)
   - Weak assertions (always-true conditions like `score >= 0`)
   - Missing negative tests (happy path only, no error paths)

5. **Gap analysis**: Cross-reference spec requirements against planned tests. Any spec requirement with zero test coverage MUST be either covered or explicitly deferred with rationale.

### Example Test Strategy Section

```markdown
## Test Strategy

### Spec-to-Test Mapping
| Test File | Spec Requirement | Real Code Path | Regression Caught |
|-----------|-----------------|----------------|-------------------|
| test_crash_recovery.py | specs/24 §checkpoint-resume | orchestrator.run() → checkpoint.write() → checkpoint.read() | Crash during run loses progress |
| test_scope_auth.py | specs/30 §authorization | auth_middleware() → scope_check() | Any token can do any action |
| test_wal_mode.py | specs/24 §wal-mode | state_persistence.init_db() → PRAGMA journal_mode | Concurrent reads cause DB locks |

### Anti-Pattern Check
- ✅ No fake executors — all tests call real orchestrator/adapter functions
- ✅ No raw HTTP — integration tests call adapter functions, not urllib
- ✅ No coverage padding — every test has specific value assertions
- ⚠️ test_app.py uses MagicMock for uvicorn — acceptable (MOCK-CONTRACT-OK: verifies host binding parameter)

### Uncovered Spec Requirements
- specs/30 §rate-limiting — DEFERRED: rate limiting strategy not yet decided (OD-TQ-003)
```

### Why This Matters

In AI-only delivery, agents optimize for the metric they're measured on. Without explicit test value planning, agents produce tests that maximize coverage while minimizing actual verification. The test strategy section forces the planner to think about **what each test proves** before writing it, preventing the accumulation of green theater that creates false confidence.

Reference: `specs/48-Test-Quality-Gates.md#REQ-TQ-011`, `.memory-bank/best-practices/test-suite-adversarial-patterns.md`
