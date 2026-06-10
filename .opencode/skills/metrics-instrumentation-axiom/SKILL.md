---
name: metrics-instrumentation-axiom
description: >
  Metric types, exact `codeops_` naming rules, bounded-label/cardinality guidance,
  instrumentation patterns, exposition formats, derived metrics, and anti-patterns.
  Load this skill when designing metrics, reviewing metric naming, choosing histogram
  buckets, auditing label cardinality, or instrumenting any service managed by Axiom.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-03-20"
  primary_spec: specs/34-Observability-And-Metrics.md
  secondary_specs:
    - specs/25-Structured-Logging-Events.md
    - specs/61-Logging-And-Observability-Overhaul.md
    - specs/47-Cost-Tracking-And-Session-Analytics.md
    - specs/58-Execution-Duration-Analytics.md
tags:
  vertical: [benchmarking, sre]
  category: observability
  core: false
---

# Metrics Instrumentation Skill (Portable)

> **"A metric is a number that tells you whether to wake someone up."**
>
> **"If you can't alert on it, dashboard it, or derive a decision from it, don't collect it."**

This skill provides portable, production-grade guidance for designing, implementing, and
operating metrics in services managed by Axiom. It covers Prometheus metric types, the
Axiom `codeops_` naming contract, bounded-label design, cardinality guardrails,
instrumentation patterns for Python and TypeScript, exposition formats, log-derived vs
direct metrics, and common anti-patterns.

**Spec grounding**: The Axiom metric registry, naming conventions, cardinality rules,
and Prometheus promotion are defined in `specs/34-Observability-And-Metrics.md#metrics-contract`,
`specs/34-Observability-And-Metrics.md#metric-naming-conventions`, and
`specs/61-Logging-And-Observability-Overhaul.md#REQ-LOH-030` through `REQ-LOH-035`.
This skill encodes those requirements as reusable instrumentation patterns applicable
to any Prometheus-compatible stack.

---

## Activation

Load this skill when:
- Designing metrics for a new service, component, or feature
- Reviewing metric naming conventions and label design
- Choosing histogram bucket boundaries
- Auditing label cardinality for explosion risk
- Instrumenting Python or TypeScript code with `prometheus_client` or `prom-client`
- Setting up a Prometheus `/metrics` endpoint
- Deriving metrics from structured log events
- Reviewing or extending the Axiom metric registry
- Evaluating whether a new metric is worth collecting

---

## 1. Prometheus Metric Types

Prometheus defines four core metric types. Choose the right one for each measurement.

### 1.1 Counter

A monotonically increasing value that resets only on process restart.

**Use for**: totals, cumulative counts, bytes transferred, errors seen.

```python
# Python (prometheus_client)
from prometheus_client import Counter

step_total = Counter(
    "codeops_step_total",
    "Total steps executed by outcome",
    ["work_item_id", "repo", "status"],
)

# Increment
step_total.labels(work_item_id="ABC-123", repo="org/repo", status="completed").inc()
```

```typescript
// TypeScript (prom-client)
import { Counter } from "prom-client";

const stepTotal = new Counter({
  name: "codeops_step_total",
  help: "Total steps executed by outcome",
  labelNames: ["work_item_id", "repo", "status"],
});

stepTotal.labels({ work_item_id: "ABC-123", repo: "org/repo", status: "completed" }).inc();
```

**Rules**:
- Counter names MUST end with `_total` (Prometheus convention).
- Never decrement a counter. If you need to track a value that goes down, use a gauge.
- Use `rate()` or `increase()` in PromQL to get per-second or per-interval rates.

### 1.2 Gauge

A value that can go up and down. Represents a current snapshot.

**Use for**: queue depth, active connections, current temperature, confidence scores.

```python
from prometheus_client import Gauge

queue_depth = Gauge(
    "codeops_queue_depth",
    "Current work queue depth",
    ["queue"],
)

# Set absolute value
queue_depth.labels(queue="default").set(42)

# Increment/decrement
queue_depth.labels(queue="default").inc()
queue_depth.labels(queue="default").dec()
```

**Rules**:
- Gauge names do NOT use `_total` suffix.
- Use gauges for "how many right now?" questions.
- For info-style gauges (always 1, carrying labels), use the `_info` suffix:
  `codeops_controller_info{version="1.2.3", controller_id="ctrl-abc"}` = 1.

### 1.3 Histogram

Samples observations (usually durations or sizes) into configurable buckets.

**Use for**: request latency, step duration, snapshot size, response time percentiles.

