---
name: hardening-spof-axiom
description: >
  Single points of failure (SPOF) detection, blast radius analysis, and remediation
  patterns for any codebase. Covers missing timeouts, no circuit breakers, no replicas,
  no fallback paths, and shared resources that stop unrelated features. Produces
  HARDEN-SPOF-* findings with Tier-3+ verifiable acceptance criteria.
version: "1.0"
tags:
  vertical: [reliability, coding]
  category: hardening
  core: false
metadata:
  related_skills:
    - hardening-anti-patterns-axiom
    - hardening-sre-axiom
    - hardening-intake-axiom
    - chaos-engineer-axiom
    - sre-ops-axiom
---

# Hardening: Single Points of Failure (SPOF)

> **"Prioritize by blast radius: how many user-facing features break if this one thing goes down?"**
>
> **"A SPOF is not just a missing replica. It's any code path that assumes a dependency is available and crashes hard if it isn't."**

This skill audits a codebase for single points of failure. It is portable — no Axiom-internal dependencies. Any agent in any repo can load it.

## When to Load This Skill

- Auditing a service for resilience gaps
- After a production outage caused by a dependency failure
- Before a major traffic event (launch, sale, migration)
- As part of a quarterly hardening battery
- When `chaos-engineer-axiom` identifies a missing fallback

---

## The SPOF Audit Prompt

Use this prompt (with the shared header from `hardening-anti-patterns-axiom`) to audit a codebase:

```
Audit this codebase and its infrastructure configuration for single
points of failure. For each SPOF, identify:

- Services, databases, caches, or queues with no replica or failover
- Third-party dependencies with no fallback path or circuit breaker
- Shared resources whose failure stops unrelated features
- Configuration or secrets sources with no redundancy
- Code paths that assume a dependency is available and crash hard
  if it isn't (no timeout, no retry, no graceful degradation)

Exclude: components where redundancy is explicitly out of scope
(local dev tools, one-off scripts, non-production environments).

Prioritize by blast radius: how many user-facing features break if
this one thing goes down?

For each finding, the acceptance_criteria should describe the
observable state when fixed: "external call to X has a timeout of
N seconds and a circuit breaker that trips after M failures, as
demonstrated by failure injection test."
```

---

## SPOF Detection Checklist

### Infrastructure SPOFs

- [ ] **Single database instance** — no read replicas, no standby
- [ ] **Single cache instance** — Redis/Memcached with no replica
- [ ] **Single message queue** — no dead-letter queue, no retry
- [ ] **Single secrets source** — Vault/Secrets Manager with no fallback
- [ ] **Single region deployment** — no multi-region or multi-AZ
- [ ] **Single CDN or load balancer** — no redundancy at the edge

### Code-Level SPOFs

- [ ] **HTTP calls without timeout** — will hang indefinitely
- [ ] **HTTP calls without retry** — single failure = user-visible error
- [ ] **HTTP calls without circuit breaker** — keeps hammering failing service
- [ ] **HTTP calls without fallback** — no degraded response when service is down
- [ ] **Database queries without timeout** — runaway query blocks connection pool
- [ ] **Cache calls without fallback** — cache miss = hard failure instead of DB fallback
- [ ] **External API calls without error handling** — exception propagates to user

### Shared Resource SPOFs

- [ ] **Single Redis for sessions + rate limiting + caching** — one failure stops all three
- [ ] **Single DB for OLTP + analytics** — analytics query starves OLTP
- [ ] **Single thread pool for all async work** — one slow task blocks all others

---

## Detection Patterns

### Grep Commands

