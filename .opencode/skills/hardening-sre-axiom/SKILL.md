---
name: hardening-sre-axiom
description: >
  Reliability and SRE hardening for any codebase. Covers missing timeouts, retries
  without backoff, missing circuit breakers, rate limiting gaps, resource leaks
  (goroutines, connections, file descriptors), missing graceful shutdown, and swallowed
  errors. Produces HARDEN-SRE-* findings with Tier-3+ verifiable acceptance criteria.
version: "1.0"
tags:
  vertical: [reliability, coding]
  category: hardening
  core: false
metadata:
  related_skills:
    - hardening-anti-patterns-axiom
    - hardening-spof-axiom
    - hardening-intake-axiom
    - sre-ops-axiom
    - chaos-engineer-axiom
---

# Hardening: Reliability & SRE

> **"Most reliability fixes are cleanly Tier-3 verifiable — you can write a test that injects a failure and observe the fallback."**
>
> **"The goal is not zero failures. The goal is graceful degradation when failures happen."**

This skill audits a codebase from a site reliability perspective. It is portable — no Axiom-internal dependencies.

## When to Load This Skill

- Auditing a service for reliability gaps
- After a production incident caused by missing timeout/retry/circuit breaker
- Before a major traffic event
- When `chaos-engineer-axiom` designs fault injection experiments
- As part of a quarterly hardening battery

---

## The SRE Audit Prompt

Use this prompt (with the shared header from `hardening-anti-patterns-axiom`):

```
Review this codebase from a site reliability perspective. Look for:

- Timeouts: missing on outbound HTTP calls, DB queries, queue ops
- Retries: missing where they'd help, retry without backoff, no cap
- Circuit breakers: missing on flaky external dependencies
- Rate limiting: missing on expensive or external-facing endpoints
- Graceful degradation: crashes instead of degraded-but-useful response
- Resource management: goroutine/thread/connection leaks, unbounded queues
- Startup and shutdown: no drain on SIGTERM, no distinct readiness probe
- Error handling: errors swallowed silently, panic where retry would do

For each finding, acceptance_criteria should be runtime-testable:
"outbound call X has a Y-second timeout and returns a fallback
response when exceeded, as demonstrated by a test that injects delay."
```

---

## SRE Audit Checklist

### Timeouts

- [ ] **HTTP calls have timeout** — both connect and read timeout
- [ ] **Database queries have timeout** — `statement_timeout` or equivalent
- [ ] **Cache operations have timeout** — Redis/Memcached calls bounded
- [ ] **Queue operations have timeout** — publish/consume bounded
- [ ] **File I/O has timeout** — especially for network-mounted filesystems

### Retries

- [ ] **Retries on transient failures** — network errors, 429, 503
- [ ] **Exponential backoff** — delay doubles with each attempt
- [ ] **Jitter added** — prevents thundering herd
- [ ] **Retry cap** — maximum attempts bounded (not infinite)
- [ ] **Non-retryable errors not retried** — 400, 401, 403, 404 not retried

### Circuit Breakers

- [ ] **Circuit breaker on flaky dependencies** — payment, email, external APIs
- [ ] **Failure threshold configured** — opens after N consecutive failures
- [ ] **Recovery timeout configured** — tries again after N seconds
- [ ] **Fallback defined** — what to return when circuit is open

### Rate Limiting

- [ ] **Rate limiting on public endpoints** — prevents abuse
- [ ] **Rate limiting on expensive operations** — prevents self-DoS
- [ ] **Rate limit headers returned** — `X-RateLimit-Remaining`, `Retry-After`
- [ ] **429 response on limit exceeded** — not 500

### Resource Management

- [ ] **Goroutines have context cancellation** — no goroutine leaks
- [ ] **Connections released in finally/defer** — no connection leaks
- [ ] **File handles closed** — no file descriptor leaks
- [ ] **Queues bounded** — no unbounded in-memory queues
- [ ] **Memory limits set** — container/process memory bounded

### Startup & Shutdown