```python
from prometheus_client import Histogram

step_duration = Histogram(
    "codeops_step_duration_seconds",
    "Step execution duration",
    ["work_item_id", "repo", "step_id", "command", "status"],
    buckets=[0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600],
)

# Observe a duration
step_duration.labels(
    work_item_id="ABC-123",
    repo="org/repo",
    step_id="step-1-1-1",
    command="run_tests",
    status="completed",
).observe(12.5)

# Or use as a context manager for automatic timing
with step_duration.labels(...).time():
    execute_step()
```

**Rules**:
- Duration histograms MUST use `_seconds` suffix (not milliseconds).
- Size histograms MUST use `_bytes` suffix.
- Choose bucket boundaries that match your SLO thresholds and expected distribution.
- Histograms generate `_bucket`, `_sum`, and `_count` sub-metrics automatically.
- Use `histogram_quantile()` in PromQL for percentile calculations.

**Bucket design guidance**:

| Measurement | Recommended Buckets | Rationale |
|---|---|---|
| HTTP request latency | `[0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]` | Web request SLOs typically target sub-second |
| Step execution duration | `[0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600]` | Steps range from sub-second to 10 minutes |
| OpenCode request duration | `[0.5, 1, 2, 5, 10, 30, 60, 120, 300]` | LLM calls range from 1s to 5 minutes |
| Snapshot size (bytes) | `[1e6, 1e7, 5e7, 1e8, 5e8, 1e9, 3e9, 5e9]` | 1MB to 5GB range |
| Checkpoint write duration | `[0.01, 0.05, 0.1, 0.5, 1, 5, 10]` | Local disk writes, usually fast |

### 1.4 Summary

Pre-calculates quantiles on the client side. Rarely preferred over histograms.

**Use for**: legacy compatibility only. Prefer histograms for new metrics.

**Why histograms over summaries**:
- Histograms are aggregatable across instances; summaries are not.
- Histograms support `histogram_quantile()` in PromQL for flexible percentile calculation.
- Summaries lock in quantile values at instrumentation time.
- The only advantage of summaries is slightly more accurate quantiles for a single instance.

**Rule**: Do NOT use summaries for new Axiom metrics. Use histograms.

---

## 2. Axiom Naming Contract

All Axiom metrics follow a strict naming contract defined in
`specs/34-Observability-And-Metrics.md#metric-naming-conventions`.

### 2.1 Prefix Rule

Every metric name MUST start with `codeops_`.

```
codeops_step_duration_seconds        # correct
codeops_verification_result_total    # correct
step_duration_seconds                # WRONG: missing prefix
axiom.step.duration.seconds        # WRONG: dots instead of underscores
```

### 2.2 Suffix Rules

| Metric Type | Required Suffix | Example |
|---|---|---|
| Counter | `_total` | `codeops_step_total` |
| Duration histogram | `_seconds` | `codeops_step_duration_seconds` |
| Size gauge/histogram | `_bytes` | `codeops_snapshot_size_bytes` |
| Info gauge | `_info` | `codeops_controller_info` |
| Timestamp gauge | `_timestamp` | `codeops_upstream_poll_last_success_timestamp` |
| Boolean gauge | (no special suffix) | `codeops_subscription_health_status` |

### 2.3 Naming Pattern

```
codeops_<component>_<measurement>[_<unit>]
```

- `<component>`: the subsystem producing the metric (e.g., `step`, `verification`,
  `opencode`, `checkpoint`, `snapshot`, `queue`, `workspace`, `controller`).
- `<measurement>`: what is being measured (e.g., `duration`, `total`, `depth`, `size`).
- `<unit>`: the unit of measurement, if applicable (`seconds`, `bytes`).

### 2.4 Naming Checklist

Before adding a new metric, verify:

1. Starts with `codeops_`?
2. Uses underscores (not dots or hyphens)?
3. Counter ends with `_total`?
4. Duration ends with `_seconds` (not `_ms`, `_milliseconds`)?
5. Size ends with `_bytes` (not `_kb`, `_mb`)?
6. Name is lowercase ASCII only?
7. Name describes what is measured, not how it is used?
8. No duplicate of an existing metric in the registry?

---

## 3. Label Design and Cardinality

Labels add dimensions to metrics. Cardinality (the number of unique label combinations)
is the primary cost driver for metrics storage and query performance.

### 3.1 Cardinality Rule (from specs/34)

> Label cardinality MUST be bounded: do not use unbounded labels like `message` or
> `reason` text. Use enumerated categories.

### 3.2 Bounded vs Unbounded Labels

