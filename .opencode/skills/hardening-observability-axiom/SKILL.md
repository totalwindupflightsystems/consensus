---
name: hardening-observability-axiom
description: >
  Observability hardening for any codebase. Covers structured logging gaps, missing
  correlation IDs, SLI/SLO definition, OpenTelemetry instrumentation, Prometheus metric
  naming and cardinality, PII in logs, and the "3am debugging" checklist. Produces
  HARDEN-OBS-* findings with Tier-3+ verifiable acceptance criteria.
version: "1.0"
tags:
  vertical: [observability, coding]
  category: hardening
  core: false
metadata:
  related_skills:
    - hardening-anti-patterns-axiom
    - hardening-intake-axiom
    - sre-ops-axiom
    - metrics-instrumentation-axiom
    - distributed-tracing-axiom
    - alert-engineering-axiom
---

# Hardening: Observability

> **"Per the operating doctrine, observability is mandatory: events must correlate by ticket id, repo id, PR id, and run id. Any finding where these correlations are missing is at minimum 'high.'"**
>
> **"What symptoms would an on-call engineer see if this service broke at 3am, and would they have enough information to debug without reading source code?"**

This skill audits a codebase's observability posture across three pillars: logging, metrics, and tracing. It is portable — no Axiom-internal dependencies.

## When to Load This Skill

- Quarterly observability audit
- Before a major launch (can you debug it in production?)
- After a production incident where debugging was hard
- When setting up a new service
- As part of a hardening battery

---

## The Observability Audit Prompt

Use this prompt (with the shared header from `hardening-anti-patterns-axiom`):

```
Review this codebase's observability posture across three pillars:

Logging:
- Critical paths with no structured logging (auth, writes, external calls)
- Unstructured log messages that can't be queried
- Sensitive data (PII, secrets, tokens) logged in cleartext
- Missing correlation IDs across service boundaries

Metrics:
- User-facing features with no SLI (latency, error rate, throughput)
- Resource metrics missing (queue depth, connection pool, cache hit rate)
- Metrics with no alerts, or alerts with no runbooks

Tracing:
- Request flows that can't be traced end-to-end
- External calls not instrumented
- Missing context propagation (request ID, user ID, tenant ID)

Per the operating doctrine, observability is mandatory:
events must correlate by ticket id, repo id, PR id, and run id.
Any finding where these correlations are missing is at minimum "high."

Also flag: what symptoms would an on-call engineer see if this
service broke at 3am, and would they have enough information to
debug without reading source code?
```

---

## Observability Audit Checklist

### Logging

- [ ] **Structured logging used** — JSON format, not string concatenation
- [ ] **Correlation ID propagated** — request ID in every log line
- [ ] **Critical paths logged** — auth, writes, external calls, errors
- [ ] **Log levels appropriate** — DEBUG/INFO/WARNING/ERROR/CRITICAL used correctly
- [ ] **No PII in logs** — email, phone, SSN, credit card not logged in plaintext
- [ ] **No secrets in logs** — tokens, passwords, API keys not logged
- [ ] **Error context included** — exception type, message, stack trace
- [ ] **Slow operations logged** — queries > 1s, requests > 5s

### Metrics

- [ ] **SLIs defined** — latency (p99), error rate, availability
- [ ] **SLOs defined** — target values for each SLI
- [ ] **Request metrics** — `http_requests_total`, `http_request_duration_seconds`
- [ ] **Error metrics** — `errors_total` by error type
- [ ] **Resource metrics** — connection pool size, queue depth, cache hit rate
- [ ] **Business metrics** — orders processed, payments charged, users created
- [ ] **Low-cardinality labels** — no user IDs, request IDs in metric labels
- [ ] **Alerts defined** — for each SLI that can breach SLO

### Tracing

