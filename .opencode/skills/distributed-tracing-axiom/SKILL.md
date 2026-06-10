---
name: distributed-tracing-axiom
description: >
  Distributed tracing fundamentals, OpenTelemetry SDK setup, span design and naming,
  context propagation for HTTP/gRPC/message-queue/async surfaces, sampling strategies,
  backend options, Axiom span hierarchy, and anti-patterns. Load this skill when
  designing trace instrumentation, reviewing span coverage, choosing sampling strategies,
  or debugging context propagation for any service managed by Axiom.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-03-20"
  primary_spec: specs/34-Observability-And-Metrics.md
  secondary_specs:
    - specs/25-Structured-Logging-Events.md
    - specs/31-OpenCode-Integration-Contract.md
    - specs/61-Logging-And-Observability-Overhaul.md
    - specs/65-Diagnostic-Workflows.md
tags:
  vertical: [sre]
  category: tracing
  core: false
---

# Distributed Tracing Skill (Portable)

> **"A trace is a story of a request told by the systems it touched."**
>
> **"If you can't see the path a request took, you can't explain why it was slow."**

This skill provides portable, production-grade guidance for designing, implementing, and
operating distributed tracing in services managed by Axiom. It covers tracing fundamentals,
OpenTelemetry SDK setup, span design and naming, context propagation across transport
boundaries, sampling strategies, backend selection, the Axiom-specific span hierarchy,
and common anti-patterns.

**Spec grounding**: The Axiom trace span hierarchy, W3C Trace Context propagation
requirement, and `correlation_id` backward-compatibility contract are defined in
`specs/34-Observability-And-Metrics.md#traces-contract`. This skill encodes those
requirements as reusable tracing patterns applicable to any OpenTelemetry-compatible stack.

---

## Activation

Load this skill when:
- Designing trace instrumentation for a new service or component
- Reviewing span coverage and naming conventions
- Choosing or tuning a sampling strategy
- Debugging broken context propagation across service boundaries
- Setting up an OpenTelemetry SDK (Python, TypeScript, Go, Java, Rust)
- Evaluating tracing backends (Jaeger, Tempo, Zipkin, Datadog APM, AWS X-Ray)
- Auditing trace-to-log and trace-to-metric correlation
- Migrating from correlation-ID-based tracing to OpenTelemetry spans

---

## 1. Tracing Fundamentals

### What Is a Distributed Trace?

A **trace** represents the end-to-end journey of a single request (or workflow) through a
distributed system. It is composed of **spans** -- individual units of work with a start time,
duration, status, and parent relationship.

Key concepts:

| Concept | Definition |
|---|---|
| **Trace** | A directed acyclic graph (DAG) of spans sharing a single `trace_id` |
| **Span** | One unit of work: a function call, HTTP request, DB query, queue publish/consume |
| **Root span** | The first span in a trace; has no parent |
| **Child span** | A span whose `parent_span_id` points to another span in the same trace |
| **Span context** | The tuple `(trace_id, span_id, trace_flags, trace_state)` that propagates across boundaries |
| **Baggage** | Key-value pairs propagated alongside span context for cross-cutting concerns |

### The Three Pillars and Tracing's Role

Tracing is one of the three observability pillars alongside **logs** and **metrics**:

| Pillar | Answers | Granularity |
|---|---|---|
| **Logs** | What happened at a point in time? | Per-event |
| **Metrics** | How is the system performing in aggregate? | Per-time-window |
| **Traces** | What path did this specific request take, and where was time spent? | Per-request |

Traces bridge logs and metrics: they provide the **request-scoped context** that lets you
correlate a spike in p99 latency (metric) with the specific slow database query (span) and
the error log that explains why.

### Trace ID and Span ID Formats

Per W3C Trace Context:
- `trace_id`: 32 hex characters (128-bit), e.g., `4bf92f3577b34da6a3ce929d0e0e4736`
- `span_id`: 16 hex characters (64-bit), e.g., `00f067aa0ba902b7`
- `trace_flags`: 2 hex characters; `01` = sampled, `00` = not sampled

---

## 2. OpenTelemetry SDK Setup

### Why OpenTelemetry?