```bash
# HTTP calls without timeout (Python)
grep -rn "requests.get\|requests.post\|requests.put\|requests.delete\|httpx.get\|httpx.post" \
  --include="*.py" | grep -v "timeout"

# HTTP calls without timeout (JavaScript/TypeScript)
grep -rn "fetch(\|axios.get\|axios.post" \
  --include="*.js" --include="*.ts" | grep -v "timeout\|signal"

# HTTP calls without timeout (Go)
grep -rn "http.Get\|http.Post\|client.Do" \
  --include="*.go" | grep -v "Timeout\|context"

# Database connections without pool limits (SQLAlchemy)
grep -rn "create_engine(" --include="*.py" | grep -v "pool_size\|max_overflow"

# Missing circuit breaker (look for retry loops without circuit state)
grep -rn "for.*range.*retry\|while.*retry\|for.*attempt" \
  --include="*.py" --include="*.go" | grep -v "circuit\|breaker\|open\|closed"

# Single-host configuration (no replica)
grep -rn "host.*=.*['\"]" --include="*.yaml" --include="*.yml" --include="*.env" \
  | grep -v "replica\|backup\|secondary\|standby\|read_host"
```

---

## Anti-Patterns with Fixes

### AP-SPOF-001: No Timeout on External HTTP Call

**Severity:** high  
**Blast radius:** All users of the feature that calls this service

```python
# BAD: Will hang indefinitely if service is down
import requests

def get_user_profile(user_id):
    response = requests.get(f"https://profile-service/users/{user_id}")
    return response.json()
```

**Fix:**
```python
# GOOD: Explicit timeout + fallback
import requests
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

_session = requests.Session()
_session.mount("https://", HTTPAdapter(
    max_retries=Retry(total=3, backoff_factor=0.5, status_forcelist=[429, 500, 502, 503, 504])
))

def get_user_profile(user_id):
    try:
        response = _session.get(
            f"https://profile-service/users/{user_id}",
            timeout=(3.05, 10)  # (connect_timeout, read_timeout)
        )
        response.raise_for_status()
        return response.json()
    except requests.Timeout:
        logger.warning("profile_service_timeout", user_id=user_id)
        return get_cached_profile(user_id)  # Fallback to cache
    except requests.RequestException as e:
        logger.error("profile_service_error", user_id=user_id, error=str(e))
        return None  # Graceful degradation
```

**Timeout guidelines:**
| Call type | Connect timeout | Read timeout |
|---|---|---|
| Internal service (same DC) | 1s | 5s |
| External API | 3s | 30s |
| Database query | — | 5s |
| Cache (Redis) | 0.5s | 1s |
| File I/O | — | 10s |

---

### AP-SPOF-002: No Circuit Breaker on Flaky Dependency

**Severity:** high  
**Blast radius:** All requests that depend on the flaky service

```python
# BAD: Keeps hammering a failing service
def call_payment_service(order_id, amount):
    for attempt in range(3):
        try:
            return requests.post("https://payment-service/charge", 
                               json={"order_id": order_id, "amount": amount},
                               timeout=10)
        except Exception:
            if attempt == 2:
                raise
```

**Fix (Python `circuitbreaker` library):**
```python
# GOOD: Circuit breaker prevents cascading failure
from circuitbreaker import circuit
import requests

@circuit(
    failure_threshold=5,      # Open after 5 consecutive failures
    recovery_timeout=60,      # Try again after 60 seconds
    expected_exception=requests.RequestException
)
def call_payment_service(order_id, amount):
    response = requests.post(
        "https://payment-service/charge",
        json={"order_id": order_id, "amount": amount},
        timeout=10
    )
    response.raise_for_status()
    return response.json()

# Circuit breaker states:
# CLOSED  → Normal operation, requests go through
# OPEN    → Failures exceeded threshold, requests fail fast (no network call)
# HALF-OPEN → After recovery_timeout, try one request to test recovery
```

**Fix (Go `gobreaker` library):**
```go
import "github.com/sony/gobreaker"

var cb *gobreaker.CircuitBreaker

func init() {
    cb = gobreaker.NewCircuitBreaker(gobreaker.Settings{
        Name:        "payment-service",
        MaxRequests: 1,
        Interval:    60 * time.Second,
        Timeout:     30 * time.Second,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures > 5
        },
    })
}

func callPaymentService(orderID string, amount float64) (PaymentResult, error) {
    result, err := cb.Execute(func() (interface{}, error) {
        return httpClient.Post("https://payment-service/charge", ...)
    })
    if err != nil {
        return PaymentResult{}, err
    }
    return result.(PaymentResult), nil
}
```

