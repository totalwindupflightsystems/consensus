---
name: test-quality-gates-axiom
description: Portable test quality gate workflow for Axiom. Enforces that high coverage is backed by high-signal tests — no assertionless tests, no tautologies, Tier-3+ runtime evidence required. Includes Test Value Doctrine to distinguish value-producing tests from green theater. Load this skill when writing tests, reviewing test quality, or verifying a work item step is "done". Applies to every repo managed by Axiom.
version: "1.1"
tags:
  vertical: [coding]
  category: testing
  core: false
---

# Test Quality Gates (Portable)

> **"95% coverage with 0% signal is worse than 80% coverage with real signal."**
>
> **"A test that passes whether the system works or not is worse than no test at all — it creates false confidence."**

This skill enforces `specs/48-Test-Quality-Gates.md` across every repo Axiom manages.
It is the operational companion to `enterprise-testing-standard` (tiers) and
`regression-testing-bug-fixes` (regression mandate).

## When to Load This Skill

Load this skill when:
- Writing new tests for any work item step
- **Planning tests during meta-planning or implementation planning** (NEW — REQ-TQ-011)
- Reviewing whether a step is "done" (verifier gate)
- Running `/axiom-verify` on any code change
- Onboarding a new repo (bootstrap quality gate defaults)
- A QA sweep finds test quality issues
- **An adversarial agent reviews test quality** (NEW — REQ-TQ-011-ADVERSARIAL)

## The Core Problem This Solves

In AI-only delivery, agents optimize for the metric they are measured on.
If the only metric is **coverage %**, agents produce:
- Tests that execute lines but assert nothing (assertionless).
- Tests that test Python builtins, not product behavior (tautology).
- Tests that inspect function signatures instead of calling functions (AST-only).
- Over-mocked tests that never reach the real execution path.

These tests pass coverage but **do not detect regressions**. The system appears healthy
but is not. This skill makes quality a first-class, mechanically-enforced gate.

## Hard Gates (Zero Tolerance)

These MUST be zero. No exceptions without explicit allowlist entry.

### Gate 1: No Assertionless Tests (REQ-TQ-001)

Every `test_*` function MUST contain at least one of:
- `assert <expression>`
- `with pytest.raises(...):`
- `with pytest.warns(...):`

**How to fix**: Add a meaningful assertion about the actual behavior.

**Allowlist exception**: If the function genuinely tests "no exception raised" for a void
function, add to the docstring:
```python
# ASSERTIONLESS-OK: verifies no exception is raised; observable behavior is void
```

### Gate 2: No Tautology Tests (REQ-TQ-002)

A tautology test has assertions but calls no product code. Examples:

```python
# TAUTOLOGY — tests Python `or`, not product code
def test_rt_bug_156_backward_compatible_none_default():
    description = None
    work_item_id = "test-item-123"
    result = description or work_item_id   # <-- no product code called
    assert result == work_item_id          # <-- tests Python semantics

# TAUTOLOGY — tests inspect.signature, not function behavior
def test_has_param():
    sig = inspect.signature(my_function)
    assert "x" in sig.parameters          # <-- never calls my_function(x=...)
```

**How to fix**: Call the actual function with real inputs and assert on the output.

**Allowlist exception**: AST/source-shape checks for security invariants are permitted.
Add to the docstring:
```python
# AST-CHECK-OK: verifies shell=True never used per specs/32-Security-Hardening-Roadmap.md
```

## Soft Gates (Configurable Thresholds)

These are ratio-based. Defaults are set in `axiom.config.yaml`.

| Gate | Default | Key |
|------|---------|-----|
| AST-only test ratio | ≤ 2% | `ast_only_ratio_max` |
| Skip/xfail ratio | ≤ 5% | `skip_xfail_ratio_max` |
| Untraced test ratio | ≤ 30% | `untraced_ratio_max` |

## Runtime Tier Gate (REQ-TQ-003)

A step is NOT "done" without Tier-3+ runtime evidence.

**Minimum for "done"**: `evidence.runtime_tier_reached >= 3`

Tier-3 means the system actually ran, not just that tests passed in isolation.
The exact Tier-3 command is defined in `axiom.config.yaml`:
```yaml
verification:
  runtime:
    required_min_tier: 3
    tier_commands:
      tier_3: "axiom run --work-item smoke-test --repo . --in-process"
```

If `evidence.runtime_tier_reached` is absent or below threshold, the verifier
MUST inject a corrective step and return `status=fail`.

## How to Run the Checker

