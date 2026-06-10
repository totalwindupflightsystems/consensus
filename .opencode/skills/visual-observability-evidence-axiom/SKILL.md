---
name: visual-observability-evidence-axiom
description: >
  Visual observability evidence guidance for Axiom-managed systems. Covers
  screenshot-as-evidence capture, flame graph interpretation, trace waterfall
  reading, heatmap analysis, log-panel reading, annotation/communication patterns,
  and Axiom evidence-bundle integration. Load this skill when capturing visual
  diagnostic evidence from dashboards, profiling tools, or tracing UIs for
  inclusion in Axiom evidence bundles.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-03-20"
  primary_specs:
    - specs/27-Evidence-Bundle-Schema.md
    - specs/34-Observability-And-Metrics.md
  secondary_specs:
    - specs/65-Diagnostic-Workflows.md
    - specs/25-Structured-Logging-Events.md
    - specs/61-Logging-And-Observability-Overhaul.md
  codeops_trace: >
    axiom:trace work_item=observability-skills-collection-01
    spec=specs/27-Evidence-Bundle-Schema.md,specs/34-Observability-And-Metrics.md,specs/65-Diagnostic-Workflows.md
    plan=phase-87-3/task-87-3-3/step-87-3-3-1
tags:
  vertical: [sre]
  category: observability
  core: false
---

# Visual Observability Evidence Skill (Portable)

> **"A screenshot without context is noise. A screenshot with time range, filters, and annotations is evidence."**

This skill defines how to capture, annotate, and integrate visual observability evidence into Axiom evidence bundles. It covers dashboards, flame graphs, trace waterfalls, heatmaps, and log panels.

**When to load this skill**: When you need to capture visual evidence from observability tools (Grafana, Jaeger, Chrome DevTools, profiling tools) for inclusion in a Axiom evidence bundle, or when interpreting visual diagnostic artifacts during incident investigation.

**Spec grounding**:
- `specs/27-Evidence-Bundle-Schema.md` — VAL-013 (screenshots, logs, benchmark results as additional evidence artifacts), REQ-EVIDENCE-INTEGRITY-001 (integrity hash)
- `specs/34-Observability-And-Metrics.md` — Dashboard panels (Repo Runner panels 1-11, Control Plane panels 1-6), metric naming, alert table
- `specs/65-Diagnostic-Workflows.md` — REQ-DIAG-070 through REQ-DIAG-080 (visual diagnosis requirements: dashboard reading, flame graphs, trace waterfalls, heatmaps, evidence capture)

---

## 1. Core Principle: Evidence, Not Decoration

Visual artifacts in Axiom are **evidence** — they must meet the same bar as test output or command transcripts. Every visual artifact must answer:

1. **What does it show?** (metric, trace, profile, log panel)
2. **When?** (exact time range with start/end timestamps)
3. **Where?** (which service, component, environment)
4. **What filters are active?** (labels, query parameters, search terms)
5. **What is the observation?** (annotation pointing to the relevant signal)

An unannotated screenshot with no time context is not evidence — it is noise.

---

## 2. Screenshot-as-Evidence

### 2.1 When Screenshots Are Required

Screenshots are required evidence when:

| Situation | Why a Screenshot |
|---|---|
| Dashboard anomaly during incident investigation | Captures the visual pattern (spike, drop, bimodal distribution) that triggered the investigation |
| UI verification for frontend work | Proves the rendered UI matches the spec (per `specs/35-Web-UI-Dashboard.md` if applicable) |
| Trace waterfall showing latency breakdown | Captures the span hierarchy and timing that text alone cannot convey |
| Flame graph showing hot path | Captures the visual call-stack proportion that identifies the bottleneck |
| Heatmap showing distribution shift | Captures the color-density pattern that reveals bimodal or shifted latency |
| Alert firing state in alert manager | Captures the active alert with its condition, severity, and duration |

Screenshots are NOT required when:

- The same information is fully captured by a command transcript or query result
- The observation is a single numeric value (use the value directly)
- The tool supports JSON/CSV export that captures the data more precisely