- [ ] **Distributed tracing enabled** — OpenTelemetry or equivalent
- [ ] **External calls instrumented** — HTTP, DB, cache, queue calls have spans
- [ ] **Context propagated** — trace ID passed in HTTP headers
- [ ] **Span attributes set** — user ID, order ID, relevant business context
- [ ] **Error spans marked** — failed spans have `error=true` attribute

### Alerting

- [ ] **Alerts on SLO breach** — not just on individual metrics
- [ ] **Every alert has a runbook** — `runbook_url` in alert definition
- [ ] **Alert severity defined** — P1/P2/P3 or equivalent
- [ ] **Alert routing defined** — who gets paged for what

---

## Detection Patterns

### Grep Commands

```bash
# Unstructured logging (f-strings in log calls)
grep -rn "logger\.info.*f\"\|logger\.error.*f\"\|logger\.warning.*f\"\|print(" \
  --include="*.py" | grep -v "structlog\|extra=\|exc_info"

# Missing correlation ID in log calls
grep -rn "logger\.\(info\|error\|warning\|debug\)(" --include="*.py" \
  | grep -v "correlation_id\|request_id\|trace_id\|span_id"

# PII in logs
grep -rn \
  -e "logger.*email\|log.*email" \
  -e "logger.*password\|log.*password" \
  -e "logger.*credit_card\|log.*card" \
  -e "logger.*ssn\|log.*social_security" \
  -e "logger.*phone\|log.*phone_number" \
  --include="*.py"

# High-cardinality metric labels
grep -rn "Counter\|Histogram\|Gauge\|Summary" --include="*.py" -A 5 \
  | grep "user_id\|request_id\|session_id\|order_id\|customer_id"

# Missing metric instrumentation on HTTP handlers
grep -rn "@app\.route\|@router\." --include="*.py" -A 20 \
  | grep -v "Counter\|Histogram\|metrics\.\|prometheus"

# Alerts without runbook
grep -rn "alert:\|alerting:" --include="*.yaml" --include="*.yml" -A 15 \
  | grep -v "runbook_url\|runbook"

# External calls without tracing
grep -rn "requests\.get\|requests\.post\|httpx\." --include="*.py" \
  | grep -v "tracer\|span\|trace\|opentelemetry"
```

---

## Anti-Patterns with Fixes

### AP-OBS-001: Unstructured Log Messages

**Severity:** medium

```python
# BAD: Unstructured — can't query by field
import logging
logger = logging.getLogger(__name__)

def process_order(order_id, user_id):
    logger.info(f"Processing order {order_id} for user {user_id}")
    # ...
    logger.error(f"Order {order_id} failed: payment declined")
```

**Fix (using `structlog`):**
```python
# GOOD: Structured JSON logging
import structlog

logger = structlog.get_logger()

def process_order(order_id, user_id):
    log = logger.bind(order_id=order_id, user_id=user_id)
    log.info("order_processing_started")
    
    try:
        result = charge_payment(order_id)
        log.info("order_completed", payment_id=result.payment_id)
        return result
    except PaymentDeclinedError as e:
        log.warning("order_payment_declined", reason=str(e))
        raise
    except Exception as e:
        log.error("order_processing_failed", error=str(e), exc_info=True)
        raise

# Output:
# {"event": "order_processing_started", "order_id": 123, "user_id": 456, "timestamp": "..."}
# {"event": "order_payment_declined", "order_id": 123, "user_id": 456, "reason": "insufficient_funds"}
```

**Fix (using Python `logging` with JSON formatter):**
```python
import logging
import json
from datetime import datetime

class JSONFormatter(logging.Formatter):
    def format(self, record):
        log_data = {
            "timestamp": datetime.utcnow().isoformat(),
            "level": record.levelname,
            "event": record.getMessage(),
            "logger": record.name,
        }
        # Add extra fields
        if hasattr(record, "order_id"):
            log_data["order_id"] = record.order_id
        if hasattr(record, "user_id"):
            log_data["user_id"] = record.user_id
        if record.exc_info:
            log_data["exception"] = self.formatException(record.exc_info)
        return json.dumps(log_data)

handler = logging.StreamHandler()
handler.setFormatter(JSONFormatter())
logging.getLogger().addHandler(handler)
```

