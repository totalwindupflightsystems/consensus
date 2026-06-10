---
name: tdd-ai-axiom
description: >
  Test-Driven Development for AI-authored code. Enforces a QnA → Spec → Test → Code cycle
  where the AI asks questions first, writes specs and tests before any implementation, and
  commits on green / reverts on red. Prevents the common AI failure mode of writing code
  first and tests as an afterthought.
version: "1.0"
synopsis: |
  Enforces strict TDD discipline for AI coding agents: start with questions to understand
  the requirement, write a spec stub, write failing tests, implement minimal code to pass,
  refactor, commit. Integrates with test-quality-gates, working-backwards, conformance-loop,
  and runtime-completeness-gate for full verification stack.
when-to-use: |
  Load when starting any new feature, bug fix, or behavior change. Load BEFORE writing any
  code. This skill should be the FIRST skill loaded in any implementation session. If code
  exists before tests, you're doing it wrong.
tags:
  vertical: [coding]
  category: testing
  core: false
---

# Test-Driven Development for AI Agents (Axiom)

> **"No code without a failing test. No test without a spec. No spec without questions."**
>
> **"Tests are prompts. Write them as natural language specs that guide the AI toward exact behavior."**
>
> **"Commit on green. Revert on red. Never let broken code pollute the AI's context."**

---

## 1. The QnA → Spec → Test → Code Cycle

This is the core workflow. Every feature, bug fix, or behavior change follows this cycle:

```
┌─────────────────────────────────────────────────────────┐
│                                                          │
│  1. QnA MODE                                            │
│     Ask questions to understand the requirement.         │
│     Don't assume. Don't guess. Ask.                     │
│                                                          │
│  2. SPEC                                                │
│     Write a spec stub with acceptance criteria.          │
│     Each AC must be testable.                           │
│                                                          │
│  3. RED (failing test)                                  │
│     Write ONE test that encodes ONE acceptance criterion.│
│     Run it. Confirm it fails for the right reason.      │
│                                                          │
│  4. GREEN (minimal implementation)                      │
│     Write the MINIMAL code to make the test pass.       │
│     No gold-plating. No edge cases beyond the test.     │
│                                                          │
│  5. REFACTOR                                            │
│     Clean up while keeping tests green.                 │
│     One code smell at a time.                           │
│                                                          │
│  6. COMMIT                                              │
│     All tests pass → commit.                            │
│     Any test fails → revert to last green state.        │
│                                                          │
│  7. REPEAT                                              │
│     Next acceptance criterion → back to step 3.         │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Why QnA First?

AI agents hallucinate when they don't understand the requirement. The QnA phase forces the agent to:
- Identify what it doesn't know
- Ask the user (or check specs) before assuming
- Build a shared understanding before writing anything

**QnA is not optional.** If the agent skips QnA and jumps to code, it's guessing.

---

## 2. Phase 1: QnA Mode

Before writing any spec, test, or code, the agent MUST enter QnA mode:

### What to Ask

| Category | Example Questions |
|---|---|
| **Behavior** | "What should happen when the user does X?" |
| **Inputs** | "What are the valid inputs? What's invalid?" |
| **Outputs** | "What does the user see when it works? When it fails?" |
| **Edge cases** | "What happens with empty input? Null? Very large input?" |
| **Error handling** | "How should errors be reported? What's recoverable?" |
| **Dependencies** | "Does this depend on any external service? Database? API?" |
| **Existing behavior** | "Is there existing code that does something similar?" |
| **Acceptance criteria** | "How will we know this is done? What's the test?" |

### QnA Rules

1. **Ask up to 7 questions** before writing anything (per Axiom convention)
2. **Check specs first** — the answer might already be in `specs/`
3. **Check memory bank** — previous work items might have context
4. **Don't ask questions you can answer yourself** — read the code first
5. **If the user says "just do it"** — still write the spec stub with your assumptions, mark them as assumptions, and ask for confirmation

### QnA Output

The QnA phase produces:
- A list of answered questions (with sources: user, spec, code)
- A list of assumptions (with "how to verify" and "impact if wrong")
- A draft spec stub with acceptance criteria

---

## 3. Phase 2: Spec Stub

After QnA, write a spec stub. This is NOT a full spec — it's the minimum needed to write tests:

```markdown
## Feature: [Name]