### 2.2 Screenshot Capture Requirements

Per `specs/65-Diagnostic-Workflows.md` REQ-DIAG-080, every screenshot MUST include:

| Requirement | How to Satisfy |
|---|---|
| **Time context** | Ensure the dashboard/tool time picker is visible in the screenshot, OR annotate with exact start/end timestamps |
| **Filters applied** | Ensure filter bars are visible, OR list all active filters in the annotation |
| **Annotations** | Add arrows, circles, or text callouts pointing to the relevant observation |
| **Format** | Save as PNG with descriptive filename |

### 2.3 Filename Convention

```
<type>_<component>_<observation>_<timestamp>.png
```

Examples:
- `dashboard_step-duration_p95-spike_2026-03-15T14-32Z.png`
- `flamegraph_opencode-request_hot-path_2026-03-15T14-45Z.png`
- `trace_waterfall_slow-verification_2026-03-15T14-38Z.png`
- `heatmap_latency_bimodal-shift_2026-03-15T15-00Z.png`
- `logpanel_error-burst_escalation-events_2026-03-15T14-35Z.png`
- `ui_health-endpoint_200-ok_2026-03-15T16-00Z.png`

### 2.4 Screenshot Storage Location

Visual evidence is stored alongside other evidence artifacts:

```
.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/
  verification.md          # References the screenshot
  outputs.md               # References if screenshot was shared externally
  screenshots/             # Directory for visual evidence
    dashboard_step-duration_p95-spike_2026-03-15T14-32Z.png
    flamegraph_opencode-request_hot-path_2026-03-15T14-45Z.png
```

For diagnostic investigations (per `specs/65-Diagnostic-Workflows.md` REQ-DIAG-090):

```
.memory-bank/work-items/<WORK_ITEM_ID>/diagnostics/
  diagnostic-2026-03-15T14-32-00Z.md    # References screenshots
  screenshots/
    dashboard_error-rate_deploy-correlation_2026-03-15T14-32Z.png
```

### 2.5 Referencing Screenshots in Evidence Bundles

In `verification.md`, reference screenshots in the Checks Executed or Acceptance Criteria Coverage tables:

```markdown
## Checks Executed

| Check | Command | Result | Duration | Output Excerpt |
|-------|---------|--------|----------|----------------|
| Dashboard latency check | Visual: Grafana Step Duration panel | pass | -- | p95 < 300s; screenshot at `screenshots/dashboard_step-duration_normal_2026-03-15T16-00Z.png` |
```

In diagnostic evidence records, reference screenshots in the `signals_consulted` field:

```yaml
signals_consulted:
  - metrics: "codeops_step_duration_seconds filtered by repo=acme/api"
  - dashboard_screenshot: "screenshots/dashboard_step-duration_p95-spike_2026-03-15T14-32Z.png"
  - trace_screenshot: "screenshots/trace_waterfall_slow-step_2026-03-15T14-38Z.png"
```

---

## 3. Flame Graph Interpretation

### 3.1 What Flame Graphs Show

A flame graph visualizes call-stack sampling data. Each horizontal bar represents a function; its width is proportional to the time (wall-clock or CPU) spent in that function and its callees. The y-axis represents stack depth.

**Key reading rules**:

| Visual Pattern | Interpretation |
|---|---|
| Wide bar at the top of the stack | This function (and its callees) consumes a large proportion of total time — it is a **hot path** |
| Wide bar at the bottom | The root caller is expected to be wide; focus on the widest bars higher in the stack |
| Narrow "tower" (deep, narrow stack) | A deeply nested call path that does not consume much total time — usually not the bottleneck |
| Plateau (wide bar with no children) | This function itself (not its callees) is consuming time — likely CPU-bound work or I/O wait |
| Multiple wide bars at the same depth | Multiple independent hot paths — optimization may need to address several areas |

### 3.2 Flame Graph Evidence Capture

When capturing a flame graph as evidence:

1. **Capture the full graph** — do not crop to a single subtree unless the full graph is also provided for context.
2. **Annotate the hot path** — circle or highlight the widest bar(s) that represent the bottleneck.
3. **Record the sampling parameters**:
   - Sampling rate (e.g., 99 Hz)
   - Duration of the profile (e.g., 30 seconds)
   - Profile type (CPU, wall-clock, memory allocation, off-CPU)
4. **Record the environment**: which component, which pod/container, which workload was active.
5. **Compare with baseline** when possible: capture a "before" flame graph from a known-good state and a "during-issue" flame graph. Place them side by side or use differential flame graphs.

### 3.3 Flame Graph Comparison Patterns

| Comparison Type | How to Capture | What It Shows |
|---|---|---|
| **Before/After** | Two flame graphs from different time periods | Whether a change (deploy, config, load) shifted the hot path |
| **Differential** | A single diff flame graph (red = grew, blue = shrank) | Which functions gained or lost time relative to baseline |
| **Cross-component** | Flame graphs from different services for the same time window | Whether the bottleneck is in the caller or the callee |

### 3.4 Flame Graph Anti-Patterns

- **Capturing only the zoomed-in subtree**: Loses context about what proportion of total time the subtree represents.
- **No sampling parameters recorded**: Without knowing the sampling rate and duration, the graph cannot be compared to baselines.
- **Confusing CPU flame graphs with wall-clock**: CPU flame graphs miss I/O wait time; wall-clock flame graphs include it. Use the right type for the investigation.

Spec ref: `specs/65-Diagnostic-Workflows.md` REQ-DIAG-071

---

## 4. Trace Waterfall Interpretation

### 4.1 What Trace Waterfalls Show

A trace waterfall displays the span hierarchy of a distributed trace. Each horizontal bar represents a span; its position on the x-axis shows when it started and ended; its indentation shows parent-child relationships.

The Axiom span hierarchy (from `specs/34-Observability-And-Metrics.md`):

```
axiom.run (root)
  axiom.phase
    axiom.task
      axiom.step
        axiom.verification
        axiom.opencode_request
```

### 4.2 Reading the Waterfall

| Visual Pattern | Interpretation |
|---|---|
| One span much wider than siblings | That span is the bottleneck — drill into its children |
| Long gap between parent start and first child start | Setup/initialization overhead in the parent before delegating to children |
| Long gap between last child end and parent end | Teardown/cleanup overhead in the parent after children complete |
| Many short sequential children | Serial execution — potential parallelization opportunity |
| Overlapping children | Parallel execution — good for throughput but check for resource contention |
| Span with error status (red) | Failed operation — check span attributes and associated log events |
| Span with no children but long duration | Leaf operation consuming time — likely an external call (API, DB, I/O) |

### 4.3 Trace Waterfall Evidence Capture

When capturing a trace waterfall as evidence:

1. **Capture the full trace** — show the root span through all leaf spans.
2. **Annotate the slow/error span** — highlight the span that is the focus of investigation.
3. **Record the trace ID** — include the `trace_id` (or `correlation_id` in v1) so the trace can be re-queried.
4. **Record span attributes** for the annotated span:
   - `step_id`, `command`, `status` (for `axiom.step` spans)
   - `verifier_type`, `result`, `score` (for `axiom.verification` spans)
   - `correlation_id`, `http_status`, `duration_ms` (for `axiom.opencode_request` spans)
5. **Correlate with logs** — note the log events associated with the annotated span (filter by `trace_id` or `correlation_id`).

### 4.4 Trace Waterfall Correlation Queries

Per `specs/65-Diagnostic-Workflows.md` REQ-DIAG-021:

| From | To | Query Pattern |
|---|---|---|
| Alert (metric condition) | Trace | Filter traces by alert time window + metric labels (`work_item_id`, `repo`) |
| Trace | Logs | Filter logs by `trace_id` (post-v1) or `correlation_id` (v1) |
| Slow span | Metrics | Query `codeops_step_duration_seconds` or `codeops_opencode_request_duration_seconds` for the same `step_id`/`command` and time window |