- [ ] **SIGTERM handled** — graceful shutdown initiated
- [ ] **In-flight requests drained** — wait for active requests to complete
- [ ] **Connections closed cleanly** — DB, cache, queue connections closed
- [ ] **Readiness probe distinct from liveness** — not ready until dependencies healthy
- [ ] **Startup timeout** — fails fast if dependencies not available at startup

### Error Handling

- [ ] **Errors not swallowed** — `except: pass` is a finding
- [ ] **Errors logged with context** — file, line, request ID, user ID
- [ ] **Panics recovered** — Go panics recovered and logged
- [ ] **Error types distinguished** — transient vs permanent errors handled differently

---

## Detection Patterns

### Grep Commands

```bash
# HTTP calls without timeout (Python)
grep -rn "requests\.get\|requests\.post\|requests\.put\|requests\.delete\|httpx\." \
  --include="*.py" | grep -v "timeout"

# HTTP calls without timeout (Go)
grep -rn "http\.Get\|http\.Post\|client\.Do\|client\.Get" \
  --include="*.go" | grep -v "Timeout\|context\.WithTimeout\|context\.WithDeadline"

# HTTP calls without timeout (JavaScript)
grep -rn "fetch(\|axios\." --include="*.js" --include="*.ts" \
  | grep -v "timeout\|signal\|AbortController"

# Retry without backoff
grep -rn "for.*range.*retry\|for.*attempt\|while.*retry" \
  --include="*.py" --include="*.go" | grep -v "sleep\|backoff\|wait\|jitter"

# Swallowed errors (Python)
grep -rn "except.*:\s*$" --include="*.py"
grep -rn "except.*:\s*pass" --include="*.py"

# Swallowed errors (Go)
grep -rn "_ = " --include="*.go" | grep "err\|error"

# Goroutine leaks (Go)
grep -rn "go func\|go\s\+\w\+(" --include="*.go" \
  | grep -v "context\|ctx\|Done()\|cancel()"

# Missing SIGTERM handler
grep -rn "signal\.signal\|signal\.Notify\|SIGTERM" \
  --include="*.py" --include="*.go" --include="*.js"
# Absence is the finding

# Unbounded queues
grep -rn "make(chan\|channel.make\|Queue()" \
  --include="*.go" --include="*.py" | grep -v "buffer\|maxsize\|capacity"
```

---

## Anti-Patterns with Fixes

### AP-SRE-001: Missing Timeout on HTTP Call

**Severity:** high

```python
# BAD: No timeout — hangs indefinitely
import requests

def call_payment_service(order_id, amount):
    response = requests.post(
        "https://payment-service/charge",
        json={"order_id": order_id, "amount": amount}
    )
    return response.json()
```

**Fix:**
```python
# GOOD: Explicit connect + read timeout
import requests

def call_payment_service(order_id, amount):
    try:
        response = requests.post(
            "https://payment-service/charge",
            json={"order_id": order_id, "amount": amount},
            timeout=(3.05, 30)  # (connect_timeout, read_timeout)
        )
        response.raise_for_status()
        return response.json()
    except requests.Timeout:
        logger.warning("payment_service_timeout", order_id=order_id)
        raise ServiceUnavailableError("Payment service timed out")
    except requests.HTTPError as e:
        logger.error("payment_service_error", order_id=order_id, status=e.response.status_code)
        raise
```

**Timeout guidelines:**

| Call type | Connect | Read | Notes |
|---|---|---|---|
| Same datacenter | 1s | 5s | Fast network |
| External API | 3s | 30s | Variable latency |
| Database | — | 5s | `statement_timeout` |
| Redis/cache | 0.5s | 1s | Should be fast |
| File I/O | — | 10s | Network mounts |

---

### AP-SRE-002: Retry Without Exponential Backoff

**Severity:** medium

```python
# BAD: Immediate retries — thundering herd
for attempt in range(3):
    try:
        return call_service()
    except Exception:
        if attempt == 2:
            raise
        # Immediate retry! All clients retry simultaneously
```