**Requirement**: [One sentence from spec or user]

### Acceptance Criteria

1. **AC-1**: Given [precondition], when [action], then [expected result]
2. **AC-2**: Given [precondition], when [action], then [expected result]
3. **AC-3**: Given [error condition], when [action], then [error handling]

### Assumptions
- A1: [assumption] — verify by [method]

### Out of Scope
- [What this does NOT do]
```

### Spec Stub Rules

1. **Every AC must be testable** — if you can't write a test for it, rewrite the AC
2. **Include error cases** — at least one AC for error handling
3. **Include edge cases** — at least one AC for boundary conditions
4. **Link to governing spec** — reference the spec requirement this implements
5. **Get confirmation** — show the spec stub to the user before writing tests

---

## 3b. Strategy Falsification for Bug Fixes (Gate 3)

For **non-mechanical bug fixes**, insert a Strategy Falsification step between Spec and Test:

```
QnA → Spec → Strategy Falsification → Test → Code
```

**Before writing any test or code**, produce a Strategy Falsification Pack:

1. **Selected hypothesis**: One-sentence testable statement of the proposed root cause and fix approach
2. **Alternatives** (≥2): Other plausible root causes or fix approaches, including one structurally different approach and the status-quo option
3. **Falsification criteria**: 2–4 conditions under which the proposed fix would fail or introduce a regression
4. **Blast radius**: Affected callers/consumers; classify as NONE/WARN/HARD BLOCK
5. **Existing-fix check**: Recent commits, open PRs, ticket comments — verdict: CLEAR or DUPLICATE

**How to produce it**:
- Dispatch `@strategy-falsifier-axiom` as a subagent, OR
- Load the `strategy-falsification-axiom` skill inline

**Record output** in `verification.md` under `## Strategy Falsification` before writing tests.

**Mechanical fix exception**: If the fix is a single-line typo, config value, or import correction, add a one-line note: "Mechanical fix — no alternatives required." and proceed directly to Test.

**HARD BLOCK condition**: If zero alternatives are documented for a non-mechanical fix, do NOT proceed to Test or Code. Document the alternatives first.

**Full spec**: `specs/77-Adversarial-Review-System.md#strategy-falsification-stage`

---

## 4. Phase 3: RED (Write Failing Test)

Write ONE test for ONE acceptance criterion. The test MUST fail before any implementation exists.

### Test Writing Rules

1. **One test, one behavior, one assertion focus**
2. **Descriptive name**: `test_[scenario]_[expected_behavior]`
3. **Call real code** — no mocking unless absolutely necessary
4. **Assert specific behavior** — not just "no error"
5. **Run the test** — confirm it fails for the RIGHT reason (missing implementation, not syntax error)

### Test Template

```python
def test_create_event_with_valid_payload_returns_201():
    """AC-1: Valid event payload is accepted and stored."""
    # Given
    payload = {"session_id": "abc-123", "event_type": "plugin.metrics.snapshot", ...}
    
    # When
    response = client.post("/api/v1/events", json=payload)
    
    # Then
    assert response.status_code == 201
    assert response.json()["id"] is not None
    
    # Verify stored
    stored = db.query("SELECT * FROM events WHERE id = %s", response.json()["id"])
    assert stored is not None
    assert stored["session_id"] == "abc-123"
```

### RED Verification

After writing the test:
```bash
# Run the test — it MUST fail
pytest tests/test_events.py::test_create_event_with_valid_payload_returns_201 -v

# Expected: FAILED (function/module not found, or assertion error)
# NOT expected: SyntaxError, ImportError (these mean the test itself is broken)
```

---

## 5. Phase 4: GREEN (Minimal Implementation)

Write the MINIMAL code to make the failing test pass. Nothing more.

### GREEN Rules

1. **Minimal means minimal** — hardcode values if that's all the test needs
2. **No gold-plating** — don't add features the test doesn't require
3. **No edge cases** — unless the test specifically checks for them
4. **Run the test** — confirm it passes
5. **Run ALL tests** — confirm nothing else broke

### GREEN Verification

```bash
# Run the specific test — it MUST pass
pytest tests/test_events.py::test_create_event_with_valid_payload_returns_201 -v

# Run ALL tests — nothing else should break
pytest tests/ -q
```

---