### 4.5 Trace Waterfall Anti-Patterns

- **Capturing only the summary view**: The summary (total duration, span count) is not evidence — the waterfall showing the span hierarchy is.
- **Missing trace ID**: Without the trace ID, the evidence cannot be independently verified by re-querying.
- **No log correlation**: A trace waterfall without correlated log events misses the "why" behind the timing.

Spec ref: `specs/65-Diagnostic-Workflows.md` REQ-DIAG-072, `specs/34-Observability-And-Metrics.md` (span hierarchy)

---

## 5. Heatmap Interpretation

### 5.1 What Heatmaps Show

A latency heatmap displays the distribution of request durations over time. The x-axis is time, the y-axis is latency (or another metric dimension), and color intensity represents the count of requests in each time-latency bucket.

Heatmaps are generated from Prometheus histogram metrics (e.g., `codeops_step_duration_seconds`, `codeops_opencode_request_duration_seconds`).

### 5.2 Reading the Heatmap

| Visual Pattern | Interpretation |
|---|---|
| Single dense band | Most requests have similar latency — normal, unimodal distribution |
| Two dense bands (bimodal) | Two populations of requests with different latencies — investigate what differentiates them (different step types, different backends, cache hit vs miss) |
| Band shifting upward over time | Latency is gradually increasing — potential resource exhaustion, memory leak, or growing workload |
| Sudden band jump | Abrupt latency change — correlate with deployment, config change, or external dependency event |
| Scattered dots above the main band | Outlier requests — check for specific correlation IDs, error conditions, or resource contention |
| Dense band disappearing | Requests stopped — check for service outage, deployment, or traffic routing change |

### 5.3 Heatmap Evidence Capture

When capturing a heatmap as evidence:

1. **Capture the full time range** — show enough history to see the transition from normal to anomalous.
2. **Annotate the anomaly** — mark the time and latency range where the pattern changed.
3. **Record the metric and bucket configuration**:
   - Which metric (e.g., `codeops_step_duration_seconds`)
   - Histogram bucket boundaries (e.g., `[0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600]`)
   - Any active filters (labels)
4. **Correlate with events** — overlay deployment markers, alert timestamps, or config change timestamps on the heatmap if the tool supports it.

### 5.4 Heatmap Anti-Patterns

- **Too-narrow time range**: Showing only the anomalous period without baseline makes it impossible to assess severity.
- **Missing bucket configuration**: Without knowing the histogram buckets, the visual resolution of the heatmap cannot be assessed.
- **Confusing count intensity with latency**: Dark color means more requests at that latency, not higher latency. A dark band at low latency is good.

Spec ref: `specs/65-Diagnostic-Workflows.md` REQ-DIAG-073

---

## 6. Log Panel Reading

### 6.1 What Log Panels Show

Log panels display structured log events filtered by time range, correlation fields, and event types. In Axiom, log events follow the schema in `specs/25-Structured-Logging-Events.md` with JSON Lines format.

### 6.2 Effective Log Panel Reading

| Technique | How | Why |
|---|---|---|
| **Filter by correlation ID** | Use `correlation_id` (v1) or `trace_id` (post-v1) to isolate a single request path | Eliminates noise from concurrent requests |
| **Filter by event type** | Use `event_type` to focus on specific lifecycle events (e.g., `step_completed`, `verification_failed`, `escalation_triggered`) | Narrows to the relevant signal |
| **Filter by severity** | Use `level` to focus on `error` and `warn` events first | Prioritizes actionable signals |
| **Time-window alignment** | Set the log panel time range to match the metric anomaly window ± alignment buffer (per REQ-DIAG-022: ±5s for clock skew, +scrape interval for metrics lag) | Ensures log events correspond to the observed metric behavior |
| **Context expansion** | Once an error event is found, expand the time window slightly to see preceding events | Captures the causal chain leading to the error |

### 6.3 Log Panel Evidence Capture

When capturing log panel output as evidence:

1. **Prefer structured export over screenshots** — export as JSON or CSV when the tool supports it. This is more precise and machine-parseable.
2. **If screenshot is necessary** — ensure the time range, active filters, and relevant log lines are all visible.
3. **Redact secrets** — per `specs/27-Evidence-Bundle-Schema.md` VAL-005 and `specs/25-Structured-Logging-Events.md` redaction rules, ensure no secrets, tokens, or PII appear in captured log output. If the log aggregation tool does not redact, manually redact before including in evidence.
4. **Record the query** — include the exact query used to filter the logs (e.g., CloudWatch Logs Insights query, Elasticsearch query, Loki LogQL).

### 6.4 Log Panel Anti-Patterns

- **Unfiltered log dump**: Including thousands of unfiltered log lines is not evidence — it is noise. Always filter to the relevant correlation ID, event type, or time window.
- **Missing query**: Without the query, the evidence cannot be reproduced or verified.
- **Secrets in log output**: Log events should already be redacted at emission time (per `specs/25`), but log aggregation tools may capture pre-redaction output in some edge cases. Always verify before including in evidence.

---

## 7. Annotation and Communication Patterns

### 7.1 Annotation Purpose

Annotations transform raw visual artifacts into evidence by marking the specific observation that supports a diagnostic hypothesis or verification claim.

### 7.2 Annotation Types

| Type | When to Use | How |
|---|---|---|
| **Arrow + label** | Pointing to a specific data point (spike, drop, error) | Draw an arrow to the exact point; label with the observation (e.g., "p95 jumped from 2s to 45s at 14:32 UTC") |
| **Highlighted region** | Marking a time range or value range | Draw a rectangle or shaded region; label with the range (e.g., "Anomalous period: 14:30-14:45 UTC") |
| **Comparison markers** | Showing before/after or baseline/anomaly | Use vertical lines or color-coded regions to separate the baseline period from the anomalous period |
| **Callout box** | Providing context that is not visible in the tool | Add a text box with the observation, hypothesis, or conclusion |
| **Deployment marker** | Correlating with a deployment or config change | Add a vertical line at the deployment timestamp with a label (e.g., "Deploy v2.3.1 at 14:28 UTC") |

### 7.3 Annotation Rules

1. **Be specific**: "Latency increased" is not an annotation. "p95 latency increased from 2.1s to 45.3s at 14:32 UTC" is.
2. **Include units**: Always include units (seconds, bytes, requests/second, percentage).
3. **Reference the metric**: Name the exact metric or signal being annotated (e.g., `codeops_step_duration_seconds{step_id="verify"}` p95).
4. **Timestamp everything**: Every annotation must include the timestamp or time range it refers to.
5. **Separate observation from interpretation**: The annotation states what is observed; the diagnostic record states what it means.

### 7.4 Communication Patterns

When sharing visual evidence in Jira comments, PR descriptions, or Slack:

| Context | Format | Content |
|---|---|---|
| **Jira comment** | Screenshot + 2-3 sentence summary | "Step duration p95 spiked to 45s at 14:32 UTC (see attached). Correlated with model API degradation. Mitigation: switched to fallback provider." |
| **PR description** | Screenshot path + observation | "Dashboard verification: `screenshots/dashboard_step-duration_normal_2026-03-15T16-00Z.png` — p95 < 300s after fix." |
| **Incident channel** | Screenshot + time range + hypothesis | "14:30-14:45 UTC: bimodal latency distribution appeared (see heatmap). Hypothesis: cache miss storm after deploy." |
| **Evidence bundle** | Screenshot path + structured reference in verification.md | Per Section 2.5 of this skill |

---

## 8. Dashboard Panel Reading Guide

### 8.1 Axiom Repo Runner Dashboard Panels

Per `specs/34-Observability-And-Metrics.md`, the Repo Runner dashboard has 11 panels. This section provides reading guidance for each.

