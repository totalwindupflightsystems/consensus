---
name: hardening-anti-patterns-axiom
description: >
  Master catalog of anti-patterns across all 6 hardening categories (SPOF, Security,
  Database, SRE, Quality, Observability). The "what NOT to do" reference for any repo
  using Axiom. Load this skill first — the other 6 category skills reference it.
  Produces findings in standard HARDEN-<category>-<slug> format with Tier-3+ verifiable
  acceptance criteria.
version: "1.0"
tags:
  vertical: [security, coding, reliability, observability, testing]
  category: hardening
  core: false
metadata:
  related_skills:
    - hardening-spof-axiom
    - hardening-security-axiom
    - hardening-database-axiom
    - hardening-sre-axiom
    - hardening-quality-axiom
    - hardening-observability-axiom
    - hardening-intake-axiom
  complements:
    - security-review-axiom
    - test-quality-gates-axiom
    - runtime-completeness-gate-axiom
    - chaos-engineer-axiom
---

# Hardening Anti-Patterns — Master Catalog

> **"The most dangerous code is the code that looks fine."**
>
> **"An anti-pattern is a pattern that seems like a good idea but consistently produces bad outcomes."**

This skill is the master reference for hardening anti-patterns across all 6 categories. Load it when you need a quick cross-category scan, or as the foundation before loading a category-specific skill.

**This skill is portable.** It has no Axiom-internal dependencies. Any agent in any repo can load it.

## When to Load This Skill

- Starting a hardening audit of any codebase
- Code review — quick cross-category anti-pattern check
- Before loading a category-specific hardening skill
- When a finding spans multiple categories
- Onboarding a new repo into Axiom
- Generating a hardening report (Markdown, HTML, or POC format)

## Report Generation

After collecting findings, generate a report in one of three formats:

| Format | Best for | Command |
|---|---|---|
| **Markdown** | PRs, Confluence, Notion, evidence bundles | `/hardening-battery --report` |
| **HTML** | Teaching tool, onboarding, executive summary | `/hardening-battery --report --format html` |
| **POC** | Convincing skeptics, developer-facing proof | `/hardening-battery --report --format poc` |

The HTML report is a **teaching tool** — it shows the failure scenario, how to find the anti-pattern in any codebase, how to fix it, and how to verify the fix. No external dependencies, works offline.

See `hardening-intake-axiom` skill for full report format specifications and teaching tool design principles.

## Scope & Limitations

**Language coverage:** Code examples and grep patterns in this skill and the 6 category skills are written for **Python, Go, and JavaScript/TypeScript**. Coverage by language:

| Language | Coverage | Notes |
|---|---|---|
| Python | ✅ Full | All patterns have Python examples and grep commands |
| Go | ✅ Full | All patterns have Go examples and grep commands |
| JavaScript | ✅ Full | Node.js patterns covered throughout |
| TypeScript | ⚠️ Partial | JS patterns apply; TS-specific patterns (type guards, strict null) not covered |
| Java | ⚠️ Partial | Key patterns: use `PreparedStatement` (not string concat) for SQL; `ProcessBuilder` list form (not `Runtime.exec(string)`) for commands; `@EntityGraph`/`JOIN FETCH` for N+1; `HttpClient` with timeout for SRE |
| Ruby | ❌ Not covered | Adapt Python patterns; use `.includes()` for N+1 (ActiveRecord) |
| PHP | ❌ Not covered | Adapt Python patterns; use PDO prepared statements for SQL injection |
| Rust | ❌ Not covered | Rust's type system prevents many patterns; focus on async/timeout patterns |

**Contributing patterns:** If your team uses a language not covered here, the anti-pattern concepts are universal — adapt the grep commands and code examples to your language. Consider contributing language-specific patterns back via PR to `specs/hardening-examples/<category>/`.

**OS requirement:** All grep patterns use Unix-style paths and flags (`--include`, `/`, etc.). These work on **Linux and macOS**. Windows repos require adapted patterns (use PowerShell `Select-String` or WSL).