---

### AP-SPOF-003: No Fallback for Cache Miss

**Severity:** medium  
**Blast radius:** All users when cache is unavailable

```python
# BAD: Cache failure = hard failure
def get_product(product_id):
    cached = redis.get(f"product:{product_id}")
    if cached:
        return json.loads(cached)
    # If Redis is down, this raises an exception!
    raise CacheError("Cache unavailable")
```

**Fix:**
```python
# GOOD: Cache failure falls back to database
def get_product(product_id):
    try:
        cached = redis.get(f"product:{product_id}")
        if cached:
            return json.loads(cached)
    except redis.RedisError as e:
        logger.warning("cache_unavailable", product_id=product_id, error=str(e))
        # Fall through to database
    
    # Fallback: query database
    product = Product.query.get(product_id)
    if product is None:
        raise ProductNotFoundError(product_id)
    
    # Try to repopulate cache (best-effort)
    try:
        redis.setex(f"product:{product_id}", 300, json.dumps(product.to_dict()))
    except redis.RedisError:
        pass  # Cache repopulation failure is acceptable
    
    return product.to_dict()
```

---

### AP-SPOF-004: Shared Resource Stops Unrelated Features

**Severity:** high  
**Blast radius:** All features sharing the resource

**Pattern:** Single Redis instance used for sessions, rate limiting, AND caching.

**Fix:** Separate Redis instances (or at minimum, separate databases within Redis) per concern:

```yaml
# GOOD: Separate Redis instances per concern
redis:
  sessions:
    host: redis-sessions.internal
    port: 6379
    db: 0
  rate_limiting:
    host: redis-ratelimit.internal
    port: 6379
    db: 0
  cache:
    host: redis-cache.internal
    port: 6379
    db: 0
```

Or use Redis Cluster with keyspace isolation:
```python
# GOOD: Namespace keys to enable future isolation
CACHE_KEY_PREFIX = "cache:"
SESSION_KEY_PREFIX = "session:"
RATELIMIT_KEY_PREFIX = "rl:"

def get_cache_key(key):
    return f"{CACHE_KEY_PREFIX}{key}"

def get_session_key(session_id):
    return f"{SESSION_KEY_PREFIX}{session_id}"
```

---

## Finding Templates

### HARDEN-SPOF-NO-TIMEOUT

```yaml
id: HARDEN-SPOF-NO-TIMEOUT
severity: high
category: spof
location: "path/to/file.py:42"
description: "HTTP call to external-service has no timeout configured."
impact: >
  If external-service is slow or unresponsive, this call will hang indefinitely.
  Under load, all threads/connections will be consumed, causing a cascading failure
  that takes down the entire service. Blast radius: all users.
recommendation: >
  Add explicit timeout: requests.get(url, timeout=(3.05, 10))
  Add retry with backoff: use requests.Session with HTTPAdapter(max_retries=Retry(...))
  Add fallback: return cached data or degraded response on timeout.
acceptance_criteria:
  - "HTTP call to external-service has timeout <= 30s, verified by blocking the service and observing timeout error within configured limit"
  - "Timeout triggers fallback response (cached data or graceful error), not unhandled exception"
  - "Metrics show request duration bounded by timeout value"
verification_tier: 3
confidence: confirmed
assumptions: "external-service is a production dependency, not a dev-only tool"
requires_human_review: false
```

### HARDEN-SPOF-NO-CIRCUIT-BREAKER