| Panel # | Name | Metric | What to Look For |
|---|---|---|---|
| 1 | Step Duration | `codeops_step_duration_seconds` | p95 trends; sudden spikes; correlation with specific `step_id` or `command` values |
| 2 | Verification Pass Rate | `codeops_verification_result_total` | Rate drops below 70% (warning threshold); correlation with specific `verifier_type` |
| 3 | Confidence Score | `codeops_confidence_score` | Score trends per signal; signals that consistently score low indicate systemic issues |
| 4 | Plan Progress | `codeops_step_total` / plan total | Stalled progress (flat line); rapid completion (verify quality) |
| 5 | OpenCode Request Duration | `codeops_opencode_request_duration_seconds` | p95 trends; correlation with model provider status; token count impact |
| 6 | Token Usage | `codeops_opencode_token_usage_total` | Cost trends; prompt vs completion ratio; budget approach/exhaustion |
| 7 | Retry and Escalation Rate | `codeops_retry_total`, `codeops_escalation_total` | Escalation spikes; retry storms; correlation with specific commands |
| 8 | Snapshot Capture/Restore Rate | `codeops_snapshot_capture_total`, `codeops_snapshot_restore_total` | Failure rate increase; correlation with S3 issues |
| 9 | Snapshot Size | `codeops_snapshot_size_bytes` | Growth trends; threshold approach (3 GB warning, 5 GB critical) |
| 10 | Snapshot GC Deletions | `codeops_snapshot_gc_deleted_total` | GC not running (zero deletions despite captures); sudden spike (catch-up GC) |
| 11 | Snapshot Checksum Mismatches | `codeops_snapshot_checksum_mismatch_total` | Any non-zero value is critical — indicates data corruption |

### 8.2 Axiom Control Plane Dashboard Panels

| Panel # | Name | Metric | What to Look For |
|---|---|---|---|
| 1 | Work Item Throughput | `codeops_work_items_enqueued_total`, `codeops_work_items_completed_total` | Enqueue/complete imbalance (growing backlog); throughput drops |
| 2 | Pod Lifecycle | `codeops_pods_created_total`, `codeops_pods_failed_total` | Failure rate by reason; OOM kills; crash loops |
| 3 | Queue Depth | `codeops_queue_depth` | Sustained depth > 50 (alert threshold); sudden spikes |
| 4 | Pod Startup Latency | `codeops_pod_startup_duration_seconds` | p95 trends; image pull delays; scheduling delays |
| 5 | Active Pods | `codeops_active_pods` | Capacity utilization; unexpected drops (mass eviction) |
| 6 | Resource Usage | Pod CPU/memory from cAdvisor | Monotonic memory growth (leak); CPU saturation |

### 8.3 Scenario-to-Panel Mapping

Per `specs/65-Diagnostic-Workflows.md` REQ-DIAG-070:

| Diagnostic Scenario | Primary Panels | Secondary Panels |
|---|---|---|
| Latency spike (REQ-DIAG-040) | Panel 1 (Step Duration), Panel 5 (OpenCode Request Duration) | Panel 6 (Token Usage) |
| Error rate after deploy (REQ-DIAG-041) | Panel 2 (Verification Pass Rate), Panel 7 (Retry/Escalation) | Panel 1 (Step Duration) |
| Memory leak (REQ-DIAG-042) | CP Panel 6 (Resource Usage), CP Panel 5 (Active Pods) | CP Panel 2 (Pod Lifecycle) |
| Cascading failure (REQ-DIAG-043) | All panels — full dashboard view | Alert timeline |
| Resource exhaustion (REQ-DIAG-045) | CP Panel 6 (Resource Usage), CP Panel 3 (Queue Depth) | CP Panel 5 (Active Pods) |
| AI latency degradation (REQ-DIAG-050) | Panel 5 (OpenCode Request Duration), Panel 1 (Step Duration) | Panel 6 (Token Usage) |
| Token budget exhaustion (REQ-DIAG-051) | Panel 6 (Token Usage), Panel 3 (Confidence Score) | Panel 7 (Retry/Escalation) |

---

## 9. Evidence Bundle Integration

### 9.1 How Visual Evidence Fits the Bundle Schema