```bash
# Full check — all gates
python3 .axiom/scripts/check_test_quality.py \
  --test-dir .axiom/tests \
  --config .axiom/axiom.config.yaml

# JSON output for CI / evidence bundle
python3 .axiom/scripts/check_test_quality.py \
  --test-dir .axiom/tests \
  --config .axiom/axiom.config.yaml \
  --json > test-quality-report.json

# Single check only
python3 .axiom/scripts/check_test_quality.py \
  --test-dir .axiom/tests \
  --check assertionless

# Exit codes: 0=pass, 1=hard-fail, 2=soft-fail, 3=config-error
```

## Evidence Tags Required

When running `/axiom-step` or `/axiom-verify`, include these in the XML evidence:

```xml
<evidence.tests_quality.score>87</evidence.tests_quality.score>
<evidence.tests_quality.assertionless>0</evidence.tests_quality.assertionless>
<evidence.tests_quality.tautology>0</evidence.tests_quality.tautology>
<evidence.tests_quality.report_path>.axiom/test-quality-report.json</evidence.tests_quality.report_path>
<evidence.runtime_tier_reached>3</evidence.runtime_tier_reached>
```

If the quality gate fails, the step MUST NOT be marked `status=ok`.

## Writing High-Quality Tests: Decision Tree

```
Is this a new test?
├── Does it call actual product code (not just inspect/ast/builtins)?
│   ├── NO  → Is it a security AST invariant? Add AST-CHECK-OK. Otherwise rewrite.
│   └── YES → Does it have at least one assert/raises/warns?
│             ├── NO  → Is the behavior genuinely void? Add ASSERTIONLESS-OK. Otherwise add assert.
│             └── YES → Does it reference a bug ID, AC, or spec?
│                       ├── NO  → Add to docstring: bug ID / AC ref / spec ref.
│                       └── YES → ✓ High-quality test.
```

## Patterns: Good vs Bad

### Bad: Tautology regression test
```python
def test_rt_bug_156_backward_compatible_none_default():
    """RT-BUG-156 regression."""
    description = None
    work_item_id = "test-item-123"
    result = description or work_item_id  # tests Python, not product
    assert result == work_item_id
```

### Good: Behavioral regression test
```python
def test_rt_bug_156_backward_compatible_none_default():
    """RT-BUG-156: when work_item_description=None, orchestrator uses work_item_id as fallback.

    Bug: _generate_plan received work_item_id instead of description.
    Fix: Added work_item_description param with None default + or-fallback.
    Prevents: Plan prompt using raw ID instead of human-readable description.
    """
    with patch("axiom.repo_runner.engine.orchestrator._generate_plan") as mock_plan:
        mock_plan.return_value = {"phases": []}
        run_orchestrator_sync(work_item_id="PROJ-123", repo="/tmp/r", in_process=True)
    call_kwargs = mock_plan.call_args.kwargs
    assert call_kwargs["work_item_description"] == "PROJ-123", (
        "RT-BUG-156: None description must fall back to work_item_id"
    )
```

### Bad: Assertionless "no exception" test
```python
def test_wait_for_session_idle_returns_on_sse_session_idle(monkeypatch):
    """SSE primary path: session.idle event triggers return."""
    # ... setup ...
    opencode_client.wait_for_session_idle(session_id="session-abc", timeout_seconds=5)
    # no assertion — passes even if function raises or returns wrong value
```

### Good: Assertionless with explicit marker + behavioral assertion
```python
def test_wait_for_session_idle_returns_on_sse_session_idle(monkeypatch):
    """SSE primary path: session.idle event triggers return without timeout.

    # ASSERTIONLESS-OK: verifies no exception is raised; observable behavior is void
    Behavioral assertion: function must complete before timeout_seconds expires.
    """
    import time
    start = time.monotonic()
    opencode_client.wait_for_session_idle(session_id="session-abc", timeout_seconds=5)
    elapsed = time.monotonic() - start
    assert elapsed < 4.0, "wait_for_session_idle should return immediately on session.idle"
```

## Applying to a New Repo

When Axiom is installed into a new repo:

1. `check_test_quality.py` is available at `.axiom/scripts/check_test_quality.py`.
2. `axiom.config.yaml` includes `verification.test_quality` and `verification.runtime` keys.
3. CI runs `check_test_quality.py` as a required check (before coverage reporting).
4. The onboarding verification step runs the quality gate and reports results.
5. The first quality gate run establishes a baseline; violations are tracked as work items.

## Test Value Doctrine (REQ-TQ-011)

> **The fundamental question every test must answer: "What real bug would this test catch?"**

Per `specs/48-Test-Quality-Gates.md#REQ-TQ-011`, this skill enforces the distinction between
**value-producing tests** (verify what the system actually needs to do) and **green theater**
(make coverage numbers go up without catching real bugs).

### During Planning (Meta-Plan + Implementation Plan)

Every plan that includes test steps MUST include a **Test Value Assessment**:

1. **Spec-to-test mapping**: Which spec requirement does each test verify?
2. **Real code path**: What actual product function does each test call?
3. **Deletion test**: What breaks if this test is deleted?
4. **Gap check**: Any spec requirements with zero test coverage?
5. **Anti-pattern check**: Do any tests match green theater patterns?

### During Writing (Implementation)

Before writing each test, ask:
```
Is this test calling real product code?
├── NO → Is it a security AST invariant? → If not, STOP and redesign.
└── YES → Does it assert specific expected values?
          ├── NO → STOP and add specific assertions.
          └── YES → Does it map to a spec requirement?
                    ├── NO → Why does this test exist? Document or remove.
                    └── YES → Would it fail if the behavior regressed?
                              ├── NO → STOP and redesign (it's a tautology).
                              └── YES → ✓ Value-producing test.
```

### During Review (Verification)

The verifier MUST check new tests against the green theater anti-pattern catalog:

| ID | Anti-Pattern | Detection | Action |
|---|---|---|---|
| AP-001 | Fake executor bypass | Test executor returns literal value without calling real functions | FAIL — replace with scripted or real executor |
| AP-002 | Raw HTTP instead of adapter | Test imports `urllib`/`httpx` but not the adapter module | FAIL — call adapter functions instead |
| AP-003 | Coverage padding | File named `*_coverage.py` with weak assertions | WARN — add behavioral assertions or merge |
| AP-004 | Source inspection | `inspect.getsource()` without also calling the function | FAIL — add behavioral test |
| AP-005 | Weak assertions | `score >= 0`, `isinstance()` alone, `try/except: pass` | FAIL — assert specific values |
| AP-006 | Missing negative tests | Only success path tested | WARN — add error path tests |
| AP-007 | Organizational false confidence | Tests in wrong directory | WARN — relocate or cross-reference |
| AP-008 | Unbounded growth untested | In-memory store without TTL/eviction test | WARN — add bounds test |
| AP-009 | String matching for errors | `"409" in str(exc)` instead of structured attributes | WARN — use structured error attributes |
| AP-010 | No crash recovery round-trip | Write tested but read-back never exercised | FAIL — test full persistence cycle |

### During Adversarial Review

At least one adversarial agent MUST review tests before claiming complete:
- **@redteam-axiom**: "Can I make all tests pass while the feature is broken?"
- **@assumption-buster-axiom**: "What undocumented assumptions do these tests make?"
- **@devils-advocate-axiom**: "Are these tests verifying the spec or just the implementation?"

Reference: `.memory-bank/best-practices/test-suite-adversarial-patterns.md`

## Code Analysis as a Complementary Gate

Tests verify behavior; `axiom analyze` verifies the code itself. Load `code-analysis-axiom` for full usage details.

Run `axiom analyze --score` alongside the test quality checker to catch:
- **Dead code** (vulture / deadcode): unreachable functions that inflate coverage numbers without being exercised by real paths.
- **High complexity** (radon / gocyclo): functions with cyclomatic complexity so high that test coverage is likely incomplete even when line coverage looks good.
- **Lint issues** (ruff / biome): style and correctness problems that tests cannot detect.

A failing `axiom analyze` score is a signal that test coverage may be hiding problems — not a replacement for the quality gates above, but a fast additional check before claiming a step done.

## Integration Points

| Surface | What changes |
|---------|-------------|
| `specs/48-Test-Quality-Gates.md` | Authoritative contract |
| `.axiom/axiom.config.yaml` | `verification.test_quality` + `verification.runtime` keys |
| `.axiom/scripts/check_test_quality.py` | Portable checker (copy to target repos) |
| `.axiom/command-registry.yaml` | `evidence.tests_quality.*` + `evidence.runtime_tier_reached` tags |
| `/axiom-step` command | Must include quality evidence tags |
| `/axiom-verify` command | Must run checker + gate on runtime tier |
| `enterprise-testing-standard` skill | Quality gates added to tier requirements |
| `regression-testing-bug-fixes` skill | Tautology ban enforced |
| `axiom-onboarding` skill | Bootstraps quality gate defaults |

## References

- `specs/48-Test-Quality-Gates.md` — authoritative contract
- `specs/00-PRD.md#verification-signal-hierarchy` — tier definitions
- `.opencode/skills/enterprise-testing-standard/SKILL.md` — tiered testing
- `.opencode/skills/regression-testing-bug-fixes/SKILL.md` — regression mandate
- `.axiom/scripts/check_test_quality.py` — portable checker

axiom:trace work_item=test-quality-gates-01 spec=specs/48-Test-Quality-Gates.md plan= doc=.opencode/skills/test-quality-gates-axiom/SKILL.md