**Finding ID scope:** `HARDEN-<category>-<slug>` IDs are **repo-scoped** — they are unique within a single repo but not globally unique. In multi-repo Jira workspaces, two repos may both produce `HARDEN-SRE-NO-TIMEOUT`. Use the Jira hierarchy (Initiative → Epic → Task) to provide the namespace, or add a repo prefix: `HARDEN-<repo>-SRE-NO-TIMEOUT`.

**Load order:** Load this skill first (shared audit header + finding format). Load `hardening-intake-axiom` last (lifecycle wiring). The 6 category skills can be loaded in any order between them.

**Toolchain:** The security skill references external tools (Snyk, Trivy, GitLeaks, pip-audit, npm audit). These are optional — the skill works without them, but CVE detection requires a dedicated SCA tool. See `hardening-security-axiom` for the full prerequisites list.

---

## Finding Format (Required for All Hardening Findings)

Every finding produced by any hardening skill MUST use this format:

```yaml
id: HARDEN-<CATEGORY>-<short-slug>
severity: critical | high | medium | low
category: spof | security | database | sre | quality | observability
location: "path/to/file.py:line_number"
description: "One or two sentences describing the issue."
impact: "What could go wrong, concretely. Include blast radius."
recommendation: "What to change. Include a short code snippet if it fits in ~10 lines."
acceptance_criteria:
  - "Testable condition 1 (Tier 3+: CLI runtime, HTTP health, or end-to-end flow)"
  - "Testable condition 2"
verification_tier: 2 | 3 | 4 | 5
  # 2 = unit test sufficient
  # 3 = CLI runtime execution required
  # 4 = server + health check required
  # 5 = end-to-end flow required
confidence: confirmed | suspected
  # confirmed = directly observed in code
  # suspected = pattern match, needs verification
assumptions: "Anything relied on that could be wrong."
requires_human_review: true | false
  # MUST be true for: all security findings, all migration findings
```

### Severity Rubric

| Severity | Meaning | Fix timeline |
|---|---|---|
| **critical** | Customer-facing impact or data loss likely | This week |
| **high** | Same impact under specific conditions | This sprint |
| **medium** | Degraded experience or maintainability risk | This quarter |
| **low** | Cleanup, hygiene, nice-to-have | When touching nearby code |

---

## Shared Audit Header

Prepend this to every hardening prompt or audit session:

```
You are a senior engineer conducting an internal code audit. This is a
defender-side review: the goal is to harden our systems before issues
reach production. Be specific, cite file paths and line numbers,
distinguish confirmed issues from suspected ones, and surface the
highest-impact findings first.

Priority order (from the Axiom operating doctrine):
  1. Safety, security, containment of harm
  2. Truthfulness and auditability
  3. Correctness relative to "done" criteria
  4. Intent alignment with repo owners
  5. Throughput

Honesty rules — non-negotiable:
- Never invent evidence. If you didn't see it, mark it suspected.
- Separate facts from assumptions.
- If confidence is low, say why and what would raise it.
- Do not produce exploit code. Produce remediation guidance.
```

---

## Category 1: SPOF Anti-Patterns

### AP-SPOF-001: Assume Dependency Is Always Up

**Pattern:**
```python
# BAD: No timeout, no error handling
response = requests.get("https://user-service/users/123")
return response.json()
```

**Why it fails:** No timeout means the request hangs indefinitely. Thread/connection exhaustion cascades to upstream callers.

**Detection:** `grep -r "requests.get\|httpx.get" --include="*.py" | grep -v "timeout"`

**Fix:** Add explicit timeout + fallback. See `hardening-spof-axiom` for full pattern.

---

### AP-SPOF-002: Single Database with No Replica

**Pattern:**
```yaml
# BAD: Single DB instance, no replica
database:
  host: db-primary.internal
```

**Why it fails:** Complete outage if primary fails. No read scaling. No failover.

**Detection:** `grep -r "host.*=.*['\"]" --include="*.yaml" | grep -v "replica\|backup\|secondary"`

---

### AP-SPOF-003: No Circuit Breaker on Flaky Dependency

**Pattern:**
```python
# BAD: Keeps retrying a failing service with no circuit breaker
for attempt in range(3):
    try:
        return call_service()
    except Exception:
        pass
```