OpenTelemetry (OTel) is the CNCF standard for telemetry collection. It provides:
- Vendor-neutral APIs and SDKs for traces, metrics, and logs
- Automatic instrumentation for common libraries (HTTP clients, DB drivers, frameworks)
- W3C Trace Context propagation out of the box
- Exporters for every major backend (OTLP, Jaeger, Zipkin, vendor-specific)

### Python SDK Setup (Axiom Primary Runtime)

```python
# Install: pip install opentelemetry-api opentelemetry-sdk opentelemetry-exporter-otlp
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource

# 1. Define resource attributes (service identity)
resource = Resource.create({
    "service.name": "axiom-repo-runner",
    "service.version": "1.0.0",
    "deployment.environment": "production",
})

# 2. Create and register the tracer provider
provider = TracerProvider(resource=resource)
processor = BatchSpanProcessor(OTLPSpanExporter(
    endpoint="http://otel-collector:4317",  # gRPC endpoint
    insecure=True,
))
provider.add_span_processor(processor)
trace.set_tracer_provider(provider)

# 3. Get a tracer (one per module/component)
tracer = trace.get_tracer("axiom.repo_runner", "1.0.0")
```

### TypeScript SDK Setup (OpenCode Plugin)

```typescript
// Install: npm install @opentelemetry/api @opentelemetry/sdk-trace-node
//          @opentelemetry/exporter-trace-otlp-grpc @opentelemetry/resources
import { NodeTracerProvider } from '@opentelemetry/sdk-trace-node';
import { BatchSpanProcessor } from '@opentelemetry/sdk-trace-base';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-grpc';
import { Resource } from '@opentelemetry/resources';
import { ATTR_SERVICE_NAME } from '@opentelemetry/semantic-conventions';

const provider = new NodeTracerProvider({
  resource: new Resource({
    [ATTR_SERVICE_NAME]: 'axiom-opencode-plugin',
  }),
});

provider.addSpanProcessor(new BatchSpanProcessor(
  new OTLPTraceExporter({ url: 'http://otel-collector:4317' })
));

provider.register();

const tracer = provider.getTracer('axiom.plugin', '1.0.0');
```

### Configuration Contract (Axiom)

Per `specs/34-Observability-And-Metrics.md#configuration`:

```yaml
observability:
  traces_enabled: false           # Post-v1: enable OpenTelemetry trace export
  traces_exporter: "otlp"         # "otlp" | "jaeger" | "zipkin"
  traces_endpoint: null            # Exporter endpoint URL (required when enabled)
```

**v1 behavior**: `traces_enabled: false`. Structured logging with `correlation_id` provides
request-level tracing via log aggregation. No trace SDK is loaded.

**Post-v1 behavior**: When `traces_enabled: true`, the SDK initializes with the configured
exporter and endpoint. The `correlation_id` field from v1 MUST be preserved as a span
attribute for backward compatibility with log-based queries.

---

## 3. Span Design and Naming

### Naming Conventions

Good span names are **low-cardinality**, **descriptive**, and **consistent**:

| Pattern | Good | Bad |
|---|---|---|
| HTTP server | `GET /api/v1/runs` | `GET /api/v1/runs/abc-123` (high cardinality) |
| HTTP client | `HTTP GET otel-collector` | `fetch` (too generic) |
| Database | `SELECT axiom.work_items` | `db.query` (no table context) |
| Queue publish | `publish axiom.steps` | `send message` (no topic) |
| Queue consume | `consume axiom.steps` | `process` (no topic) |
| Internal function | `axiom.step.execute` | `doWork` (meaningless) |