**Fix (using `tenacity`):**
```python
# GOOD: Exponential backoff with jitter
from tenacity import (
    retry, stop_after_attempt, wait_exponential, wait_random,
    retry_if_exception_type, before_sleep_log
)
import logging

logger = logging.getLogger(__name__)

@retry(
    stop=stop_after_attempt(3),
    wait=wait_exponential(multiplier=1, min=1, max=10) + wait_random(0, 1),
    retry=retry_if_exception_type((requests.Timeout, requests.ConnectionError)),
    before_sleep=before_sleep_log(logger, logging.WARNING)
)
def call_service():
    response = requests.get("https://service/api", timeout=10)
    response.raise_for_status()
    return response.json()
```

**Fix (manual implementation):**
```python
import random
import time

def call_with_retry(url, max_retries=3, base_delay=1.0):
    last_exception = None
    for attempt in range(max_retries):
        try:
            response = requests.get(url, timeout=10)
            response.raise_for_status()
            return response.json()
        except (requests.Timeout, requests.ConnectionError) as e:
            last_exception = e
            if attempt < max_retries - 1:
                # Exponential backoff with jitter
                delay = base_delay * (2 ** attempt) + random.uniform(0, 1)
                logger.warning(f"Attempt {attempt + 1} failed, retrying in {delay:.2f}s")
                time.sleep(delay)
    raise last_exception
```

---

### AP-SRE-003: Swallowed Errors

**Severity:** medium–high

```python
# BAD: Error silently ignored
def process_payment(order_id):
    try:
        result = payment_service.charge(order_id)
        return result
    except Exception:
        pass  # Error swallowed! Order never charged, no alert, no log
```

**Fix:**
```python
# GOOD: Log error with context, re-raise or return error state
def process_payment(order_id):
    try:
        result = payment_service.charge(order_id)
        return result
    except PaymentDeclinedError as e:
        # Expected error — log at INFO, return error state
        logger.info("payment_declined", order_id=order_id, reason=str(e))
        return PaymentResult(success=False, error="payment_declined")
    except PaymentServiceError as e:
        # Unexpected error — log at ERROR, re-raise
        logger.error("payment_service_error", order_id=order_id, error=str(e))
        raise
    except Exception as e:
        # Unknown error — log at CRITICAL, re-raise
        logger.critical("unexpected_payment_error", order_id=order_id, error=str(e))
        raise
```

---

### AP-SRE-004: Goroutine Leak (Go)

**Severity:** high

```go
// BAD: Goroutine blocks forever if nobody reads resultChan
func processItems(items []Item) []Result {
    resultChan := make(chan Result)
    
    for _, item := range items {
        go func(i Item) {
            result := processItem(i)
            resultChan <- result  // Blocks if nobody reads!
        }(item)
    }
    
    results := make([]Result, len(items))
    for i := range results {
        results[i] = <-resultChan
    }
    return results
}
```

**Fix:**
```go
// GOOD: Context-based cancellation prevents goroutine leak
func processItems(ctx context.Context, items []Item) ([]Result, error) {
    resultChan := make(chan Result, len(items))  // Buffered channel
    
    var wg sync.WaitGroup
    for _, item := range items {
        wg.Add(1)
        go func(i Item) {
            defer wg.Done()
            
            select {
            case <-ctx.Done():
                return  // Exit if context cancelled
            default:
                result := processItem(i)
                select {
                case resultChan <- result:
                case <-ctx.Done():
                    return
                }
            }
        }(item)
    }
    
    // Close channel when all goroutines done
    go func() {
        wg.Wait()
        close(resultChan)
    }()
    
    results := make([]Result, 0, len(items))
    for result := range resultChan {
        results = append(results, result)
    }
    return results, ctx.Err()
}
```

**Detecting goroutine leaks:**
```go
// Use goleak in tests
import "go.uber.org/goleak"

func TestProcessItems(t *testing.T) {
    defer goleak.VerifyNone(t)  // Fails if goroutines leaked
    
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    results, err := processItems(ctx, testItems)
    // ...
}
```