Per `specs/27-Evidence-Bundle-Schema.md`:

- **VAL-013** (MAY): "Include screenshots, logs, or benchmark results as additional evidence artifacts." This skill elevates screenshots from optional to recommended when the observation is inherently visual.
- **VAL-003** (MUST): "Checks Executed lists actual commands run with real output excerpts." For visual checks, the "command" is the dashboard/tool query, and the "output excerpt" is a reference to the screenshot file.
- **VAL-005** (MUST): "No secrets, tokens, API keys, or PII appear anywhere in the evidence bundle." This applies to screenshots — redact any visible secrets before saving.

### 9.2 Verification.md Integration Pattern

```markdown
## Checks Executed

| Check | Command | Result | Duration | Output Excerpt |
|-------|---------|--------|----------|----------------|
| Dashboard health | Visual: Grafana Repo Runner dashboard, panels 1-7 | pass | -- | All metrics within normal ranges; screenshot at `screenshots/dashboard_all-panels_normal_2026-03-15T16-00Z.png` |
| Trace waterfall | Visual: Jaeger trace for correlation_id=abc-123 | pass | -- | All spans < 10s; no errors; screenshot at `screenshots/trace_waterfall_normal_2026-03-15T16-05Z.png` |
```

### 9.3 Diagnostic Evidence Integration Pattern

Per `specs/65-Diagnostic-Workflows.md` REQ-DIAG-090 through REQ-DIAG-092:

```yaml
---
type: diagnostic
timestamp: "2026-03-15T14:32:00Z"
trigger: "Alert: CodeOpsStepDurationHigh"
severity: P2
signals_consulted:
  - metrics: "codeops_step_duration_seconds filtered by repo=acme/api"
  - dashboard_screenshot: "screenshots/dashboard_step-duration_p95-spike_2026-03-15T14-32Z.png"
  - trace_screenshot: "screenshots/trace_waterfall_slow-verification_2026-03-15T14-38Z.png"
  - flamegraph: "screenshots/flamegraph_opencode-request_hot-path_2026-03-15T14-45Z.png"
hypotheses_tested:
  - hypothesis: "Model API latency degradation"
    evidence: "OpenCode request p95 increased from 8s to 45s (see dashboard screenshot). Trace waterfall confirms 42s spent in axiom.opencode_request span."
    result: confirmed
root_cause: "Upstream model API degradation"
confidence: high
mitigation_applied: "Switched to fallback model provider"
verification_result: resolved
---
```

### 9.4 Integrity and Immutability

- Screenshots stored in `runs/<RUN_ID>/screenshots/` are **immutable** once the run completes (per `specs/27-Evidence-Bundle-Schema.md` lifecycle rules).
- The `integrity_hash` in `verification.md` covers the markdown body, not the screenshot files themselves. However, screenshot filenames referenced in the markdown body are part of the integrity-protected content.
- If a screenshot needs correction (e.g., secrets were visible), create a new run with the corrected screenshot — do not modify the immutable run.

---

## 10. Chrome DevTools MCP Integration

When using the `chrome-devtools` MCP server for browser-based visual evidence capture:

### 10.1 Available Tools for Evidence Capture

| Tool | Use Case |
|---|---|
| `take_screenshot` | Capture full-page or element-specific screenshots of dashboards, UIs, or tool outputs |
| `take_snapshot` | Capture the accessibility tree for DOM-based evidence (complements visual screenshots) |
| `list_console_messages` | Capture console errors/warnings as supplementary evidence |
| `list_network_requests` | Capture network activity (API calls, SSE connections) as supplementary evidence |
| `evaluate_script` | Extract specific data points from the page for structured evidence |

### 10.2 Evidence Capture Workflow (Chrome DevTools)

