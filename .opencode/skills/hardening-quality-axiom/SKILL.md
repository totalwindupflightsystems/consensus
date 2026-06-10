---
name: hardening-quality-axiom
description: >
  Test coverage gap detection for any codebase. Focuses on missing tests that would
  actually catch bugs — not just line coverage. Covers critical paths with no tests,
  error paths with no tests, edge cases, integration boundaries, assertionless tests,
  tautology tests, and flaky tests. Complements test-quality-gates-axiom.
  Produces HARDEN-QUAL-* findings with Tier-3+ verifiable acceptance criteria.
version: "1.0"
tags:
  vertical: [testing, coding]
  category: hardening
  core: false
metadata:
  related_skills:
    - hardening-anti-patterns-axiom
    - hardening-intake-axiom
    - test-quality-gates-axiom
    - enterprise-testing-standard
    - regression-testing-bug-fixes
  note: >
    This skill complements test-quality-gates-axiom (enforcement of test quality rules)
    and enterprise-testing-standard (tiered testing hierarchy). Use this skill for
    gap discovery. Use test-quality-gates-axiom for enforcement.
---

# Hardening: Quality & Test Coverage Gaps

> **"Don't just report low line coverage — report missing tests that would actually catch bugs."**
>
> **"The shift from 'add a test for X' to 'when X receives empty list, it returns empty result' is the difference between a finding Axiom can close at Tier 3 and one that spins forever."**

This skill identifies test coverage gaps that matter — not just low line coverage, but missing tests for critical paths, error paths, and integration boundaries. It is portable — no Axiom-internal dependencies.

## When to Load This Skill

- Quarterly quality audit of a codebase
- After a production bug that tests should have caught
- Before a major release
- When `test-quality-gates-axiom` flags quality issues
- As part of a hardening battery

---

## The Quality Audit Prompt

Use this prompt (with the shared header from `hardening-anti-patterns-axiom`):

```
Review this codebase's tests and identify coverage gaps that matter.
Don't just report low line coverage — report missing tests that
would actually catch bugs.

Look for:

- Critical paths with no tests: auth, payments, data writes
- Error paths with no tests: what happens when a dep returns an error?
- Edge cases with no tests: empty inputs, max-size, unicode, concurrency
- Integration boundaries with no tests: anywhere this service talks
  to another service, DB, queue, or third party
- Tests that don't actually test what their name says (always-pass assertions)
- Flaky tests: depend on timing, ordering, or external state

For each finding, acceptance_criteria should describe the specific
test to add and the specific behavior it verifies — not "add a test
for function X" but "when function X receives an empty list, it
returns an empty result rather than raising."
```

---

## Quality Audit Checklist

### Critical Path Coverage

- [ ] **Authentication flow tested** — login, logout, token refresh, invalid credentials
- [ ] **Authorization tested** — access denied for wrong role, access granted for correct role
- [ ] **Payment/billing flow tested** — charge, refund, failure handling
- [ ] **Data write paths tested** — create, update, delete with verification
- [ ] **Data migration tested** — before/after state verified

### Error Path Coverage

- [ ] **Dependency failure tested** — what happens when DB/cache/queue is down?
- [ ] **Timeout tested** — what happens when external service times out?
- [ ] **Invalid input tested** — empty, null, too long, wrong type, malicious
- [ ] **Concurrent access tested** — race conditions, deadlocks
- [ ] **Partial failure tested** — what if step 2 of 3 fails?

### Edge Case Coverage

- [ ] **Empty collections** — empty list, empty string, empty dict
- [ ] **Boundary values** — 0, -1, MAX_INT, MAX_STRING_LENGTH
- [ ] **Unicode/encoding** — non-ASCII characters, emoji, RTL text
- [ ] **Timezone edge cases** — DST transitions, UTC vs local
- [ ] **Concurrency** — multiple requests modifying same resource

### Integration Boundary Coverage