**Why it fails:** Keeps hammering a failing service. Delays recovery. Wastes resources.

**Detection:** Look for retry loops without circuit breaker state tracking.

---

### AP-SPOF-004: Shared Resource Stops Unrelated Features

**Pattern:** A single Redis instance used for both session storage AND rate limiting AND caching. When Redis goes down, all three features fail simultaneously.

**Why it fails:** Blast radius is the entire application, not just one feature.

**Detection:** Audit shared infrastructure dependencies. Map which features depend on each shared resource.

---

## Category 2: Security Anti-Patterns

### AP-SEC-001: SQL Injection via String Concatenation

**Pattern:**
```python
# BAD: String concatenation
query = f"SELECT * FROM users WHERE username = '{username}'"
cursor.execute(query)
```

**Why it fails:** User input treated as SQL code. Attacker can extract, modify, or delete data.

**Detection:** `grep -r "execute.*f\"\|execute.*format\|execute.*%" --include="*.py" | grep -v "parameterized\|%s\|:param"`

**Fix:** Use parameterized queries. See `hardening-security-axiom`.

---

### AP-SEC-002: Command Injection via shell=True

**Pattern:**
```python
# BAD: shell=True with user input
subprocess.run(f"ls {dirname}", shell=True)
```

**Why it fails:** Shell interprets `;`, `|`, `&`, `$()`. Full system compromise possible.

**Detection:** `grep -r "subprocess.*shell=True\|os.system(" --include="*.py"`

**Fix:** Pass arguments as list, `shell=False`. See `hardening-security-axiom`.

---

### AP-SEC-003: Hardcoded Secrets

**Pattern:**
```python
# BAD: Secrets in code
API_KEY = "sk-live-abc123xyz"
DATABASE_PASSWORD = "super_secret_123"
```

**Why it fails:** Secrets exposed in version control. Cannot rotate without code change.

**Detection:** `grep -r "password.*=.*['\"\|api_key.*=.*['\"]\|secret.*=.*['\"]" --include="*.py" | grep -v "os.environ\|os.getenv\|get_secret"`

---

### AP-SEC-004: Authentication Without Authorization

**Pattern:**
```python
# BAD: Checks login but not permissions
if not current_user.is_authenticated:
    return 401
# Anyone logged in can do anything!
User.query.filter_by(id=user_id).delete()
```

**Why it fails:** Authentication ≠ Authorization. Any logged-in user can perform any action.

**Detection:** Review all `@app.route` handlers for missing permission checks.

---

### AP-SEC-005: Unsafe YAML/JSON Parsing

**Pattern:**
```python
# BAD: Unsafe YAML loader
import yaml
data = yaml.load(user_input)  # Allows arbitrary code execution!
```

**Why it fails:** `yaml.load()` can execute arbitrary Python code via `!!python/object`.

**Detection:** `grep -r "yaml.load(" --include="*.py" | grep -v "Loader=yaml.SafeLoader\|yaml.safe_load"`

---

## Category 3: Database Anti-Patterns

### AP-DB-001: N+1 Query Problem

**Pattern:**
```python
# BAD: 1 query for posts, N queries for authors
posts = Post.objects.all()
for post in posts:
    print(post.author.name)  # Separate query per post!
```

**Why it fails:** 100 posts = 101 queries. Massive performance degradation at scale.

**Detection:** `grep -r "for.*in.*objects.all()" --include="*.py" -A 5 | grep -v "select_related\|prefetch_related"`

---

### AP-DB-002: Transaction Spanning HTTP Call

**Pattern:**
```python
# BAD: Transaction held open during HTTP call
with db.transaction():
    order.status = "processing"
    db.commit()
    response = requests.post("https://payment-service/charge", ...)  # HTTP inside transaction!
    order.status = "completed" if response.ok else "failed"
    db.commit()
```

**Why it fails:** Database locks held during network I/O. Deadlocks, lock contention, connection pool exhaustion.

**Detection:** `grep -r "with.*transaction\|@transaction.atomic" --include="*.py" -A 20 | grep "requests\|httpx\|fetch"`

