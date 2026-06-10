---
name: regression-testing-bug-fixes
description: Portable regression testing rules for bug fixes. Load this skill when fixing bugs found by QA sweeps, runtime testing, or user reports. Enforces the "every bug fix gets a regression test" mandate so bugs never recur in autonomous Ralph loop iterations.
version: "1.0"
tags:
  vertical: [coding]
  category: testing
  core: false
---

# Regression Testing for Bug Fixes

> **"A bug fix without a regression test is a bug waiting to come back."**

This skill enforces the regression test mandate for all bug fixes in Axiom. It is especially critical in the Ralph loop (autonomous builder agent) where each iteration starts with fresh context -- regression tests are the only durable, machine-checkable guarantee that a fix stays fixed.

## When to Load This Skill

- Fixing any bug from a bug registry (`.memory-bank/work-items/*/bug-registry.md`)
- Fixing a bug found during QA sweeps, runtime testing, or user reports
- Backfilling regression tests for previously-fixed bugs that lack them
- Reviewing whether a bug fix step is complete

## Core Rules (Non-Negotiable)

1. **Every bug fix MUST include a regression test.** No exceptions. The test must live in `.axiom/tests/`.
2. **The test MUST fail if the fix is reverted.** This is the red-green proof. If the test passes regardless of the fix, it is not a regression test.
3. **The test name MUST reference the bug ID.** Format: `test_rt_bug_NNN_short_description` (e.g., `test_rt_bug_001_null_fields_omitted`).
4. **A bug fix step is NOT complete without the regression test passing.** The verify command for the step must include running the regression test.
5. **Regression tests MUST call actual product code.** Tests that only inspect function signatures, AST, or source text (without also calling the function) are tautologies and do NOT count as regression tests. See REQ-TQ-002 in `specs/48-Test-Quality-Gates.md`.
6. **Regression tests MUST have at least one assertion.** Assertionless regression tests are hard-fail violations per REQ-TQ-001. "No exception raised" is only acceptable with an explicit `# ASSERTIONLESS-OK:` marker AND a timing/side-effect assertion where possible.

## Naming Convention

```
test_rt_bug_NNN_short_description
```

Examples:
- `test_rt_bug_001_null_fields_omitted_from_payload`
- `test_rt_bug_002_message_endpoint_used_not_command`
- `test_rt_bug_003_sse_idle_detection_not_polling`
- `test_rt_bug_007_invalid_date_no_traceback`
- `test_rt_bug_009_default_port_4096`
- `test_rt_bug_011_port_range_validation`
- `test_rt_bug_017_no_test_prefix_dataclass`

## Test Structure Template

```python
def test_rt_bug_NNN_short_description():
    """Regression test for RT-BUG-NNN: <one-line summary>.

    Bug: <what was broken>
    Root cause: <why it was broken>
    Fix: <what was changed>
    Prevents: <what this test catches if the fix is reverted>

    Bug registry: .memory-bank/work-items/<WORK_ITEM>/bug-registry.md
    """
    # Arrange: set up the conditions that triggered the bug
    ...

    # Act: execute the code path that was broken
    ...

    # Assert: verify the CORRECT behavior (must fail if fix reverted)
    assert result == expected, (
        "RT-BUG-NNN regression: <description of what went wrong>"
    )
```

## Regression Test Patterns

### Pattern 1: API Payload Regression (RT-BUG-001 example)

The bug: `send_session_command` included `None` values as JSON `null` in the payload, causing HTTP 400 from OpenCode's Zod validation.

```python
def test_rt_bug_001_null_fields_omitted_from_payload():
    """Regression test for RT-BUG-001: null optional fields sent in payload.

    Bug: Payload included None values as JSON null for optional fields.
    Root cause: Dict comprehension included all fields unconditionally.
    Fix: Build payload conditionally -- only include non-None fields.
    Prevents: HTTP 400 from OpenCode Zod validation on null optional fields.
    """
    # Arrange
    client = OpenCodeClient(base_url="http://test:4096")

    # Act: call with only required fields (all optionals are None)
    with unittest.mock.patch.object(client, "_request_json") as mock_req:
        mock_req.return_value = {"messageID": "msg-1"}
        client.send_session_command(
            session_id="sess-1",
            command="/verify-step",
        )

    # Assert: payload must NOT contain null optional fields
    actual_payload = mock_req.call_args[1]["json_body"]
    assert actual_payload == {"command": "/verify-step"}, (
        f"RT-BUG-001 regression: payload contains extra fields: {actual_payload}"
    )
    for field in ("arguments", "agent", "model", "messageID", "variant", "parts"):
        assert field not in actual_payload, (
            f"RT-BUG-001 regression: null field '{field}' present in payload"
        )
```

### Pattern 2: Endpoint Selection Regression (RT-BUG-002 example)