## 6. Phase 5: REFACTOR

Clean up the implementation while keeping all tests green.

### Refactor Rules

1. **One smell at a time** — don't refactor everything at once
2. **Tests stay green** — run tests after each refactor step
3. **No new behavior** — refactoring changes structure, not behavior
4. **If tests break** — revert the refactor, it changed behavior

---

## 7. Phase 6: COMMIT or REVERT

### If all tests pass: COMMIT

```bash
git add -A
git commit -m "test + implement: [behavior description]

AC: [which acceptance criterion this satisfies]
Spec: [spec reference]
Trace: axiom:trace work_item=<ID> spec=<REF> test=<TEST_FILE>"
```

### If any test fails: REVERT

```bash
git checkout -- .
# Back to last green state. Figure out what went wrong before trying again.
```

**Never commit broken code.** Broken code in the repo pollutes the AI's context for the next session.

---

## 8. Context Pollution Prevention

This is the #1 reason TDD matters for AI coding:

**LLMs cannot distinguish between working and broken code in their context window.**

If broken code exists in the repo, the AI will:
- Reference it as if it works
- Build on top of it
- Generate more broken code that depends on the broken code
- Create a cascade of errors that's hard to untangle

**TDD prevents this** by ensuring:
- Code is always in a working state (green)
- Broken states are immediately reverted
- The AI always builds on verified working code

---

## 9. Integration with Other Axiom Skills

| Skill | When to Load Alongside TDD | Why |
|---|---|---|
| `test-quality-gates-axiom` | Always | Verify tests have assertions, aren't tautologies |
| `working-backwards-axiom` | During QnA/Spec phase | Plan from user experience backward |
| `conformance-testing-loop` | When verifying existing behavior | TDD for new code, conformance for existing |
| `runtime-completeness-gate-axiom` | After GREEN phase | Verify implementation is actually wired end-to-end |
| `regression-testing-bug-fixes` | When fixing bugs | TDD cycle + mandatory regression test |
| `enterprise-testing-standard` | When assessing "done" | TDD is Tier 1-2; need Tier 3+ for completion |
| `api-contract-validator-axiom` | When building APIs | Contract-driven TDD (OpenAPI as spec source) |
| `protocol-testing` | When testing HTTP/gRPC/SSE | Real protocol testing, not mocks |

---

## 10. Anti-Patterns (What NOT to Do)

### ❌ Write code first, tests after
**Why it's wrong**: Tests become documentation of whatever the AI generated, not verification of requirements. The test doesn't drive the design.

### ❌ Write test and implementation in the same prompt
**Why it's wrong**: The AI optimizes the test to pass the implementation (or vice versa). Neither drives the other.

### ❌ Write multiple tests before implementing
**Why it's wrong**: Loses the feedback loop. You don't know which test to focus on. Context gets overloaded.

### ❌ Skip QnA and jump to code
**Why it's wrong**: The AI guesses at requirements. Guesses compound into wrong behavior.

### ❌ Leave failing tests in the repo
**Why it's wrong**: Broken tests pollute context. The AI can't tell which failures are expected vs unexpected.

### ❌ Mock everything
**Why it's wrong**: Tests pass but real code paths are never exercised. "Looks built" ≠ "actually works."

---

## 11. Checklist

Before starting any implementation:
- [ ] QnA phase completed (questions asked, answers documented)
- [ ] Spec stub written with testable acceptance criteria
- [ ] User confirmed spec stub (or assumptions documented)
- [ ] **Bug fix only**: Strategy Falsification Pack produced and recorded in `verification.md` (or mechanical-fix exception noted)

For each acceptance criterion:
- [ ] ONE failing test written (RED)
- [ ] Test confirmed failing for the right reason
- [ ] Minimal implementation written (GREEN)
- [ ] Test confirmed passing
- [ ] ALL tests confirmed passing (no regressions)
- [ ] Code reviewed for smells
- [ ] Refactored if needed (tests stay green)
- [ ] Committed with trace markers

After all acceptance criteria:
- [ ] Test quality gates pass (no assertionless, no tautologies)
- [ ] Runtime tier evidence captured (Tier 3+ for "done")
- [ ] Runtime completeness gate checked (actually wired end-to-end)

axiom:trace spec=specs/48-Test-Quality-Gates.md