---

### AP-DB-003: Table-Locking DDL Migration

**Pattern:**
```sql
-- BAD: Blocks all reads and writes
ALTER TABLE users ADD COLUMN phone VARCHAR(20);
```

**Why it fails:** `ALTER TABLE` acquires ACCESS EXCLUSIVE lock. Blocks all reads and writes. Can take minutes on large tables.

**Detection:** `grep -r "ALTER TABLE\|ADD COLUMN\|DROP COLUMN" --include="*.sql" --include="*.py"`

**Note:** All migration findings MUST have `requires_human_review: true`.

---

### AP-DB-004: Unbounded Connection Pool

**Pattern:**
```python
# BAD: No pool limits
engine = create_engine("postgresql://user:pass@host/db")
# Default pool_size=5, max_overflow=10, no timeout
```

**Why it fails:** Under load, all connections consumed. New requests wait indefinitely.

**Detection:** `grep -r "create_engine\|Engine(" --include="*.py" | grep -v "pool_size\|max_overflow"`

---

## Category 4: SRE Anti-Patterns

### AP-SRE-001: Missing Timeout on External Call

**Pattern:**
```python
# BAD: No timeout
response = requests.get("https://external-service/api")
```

**Why it fails:** Will hang indefinitely if service is down. Thread/connection exhaustion.

**Detection:** `grep -r "requests.get\|requests.post\|httpx.get" --include="*.py" | grep -v "timeout"`

---

### AP-SRE-002: Retry Without Backoff (Thundering Herd)

**Pattern:**
```python
# BAD: Immediate retries
for attempt in range(3):
    try:
        return call_service()
    except Exception:
        pass  # Immediate retry!
```

**Why it fails:** All clients retry simultaneously. Thundering herd makes overload worse.

**Detection:** `grep -r "for.*range.*retry\|while.*retry" --include="*.py" | grep -v "sleep\|backoff\|wait"`

---

### AP-SRE-003: Goroutine/Thread Leak

**Pattern:**
```go
// BAD: Goroutine never terminates
go func() {
    result := expensiveOperation()
    resultChan <- result  // Blocks forever if nobody reads!
}()
```

**Why it fails:** Goroutines accumulate. Memory leak. File descriptor exhaustion. Process crashes.

**Detection:** `grep -r "go func\|goroutine" --include="*.go" | grep -v "context\|ctx\|Done()"`

---

### AP-SRE-004: No Graceful Shutdown

**Pattern:**
```python
# BAD: No SIGTERM handling
app.run(host="0.0.0.0", port=8080)
# Process killed immediately, in-flight requests fail
```

**Why it fails:** In-flight requests terminated abruptly. Database connections not closed cleanly.

**Detection:** `grep -r "signal.signal\|SIGTERM" --include="*.py" --include="*.go"` — absence is the finding.

---

### AP-SRE-005: Swallowed Errors

**Pattern:**
```python
# BAD: Error silently ignored
try:
    result = risky_operation()
except Exception:
    pass  # Error swallowed!
```

**Why it fails:** Failures invisible. No alerting. No debugging context. Silent data corruption possible.

**Detection:** `grep -r "except.*:\s*$\|except.*pass" --include="*.py"`

---

## Category 5: Quality Anti-Patterns

### AP-QUAL-001: Assertionless Test

**Pattern:**
```python
# BAD: No assertions
def test_user_creation():
    user = User(name="Alice")
    db.session.add(user)
    db.session.commit()
    # No assertion! Always passes!
```

**Why it fails:** Test passes even if code is broken. False confidence.

**Detection:** `grep -r "def test_" --include="*.py" -A 10 | grep -v "assert\|pytest.raises\|pytest.warns"`

---

### AP-QUAL-002: Tautology Test

**Pattern:**
```python
# BAD: Tests Python semantics, not product code
def test_rt_bug_156():
    result = None or "default"
    assert result == "default"  # Tests Python `or`, not your code!
```

**Why it fails:** Always passes. Doesn't verify product behavior. Wastes CI resources.

**Detection:** `grep -r "assert.*==.*\b\w\+\b" --include="*.py" | grep -v "expected\|result\|actual"`