- [ ] **Service-to-service calls tested** — with real or realistic mock
- [ ] **Database interactions tested** — with real test database
- [ ] **Queue interactions tested** — publish and consume verified
- [ ] **Third-party API tested** — with recorded responses or sandbox

### Test Quality

- [ ] **No assertionless tests** — every test has at least one assertion
- [ ] **No tautology tests** — assertions verify product behavior, not Python semantics
- [ ] **No over-mocked tests** — integration paths tested with real dependencies
- [ ] **No flaky tests** — tests pass consistently across 10 runs
- [ ] **Test names describe behavior** — `test_login_with_invalid_password_returns_401`

---

## Detection Patterns

### Grep Commands

```bash
# Find assertionless tests
grep -rn "def test_" --include="*.py" -A 15 \
  | grep -B 15 "^--$\|^def test_" \
  | grep -v "assert\|pytest\.raises\|pytest\.warns\|ASSERTIONLESS-OK"

# Find tautology tests (asserting variable equals itself)
grep -rn "assert\s\+\(\w\+\)\s*==\s*\1" --include="*.py"

# Find tests with only mock assertions (no real behavior verified)
grep -rn "def test_" --include="*.py" -A 20 \
  | grep "assert.*called\|assert.*call_count\|assert.*called_with" \
  | grep -v "assert.*result\|assert.*response\|assert.*status"

# Find timing-dependent tests (flaky risk)
grep -rn "time\.sleep\|asyncio\.sleep\|threading\.sleep" \
  --include="test_*.py" --include="*_test.py"

# Find tests that don't test error paths
find . -name "test_*.py" -o -name "*_test.py" | xargs grep -l "def test_" \
  | xargs grep -L "pytest\.raises\|except\|error\|fail\|invalid\|not_found"

# Find critical paths without tests
# (Check if auth/payment/write functions have corresponding test files)
grep -rn "def.*login\|def.*authenticate\|def.*charge\|def.*payment" \
  --include="*.py" | grep -v "test_"
```

---

## Anti-Patterns with Fixes

### AP-QUAL-001: Assertionless Test

**Severity:** medium

```python
# BAD: Test with no assertions — always passes
def test_user_creation():
    user = User(name="Alice", email="alice@example.com")
    db.session.add(user)
    db.session.commit()
    # No assertion! Test passes even if User() raises an exception
    # (because the exception would fail the test, but silently)
```

**Fix:**
```python
# GOOD: Verify the actual state change
def test_user_creation():
    user = User(name="Alice", email="alice@example.com")
    db.session.add(user)
    db.session.commit()
    
    # Verify user was persisted
    saved_user = User.query.filter_by(email="alice@example.com").first()
    assert saved_user is not None, "User should be saved to database"
    assert saved_user.name == "Alice", f"Expected name 'Alice', got {saved_user.name!r}"
    assert saved_user.id is not None, "User should have a database ID"
```

---

### AP-QUAL-002: Missing Error Path Test

**Severity:** medium

```python
# BAD: Only happy path tested
def test_get_user():
    user = get_user(user_id=1)
    assert user.name == "Alice"

# Missing tests for:
# - user_id that doesn't exist
# - database connection failure
# - invalid user_id type
```

**Fix:**
```python
# GOOD: Test all paths
def test_get_user_success():
    user = get_user(user_id=1)
    assert user.name == "Alice"
    assert user.email == "alice@example.com"

def test_get_user_not_found():
    with pytest.raises(UserNotFoundError) as exc_info:
        get_user(user_id=99999)
    assert "99999" in str(exc_info.value)

def test_get_user_database_error():
    with patch("db.session.query") as mock_query:
        mock_query.side_effect = OperationalError("Connection refused", None, None)
        with pytest.raises(ServiceUnavailableError):
            get_user(user_id=1)

def test_get_user_invalid_id():
    with pytest.raises((ValueError, TypeError)):
        get_user(user_id="not-an-integer")
```

---

### AP-QUAL-003: Missing Integration Boundary Test

**Severity:** high