---

### AP-OBS-002: Missing Correlation ID

**Severity:** high

```python
# BAD: No correlation ID — can't trace request across services
@app.route("/api/orders", methods=["POST"])
def create_order():
    logger.info("Creating order")
    order = Order.create(request.json)
    notify_warehouse(order.id)  # No correlation ID passed!
    logger.info("Order created")
    return jsonify(order.to_dict())
```

**Fix:**
```python
# GOOD: Correlation ID propagated across all services
import uuid
from contextvars import ContextVar
from functools import wraps

correlation_id_var: ContextVar[str] = ContextVar("correlation_id", default="")

def get_correlation_id() -> str:
    return correlation_id_var.get() or str(uuid.uuid4())

@app.before_request
def set_correlation_id():
    # Accept from upstream or generate new
    correlation_id = (
        request.headers.get("X-Correlation-ID") or
        request.headers.get("X-Request-ID") or
        str(uuid.uuid4())
    )
    correlation_id_var.set(correlation_id)
    g.correlation_id = correlation_id

@app.after_request
def add_correlation_header(response):
    response.headers["X-Correlation-ID"] = get_correlation_id()
    return response

@app.route("/api/orders", methods=["POST"])
def create_order():
    correlation_id = get_correlation_id()
    logger.info("order_creation_started", correlation_id=correlation_id)
    
    order = Order.create(request.json)
    
    # Propagate to downstream service
    notify_warehouse(
        order.id,
        headers={"X-Correlation-ID": correlation_id}
    )
    
    logger.info("order_created", 
                correlation_id=correlation_id,
                order_id=order.id)
    return jsonify(order.to_dict())
```

---

### AP-OBS-003: High-Cardinality Metric Labels

**Severity:** high

```python
# BAD: User ID in metric label — millions of time series!
from prometheus_client import Counter, Histogram

http_requests = Counter(
    "http_requests_total",
    "Total HTTP requests",
    ["user_id", "endpoint", "status"]  # user_id = millions of values!
)

# This creates a new time series for EVERY user
# With 1M users: 1M × endpoints × statuses = billions of time series
# Prometheus will crash or become unusably slow
```

**Fix:**
```python
# GOOD: Low-cardinality labels only
from prometheus_client import Counter, Histogram

http_requests = Counter(
    "http_requests_total",
    "Total HTTP requests",
    ["method", "endpoint", "status_code"]  # Low cardinality: ~100 combinations
)

http_request_duration = Histogram(
    "http_request_duration_seconds",
    "HTTP request duration",
    ["method", "endpoint"],
    buckets=[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]
)

# User ID goes in logs and traces, NOT metrics
@app.route("/api/orders", methods=["POST"])
def create_order():
    with http_request_duration.labels(method="POST", endpoint="/api/orders").time():
        result = process_order(request.json)
        
    http_requests.labels(
        method="POST",
        endpoint="/api/orders",
        status_code="200"
    ).inc()
    
    # User ID in structured log (queryable, not a metric label)
    logger.info("order_created",
                user_id=current_user.id,  # In log, not metric
                order_id=result.id)
    
    return jsonify(result.to_dict())
```

**Cardinality guidelines:**
| Label | Max unique values | OK? |
|---|---|---|
| `method` | ~10 (GET, POST, PUT, DELETE...) | ✅ |
| `endpoint` | ~100 (your API routes) | ✅ |
| `status_code` | ~20 (200, 201, 400, 401, 403, 404, 500...) | ✅ |
| `user_id` | Millions | ❌ |
| `request_id` | Infinite | ❌ |
| `order_id` | Millions | ❌ |

---

### AP-OBS-004: Alert Without Runbook

**Severity:** medium