Rules:
1. **Use dot-separated namespaces**: `axiom.run`, `axiom.step.execute`, `axiom.opencode_request`
2. **Keep cardinality bounded**: Parameterize IDs as span attributes, not in the span name
3. **Include the operation verb**: `GET`, `POST`, `SELECT`, `publish`, `consume`
4. **Match the semantic conventions**: Use [OTel semantic conventions](https://opentelemetry.io/docs/specs/semconv/) for HTTP, DB, messaging, etc.

### Span Attributes

Attributes are key-value pairs attached to a span. They provide the detail that span names
intentionally omit:

| Category | Attribute | Example |
|---|---|---|
| **Axiom domain** | `axiom.work_item_id` | `"opencode-plugin-01"` |
| | `axiom.repo` | `"github.com/org/repo"` |
| | `axiom.run_id` | `"2026-03-20T10-30-00Z_01"` |
| | `axiom.phase_id` | `"phase-87-2"` |
| | `axiom.task_id` | `"task-87-2-3"` |
| | `axiom.step_id` | `"step-87-2-3-1"` |
| | `axiom.correlation_id` | `"req-abc-123"` |
| **HTTP** | `http.request.method` | `"GET"` |
| | `url.full` | `"http://localhost:4096/api/v1/runs"` |
| | `http.response.status_code` | `200` |
| **Database** | `db.system` | `"postgresql"` |
| | `db.statement` | `"SELECT * FROM work_items WHERE id = $1"` |
| **Messaging** | `messaging.system` | `"nats"` |
| | `messaging.destination.name` | `"axiom.steps"` |

### Span Status

Set span status to communicate success or failure:

| Status | When |
|---|---|
| `OK` | Operation completed successfully |
| `ERROR` | Operation failed; attach exception details via `record_exception()` |
| `UNSET` | Default; let the consumer decide based on attributes |

**Rule**: Always set `ERROR` status when an exception is caught and the span represents a
failed operation. Always call `record_exception(e)` before setting status to capture the
stack trace as a span event.

```python
with tracer.start_as_current_span("axiom.step.execute") as span:
    span.set_attribute("axiom.step_id", step_id)
    try:
        result = execute_step(step_id)
        span.set_status(trace.StatusCode.OK)
    except Exception as e:
        span.record_exception(e)
        span.set_status(trace.StatusCode.ERROR, str(e))
        raise
```

### Span Events

Span events are timestamped annotations within a span's lifetime. Use them for:
- Exception recording (`record_exception`)
- Significant state transitions within a long-running span
- Retry attempts within a single span

Do NOT use span events as a replacement for structured log events. Span events are
trace-scoped; log events are independently queryable.

---

## 4. Axiom Span Hierarchy

Per `specs/34-Observability-And-Metrics.md#post-v1-opentelemetry-trace-spans`, the Axiom
trace hierarchy is:

```
axiom.run (root span)
  +-- axiom.phase
        +-- axiom.task
              +-- axiom.step
                    +-- axiom.verification
                    +-- axiom.opencode_request
```

| Span Name | Parent | Key Attributes | Notes |
|---|---|---|---|
| `axiom.run` | None (root) | `work_item_id`, `repo`, `run_id` | One per work-item execution |
| `axiom.phase` | `axiom.run` | `phase_id` | One per plan phase |
| `axiom.task` | `axiom.phase` | `task_id` | One per plan task |
| `axiom.step` | `axiom.task` | `step_id`, `command`, `status` | One per plan step execution |
| `axiom.verification` | `axiom.step` | `verifier_type`, `result`, `score` | One per verifier invocation |
| `axiom.opencode_request` | `axiom.step` | `correlation_id`, `http_status`, `duration_ms` | One per OpenCode HTTP call |

### Correlation ID Backward Compatibility

The `correlation_id` field from v1 structured logging MUST be preserved as a span attribute
on `axiom.opencode_request` spans even after OpenTelemetry adoption. This ensures:
- Existing log-based queries continue to work
- Traces can be correlated with log events via `correlation_id`
- Migration from log-based tracing to span-based tracing is non-breaking

---

## 5. Context Propagation

Context propagation is how trace identity crosses process, network, and async boundaries.
Broken propagation creates orphaned spans and disconnected traces.

### W3C Trace Context (Required)

Axiom requires W3C Trace Context propagation per `specs/34-Observability-And-Metrics.md`.
This uses two HTTP headers:

| Header | Format | Example |
|---|---|---|
| `traceparent` | `{version}-{trace_id}-{span_id}-{trace_flags}` | `00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01` |
| `tracestate` | Vendor-specific key-value pairs | `axiom=run_id:abc123` |

### Propagation by Transport

| Transport | Propagation Mechanism | OTel Support |
|---|---|---|
| **HTTP** | `traceparent` + `tracestate` headers | Built-in with `W3CTraceContextPropagator` |
| **gRPC** | `traceparent` + `tracestate` metadata | Built-in with gRPC interceptors |
| **Message queues** (NATS, Kafka, SQS) | Message headers/attributes | Manual injection into message metadata |
| **Async tasks** (Celery, background jobs) | Task headers or context carrier | Manual injection into task kwargs |
| **WebSocket/SSE** | Initial handshake headers | Inject on connection; child spans per message |

### HTTP Propagation (Automatic)

With OTel auto-instrumentation, HTTP propagation is automatic for supported libraries:

```python
# Python: requests, httpx, aiohttp, urllib3
from opentelemetry.instrumentation.requests import RequestsInstrumentor
RequestsInstrumentor().instrument()

# All outgoing requests now carry traceparent/tracestate automatically
import requests
requests.get("http://opencode-server:4096/health")
# ^ traceparent header injected automatically
```

### Message Queue Propagation (Manual)

For message queues, you must manually inject and extract context:

```python
from opentelemetry import context, trace
from opentelemetry.propagate import inject, extract

# Producer: inject context into message headers
def publish_message(topic: str, payload: dict):
    headers = {}
    inject(headers)  # Injects traceparent + tracestate
    broker.publish(topic, payload, headers=headers)

# Consumer: extract context from message headers
def consume_message(message):
    ctx = extract(message.headers)
    with tracer.start_as_current_span(
        f"consume {message.topic}",
        context=ctx,
        kind=trace.SpanKind.CONSUMER,
    ) as span:
        process(message)
```

### Async / Background Job Propagation

For background jobs (Celery, thread pools, asyncio tasks):

```python
from opentelemetry.context import attach, detach, get_current

# Capture context before dispatching
token = None
ctx = get_current()

def background_task():
    token = attach(ctx)  # Restore parent context in worker thread
    try:
        with tracer.start_as_current_span("background.process"):
            do_work()
    finally:
        detach(token)

executor.submit(background_task)
```

### Propagation Debugging Checklist

When traces appear disconnected:

1. **Check headers**: Is `traceparent` present in outgoing requests? Use `curl -v` or network inspector.
2. **Check extractors**: Is the receiving service extracting context? Log `trace.get_current_span().get_span_context()`.
3. **Check propagator registration**: Is `W3CTraceContextPropagator` set as the global propagator?
4. **Check middleware order**: Is the tracing middleware registered before route handlers?
5. **Check async boundaries**: Are you attaching context in worker threads/tasks?
6. **Check queue headers**: Are message headers carrying `traceparent`?

---

## 6. Sampling Strategies

Sampling controls which traces are recorded and exported. The goal is to capture enough
data for debugging without overwhelming storage or network.

### Sampling Types

| Strategy | Description | Use Case |
|---|---|---|
| **Always-on** (`AlwaysOnSampler`) | Record every trace | Dev/staging; low-traffic services |
| **Always-off** (`AlwaysOffSampler`) | Record no traces | Disable tracing without removing instrumentation |
| **Probability** (`TraceIdRatioBased`) | Sample N% of traces by trace ID hash | Production baseline; predictable cost |
| **Rate-limiting** | Sample up to N traces/second | Protect backend from burst traffic |
| **Parent-based** (`ParentBasedSampler`) | Respect parent's sampling decision | Default for child services; prevents orphaned spans |
| **Tail-based** (collector-side) | Decide after trace completes, based on duration/error/attributes | Capture all errors + slow traces; requires OTel Collector |

### Recommended Configuration

For Axiom services:

| Environment | Head Sampler | Tail Sampler | Rationale |
|---|---|---|---|
| **Development** | `AlwaysOnSampler` | None | Full visibility for debugging |
| **Staging** | `ParentBased(TraceIdRatioBased(1.0))` | None | Full visibility; staging traffic is low |
| **Production** | `ParentBased(TraceIdRatioBased(0.1))` | Error + slow-trace filter at Collector | 10% baseline + 100% of errors and p99 outliers |

### Tail-Based Sampling with OTel Collector

Tail-based sampling runs in the OTel Collector, not in the application SDK:

```yaml
# otel-collector-config.yaml
processors:
  tail_sampling:
    decision_wait: 10s
    policies:
      - name: errors-always
        type: status_code
        status_code: { status_codes: [ERROR] }
      - name: slow-traces
        type: latency
        latency: { threshold_ms: 5000 }
      - name: baseline
        type: probabilistic
        probabilistic: { sampling_percentage: 10 }
```

### Sampling Decision Rules

1. **Never drop error traces**: Errors are the most valuable signal. Use tail-based sampling or a composite sampler that always keeps `ERROR` status spans.
2. **Respect parent decisions**: Use `ParentBasedSampler` as the root sampler to prevent orphaned child spans when the parent was sampled.
3. **Keep sampling deterministic by trace ID**: `TraceIdRatioBased` ensures the same trace ID always gets the same sampling decision across services.
4. **Document your sampling rate**: Include the effective sampling rate in dashboards and runbooks so operators know what percentage of traffic is visible.

---

## 7. Backend Options

### Comparison Matrix

| Backend | Protocol | Storage | Query Language | OSS/Commercial | Best For |
|---|---|---|---|---|---|
| **Jaeger** | OTLP, Jaeger Thrift | Elasticsearch, Cassandra, Badger | Jaeger UI search | OSS (CNCF) | Kubernetes-native; strong OTel integration |
| **Grafana Tempo** | OTLP, Jaeger, Zipkin | Object storage (S3, GCS, Azure Blob) | TraceQL | OSS (Grafana Labs) | Cost-effective at scale; Grafana integration |
| **Zipkin** | OTLP, Zipkin | Elasticsearch, Cassandra, MySQL | Zipkin UI search | OSS | Simple setup; legacy compatibility |
| **Datadog APM** | OTLP, dd-trace | Datadog SaaS | Datadog trace search | Commercial | Full-stack APM; anomaly detection |
| **AWS X-Ray** | OTLP (via Collector), X-Ray SDK | AWS managed | X-Ray console + analytics | Commercial (AWS) | AWS-native services; Lambda tracing |
| **Honeycomb** | OTLP | Honeycomb SaaS | BubbleUp, query builder | Commercial | High-cardinality exploration; debugging |

### Backend Selection Criteria

1. **Existing stack**: If you already run Grafana, prefer Tempo. If Datadog, use Datadog APM.
2. **Scale**: For >1M spans/day, prefer object-storage backends (Tempo, Jaeger with ES).
3. **Query needs**: If you need ad-hoc high-cardinality queries, prefer Honeycomb or Tempo with TraceQL.
4. **Cost**: Object-storage backends (Tempo) are cheapest at scale. SaaS backends charge per span ingested.
5. **Retention**: Define retention policy before choosing. Most backends support configurable TTL.

### Axiom Default

Per `specs/34-Observability-And-Metrics.md#configuration`, Axiom supports three exporter
types: `otlp`, `jaeger`, `zipkin`. The recommended default is `otlp` pointing at an
OpenTelemetry Collector, which can then fan out to any backend.

---

## 8. Trace-to-Log and Trace-to-Metric Correlation

### Trace-to-Log Correlation

Inject `trace_id` and `span_id` into structured log events so log queries can jump to the
owning trace:

```python
import logging
from opentelemetry import trace

class TraceContextFilter(logging.Filter):
    def filter(self, record):
        span = trace.get_current_span()
        ctx = span.get_span_context()
        record.trace_id = format(ctx.trace_id, '032x') if ctx.trace_id else ""
        record.span_id = format(ctx.span_id, '016x') if ctx.span_id else ""
        return True

logger = logging.getLogger("axiom")
logger.addFilter(TraceContextFilter())
```

For Axiom, the `correlation_id` field from `specs/25-Structured-Logging-Events.md` serves
as the v1 trace-to-log bridge. Post-v1, both `correlation_id` and `trace_id` SHOULD appear
in log events for full cross-referencing.

### Trace-to-Metric Correlation (Exemplars)

Exemplars link a metric data point to a specific trace that contributed to it:

```python
from opentelemetry import metrics, trace

meter = metrics.get_meter("axiom.repo_runner")
histogram = meter.create_histogram("codeops_step_duration_seconds")

# Record with exemplar (automatic when tracing is active)
histogram.record(duration_seconds, attributes={"step_id": step_id})
# ^ OTel SDK automatically attaches trace_id/span_id as exemplar
```

In Grafana, exemplars appear as dots on metric graphs that link directly to the trace view.

---

## 9. Common Anti-Patterns

### Anti-Pattern Catalog

| Anti-Pattern | Problem | Fix |
|---|---|---|
| **Span-per-line** | Creating a span for every log line or trivial function | Only create spans for meaningful units of work (network calls, DB queries, significant processing) |
| **High-cardinality span names** | Including IDs, timestamps, or user input in span names | Use parameterized names; put variable data in attributes |
| **Missing error status** | Not setting `ERROR` status on failed spans | Always `record_exception()` + `set_status(ERROR)` in catch blocks |
| **Orphaned spans** | Broken context propagation creates disconnected traces | Use `ParentBasedSampler`; verify propagation at every boundary |
| **Unbounded attributes** | Storing full request/response bodies as span attributes | Store summaries or hashes; use log events for full payloads |
| **Sampling mismatch** | Different services using different sampling rates | Use `ParentBasedSampler` so child services respect parent decisions |
| **No resource attributes** | Spans without `service.name` or `deployment.environment` | Always configure `Resource` with service identity |
| **Synchronous export** | Using `SimpleSpanProcessor` in production | Use `BatchSpanProcessor` with appropriate queue size and export interval |
| **Trace-only debugging** | Relying solely on traces without correlating logs and metrics | Use trace-to-log and trace-to-metric correlation (exemplars) |
| **Ignoring baggage** | Not propagating cross-cutting context (tenant ID, feature flags) | Use OTel Baggage for context that every service needs |

### Performance Guardrails

| Setting | Recommended Value | Why |
|---|---|---|
| `BatchSpanProcessor.max_queue_size` | 2048 (default) | Prevents unbounded memory growth |
| `BatchSpanProcessor.max_export_batch_size` | 512 (default) | Balances throughput and latency |
| `BatchSpanProcessor.export_timeout_millis` | 30000 | Prevents export hangs from blocking the queue |
| `BatchSpanProcessor.schedule_delay_millis` | 5000 (default) | Balances export frequency and batch efficiency |
| Attribute value length | Truncate at 1024 chars | Prevents oversized spans |
| Attribute count per span | Max 128 | Prevents attribute bloat |
| Span event count | Max 128 per span | Prevents event bloat on long-running spans |

---

## 10. Migration: Correlation-ID to OpenTelemetry

For Axiom, the migration path from v1 (correlation-ID-based) to post-v1 (OpenTelemetry)
is defined in `specs/34-Observability-And-Metrics.md`:

### Migration Steps

1. **Keep `correlation_id` as a span attribute**: Every `axiom.opencode_request` span MUST
   include `axiom.correlation_id` as an attribute, preserving backward compatibility.

2. **Dual-write period**: During migration, emit both structured log events (with
   `correlation_id`) and OpenTelemetry spans. This allows validation that traces match
   log-reconstructed request flows.

3. **Validate trace completeness**: For each `correlation_id`, verify that the OTel trace
   contains the same set of operations visible in the log stream.

4. **Deprecate log-only tracing**: Once OTel traces are validated, log-based trace
   reconstruction becomes a fallback, not the primary path.

5. **Preserve log correlation**: Even after full OTel adoption, structured log events SHOULD
   include `trace_id` and `span_id` fields for cross-referencing.

### Migration Checklist

- [ ] `traces_enabled: true` in `.axiom/axiom.config.yaml`
- [ ] `traces_exporter` and `traces_endpoint` configured
- [ ] `correlation_id` preserved as span attribute on all `axiom.opencode_request` spans
- [ ] W3C Trace Context headers (`traceparent`, `tracestate`) injected on all outgoing HTTP requests
- [ ] `ParentBasedSampler` configured as root sampler
- [ ] `BatchSpanProcessor` used (not `SimpleSpanProcessor`)
- [ ] Resource attributes set (`service.name`, `service.version`, `deployment.environment`)
- [ ] Trace-to-log correlation verified (log events include `trace_id`)
- [ ] Sampling rate documented in operational runbook
- [ ] Backend retention policy configured

---

## 11. Operational Checklist

### Pre-Production Tracing Readiness

- [ ] **SDK initialized**: `TracerProvider` registered with correct `Resource` attributes
- [ ] **Exporter configured**: OTLP/Jaeger/Zipkin exporter pointing at collector or backend
- [ ] **Propagator set**: `W3CTraceContextPropagator` registered as global propagator
- [ ] **Auto-instrumentation**: HTTP client/server, DB driver, and framework instrumentation enabled
- [ ] **Manual spans**: Business-critical operations have explicit spans with domain attributes
- [ ] **Error handling**: All catch blocks call `record_exception()` + `set_status(ERROR)`
- [ ] **Sampling configured**: Head sampling in SDK + tail sampling in Collector (if applicable)
- [ ] **Batch processor**: `BatchSpanProcessor` with appropriate queue/batch/timeout settings
- [ ] **Correlation**: `correlation_id` and/or `trace_id` present in structured log events
- [ ] **Dashboard**: Trace search and service map available in observability UI
- [ ] **Alerts**: Alert on trace export failures (`otelcol_exporter_send_failed_spans`)
- [ ] **Runbook**: Tracing troubleshooting runbook exists and is linked from alerts

### Span Coverage Review Checklist

For each service, verify spans exist for:

- [ ] Inbound HTTP/gRPC requests (auto-instrumented)
- [ ] Outbound HTTP/gRPC requests (auto-instrumented)
- [ ] Database queries (auto-instrumented or manual)
- [ ] Message queue publish and consume operations (manual)
- [ ] Background job dispatch and execution (manual)
- [ ] Cache operations (manual, if latency-sensitive)
- [ ] External API calls (auto-instrumented or manual)
- [ ] Axiom domain operations: run, phase, task, step, verification (manual)

---

## 12. Quick Reference

### OTel SDK Cheat Sheet

| Operation | Python | TypeScript |
|---|---|---|
| Get tracer | `trace.get_tracer("name")` | `provider.getTracer("name")` |
| Start span | `with tracer.start_as_current_span("name"):` | `tracer.startActiveSpan("name", (span) => { ... })` |
| Set attribute | `span.set_attribute("key", value)` | `span.setAttribute("key", value)` |
| Record exception | `span.record_exception(e)` | `span.recordException(e)` |
| Set status OK | `span.set_status(StatusCode.OK)` | `span.setStatus({ code: SpanStatusCode.OK })` |
| Set status ERROR | `span.set_status(StatusCode.ERROR, msg)` | `span.setStatus({ code: SpanStatusCode.ERROR, message: msg })` |
| End span | Automatic (context manager / callback) | `span.end()` |
| Inject context | `inject(carrier)` | `propagation.inject(context.active(), carrier)` |
| Extract context | `ctx = extract(carrier)` | `const ctx = propagation.extract(context.active(), carrier)` |

### Span Kind Reference

| Kind | Use When |
|---|---|
| `INTERNAL` | Default; in-process operations |
| `SERVER` | Handling an inbound request (HTTP server, gRPC server) |
| `CLIENT` | Making an outbound request (HTTP client, gRPC client, DB query) |
| `PRODUCER` | Publishing a message to a queue/topic |
| `CONSUMER` | Consuming a message from a queue/topic |

---

## Spec References

- `specs/34-Observability-And-Metrics.md` -- Trace span hierarchy, W3C propagation, configuration contract
- `specs/25-Structured-Logging-Events.md` -- Structured log event schema, `correlation_id` field
- `specs/31-OpenCode-Integration-Contract.md` -- OpenCode server lifecycle, HTTP contract
- `specs/61-Logging-And-Observability-Overhaul.md` -- Logging and observability overhaul
- `specs/65-Diagnostic-Workflows.md` -- Diagnostic workflows referencing trace data
- `specs/66-Predictive-Observability.md` -- Predictive observability using trace-derived signals

## Related Skills

- `dashboard-design-axiom` -- Dashboard design patterns (trace visualization panels)
- `alert-engineering-axiom` -- Alert engineering (trace-derived alerts)
- `sre-ops-axiom` -- SRE operations (trace-informed SLOs and error budgets)
- `protocol-testing` -- Protocol testing (trace propagation verification)