```python
# BAD: Service-to-service call only tested with mock
@patch("services.payment.charge")
def test_process_order(mock_charge):
    mock_charge.return_value = {"status": "success", "payment_id": "pay_123"}
    result = process_order(order_id=1)
    assert result["status"] == "completed"
# Never tests actual HTTP call, serialization, error handling
```

**Fix:**
```python
# GOOD: Integration test with real HTTP (using responses library or test server)
import responses

@responses.activate
def test_process_order_integration():
    # Register realistic mock response
    responses.add(
        responses.POST,
        "https://payment-service/charge",
        json={"status": "success", "payment_id": "pay_123"},
        status=200
    )
    
    result = process_order(order_id=1)
    
    # Verify the actual HTTP call was made
    assert len(responses.calls) == 1
    request_body = json.loads(responses.calls[0].request.body)
    assert request_body["order_id"] == 1
    
    # Verify the result
    assert result["status"] == "completed"
    assert result["payment_id"] == "pay_123"

@responses.activate
def test_process_order_payment_failure():
    responses.add(
        responses.POST,
        "https://payment-service/charge",
        json={"error": "insufficient_funds"},
        status=402
    )
    
    result = process_order(order_id=1)
    assert result["status"] == "failed"
    assert result["error"] == "payment_declined"
```

---

### AP-QUAL-004: Flaky Test (Timing Dependency)

**Severity:** medium

```python
# BAD: Timing-dependent test — flaky on slow CI
def test_cache_expiry():
    cache.set("key", "value", ttl=1)
    time.sleep(1)  # Flaky! Might not be enough on slow CI
    assert cache.get("key") is None
```

**Fix:**
```python
# GOOD: Use freezegun to control time
from freezegun import freeze_time
from datetime import datetime, timedelta

def test_cache_expiry():
    with freeze_time("2026-01-01 12:00:00"):
        cache.set("key", "value", ttl=60)
        assert cache.get("key") == "value"
    
    with freeze_time("2026-01-01 12:01:01"):  # 61 seconds later
        assert cache.get("key") is None

# GOOD: Mock the time function
def test_cache_expiry_with_mock():
    with patch("time.time") as mock_time:
        mock_time.return_value = 1000.0
        cache.set("key", "value", ttl=60)
        
        mock_time.return_value = 1061.0  # 61 seconds later
        assert cache.get("key") is None
```

---

### AP-QUAL-005: Test That Doesn't Test What It Claims

**Severity:** medium

```python
# BAD: Test name says "test_user_can_login" but tests nothing real
def test_user_can_login():
    # Tests that the login function exists and doesn't raise
    # But doesn't verify the user is actually logged in!
    login(username="alice", password="password123")
    # No assertion about session, token, or login state
```

**Fix:**
```python
# GOOD: Test verifies the actual behavior claimed by the name
def test_user_can_login_with_valid_credentials():
    response = client.post("/api/auth/login", json={
        "username": "alice",
        "password": "password123"
    })
    
    assert response.status_code == 200
    data = response.json()
    assert "access_token" in data, "Login should return access_token"
    assert "refresh_token" in data, "Login should return refresh_token"
    
    # Verify token is valid
    token_response = client.get("/api/me", headers={
        "Authorization": f"Bearer {data['access_token']}"
    })
    assert token_response.status_code == 200
    assert token_response.json()["username"] == "alice"

def test_user_cannot_login_with_wrong_password():
    response = client.post("/api/auth/login", json={
        "username": "alice",
        "password": "wrong_password"
    })
    
    assert response.status_code == 401
    assert "access_token" not in response.json()
```

---

## Property-Based Testing for Edge Cases

Property-based testing generates hundreds of test cases automatically, finding edge cases you wouldn't think to write:

```python
# Using Hypothesis (Python)
from hypothesis import given, strategies as st, settings
from hypothesis import HealthCheck

@given(st.text())
def test_username_validation_never_crashes(username):
    """Username validation should never raise an unhandled exception."""
    try:
        result = validate_username(username)
        assert isinstance(result, bool)
    except ValidationError:
        pass  # Expected for invalid usernames

@given(st.lists(st.integers()))
def test_pagination_with_any_list(items):
    """Pagination should work for any list, including empty."""
    page_size = 10
    page = paginate(items, page=1, page_size=page_size)
    assert len(page) <= page_size
    assert len(page) <= len(items)

@given(
    st.text(min_size=1, max_size=100),
    st.integers(min_value=1, max_value=1000)
)
def test_search_with_any_query(query, limit):
    """Search should return at most `limit` results for any query."""
    results = search(query=query, limit=limit)
    assert len(results) <= limit
```

---

## Mutation Testing

Mutation testing verifies that your tests actually catch bugs by introducing small code changes (mutations) and checking if tests fail:

```bash
# Python: mutmut
pip install mutmut
mutmut run --paths-to-mutate src/

# Check results
mutmut results

# Example output:
# Survived mutations (tests didn't catch these):
# - src/auth.py:42: Changed ">" to ">=" in password length check
# - src/payment.py:87: Changed "==" to "!=" in status check
# Action: Add tests for these boundary conditions
```

**Interpreting results:**
- **Killed mutation**: Test caught the change → good test
- **Survived mutation**: Test didn't catch the change → test gap
- **Target kill rate**: > 80% for critical paths

---

## Finding Templates

### HARDEN-QUAL-NO-ERROR-PATH

```yaml
id: HARDEN-QUAL-NO-ERROR-PATH
severity: medium
category: quality
location: "tests/test_payment.py"
description: "process_payment() has no test for payment service failure."
impact: >
  Payment service failures are not tested. If the service returns 402 or 500,
  the error handling code is untested. Bugs in error handling will reach production.
recommendation: >
  Add test: when payment service returns 402, process_payment() returns
  PaymentResult(success=False, error="payment_declined") rather than raising.
  Add test: when payment service returns 500, process_payment() raises
  ServiceUnavailableError rather than returning success.
acceptance_criteria:
  - "Test exists: test_process_payment_payment_declined — sends 402 response, verifies PaymentResult(success=False)"
  - "Test exists: test_process_payment_service_error — sends 500 response, verifies ServiceUnavailableError raised"
  - "Both tests pass in CI: pytest tests/test_payment.py -v shows PASSED"
  - "Inject payment service failure at runtime; verify error response returned (not silent success)"
verification_tier: 3
confidence: confirmed
assumptions: "payment service is mocked in tests using responses library"
requires_human_review: false
```

### HARDEN-QUAL-FLAKY-TEST

```yaml
id: HARDEN-QUAL-FLAKY-TEST
severity: medium
category: quality
location: "tests/test_cache.py:42"
description: "test_cache_expiry uses time.sleep(1) — timing-dependent and flaky."
impact: >
  Test fails intermittently on slow CI machines. Erodes trust in test suite.
  Team starts ignoring test failures. Real bugs get missed.
recommendation: >
  Replace time.sleep(1) with freezegun to control time deterministically:
  with freeze_time("2026-01-01 12:01:01"): assert cache.get("key") is None
acceptance_criteria:
  - "Test passes consistently across 10 consecutive runs: pytest tests/test_cache.py --count=10"
  - "Test does not use time.sleep()"
  - "Test uses freezegun or mock time to control time"
verification_tier: 3
confidence: confirmed
assumptions: "freezegun is compatible with the cache implementation"
requires_human_review: false
```

---

## Acceptance Criteria Templates (Tier 3+)

1. **Critical path tested**: Run test suite → verify test for auth/payment/write path exists and passes
2. **Error path tested**: Inject dependency failure → verify test catches error and verifies correct behavior
3. **Integration boundary tested**: Run integration test → verify real HTTP call made → verify response handled
4. **Flaky test fixed**: Run test 10 times → verify consistent pass/fail
5. **Mutation kill rate**: Run mutmut → verify kill rate > 80% for critical modules

---

axiom:trace work_item=hardening-skills-01 spec=hardening-quality-axiom jira_ref=SWDE-7 plan=phase-1/task-6/step-1