| Label Type | Example | Cardinality | Verdict |
|---|---|---|---|
| Enumerated status | `status` in `{completed, failed, skipped}` | 3 | Safe |
| Enumerated reason | `reason` in `{oom_killed, crash, evicted, timeout}` | 4 | Safe |
| Repo identifier | `repo` = `"org/repo-name"` | ~10-100 per deployment | Safe with monitoring |
| Work item ID | `work_item_id` = `"ABC-123"` | Unbounded over time | Use with caution (see below) |
| Step ID | `step_id` = `"step-1-1-1"` | ~10-50 per work item | Acceptable on histograms |
| Free-text message | `message` = `"Connection refused..."` | Unbounded | NEVER use as label |
| Full error text | `error` = `"Traceback..."` | Unbounded | NEVER use as label |
| File path | `path` = `"/home/user/..."` | Unbounded | NEVER use as label |
| User input | `query` = `"..."` | Unbounded | NEVER use as label |

### 3.3 Work Item ID Label Guidance

The `work_item_id` label appears on many Axiom metrics. It is technically unbounded
(new work items are created over time), but in practice:

- Active work items at any time: typically 1-10.
- Completed work items accumulate but their time series become stale.
- Prometheus handles stale series via its staleness mechanism (5-minute mark).

**Guidance**: `work_item_id` is acceptable as a label on repo runner metrics because
the active cardinality is bounded. However:
- Do NOT use `work_item_id` on controller-level aggregate metrics (use `repo` instead).
- Monitor `prometheus_tsdb_head_series` for unexpected growth.
- Set a recording rule to pre-aggregate high-cardinality queries.

### 3.4 Cardinality Budget

For any single metric, the total label cardinality (product of all label value counts)
SHOULD stay below 1,000 active time series. If a metric risks exceeding this:

1. Remove the highest-cardinality label.
2. Replace it with a recording rule that pre-aggregates.
3. Move the high-cardinality dimension to structured log events instead.

### 3.5 Label Naming Rules

- Labels use the same field names as `specs/25-Structured-Logging-Events.md` correlation
  fields where applicable (`work_item_id`, `repo`, `run_id`, `step_id`, etc.).
- Label names are lowercase with underscores.
- Label values are strings. Use enumerated values from the event vocabulary.
- Empty label values are equivalent to the label not being set. Prefer not setting the
  label over setting it to `""`.

---

## 4. Axiom Metric Registry

The authoritative metric registry is in `specs/34-Observability-And-Metrics.md#metrics-contract`.
This section provides a categorized reference with instrumentation notes.

### 4.1 Repo Runner Metrics

These metrics are emitted by the repo runner process (`axiom run`).

| Metric | Type | Key Labels | Source Events | Notes |
|---|---|---|---|---|
| `codeops_step_duration_seconds` | Histogram | `work_item_id`, `repo`, `step_id`, `command`, `status` | `step_completed`, `step_failed` | Primary step latency metric. Use `duration_ms` field / 1000. |
| `codeops_step_total` | Counter | `work_item_id`, `repo`, `status` | `step_completed`, `step_failed`, `step_skipped` | Step outcome counter. `status` in `{completed, failed, skipped}`. |
| `codeops_verification_result_total` | Counter | `work_item_id`, `repo`, `verifier_type`, `result` | `verification_passed`, `verification_failed` | `result` in `{passed, failed}`. |
| `codeops_confidence_score` | Gauge | `work_item_id`, `repo`, `signal` | `run_completed` | 0-100 per signal. |
| `codeops_opencode_request_duration_seconds` | Histogram | `work_item_id`, `repo`, `command`, `http_status` | `opencode_response_received` | LLM call latency. |
| `codeops_opencode_token_usage_total` | Counter | `work_item_id`, `repo`, `token_type` | `opencode_response_received` | `token_type` in `{prompt_tokens, completion_tokens}`. |
| `codeops_checkpoint_write_duration_seconds` | Histogram | `work_item_id`, `repo` | `checkpoint_written` | Checkpoint persistence latency. |
| `codeops_plan_injection_total` | Counter | `work_item_id`, `repo`, `verifier_type` | `plan_injection` | Verifier-triggered plan changes. |
| `codeops_retry_total` | Counter | `work_item_id`, `repo`, `command`, `retry_reason` | `step_retried` | `retry_reason` MUST be enumerated. |
| `codeops_escalation_total` | Counter | `work_item_id`, `repo`, `reason` | `escalation_triggered` | `reason` MUST be enumerated. |

### 4.2 Snapshot/Restore Metrics

Emitted only when `snapshot.enabled: true`. See `specs/41-Runtime-Snapshot-And-Restore.md`.