```python
def test_rt_bug_002_message_endpoint_used_not_command():
    """Regression test for RT-BUG-002: /command endpoint returns HTTP 500.

    Bug: send_session_command used POST /session/{id}/command.
    Root cause: OpenCode 1.2.6 /command endpoint has server-side bug.
    Fix: Use POST /session/{id}/message with user message parts.
    Prevents: HTTP 500 from OpenCode when sending commands.
    """
    client = OpenCodeClient(base_url="http://test:4096")

    with unittest.mock.patch.object(client, "_request_json") as mock_req:
        mock_req.return_value = {"id": "msg-1"}
        client.send_session_command(session_id="sess-1", command="/help")

    # Assert: must use /message endpoint, not /command
    call_url = mock_req.call_args[1].get("url", mock_req.call_args[0][1])
    assert "/message" in call_url, (
        f"RT-BUG-002 regression: using {call_url} instead of /message endpoint"
    )
    assert "/command" not in call_url, (
        f"RT-BUG-002 regression: still using /command endpoint: {call_url}"
    )
```

### Pattern 3: Config Default Regression (RT-BUG-009 example)

```python
def test_rt_bug_009_default_port_4096():
    """Regression test for RT-BUG-009: workspace_bootstrap defaults to port 8100.

    Bug: poll_opencode_health() defaulted to port 8100.
    Root cause: Port 8100 is for repo-runner, not OpenCode.
    Fix: Changed default to 4096 per specs/31 section 1.5.2.
    Prevents: Health checks polling wrong port on fresh installs.
    """
    import inspect
    from axiom.repo_runner.engine.workspace_bootstrap import poll_opencode_health

    sig = inspect.signature(poll_opencode_health)
    port_param = sig.parameters.get("port")
    assert port_param is not None, "poll_opencode_health must have a 'port' parameter"
    assert port_param.default == 4096, (
        f"RT-BUG-009 regression: default port is {port_param.default}, expected 4096"
    )
```

### Pattern 4: Input Validation Regression (RT-BUG-007 example)

```python
def test_rt_bug_007_invalid_date_no_traceback(capsys):
    """Regression test for RT-BUG-007: invalid date exposes Python traceback.

    Bug: axiom todo query --from-date invalid-date showed full traceback.
    Root cause: No try/except around _parse_date() call.
    Fix: Catch ValueError and print clean error message.
    Prevents: Users seeing raw Python tracebacks for invalid input.
    """
    import subprocess
    result = subprocess.run(
        ["axiom", "todo", "query", "--from-date", "invalid-date"],
        capture_output=True, text=True,
    )
    # Must NOT contain traceback
    assert "Traceback" not in result.stderr, (
        f"RT-BUG-007 regression: traceback exposed to user:\n{result.stderr}"
    )
    # Must contain clean error message
    combined = result.stdout + result.stderr
    assert "ERROR" in combined or "error" in combined, (
        "RT-BUG-007 regression: no error message for invalid date"
    )
```

### Pattern 5: Documentation Accuracy Regression (RT-BUG-006 example)

```python
def test_rt_bug_006_agents_md_health_endpoint():
    """Regression test for RT-BUG-006: AGENTS.md documents wrong health endpoint.

    Bug: AGENTS.md said /global/health but actual endpoint is /health.
    Root cause: Conflation of OpenCode server endpoint with Axiom server.
    Fix: Changed AGENTS.md to reference /health.
    Prevents: Users following docs and getting 404.
    """
    from pathlib import Path
    agents_md = Path("AGENTS.md").read_text()
    # The Tier 4 verification section must reference /health, not /global/health
    assert "curl -sf http://127.0.0.1:8100/health" in agents_md, (
        "RT-BUG-006 regression: AGENTS.md does not contain correct health URL"
    )
```

### Pattern 6: Missing Dependency Regression (RT-BUG-004/005 example)

```python
def test_rt_bug_004_pytest_asyncio_installed():
    """Regression test for RT-BUG-004: pytest-asyncio not in dev deps.

    Bug: 186 async tests failed with 'async def not natively supported'.
    Root cause: pytest-asyncio declared in pyproject.toml but not installed.
    Fix: Ensured pip install -e '.[dev]' installs pytest-asyncio.
    Prevents: Async test suite silently failing in new environments.
    """
    try:
        import pytest_asyncio  # noqa: F401
    except ImportError:
        import pytest
        pytest.fail(
            "RT-BUG-004 regression: pytest-asyncio not installed. "
            "Run: pip install 'pytest-asyncio>=0.24'"
        )


def test_rt_bug_005_pytest_httpx_installed():
    """Regression test for RT-BUG-005: pytest-httpx not in dev deps.

    Bug: 3 test files failed collection due to missing pytest-httpx.
    Root cause: pytest-httpx declared in pyproject.toml but not installed.
    Fix: Ensured pip install -e '.[dev]' installs pytest-httpx.
    Prevents: Integration test files failing to collect in new environments.
    """
    try:
        import pytest_httpx  # noqa: F401
    except ImportError:
        import pytest
        pytest.fail(
            "RT-BUG-005 regression: pytest-httpx not installed. "
            "Run: pip install 'pytest-httpx>=0.34'"
        )
```