---

### AP-SRE-005: Missing Graceful Shutdown

**Severity:** medium

```python
# BAD: No SIGTERM handling — in-flight requests killed abruptly
from flask import Flask
app = Flask(__name__)

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080)
```

**Fix (Python/Flask with Gunicorn):**
```python
# GOOD: Gunicorn handles graceful shutdown via SIGTERM
# gunicorn.conf.py
timeout = 30          # Worker timeout
graceful_timeout = 30  # Time to finish in-flight requests on SIGTERM
worker_class = "gthread"
threads = 4
```

**Fix (Python with custom SIGTERM handler):**
```python
import signal
import sys
import threading

shutdown_event = threading.Event()

def handle_sigterm(signum, frame):
    logger.info("sigterm_received", message="Initiating graceful shutdown")
    shutdown_event.set()

signal.signal(signal.SIGTERM, handle_sigterm)
signal.signal(signal.SIGINT, handle_sigterm)

# In request handler
def process_request():
    if shutdown_event.is_set():
        return {"error": "Service shutting down"}, 503
    # ... handle request
```

**Fix (Go with graceful shutdown):**
```go
func main() {
    server := &http.Server{Addr: ":8080", Handler: router}
    
    // Start server in goroutine
    go func() {
        if err := server.ListenAndServe(); err != http.ErrServerClosed {
            log.Fatalf("Server error: %v", err)
        }
    }()
    
    // Wait for interrupt signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
    <-quit
    
    log.Println("Shutting down server...")
    
    // Give in-flight requests 30 seconds to complete
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := server.Shutdown(ctx); err != nil {
        log.Fatalf("Server forced to shutdown: %v", err)
    }
    
    log.Println("Server exited cleanly")
}
```

---

### AP-SRE-006: Rate Limiting Missing on Public Endpoint

**Severity:** medium–high

```python
# BAD: No rate limiting on expensive endpoint
@app.route("/api/search")
def search():
    query = request.args.get("q")
    results = full_text_search(query)  # Expensive operation!
    return jsonify(results)
# Anyone can hammer this endpoint
```

**Fix (using `flask-limiter`):**
```python
from flask_limiter import Limiter
from flask_limiter.util import get_remote_address

limiter = Limiter(
    app=app,
    key_func=get_remote_address,
    default_limits=["200 per day", "50 per hour"]
)

@app.route("/api/search")
@limiter.limit("10 per minute")  # Stricter limit for expensive endpoint
def search():
    query = request.args.get("q")
    results = full_text_search(query)
    return jsonify(results)
```

**Fix (Redis-based token bucket):**
```python
import redis
import time

r = redis.Redis()

def is_rate_limited(user_id: str, limit: int = 10, window: int = 60) -> bool:
    """Sliding window rate limiter using Redis."""
    key = f"ratelimit:{user_id}"
    now = time.time()
    window_start = now - window
    
    pipe = r.pipeline()
    pipe.zremrangebyscore(key, 0, window_start)  # Remove old entries
    pipe.zadd(key, {str(now): now})              # Add current request
    pipe.zcard(key)                               # Count requests in window
    pipe.expire(key, window)                      # Set TTL
    results = pipe.execute()
    
    request_count = results[2]
    return request_count > limit
```

---

## Finding Templates

### HARDEN-SRE-NO-TIMEOUT

```yaml
id: HARDEN-SRE-NO-TIMEOUT
severity: high
category: sre
location: "path/to/service.py:42"
description: "HTTP call to payment-service has no timeout configured."
impact: >
  If payment-service is slow or unresponsive, this call hangs indefinitely.
  Under load, all threads are consumed, causing a cascading failure.
  Blast radius: all checkout flows.
recommendation: >
  Add timeout=(3.05, 30) to requests.post() call.
  Add fallback: queue payment for retry on timeout.
acceptance_criteria:
  - "Block payment-service; verify timeout fires within 30s; verify fallback response returned"
  - "Request duration metric shows p99 < 31s (bounded by timeout)"
  - "Timeout triggers structured log event with order_id and timeout_seconds fields"
verification_tier: 3
confidence: confirmed
assumptions: "payment-service is a synchronous dependency"
requires_human_review: false
```