| Metric | Type | Key Labels | Notes |
|---|---|---|---|
| `codeops_snapshot_capture_duration_seconds` | Histogram | `work_item_id`, `repo`, `storage_target`, `status` | Capture latency. |
| `codeops_snapshot_capture_total` | Counter | `work_item_id`, `repo`, `storage_target`, `status` | `status` in `{completed, failed, skipped}`. |
| `codeops_snapshot_size_bytes` | Gauge | `work_item_id`, `repo`, `storage_target` | Last snapshot size. |
| `codeops_snapshot_upload_duration_seconds` | Histogram | `work_item_id`, `repo` | S3 upload latency. |
| `codeops_snapshot_upload_total` | Counter | `work_item_id`, `repo`, `status` | S3 upload outcomes. |
| `codeops_snapshot_restore_duration_seconds` | Histogram | `work_item_id`, `repo`, `storage_target`, `status` | Restore latency. |
| `codeops_snapshot_restore_total` | Counter | `work_item_id`, `repo`, `storage_target`, `status` | Restore outcomes. |
| `codeops_snapshot_gc_deleted_total` | Counter | `work_item_id`, `repo` | GC deletion count. |
| `codeops_snapshot_gc_duration_seconds` | Histogram | `work_item_id`, `repo` | GC duration. |
| `codeops_snapshot_checksum_mismatch_total` | Counter | `work_item_id`, `repo` | Integrity failures. |

### 4.3 Control Plane Metrics

Emitted by the controller process (`axiom serve`) in Full Automated mode.

| Metric | Type | Key Labels | Notes |
|---|---|---|---|
| `codeops_work_items_enqueued_total` | Counter | `repo` | Work items placed on queue. |
| `codeops_work_items_completed_total` | Counter | `repo`, `status` | Work items by outcome. |
| `codeops_pods_created_total` | Counter | `repo` | Workspace containers created. |
| `codeops_pods_failed_total` | Counter | `repo`, `reason` | `reason` in `{oom_killed, crash, evicted, timeout}`. |
| `codeops_queue_depth` | Gauge | `queue` | Current queue depth. |
| `codeops_pod_startup_duration_seconds` | Histogram | `repo` | Pod creation to Running. |
| `codeops_active_pods` | Gauge | `repo` | Currently running containers. |
| `codeops_controller_info` | Gauge | `version`, `controller_id` | Always 1. Identity metric. |
| `codeops_controller_uptime_seconds` | Gauge | `controller_id` | Seconds since start. |

### 4.4 Controller Metrics (from specs/61)

Additional metrics promoted to required by `specs/61-Logging-And-Observability-Overhaul.md#REQ-LOH-032`:

| Metric | Type | Key Labels | Notes |
|---|---|---|---|
| `codeops_workspace_provisioning_duration_seconds` | Histogram | `repo`, `status` | Provision request to ready/failed. |
| `codeops_workspace_active` | Gauge | `repo` | Running workspace containers. |
| `codeops_workspace_total` | Counter | `repo`, `status` | `status` in `{ready, failed, completed, oom_killed, timeout}`. |
| `codeops_queue_messages_total` | Counter | `queue`, `status` | `status` in `{completed, failed, requeued}`. |
| `codeops_opencode_instances_active` | Gauge | `repo` | Running OpenCode instances. |
| `codeops_opencode_aggregate_tokens_total` | Counter | `repo`, `token_type` | Aggregated across workspaces. |
| `codeops_opencode_aggregate_cost_usd` | Counter | `repo` | Aggregated cost. |
| `codeops_session_data_shipped_total` | Counter | `repo`, `status` | Session shipping outcomes. |
| `codeops_session_data_shipped_bytes` | Counter | `repo` | Total bytes shipped. |
| `codeops_log_events_total` | Counter | `component`, `level` | Log event counts. |
| `codeops_log_events_dropped_total` | Counter | `component`, `reason` | Dropped events. |

### 4.5 Upstream Tracking and Benchmark Metrics

From `specs/34-Observability-And-Metrics.md#upstream-tracking-and-benchmark-metrics`:

| Metric | Type | Key Labels | Notes |
|---|---|---|---|
| `codeops_upstream_syncs_total` | Counter | `subscription_id`, `mode`, `status` | Sync executions. |
| `codeops_upstream_merge_conflicts_total` | Counter | `subscription_id` | Merge conflicts. |
| `codeops_upstream_jira_tickets_created_total` | Counter | `subscription_id` | Jira tickets from tracking. |
| `codeops_upstream_poll_last_success_timestamp` | Gauge | `subscription_id` | Last successful poll. |
| `codeops_upstream_sync_duration_seconds` | Histogram | `subscription_id`, `mode`, `status` | Sync duration. |
| `codeops_benchmark_score` | Gauge | `subscription_id`, `dimension` | Latest score by dimension. |
| `codeops_benchmark_bootstrap_duration_seconds` | Histogram | `subscription_id` | Bootstrap duration. |
| `codeops_benchmark_scoring_duration_seconds` | Histogram | `subscription_id` | Scorecard computation. |
| `codeops_benchmark_divergence_count` | Gauge | `subscription_id` | Active divergences. |
| `codeops_subscription_health_status` | Gauge | `subscription_id` | 1 = healthy, 0 = degraded. |