1. **Navigate** to the dashboard or tool URL using `navigate_page`.
2. **Wait** for data to load using `wait_for` with expected text/elements.
3. **Set time range** — use `click` and `fill` to set the dashboard time picker to the investigation window.
4. **Apply filters** — use `click` and `fill` to set label filters (e.g., `work_item_id`, `repo`).
5. **Capture screenshot** — use `take_screenshot` with `filePath` pointing to the evidence directory.
6. **Capture supplementary data** — use `evaluate_script` to extract metric values, `list_network_requests` to capture API calls.
7. **Annotate** — if the tool does not support in-tool annotations, note the observation in the evidence record and reference the screenshot.

### 10.3 Chrome DevTools Anti-Patterns

- **Capturing before data loads**: Always use `wait_for` to ensure the dashboard has rendered data before capturing.
- **Missing time range setup**: The default time range may not match the investigation window — always set it explicitly.
- **Capturing at default viewport size**: Use `resize_page` to ensure the dashboard is fully visible without scrolling when possible.

---

## 11. Checklist: Visual Evidence Quality Gate

Before including visual evidence in an evidence bundle, verify:

- [ ] **Time context present**: Time range is visible in the screenshot or documented in the annotation
- [ ] **Filters documented**: All active filters are visible or listed
- [ ] **Annotation present**: The specific observation is marked (arrow, highlight, callout)
- [ ] **Filename follows convention**: `<type>_<component>_<observation>_<timestamp>.png`
- [ ] **Stored in correct location**: `runs/<RUN_ID>/screenshots/` or `diagnostics/screenshots/`
- [ ] **Referenced in verification.md or diagnostic record**: The screenshot path appears in the evidence markdown
- [ ] **No secrets visible**: No tokens, API keys, passwords, or PII in the screenshot
- [ ] **Comparison baseline provided** (when applicable): Before/after or baseline/anomaly screenshots are paired
- [ ] **Query recorded** (for log panels): The exact query used to produce the view is documented
- [ ] **Sampling parameters recorded** (for flame graphs): Rate, duration, and profile type are documented

---

## 12. Quick Reference: Visual Evidence by Diagnostic Phase

Per `specs/65-Diagnostic-Workflows.md` REQ-DIAG-001 through REQ-DIAG-009:

| Diagnostic Phase | Typical Visual Evidence |
|---|---|
| 1. Detect | Alert screenshot showing firing condition and severity |
| 2. Triage | Service health dashboard overview screenshot |
| 3. Scope | Dashboard with time range aligned to incident window; error rate and latency panels |
| 4. Hypothesize | (Usually text-based — no visual evidence typical) |
| 5. Investigate | Trace waterfalls, flame graphs, heatmaps, filtered log panels |
| 6. Confirm | Side-by-side comparison: anomalous vs baseline dashboards/traces |
| 7. Mitigate | (Usually command-based — no visual evidence typical) |
| 8. Verify | Post-mitigation dashboard showing return to baseline; same panels as Scope phase |

---

## 13. Anti-Pattern Summary

| Anti-Pattern | Why It Fails | Fix |
|---|---|---|
| Screenshot with no time range | Cannot be correlated with other signals or reproduced | Always include visible time picker or annotated timestamps |
| Screenshot with no filters shown | Cannot determine what subset of data is displayed | Show filter bars or document filters in annotation |
| Unannotated screenshot | Reviewer must guess what the observation is | Add arrows, highlights, or callout boxes |
| Cropped flame graph | Loses proportion context (how much of total time?) | Capture full graph; annotate the hot path |
| Trace summary instead of waterfall | Loses the span hierarchy and timing relationships | Capture the full waterfall view |
| Heatmap with too-narrow time range | Cannot distinguish anomaly from normal variation | Show enough baseline to establish the normal pattern |
| Unfiltered log dump | Noise overwhelms signal | Filter by correlation ID, event type, or severity first |
| Secrets visible in screenshot | Violates VAL-005 and security policy | Redact before saving; verify before including in evidence |
| Screenshot stored outside evidence directory | Not discoverable; not covered by evidence lifecycle rules | Store in `runs/<RUN_ID>/screenshots/` or `diagnostics/screenshots/` |
| No query recorded for log panel | Evidence cannot be reproduced or verified | Include the exact query in the evidence record |