```yaml
id: HARDEN-SPOF-NO-CIRCUIT-BREAKER
severity: high
category: spof
location: "path/to/file.py:87"
description: "No circuit breaker on calls to payment-service."
impact: >
  When payment-service is degraded, all payment requests will fail slowly (waiting
  for timeout) rather than failing fast. Under sustained failure, this exhausts
  connection pools and cascades to other features. Blast radius: all checkout flows.
recommendation: >
  Add circuit breaker using circuitbreaker library (Python) or gobreaker (Go).
  Configure: failure_threshold=5, recovery_timeout=60s.
  Add fallback: queue payment for retry, show "payment processing" to user.
acceptance_criteria:
  - "Circuit breaker opens after 5 consecutive failures to payment-service, verified by failure injection test"
  - "While circuit is open, requests fail fast (< 10ms) without making network calls"
  - "Circuit transitions to HALF-OPEN after 60s and closes on successful request"
  - "Circuit state transitions logged with structured fields: circuit_name, state, failure_count"
verification_tier: 3
confidence: confirmed
assumptions: "payment-service is a synchronous dependency in the checkout flow"
requires_human_review: false
```

---

## Blast Radius Calculation

For each SPOF, estimate blast radius before prioritizing:

```
Blast Radius Score = (Users Affected %) × (Feature Criticality) × (Recovery Time)

Where:
  Users Affected %: What % of active users hit this code path?
  Feature Criticality: 1=nice-to-have, 2=important, 3=revenue-critical, 4=safety-critical
  Recovery Time: 1=auto-recovers in <1min, 2=manual fix in <1hr, 3=manual fix in hours, 4=data loss
```

**Example:**
- Payment service SPOF: 100% users × 4 (revenue-critical) × 3 (manual fix) = **1200** → Fix this week
- Profile picture SPOF: 10% users × 1 (nice-to-have) × 1 (auto-recovers) = **10** → Fix this quarter

---

## Real-World Incidents

### AWS US-EAST-1 Outage (October 2025)
- **Duration**: 15 hours 32 minutes
- **Root Cause**: Race condition in DynamoDB DNS management — a single-region SPOF
- **Impact**: 17 million user reports, 3,500 organizations affected
- **Lesson**: Even "global" services can have regional SPOFs. US-EAST-1 is AWS's oldest hub; many "global" stacks route through it.

### Cloudflare Outage (June 2022)
- **Duration**: ~1 hour
- **Root Cause**: BGP route change that removed Cloudflare from the internet
- **Impact**: ~19% of Cloudflare's network unreachable
- **Lesson**: Network infrastructure is a SPOF even for infrastructure providers.

### GitHub Outage (October 2018)
- **Duration**: 24 hours 11 minutes
- **Root Cause**: Network partition between US-East and US-West data centers
- **Impact**: Inconsistent data, degraded service
- **Lesson**: Distributed systems need explicit partition handling, not just replication.

---

## Recommended Libraries

| Language | Circuit Breaker | Retry | Timeout |
|---|---|---|---|
| Python | `circuitbreaker`, `pybreaker` | `tenacity` | `requests` timeout param |
| Go | `gobreaker` (Sony), `hystrix-go` | `retry-go` | `context.WithTimeout` |
| Node.js | `opossum` | `async-retry` | `AbortController` |
| Java | `Resilience4j`, `Hystrix` | `Resilience4j` | `HttpClient` timeout |

---

## Acceptance Criteria Templates (Tier 3+)

All SPOF acceptance criteria must be verifiable at Tier 3 (CLI runtime) or higher:

1. **Timeout enforcement**: Block downstream service → verify timeout fires within N seconds → verify fallback response returned
2. **Circuit breaker**: Inject 5+ consecutive failures → verify circuit opens → verify requests fail fast (< 10ms) → wait recovery_timeout → verify circuit closes on success
3. **Fallback response**: Disable dependency → verify degraded-but-useful response returned → verify no unhandled exception
4. **Replica failover**: Kill primary → verify traffic routes to replica → verify no data loss
5. **Blast radius isolation**: Fail shared resource → verify only dependent features fail, unrelated features continue

---

axiom:trace work_item=hardening-skills-01 spec=hardening-spof-axiom jira_ref=SWDE-7 plan=phase-1/task-2/step-1