---

## 5. Instrumentation Patterns

### 5.1 Python: prometheus_client

The `prometheus_client` library is the required Prometheus client for Axiom Python
services (per `specs/61-Logging-And-Observability-Overhaul.md#REQ-LOH-031`).

#### Setup

```python
from prometheus_client import (
    Counter, Gauge, Histogram, Info,
    start_http_server, generate_latest,
    REGISTRY, CollectorRegistry,
)

# Start a dedicated metrics server on a separate port
start_http_server(port=9090)
```

#### Metric Registration Pattern

Register metrics at module level (not inside functions). This ensures metrics are
created once and reused across calls.

```python
# metrics.py — central metric registry module
from prometheus_client import Counter, Gauge, Histogram

# Repo runner metrics
STEP_DURATION = Histogram(
    "codeops_step_duration_seconds",
    "Step execution duration",
    ["work_item_id", "repo", "step_id", "command", "status"],
    buckets=[0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600],
)

STEP_TOTAL = Counter(
    "codeops_step_total",
    "Total steps executed by outcome",
    ["work_item_id", "repo", "status"],
)

VERIFICATION_RESULT = Counter(
    "codeops_verification_result_total",
    "Verification outcomes by verifier type",
    ["work_item_id", "repo", "verifier_type", "result"],
)

CONFIDENCE_SCORE = Gauge(
    "codeops_confidence_score",
    "Final confidence score per signal",
    ["work_item_id", "repo", "signal"],
)
```

#### Usage in Application Code

```python
from .metrics import STEP_DURATION, STEP_TOTAL

def execute_step(step, context):
    with STEP_DURATION.labels(
        work_item_id=context.work_item_id,
        repo=context.repo,
        step_id=step.id,
        command=step.command,
        status="in_progress",  # updated after completion
    ).time():
        result = step.run()

    STEP_TOTAL.labels(
        work_item_id=context.work_item_id,
        repo=context.repo,
        status=result.status,
    ).inc()
```

### 5.2 TypeScript: prom-client

For TypeScript services (e.g., the OpenCode plugin), use `prom-client`.

```typescript
import { Counter, Gauge, Histogram, Registry, collectDefaultMetrics } from "prom-client";

const register = new Registry();
collectDefaultMetrics({ register });

const stepDuration = new Histogram({
  name: "codeops_step_duration_seconds",
  help: "Step execution duration",
  labelNames: ["work_item_id", "repo", "step_id", "command", "status"],
  buckets: [0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600],
  registers: [register],
});

// Expose via HTTP
app.get("/metrics", async (req, res) => {
  res.set("Content-Type", register.contentType);
  res.end(await register.metrics());
});
```

### 5.3 Log-Derived Metrics (v1 Pattern)

In v1, metrics are derived from structured log events rather than direct instrumentation.
This is the bridge pattern until Prometheus endpoints are deployed.

```python
import json
import sys
from collections import defaultdict

class LogDerivedMetrics:
    """Derive metrics from structured log events (v1 pattern).

    Reads JSON Lines from the event stream and maintains in-memory
    counters/histograms that can be queried or exported.
    """

    def __init__(self):
        self.counters = defaultdict(int)
        self.histograms = defaultdict(list)

    def process_event(self, event: dict) -> None:
        event_type = event.get("event_type", "")

        if event_type in ("step_completed", "step_failed", "step_skipped"):
            status = event_type.replace("step_", "")
            key = (
                "codeops_step_total",
                event.get("work_item_id", ""),
                event.get("repo", ""),
                status,
            )
            self.counters[key] += 1

            if "duration_ms" in event and event_type != "step_skipped":
                duration_s = event["duration_ms"] / 1000.0
                hist_key = (
                    "codeops_step_duration_seconds",
                    event.get("work_item_id", ""),
                    event.get("repo", ""),
                    event.get("step_id", ""),
                    event.get("command", ""),
                    status,
                )
                self.histograms[hist_key].append(duration_s)

        elif event_type in ("verification_passed", "verification_failed"):
            result = "passed" if event_type == "verification_passed" else "failed"
            key = (
                "codeops_verification_result_total",
                event.get("work_item_id", ""),
                event.get("repo", ""),
                event.get("verifier_type", ""),
                result,
            )
            self.counters[key] += 1
```