```yaml
# BAD: Alert with no runbook — on-call engineer has no guidance
groups:
  - name: api_alerts
    rules:
      - alert: HighErrorRate
        expr: error_rate > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High error rate detected"
          # No runbook_url!
```

**Fix:**
```yaml
# GOOD: Alert with runbook link
groups:
  - name: api_alerts
    rules:
      - alert: HighErrorRate
        expr: |
          sum(rate(http_requests_total{status_code=~"5.."}[5m]))
          / sum(rate(http_requests_total[5m])) > 0.05
        for: 5m
        labels:
          severity: critical
          team: backend
        annotations:
          summary: "High error rate: {{ $value | humanizePercentage }}"
          description: "Error rate has exceeded 5% for 5 minutes"
          runbook_url: "https://wiki.internal/runbooks/high-error-rate"
          dashboard_url: "https://grafana.internal/d/api-overview"
```

**Runbook template:**
```markdown
# Runbook: High Error Rate

## Symptoms
- Alert: HighErrorRate fires
- Users report errors on [feature]

## Triage (< 5 minutes)
1. Check dashboard: https://grafana.internal/d/api-overview
2. Check recent deployments: `kubectl rollout history deployment/api`
3. Check error logs: `kubectl logs -l app=api | grep ERROR | tail -50`

## Mitigation
- If caused by recent deploy: `kubectl rollout undo deployment/api`
- If caused by downstream service: enable feature flag `DISABLE_[FEATURE]`
- If caused by DB: check connection pool metrics

## Escalation
- If not resolved in 15 minutes: page @backend-oncall
- If data loss suspected: page @incident-commander
```

---

### AP-OBS-005: Missing SLI/SLO Definition

**Severity:** high

```python
# BAD: No metrics, no SLIs, no SLOs
@app.route("/api/checkout", methods=["POST"])
def checkout():
    result = process_checkout(request.json)
    return jsonify(result)
# If this breaks at 3am, on-call has no metrics to look at
```

**Fix:**
```python
# GOOD: Instrumented with SLI metrics
from prometheus_client import Counter, Histogram
import time

checkout_requests = Counter(
    "checkout_requests_total",
    "Total checkout requests",
    ["status"]  # success, payment_declined, error
)

checkout_duration = Histogram(
    "checkout_duration_seconds",
    "Checkout request duration",
    buckets=[0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]
)

@app.route("/api/checkout", methods=["POST"])
def checkout():
    start = time.time()
    try:
        result = process_checkout(request.json)
        checkout_requests.labels(status="success").inc()
        return jsonify(result)
    except PaymentDeclinedError:
        checkout_requests.labels(status="payment_declined").inc()
        return {"error": "payment_declined"}, 402
    except Exception as e:
        checkout_requests.labels(status="error").inc()
        logger.error("checkout_error", error=str(e))
        return {"error": "internal_error"}, 500
    finally:
        checkout_duration.observe(time.time() - start)
```

**SLO definition:**
```yaml
# slo.yaml
slos:
  checkout_availability:
    description: "Checkout endpoint availability"
    sli:
      query: |
        sum(rate(checkout_requests_total{status="success"}[5m]))
        / sum(rate(checkout_requests_total[5m]))
    target: 0.999  # 99.9% success rate
    window: 30d
    
  checkout_latency:
    description: "Checkout p99 latency"
    sli:
      query: |
        histogram_quantile(0.99, 
          sum(rate(checkout_duration_seconds_bucket[5m])) by (le))
    target: 2.0  # p99 < 2 seconds
    window: 5m
```

---

## The "3am Debugging" Checklist

When an on-call engineer is paged at 3am, they need to answer these questions **without reading source code**:

### What is failing?
- [ ] Error rate metric shows which endpoints are failing
- [ ] Error logs show what errors are occurring
- [ ] Traces show where in the request flow failures happen

### When did it start?
- [ ] Metrics have timestamps
- [ ] Deployment events visible on dashboards
- [ ] Correlation between deploy time and error spike