---

### AP-QUAL-003: Over-Mocking (Testing the Mock, Not the Code)

**Pattern:**
```python
# BAD: Everything mocked, nothing real tested
@patch('services.payment.charge')
@patch('services.email.send')
@patch('services.db.save')
def test_process_order(mock_save, mock_email, mock_charge):
    mock_charge.return_value = {"status": "success"}
    result = process_order(order_id=1)
    assert result == True  # But we mocked everything!
```

**Why it fails:** Tests mocks, not real code. Integration bugs not caught.

**Detection:** Count `@patch` decorators per test file. High ratio = over-mocking risk.

---

### AP-QUAL-004: Missing Error Path Tests

**Pattern:**
```python
# BAD: Only happy path tested
def test_get_user():
    user = get_user(user_id=1)
    assert user.name == "Alice"
# No test for: user not found, database error, timeout
```

**Why it fails:** Error handling bugs not caught. Production failures not detected in CI.

**Detection:** `grep -r "def test_" --include="*.py" | grep -v "error\|fail\|exception\|invalid\|missing\|not_found"`

---

### AP-QUAL-005: Flaky Test (Timing/Ordering Dependency)

**Pattern:**
```python
# BAD: Test depends on timing
def test_cache_expiry():
    cache.set("key", "value", ttl=1)
    time.sleep(1)  # Flaky! Timing-dependent
    assert cache.get("key") is None
```

**Why it fails:** Passes on fast machines, fails on slow CI. Erodes trust in test suite.

**Detection:** `grep -r "time.sleep\|asyncio.sleep" --include="*.py" --include="*.js"` in test files.

---

## Category 6: Observability Anti-Patterns

### AP-OBS-001: Unstructured Log Messages

**Pattern:**
```python
# BAD: Unstructured log
logger.info(f"User {user_id} logged in from {ip_address}")
```

**Why it fails:** Can't query by field. Can't aggregate. Hard to debug in production.

**Detection:** `grep -r "logger.info.*f\"\|logger.error.*f\"" --include="*.py" | grep -v "structlog\|extra="`

---

### AP-OBS-002: Missing Correlation ID

**Pattern:**
```python
# BAD: No correlation ID
def process_request():
    logger.info("Request received")
    call_service_a()
    call_service_b()
```

**Why it fails:** Can't trace request across services. Debugging distributed systems is impossible.

**Detection:** `grep -r "def.*request\|@app.route" --include="*.py" -A 10 | grep -v "correlation_id\|request_id\|trace_id"`

---

### AP-OBS-003: High-Cardinality Metric Labels

**Pattern:**
```python
# BAD: User ID in metric label
http_requests_total = Counter('http_requests_total', '...', ['user_id', 'endpoint'])
```

**Why it fails:** Millions of users = millions of time series. Prometheus crashes.

**Detection:** `grep -r "Counter\|Histogram\|Gauge" --include="*.py" -A 3 | grep "user_id\|request_id\|session_id"`

---

### AP-OBS-004: PII in Logs

**Pattern:**
```python
# BAD: Logging PII
logger.info(f"User login: email={email}, password={password}")
```

**Why it fails:** GDPR/HIPAA/PCI-DSS violations. Security risk if logs leaked.

**Detection:** `grep -r "logger.*email\|logger.*password\|logger.*credit_card\|logger.*ssn" --include="*.py"`

---

### AP-OBS-005: Alert Without Runbook

**Pattern:**
```yaml
# BAD: Alert with no runbook link
- alert: HighErrorRate
  expr: error_rate > 0.05
  # No runbook_url!
```

**Why it fails:** On-call engineer gets paged at 3am with no guidance. Longer MTTR.

**Detection:** `grep -r "alert:\|alerting:" --include="*.yaml" --include="*.yml" -A 10 | grep -v "runbook_url\|runbook"`

---

## Cross-Category Anti-Patterns

### AP-CROSS-001: "Looks Built" ≠ "Actually Works"

The most dangerous anti-pattern in AI-assisted development. Components pass unit tests in isolation but fail when wired together. See `runtime-completeness-gate-axiom` for the full detection protocol.