**Omit-not-null semantics**: Metrics derived from optional event fields (e.g.,
`duration_ms`) are only emitted when the source field is present. Missing fields
do NOT produce zero-value metrics. This is a spec requirement from
`specs/34-Observability-And-Metrics.md#compatibility-with-structured-logging-event-vocabulary`.

---

## 6. Exposition Formats

### 6.1 Prometheus Text Format

The standard exposition format for `/metrics` endpoints.

```
# HELP codeops_step_total Total steps executed by outcome
# TYPE codeops_step_total counter
codeops_step_total{work_item_id="ABC-123",repo="org/repo",status="completed"} 42
codeops_step_total{work_item_id="ABC-123",repo="org/repo",status="failed"} 3
codeops_step_total{work_item_id="ABC-123",repo="org/repo",status="skipped"} 7

# HELP codeops_step_duration_seconds Step execution duration
# TYPE codeops_step_duration_seconds histogram
codeops_step_duration_seconds_bucket{work_item_id="ABC-123",repo="org/repo",step_id="step-1",command="run_tests",status="completed",le="0.1"} 0
codeops_step_duration_seconds_bucket{work_item_id="ABC-123",repo="org/repo",step_id="step-1",command="run_tests",status="completed",le="1"} 5
codeops_step_duration_seconds_bucket{work_item_id="ABC-123",repo="org/repo",step_id="step-1",command="run_tests",status="completed",le="10"} 18
codeops_step_duration_seconds_bucket{work_item_id="ABC-123",repo="org/repo",step_id="step-1",command="run_tests",status="completed",le="+Inf"} 20
codeops_step_duration_seconds_sum{work_item_id="ABC-123",repo="org/repo",step_id="step-1",command="run_tests",status="completed"} 87.3
codeops_step_duration_seconds_count{work_item_id="ABC-123",repo="org/repo",step_id="step-1",command="run_tests",status="completed"} 20
```

### 6.2 OpenMetrics Format

The newer standard (superset of Prometheus text format). Use when the scraper supports it.

Key differences:
- Counters use `_total` suffix and `# TYPE ... counter` (same as Prometheus).
- `# EOF` marker at end of exposition.
- Supports exemplars (trace ID linkage).

```
# TYPE codeops_step_total counter
# HELP codeops_step_total Total steps executed by outcome
codeops_step_total{work_item_id="ABC-123",repo="org/repo",status="completed"} 42 # {trace_id="abc123def456"}
# EOF
```

### 6.3 Endpoint Configuration

Per `specs/61-Logging-And-Observability-Overhaul.md#REQ-LOH-030`:

- Endpoint: `GET /metrics`
- Port: configurable via `observability.metrics_port` (default: `9090`)
- Separate from the main API port (`8200` for controller, `8100` for repo runner)
- No authentication by default (standard Prometheus pattern)
- Optional basic auth via `AXIOM_METRICS_PASSWORD` environment variable

---

## 7. Derived Metrics and Recording Rules

### 7.1 Recording Rules

Pre-compute expensive queries as recording rules to reduce query-time load.

```yaml
# prometheus-rules.yml
groups:
  - name: codeops_recording_rules
    interval: 30s
    rules:
      # Step success rate (5-minute window)
      - record: axiom:step_success_rate_5m
        expr: |
          sum(rate(codeops_step_total{status="completed"}[5m])) by (repo)
          /
          sum(rate(codeops_step_total[5m])) by (repo)

      # Step duration p95 (5-minute window)
      - record: axiom:step_duration_p95_5m
        expr: |
          histogram_quantile(0.95,
            sum(rate(codeops_step_duration_seconds_bucket[5m])) by (repo, le)
          )

      # Verification pass rate (15-minute window)
      - record: axiom:verification_pass_rate_15m
        expr: |
          sum(rate(codeops_verification_result_total{result="passed"}[15m])) by (repo, verifier_type)
          /
          sum(rate(codeops_verification_result_total[15m])) by (repo, verifier_type)

      # OpenCode cost rate (per hour)
      - record: axiom:opencode_cost_rate_per_hour
        expr: |
          sum(rate(codeops_opencode_aggregate_cost_usd[1h])) by (repo) * 3600
```

### 7.2 Metric-to-Alert Bridge

Recording rules feed into alert rules. See the `alert-engineering-axiom` skill for
the full alert contract. Key bridge points:

| Recording Rule | Alert | Threshold |
|---|---|---|
| `axiom:step_duration_p95_5m` | `CodeOpsStepDurationHigh` | > 600s for 15m |
| `axiom:verification_pass_rate_15m` | `CodeOpsVerificationFailureRateHigh` | < 0.7 for 30m |
| `axiom:opencode_cost_rate_per_hour` | (custom) | Operator-defined budget |

---

## 8. Security and Redaction

### 8.1 Label Safety (from specs/61 REQ-LOH-081)

Prometheus metric labels MUST NOT contain:
- API keys, tokens, or credentials
- File paths from the workspace (could reveal repo structure)
- PII (names, emails)
- Full error messages (use enumerated error categories instead)

**Allowed label values**: enumerated status codes, repo identifiers, component names,
model IDs, agent names.

### 8.2 Metrics Endpoint Security (from specs/61 REQ-LOH-035)

- Served on a separate port from the main API
- No authentication by default (standard Prometheus pattern)
- Optional basic auth via `AXIOM_METRICS_PASSWORD`
- When password is set: HTTP Basic auth with username `prometheus`
- When password is NOT set AND bound to non-loopback: emit `metrics_auth_warning`

### 8.3 No Sensitive Data in Help Text

The `# HELP` line in Prometheus exposition MUST NOT contain sensitive information.
Keep help text generic and descriptive.

```
# HELP codeops_step_total Total steps executed by outcome          # correct
# HELP codeops_step_total Steps for repo github.com/secret/repo    # WRONG
```

---

## 9. Anti-Patterns

### 9.1 Unbounded Label Cardinality

**Problem**: Using free-text fields as labels causes cardinality explosion.

```python
# BAD: unbounded label
errors.labels(message=str(exception)).inc()

# GOOD: enumerated category
errors.labels(error_class=classify_error(exception)).inc()
```

### 9.2 Metric per Endpoint (URL Path Labels)

**Problem**: Using full URL paths as labels creates a metric per endpoint.

```python
# BAD: unbounded path label
requests.labels(path=request.path).inc()

# GOOD: route template label
requests.labels(route="/api/v1/runs/{run_id}").inc()
```

### 9.3 Timestamps in Labels

**Problem**: Putting timestamps in labels creates a new time series every second.

```python
# BAD: timestamp label
gauge.labels(timestamp=datetime.now().isoformat()).set(1)

# GOOD: use the metric's own timestamp (automatic in Prometheus)
gauge.set(1)
```

### 9.4 Counters That Reset

**Problem**: Resetting a counter to zero on application logic (not process restart).

```python
# BAD: manual reset
counter._value.set(0)

# GOOD: counters are monotonic; use rate() in PromQL for windowed views
```

### 9.5 Gauge for Cumulative Values

**Problem**: Using a gauge for something that only increases.

```python
# BAD: gauge for cumulative bytes
bytes_gauge.set(total_bytes_sent)  # goes up but never down

# GOOD: counter for cumulative values
bytes_counter.inc(new_bytes_sent)
```

### 9.6 Missing Unit Suffix

**Problem**: Duration metric without `_seconds` suffix.

```python
# BAD: ambiguous unit
Histogram("codeops_step_duration", ...)       # seconds? milliseconds?

# GOOD: explicit unit
Histogram("codeops_step_duration_seconds", ...)
```

### 9.7 Metric Name Collisions

**Problem**: Two different measurements sharing the same metric name.

```python
# BAD: same name, different semantics in different modules
Counter("codeops_errors_total", "HTTP errors", ["status"])
Counter("codeops_errors_total", "Validation errors", ["type"])

# GOOD: distinct names
Counter("codeops_http_errors_total", "HTTP errors", ["status"])
Counter("codeops_validation_errors_total", "Validation errors", ["type"])
```

### 9.8 Observing Zero-Value Durations

**Problem**: Recording 0-second durations for skipped or no-op operations.

```python
# BAD: pollutes histogram with meaningless zeros
if step.skipped:
    duration_histogram.observe(0)

# GOOD: only observe real durations (omit-not-null)
if not step.skipped and duration_ms is not None:
    duration_histogram.observe(duration_ms / 1000.0)
```

### 9.9 Too Many Buckets

**Problem**: Excessive histogram buckets increase storage and scrape time.

```python
# BAD: 50 buckets for a simple latency metric
Histogram("codeops_request_seconds", ..., buckets=[i * 0.1 for i in range(50)])

# GOOD: 8-12 buckets aligned to SLO thresholds
Histogram("codeops_request_seconds", ..., buckets=[0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10])
```

### 9.10 Metrics Without Consumers

**Problem**: Collecting metrics that no dashboard, alert, or recording rule uses.