### What changed?
- [ ] Recent deployments visible (Kubernetes events, deployment annotations)
- [ ] Config changes logged
- [ ] Feature flag changes logged

### How bad is it?
- [ ] Error rate metric shows % of requests failing
- [ ] User impact metric shows how many users affected
- [ ] SLO burn rate shows how fast error budget is depleting

### How do I fix it?
- [ ] Every alert has a runbook URL
- [ ] Runbook has rollback procedure
- [ ] Runbook has escalation path

---

## OpenTelemetry Quick Start

```python
# Install: pip install opentelemetry-sdk opentelemetry-exporter-otlp

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

# Setup (once at startup)
provider = TracerProvider()
exporter = OTLPSpanExporter(endpoint="http://otel-collector:4317")
provider.add_span_processor(BatchSpanProcessor(exporter))
trace.set_tracer_provider(provider)

tracer = trace.get_tracer(__name__)

# Use in code
def process_order(order_id):
    with tracer.start_as_current_span("process_order") as span:
        span.set_attribute("order.id", order_id)
        span.set_attribute("user.id", get_current_user_id())
        
        try:
            result = charge_payment(order_id)
            span.set_attribute("payment.id", result.payment_id)
            return result
        except Exception as e:
            span.record_exception(e)
            span.set_status(trace.StatusCode.ERROR, str(e))
            raise
```

---

## Finding Templates

### HARDEN-OBS-NO-CORRELATION-ID

```yaml
id: HARDEN-OBS-NO-CORRELATION-ID
severity: high
category: observability
location: "src/api/orders.py:42"
description: "Order creation endpoint does not propagate correlation ID to downstream services."
impact: >
  When an order fails, on-call cannot trace the request across services.
  Debugging requires reading source code and guessing which log lines belong
  to the same request. MTTR increases significantly.
recommendation: >
  1. Add X-Correlation-ID header to all outbound HTTP calls
  2. Include correlation_id in all log events
  3. Return X-Correlation-ID in response headers
acceptance_criteria:
  - "Create order via API; verify X-Correlation-ID in response header"
  - "Check warehouse service logs; verify same correlation_id appears"
  - "All log events for the request include correlation_id field"
verification_tier: 3
confidence: confirmed
assumptions: "Downstream services accept and log X-Correlation-ID header"
requires_human_review: false
```

### HARDEN-OBS-HIGH-CARDINALITY-LABELS

```yaml
id: HARDEN-OBS-HIGH-CARDINALITY-LABELS
severity: high
category: observability
location: "src/metrics.py:15"
description: "http_requests_total metric uses user_id as a label — high cardinality."
impact: >
  With 1M users, this creates 1M+ time series. Prometheus memory usage grows
  unboundedly. At scale, Prometheus becomes slow or crashes, taking down all
  alerting and dashboards.
recommendation: >
  Remove user_id from metric labels. Use method, endpoint, status_code instead.
  Log user_id in structured logs for per-user debugging.
acceptance_criteria:
  - "http_requests_total metric has no user_id label"
  - "Prometheus cardinality check: SELECT count(*) FROM http_requests_total shows < 1000 series"
  - "user_id appears in structured log events, not metric labels"
verification_tier: 4
confidence: confirmed
assumptions: "Prometheus is the metrics backend"
requires_human_review: false
```

---

## Acceptance Criteria Templates (Tier 3+)

1. **Structured logging**: Trigger event → grep logs → verify JSON format with required fields
2. **Correlation ID**: Make request → check response header → check downstream logs → verify same ID
3. **Metric cardinality**: Query Prometheus → verify series count < threshold
4. **Alert has runbook**: Check alert definition → verify `runbook_url` present → verify URL resolves
5. **SLO defined**: Query Prometheus → verify SLI metric exists → verify alert fires when SLO breached

---

axiom:trace work_item=hardening-skills-01 spec=hardening-observability-axiom jira_ref=SWDE-7 plan=phase-1/task-7/step-1