## Test Placement Rules

1. **Co-locate with related tests.** Put the regression test in the same test file/module as the code it tests.
   - OpenCode client bugs -> `tests/repo_runner/adapters/test_opencode_client.py`
   - CLI validation bugs -> `tests/cli/test_cli_run.py`
   - TODO archive bugs -> `tests/cli/test_todo_archive.py` or `tests/scripts/test_query_todo_archive.py`
   - Config default bugs -> `tests/repo_runner/engine/test_workspace_bootstrap.py`

2. **Group regression tests together.** Within a test file, group regression tests at the end under a comment block:
   ```python
   # --- Regression tests (bug fixes) ---

   def test_rt_bug_001_null_fields_omitted():
       ...

   def test_rt_bug_009_default_port_4096():
       ...
   ```

3. **Never create a separate `tests/regression/` directory.** Regression tests belong with the code they protect.

## Verification Commands

After writing a regression test, verify it with:

```bash
# Run the specific regression test
cd .axiom && python3 -m pytest tests/ -k "rt_bug_NNN" -v

# Run all regression tests for a QA sweep
cd .axiom && python3 -m pytest tests/ -k "rt_bug" -v

# Verify the test would fail without the fix (optional but recommended)
# Temporarily revert the fix, run the test, confirm it fails, then re-apply
```

## Backfill Process

When bug fixes were completed without regression tests (as happened with RT-BUG-001, RT-BUG-002, RT-BUG-003):

1. Create a dedicated backfill step in the plan (e.g., `phase-6/step-15`)
2. Write regression tests for each bug using the patterns above
3. Verify each test fails if the fix is reverted (or at minimum, verify it tests the specific behavior)
4. Run all regression tests together: `pytest -k "rt_bug_001 or rt_bug_002 or rt_bug_003" -v`
5. Update the TODO to mark the backfill step complete

## Integration with Ralph Loop

The Ralph loop (`PROMPT.md`) enforces this skill via:

1. **PROMPT.md "Regression Test Requirement (Bug Fixes)"** -- the mandate
2. **TODO.md Phase 6 steps** -- each step names the regression test to create
3. **Bug registry** -- canonical source of bug details for test docstrings
4. **Verifier captain** -- checks that regression test evidence exists before marking PASS

The verifier should reject any bug fix step that:
- Has no regression test in the evidence transcript
- Has a regression test with a generic name (no bug ID)
- Has a regression test that doesn't assert the specific fixed behavior

## Anti-Patterns

| Anti-Pattern | Why It's Bad | What to Do Instead |
|---|---|---|
| Fix without test | Bug can recur in next iteration | Always write regression test |
| Generic test name | Can't trace which bug it prevents | Use `test_rt_bug_NNN_*` format |
| Test passes without fix | Doesn't actually prevent regression | Assert the specific fixed behavior |
| Separate regression directory | Tests are disconnected from code | Co-locate with related tests |
| Docstring without bug context | Future developers can't understand why | Include bug ID, symptom, root cause, fix |
| **Tautology regression test** | **Tests Python semantics, not product behavior** | **Call the actual function with real inputs** |
| **Assertionless regression test** | **Passes even if fix is reverted** | **Add assertion on the specific fixed behavior** |
| **Signature-only test** | **Never calls the function, can't detect runtime bugs** | **Call the function and assert on output** |

## References

- `PROMPT.md` -- "Regression Test Requirement (Bug Fixes)" section
- `specs/00-PRD.md#verification-signal-hierarchy` -- regression tests are Tier 1 minimum
- `specs/09-Baby-Steps-Methodology.md` -- each bug fix is one baby step
- `.opencode/skills/enterprise-testing-standard/SKILL.md` -- tiered testing standard
- `.memory-bank/best-practices/regression-testing-bug-fixes.md` -- companion best practice
- `.memory-bank/best-practices/enterprise-grade-testing.md` -- enterprise testing standard
- `.memory-bank/best-practices/pytest-testing-ecosystem.md` -- pytest patterns

## Trace

axiom:trace work_item=runtime-package-test-01 spec=specs/00-PRD.md,specs/09-Baby-Steps-Methodology.md plan=phase-6 doc=.opencode/skills/regression-testing-bug-fixes/SKILL.md,.memory-bank/best-practices/regression-testing-bug-fixes.md evidence=.memory-bank/work-items/runtime-package-test-01/bug-registry.md