**Rule**: Every metric SHOULD have at least one consumer (dashboard panel, alert rule,
or recording rule). If a metric has no consumer after 30 days, consider removing it.

---

## 10. Operational Checklists

### 10.1 New Metric Checklist

Before adding a new metric to the Axiom registry:

- [ ] Name follows `codeops_<component>_<measurement>[_<unit>]` pattern
- [ ] Name starts with `codeops_`
- [ ] Correct suffix for type (`_total`, `_seconds`, `_bytes`, `_info`)
- [ ] All labels are bounded (enumerated values, not free text)
- [ ] Estimated cardinality < 1,000 active time series
- [ ] Source event(s) identified in `specs/25-Structured-Logging-Events.md`
- [ ] Help text is descriptive and contains no sensitive data
- [ ] At least one consumer planned (dashboard, alert, or recording rule)
- [ ] Added to `specs/34-Observability-And-Metrics.md` metric registry table
- [ ] No name collision with existing metrics

### 10.2 Metrics Endpoint Health Check

```bash
# Verify endpoint is serving
curl -sf http://localhost:9090/metrics | head -5

# Count total metric families
curl -sf http://localhost:9090/metrics | grep "^# TYPE" | wc -l

# Check for cardinality (total time series)
curl -sf http://localhost:9090/metrics | grep -v "^#" | grep -v "^$" | wc -l

# Verify specific metric exists
curl -sf http://localhost:9090/metrics | grep "codeops_step_total"

# Check for label safety (no file paths or secrets)
curl -sf http://localhost:9090/metrics | grep -iE "(password|token|secret|/home/|/tmp/)"
# Expected: no output
```

### 10.3 Prometheus Scrape Verification

```bash
# Check Prometheus targets
curl -sf http://prometheus:9090/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health: .health, lastScrape: .lastScrape}'

# Check for scrape errors
curl -sf http://prometheus:9090/api/v1/targets | jq '.data.activeTargets[] | select(.health != "up")'

# Verify metric is being scraped
curl -sf 'http://prometheus:9090/api/v1/query?query=codeops_step_total' | jq '.data.result | length'
```

### 10.4 Cardinality Audit

```promql
# Top 10 metrics by cardinality
topk(10, count by (__name__)({__name__=~"codeops_.*"}))

# Total active time series for Axiom metrics
count({__name__=~"codeops_.*"})

# Cardinality per label for a specific metric
count by (status)(codeops_step_total)
count by (repo)(codeops_step_total)
count by (work_item_id)(codeops_step_total)
```

---

## 11. Migration: Log-Derived to Direct Instrumentation

Axiom v1 uses log-derived metrics. Post-v1 adds direct Prometheus instrumentation.
The migration path:

### Phase 1: Log-Derived Only (v1)

- Metrics computed from structured log events in the event stream.
- No Prometheus client library required.
- Monitoring via log aggregation queries (CloudWatch Insights, Kibana, Loki).

### Phase 2: Dual Emission (transition)

- Add `prometheus_client` to the repo runner and controller.
- Emit metrics via both log events AND Prometheus `/metrics`.
- Compare values to validate consistency.
- Dashboards and alerts migrate from log queries to PromQL.

### Phase 3: Direct Instrumentation (post-v1)

- Prometheus `/metrics` is the primary metrics source.
- Log events continue for audit trail and correlation.
- Recording rules and alert rules reference Prometheus metrics.
- Log-derived metric computation can be removed once Prometheus is stable.

**Key invariant**: The metric names, types, and label schemas are identical across all
phases. Only the collection mechanism changes. This ensures dashboards and alerts work
without modification during migration.

---

## 12. Cross-Skill References

| Skill | Relationship |
|---|---|
| `alert-engineering-axiom` | Consumes metrics for alert rules. Defines threshold types and routing. |
| `dashboard-design-axiom` | Consumes metrics for dashboard panels. Defines panel types and layout. |
| `distributed-tracing-axiom` | Trace-to-metric correlation. Exemplars link traces to metric samples. |
| `axiom-structured-logging-events` | Source events for log-derived metrics. Field naming alignment. |
| `sre-ops-axiom` | SLO/SLI definitions consume metrics. Error budget calculations. |
| `performance-benchmark-axiom` | Performance budgets reference metric thresholds. |

---

axiom:trace work_item=observability-skills-collection-01 spec=specs/34-Observability-And-Metrics.md,specs/61-Logging-And-Observability-Overhaul.md plan=phase-87-2/task-87-2-4/step-87-2-4-1 impl=.opencode/skills/metrics-instrumentation-axiom/SKILL.md test= doc= ops= prompt= evidence= commit=