**Signals:**
- All unit tests pass but the feature doesn't work end-to-end
- Components built separately, never tested together
- "Done" claimed based on test coverage %, not runtime behavior

### AP-CROSS-002: Verification Theater

Tests that pass but prove component shape, not system behavior. See `test-quality-gates-axiom` for the full detection protocol.

**Signals:**
- High coverage % but bugs still reach production
- Tests mock everything, test nothing real
- Tests verify function signatures, not function behavior

### AP-CROSS-003: Spec Drift

Code behavior diverges from specs over time. No one notices until production incident.

**Signals:**
- Specs not updated when behavior changes
- No spec-conformance tests
- "We'll update the docs later" (never happens)

---

## Quick Scan Checklist (Cross-Category)

Run this before any code review or hardening audit:

```bash
# SPOF: HTTP calls without timeout
grep -r "requests.get\|requests.post\|httpx" --include="*.py" | grep -v "timeout"

# Security: shell=True
grep -r "shell=True" --include="*.py"

# Security: hardcoded secrets
grep -r "password.*=.*['\"\|api_key.*=.*['\"]" --include="*.py" | grep -v "os.environ\|os.getenv"

# Security: unsafe YAML
grep -r "yaml.load(" --include="*.py" | grep -v "SafeLoader\|safe_load"

# Database: N+1 risk
grep -r "for.*in.*objects.all()" --include="*.py" | grep -v "select_related\|prefetch_related"

# Database: transaction + HTTP
grep -r "with.*transaction" --include="*.py" -A 20 | grep "requests\|httpx"

# SRE: swallowed errors
grep -r "except.*:\s*$\|except.*pass" --include="*.py"

# Quality: assertionless tests
grep -r "def test_" --include="*.py" -A 10 | grep -v "assert\|raises\|warns"

# Observability: unstructured logs
grep -r "logger.info.*f\"\|print(" --include="*.py" | grep -v "structlog"

# Observability: PII in logs
grep -r "logger.*password\|logger.*email\|logger.*token" --include="*.py"
```

---

## Anti-Pattern Severity Matrix

| Anti-Pattern | Default Severity | Requires Human Review |
|---|---|---|
| SQL injection | critical | ✅ yes |
| Command injection (shell=True) | critical | ✅ yes |
| Hardcoded secrets | critical | ✅ yes |
| Missing auth check | critical | ✅ yes |
| Unsafe YAML parsing | high | ✅ yes |
| Missing timeout on external call | high | no |
| No circuit breaker | high | no |
| N+1 query | medium–high | no |
| Transaction spanning HTTP | high | no |
| Table-locking migration | high | ✅ yes |
| Unbounded connection pool | high | no |
| Retry without backoff | medium | no |
| Goroutine/thread leak | high | no |
| No graceful shutdown | medium | no |
| Swallowed errors | medium | no |
| Assertionless test | medium | no |
| Tautology test | medium | no |
| Over-mocking | medium | no |
| Missing error path test | medium | no |
| Flaky test | medium | no |
| Unstructured logging | medium | no |
| Missing correlation ID | high | no |
| High-cardinality metric labels | high | no |
| PII in logs | critical | ✅ yes |
| Alert without runbook | medium | no |

---

## References

- `hardening-spof-axiom` — Full SPOF detection and remediation patterns
- `hardening-security-axiom` — Full security audit workflow
- `hardening-database-axiom` — Full DB and data layer patterns
- `hardening-sre-axiom` — Full reliability/SRE patterns
- `hardening-quality-axiom` — Full test coverage gap patterns
- `hardening-observability-axiom` — Full observability patterns
- `hardening-intake-axiom` — How to run the battery and wire into Axiom
- `security-review-axiom` — STRIDE threat modeling and security gate checklist
- `test-quality-gates-axiom` — Test quality enforcement
- `runtime-completeness-gate-axiom` — Wiring gap and verification theater detection

---

axiom:trace work_item=hardening-skills-01 spec=hardening-anti-patterns-axiom jira_ref=SWDE-7 plan=phase-1/task-1/step-1