### HARDEN-SRE-SWALLOWED-ERROR

```yaml
id: HARDEN-SRE-SWALLOWED-ERROR
severity: high
category: sre
location: "path/to/processor.py:156"
description: "Exception in process_payment() is silently swallowed."
impact: >
  Payment failures are invisible. No alerting, no logging, no retry.
  Orders appear to succeed but are never charged. Revenue loss and
  customer confusion.
recommendation: >
  Replace bare except: pass with:
  - Log error at ERROR level with order_id and exception details
  - Re-raise or return explicit error state
  - Add alert on payment_error metric
acceptance_criteria:
  - "Inject payment service failure; verify error logged with order_id, error_type, error_message"
  - "Inject payment service failure; verify caller receives error response (not silent success)"
  - "payment_error metric incremented on failure, visible in Prometheus"
verification_tier: 3
confidence: confirmed
assumptions: "Caller expects error to propagate"
requires_human_review: false
```

---

## Recommended Libraries

| Language | Retry | Circuit Breaker | Rate Limiting |
|---|---|---|---|
| Python | `tenacity` | `circuitbreaker`, `pybreaker` | `flask-limiter`, `slowapi` |
| Go | `retry-go`, `backoff` | `gobreaker` (Sony) | `golang.org/x/time/rate` |
| Node.js | `async-retry`, `p-retry` | `opossum` | `express-rate-limit` |
| Java | `Resilience4j` | `Resilience4j` | `Resilience4j` |

### If Your Repo Uses Different Libraries

**The patterns are library-agnostic.** Retry, circuit breaker, and timeout are behavioral concepts — not library requirements. Any implementation that achieves the same observable behavior satisfies the acceptance criteria. You do not need to add a new dependency to fix a hardening finding.

**Retry without a library (manual implementation):**

```python
# Manual retry with exponential backoff — no library needed
import time, random
def with_retry(fn, max_attempts=3, base_delay=1.0):
    for attempt in range(max_attempts):
        try:
            return fn()
        except Exception as e:
            if attempt == max_attempts - 1:
                raise
            delay = base_delay * (2 ** attempt) + random.uniform(0, 1)
            time.sleep(delay)
```

**Circuit breaker without a new library:**

If your repo already has a circuit breaker implementation, use it — the acceptance criteria only require the behavior, not a specific library:

- **Java**: Resilience4j is the standard replacement for Hystrix. If you already have Resilience4j, use it.
- **.NET**: Polly is the standard circuit breaker library.
- **Ruby**: `circuitbox` gem is the common choice.
- **Any language**: If you have an existing internal circuit breaker utility, use that instead of adding a new dependency.

**Acceptance criteria are behavior-based, not library-based.** A finding that says "circuit breaker opens after 5 failures" is satisfied by any implementation — library or manual — that opens after 5 failures. The verifier tests the behavior, not the import statement.

**When no library is available:** Implement the pattern manually (see retry example above). A manual implementation is fully acceptable as long as it satisfies the acceptance criteria. Prefer a manual implementation over adding a new dependency if the dependency would require significant approval overhead.

---

## Acceptance Criteria Templates (Tier 3+)

1. **Timeout**: Block downstream → verify timeout fires within N seconds → verify fallback returned
2. **Retry backoff**: Inject failures → verify retry delays increase exponentially → verify jitter present
3. **Circuit breaker**: Inject 5+ failures → verify circuit opens → verify fail-fast (< 10ms) → verify recovery
4. **Rate limiting**: Send N+1 requests → verify 429 returned → verify `Retry-After` header present
5. **Graceful shutdown**: Send SIGTERM during request → verify request completes → verify clean exit log

---

axiom:trace work_item=hardening-skills-01 spec=hardening-sre-axiom jira_ref=SWDE-7 plan=phase-1/task-5/step-1
