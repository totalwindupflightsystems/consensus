---
name: dashboard-design-axiom
description: >
  Dashboard design principles, panel types, layout patterns, tool-specific guidance,
  dashboard-as-code practices, and anti-patterns for observability dashboards. Load this
  skill when designing, reviewing, or implementing operational dashboards for any service
  managed by Axiom. Grounded in specs/34-Observability-And-Metrics.md#dashboard-expectations.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-03-20"
  primary_spec: specs/34-Observability-And-Metrics.md
  secondary_specs:
    - specs/25-Structured-Logging-Events.md
    - specs/61-Logging-And-Observability-Overhaul.md
    - specs/34-Observability-And-Metrics.md
tags:
  vertical: [sre, ops]
  category: observability
  core: false
---

# Dashboard Design Skill (Portable)

> **"A dashboard that requires explanation is a dashboard that failed."**
>
> **"Overview first, zoom and filter, then details on demand."** -- Shneiderman's mantra

This skill provides portable, production-grade guidance for designing operational dashboards
that surface the right signals at the right time. It covers panel types, layout patterns,
filter design, tool-specific guidance, dashboard-as-code practices, and common anti-patterns.

**Spec grounding**: The Axiom dashboard panel sets, required filters, and layout expectations
are defined in `specs/34-Observability-And-Metrics.md#dashboard-expectations`. This skill
encodes those requirements as reusable design patterns applicable to any observability stack.

---

## Activation

Load this skill when:
- Designing a new operational dashboard (Grafana, Datadog, CloudWatch, etc.)
- Reviewing an existing dashboard for completeness or usability
- Implementing dashboard-as-code (JSON/YAML/Terraform/Pulumi dashboard definitions)
- Translating spec-defined panel sets into concrete dashboard layouts
- Auditing dashboard coverage against alerting rules or SLOs

---

## 1. Dashboard Design Principles

### 1.1 Information Hierarchy

Follow the overview-to-drill-down pattern for every dashboard:

| Layer | Purpose | Typical Panels | Interaction |
|-------|---------|----------------|-------------|
| **Overview** | At-a-glance health; answer "is anything broken?" in < 5 seconds | Status indicators, key gauges, error rate sparklines | Entry point; links to detail |
| **Triage** | Narrow scope to the affected component or dimension | Time series with breakdown by label, top-N tables | Filter by label values |
| **Detail** | Root-cause investigation; raw data access | Histograms, heatmaps, log panels, trace links | Drill-down from triage |

**Rule**: Every dashboard MUST have an overview row. Dashboards that open directly into
detail panels force operators to build mental context before they can act.

### 1.2 The Four Golden Signals

Every service dashboard SHOULD include panels for the four golden signals (per Google SRE):

1. **Latency** -- Distribution of request durations (use histograms, not averages).
2. **Traffic** -- Request rate or throughput (use counters with `rate()` or `irate()`).
3. **Errors** -- Error rate or error ratio (use counters with label filters).
4. **Saturation** -- Resource utilization approaching limits (use gauges with thresholds).

### 1.3 USE and RED Methods

| Method | Scope | Signals |
|--------|-------|---------|
| **USE** (Utilization, Saturation, Errors) | Infrastructure resources (CPU, memory, disk, network) | Gauge + counter panels per resource |
| **RED** (Rate, Errors, Duration) | Request-driven services | Counter rate + error ratio + histogram panels |

Choose USE for infrastructure dashboards, RED for service dashboards, or combine both
for full-stack visibility.

---

## 2. Panel Types and When to Use Them

### 2.1 Panel Selection Guide

| Panel Type | Best For | Avoid When | Example Metric |
|------------|----------|------------|----------------|
| **Time series** | Trends over time; rate changes; correlating events | Single-point-in-time values | `rate(codeops_step_total[5m])` |
| **Stat / Single value** | Current value of a key indicator; at-a-glance health | Showing trends or distributions | `codeops_queue_depth` |
| **Gauge** | Current value relative to a known range or threshold | Unbounded metrics | `codeops_confidence_score` (0-100) |
| **Bar chart** | Comparing categories; showing distribution across labels | Time-based trends | `codeops_step_total` by `status` |
| **Histogram / Heatmap** | Latency distributions; identifying percentile shifts | Small sample sizes | `codeops_step_duration_seconds` |
| **Table** | Top-N lists; detailed breakdowns; sortable dimensions | Real-time monitoring (too dense) | Top 10 slowest steps by p99 |
| **Log panel** | Correlated log lines; event context during investigation | Primary monitoring (use metrics) | Structured log events filtered by `run_id` |
| **Alert list** | Active alerts; recent alert history | Replacing metric panels | Prometheus Alertmanager feed |

### 2.2 Histogram and Percentile Best Practices

- Always show p50, p95, and p99 together. A single percentile hides distribution shape.
- Use `histogram_quantile()` in Prometheus or equivalent native histogram support.
- Set meaningful bucket boundaries aligned to SLO thresholds (e.g., 100ms, 250ms, 500ms, 1s, 5s, 10s, 30s, 60s, 300s, 600s for step durations).
- Avoid `avg()` for latency. Averages hide tail latency and are misleading for skewed distributions.

### 2.3 Counter and Rate Best Practices

- Always apply `rate()` or `increase()` to counters before graphing. Raw counter values are monotonically increasing and not useful as time series.
- Use `rate()` for per-second rates; use `increase()` for absolute counts over a window.
- Match the `rate()` window to at least 4x the scrape interval (e.g., `rate(...[5m])` for 15s scrape).
- For error ratios, compute `rate(errors[5m]) / rate(total[5m])` rather than separate panels.

---

## 3. Axiom Dashboard Panel Sets

These panel sets are defined in `specs/34-Observability-And-Metrics.md#dashboard-expectations`
and MUST be implemented when Prometheus-based metrics are available (post-v1).

### 3.1 Repo Runner Dashboard

| # | Panel | Type | Metric(s) | Filters |
|---|-------|------|-----------|---------|
| 1 | Step Duration | Histogram | `codeops_step_duration_seconds` p50/p95/p99 | `repo`, `work_item_id` |
| 2 | Verification Pass Rate | Gauge | `codeops_verification_result_total{result="passed"}` / total | `verifier_type` |
| 3 | Confidence Score | Gauge | `codeops_confidence_score` | `work_item_id`, `signal` |
| 4 | Plan Progress | Bar | `codeops_step_total{status="completed"}` / plan total | `work_item_id` |
| 5 | OpenCode Request Duration | Histogram | `codeops_opencode_request_duration_seconds` p50/p95/p99 | `command` |
| 6 | Token Usage | Counter (stacked) | `codeops_opencode_token_usage_total` | `token_type` |
| 7 | Retry and Escalation Rate | Time series | `codeops_retry_total`, `codeops_escalation_total` | `repo` |
| 8 | Snapshot Capture/Restore Rate | Bar | `codeops_snapshot_capture_total`, `codeops_snapshot_restore_total` | `status` |
| 9 | Snapshot Size | Time series | `codeops_snapshot_size_bytes` | `work_item_id` |
| 10 | Snapshot GC Deletions | Bar | `codeops_snapshot_gc_deleted_total` | -- |
| 11 | Snapshot Checksum Mismatches | Stat (red if > 0) | `codeops_snapshot_checksum_mismatch_total` 24h rolling | -- |

**Required filters**: `work_item_id`, `repo`, `run_id`.

**Layout**: Panels 1-7 form the core runner overview. Panels 8-11 are a collapsible
"Snapshot" row, only populated when `snapshot.enabled: true`.

### 3.2 Control Plane Dashboard

| # | Panel | Type | Metric(s) | Filters |
|---|-------|------|-----------|---------|
| 1 | Work Item Throughput | Time series | `codeops_work_items_enqueued_total`, `codeops_work_items_completed_total` | `repo` |
| 2 | Pod Lifecycle | Time series | `codeops_pods_created_total`, `codeops_pods_failed_total` | `reason` |
| 3 | Queue Depth | Gauge | `codeops_queue_depth` | `queue` |
| 4 | Pod Startup Latency | Histogram | `codeops_pod_startup_duration_seconds` p50/p95/p99 | `repo` |
| 5 | Active Pods | Gauge | `codeops_active_pods` | `repo` |
| 6 | Resource Usage | Time series | Pod memory/CPU (Kubernetes metrics-server) | `namespace` |

**Required filters**: `repo`, `namespace`.

---

## 4. Layout Patterns

### 4.1 Row-Based Organization

Organize dashboards into collapsible rows, each representing a logical group:

```
Row 1: Overview (status indicators, key gauges)
Row 2: Latency & Throughput (time series, histograms)
Row 3: Errors & Failures (error rates, failure breakdowns)
Row 4: Resources & Saturation (CPU, memory, disk, queue depth)
Row 5: Dependencies (upstream/downstream health, external API latency)
Row 6: [Collapsible] Detailed Breakdowns (tables, top-N, per-label drilldowns)
```

### 4.2 Consistent Time Range and Refresh

- Default time range: 1 hour (short enough for incident response, long enough for context).
- Default refresh: 30 seconds for operational dashboards; 5 minutes for capacity dashboards.
- All panels on a dashboard MUST share the same time range. Mixed time ranges cause confusion.

### 4.3 Annotation Layers

Add annotation layers for key events that affect metric interpretation:
- **Deployments**: Mark deploy timestamps so metric changes can be correlated.
- **Alerts**: Show when alerts fired and resolved.
- **Config changes**: Mark configuration rollouts.
- **Incidents**: Show incident start/end windows.

### 4.4 Variable Templates (Grafana)

Use dashboard variables (template variables) for:
- `$repo` -- Filter all panels by repository.
- `$work_item_id` -- Filter by work item.
- `$namespace` -- Filter by Kubernetes namespace.
- `$interval` -- Dynamic aggregation interval (e.g., `$__rate_interval`).

Variables MUST cascade: selecting `$repo` should filter the available values for
`$work_item_id` to only those work items in that repo.

---

## 5. Filter Design

### 5.1 Required Filters per Dashboard Type

| Dashboard | Required Filters | Optional Filters |
|-----------|-----------------|------------------|
| Repo Runner | `work_item_id`, `repo`, `run_id` | `step_id`, `command`, `verifier_type` |
| Control Plane | `repo`, `namespace` | `queue`, `reason` |
| Cost/Analytics | `repo`, `work_item_id` | `token_type`, `model` |

### 5.2 Filter Placement

- Place filters at the top of the dashboard, visible without scrolling.
- Use multi-select where appropriate (e.g., multiple repos).
- Include an "All" option as the default for each filter.
- Filters MUST apply to all panels on the dashboard. Panels that ignore a filter confuse operators.

---

## 6. Dashboard-as-Code

### 6.1 Principles

- **Version control**: Dashboard definitions MUST live in the repository, not only in the UI.
- **Reproducibility**: Any dashboard MUST be recreatable from its code definition alone.
- **Review**: Dashboard changes go through the same PR review process as code changes.
- **Idempotency**: Applying the same definition twice produces the same result.

### 6.2 File Locations

| Tool | Format | Location |
|------|--------|----------|
| Grafana | JSON (provisioning format) | `.axiom/dashboards/<name>.json` |
| Grafana (Grafonnet) | Jsonnet | `.axiom/dashboards/<name>.jsonnet` |
| Datadog | Terraform HCL | `infra/monitoring/dashboards/<name>.tf` |
| CloudWatch | JSON (CloudFormation) | `infra/monitoring/dashboards/<name>.json` |

### 6.3 Grafana-Specific Guidance

- Use Grafana provisioning format (JSON with `__inputs` and `__requires` for portability).
- Pin the Grafana schema version in the JSON (`schemaVersion` field).
- Use `${DS_PROMETHEUS}` as the datasource variable for portability across environments.
- Include `tags` in the dashboard JSON for discoverability (e.g., `["axiom", "repo-runner"]`).
- Set `editable: false` in provisioned dashboards to prevent UI drift from the code definition.

### 6.4 Dashboard Testing

- Validate JSON/Jsonnet syntax in CI.
- Use `grafana-dashboard-linter` or equivalent to check for common issues (missing datasource, unbounded queries, missing units).
- Verify that all referenced metrics exist in the metric catalog (cross-reference with `specs/34-Observability-And-Metrics.md`).

---

## 7. Tool-Specific Guidance

### 7.1 Grafana

- Use mixed datasources sparingly; prefer one datasource per dashboard.
- Set panel units explicitly (`s` for seconds, `bytes` for sizes, `percentunit` for ratios).
- Use `legendFormat` to produce readable legend labels (e.g., `{{repo}} - {{status}}`).
- Enable "Shared crosshair" or "Shared tooltip" for time-correlated investigation.
- Use "Repeat" panels with template variables for per-repo or per-work-item views.

### 7.2 Datadog

- Use template variables for `env`, `service`, and `version` (Unified Service Tagging).
- Group widgets into groups (collapsible sections) matching the row-based layout pattern.
- Use `formula` queries for derived metrics (error ratios, percentiles from distributions).
- Set `live_span` to `1h` for operational dashboards.

### 7.3 CloudWatch

- Use CloudWatch dashboards with metric math for derived values.
- Set period to match the metric resolution (1 minute for standard, 1 second for high-res).
- Use `ANOMALY_DETECTION` bands for baseline comparison.
- Link dashboards to CloudWatch Alarms for alert context.

### 7.4 Prometheus + Console Templates

- For lightweight dashboards without Grafana, use Prometheus console templates.
- Keep console templates in `.axiom/consoles/` with `.html` extension.
- Use `{{ template "head" . }}` and `{{ template "prom_content_head" . }}` for consistent styling.

---

## 8. Anti-Patterns

### 8.1 Dashboard Anti-Patterns (MUST avoid)

| Anti-Pattern | Problem | Fix |
|-------------|---------|-----|
| **Wall of graphs** | Too many panels with no hierarchy; operators cannot find what matters | Apply overview-to-detail hierarchy; collapse non-critical rows |
| **Average-only latency** | Averages hide tail latency; p99 problems invisible | Always show p50, p95, p99 together |
| **Raw counter graphs** | Monotonically increasing lines with no useful shape | Apply `rate()` or `increase()` before graphing |
| **Unbounded label cardinality** | Dashboard queries explode with high-cardinality labels | Use only enumerated label values; aggregate or drop high-cardinality dimensions |
| **No filters** | Operators cannot scope to the affected component | Add required filters per dashboard type (see section 5) |
| **Mixed time ranges** | Panels show different windows; correlation impossible | All panels share the dashboard time range |
| **UI-only dashboards** | Dashboard definitions exist only in the tool UI; no version control | Use dashboard-as-code (see section 6) |
| **Stale dashboards** | Dashboards reference metrics that no longer exist or have changed names | Validate metric references in CI; cross-check with metric catalog |
| **Alert-free dashboards** | Dashboard shows problems but has no linked alerts | Every critical panel should have a corresponding alert rule |
| **Missing units** | Panels show raw numbers without context (is 1000 bytes or seconds?) | Set explicit units on every panel |

### 8.2 Query Anti-Patterns

| Anti-Pattern | Problem | Fix |
|-------------|---------|-----|
| `avg(rate(...))` for latency | Hides distribution; masks tail latency | Use `histogram_quantile()` with explicit percentiles |
| `rate(...[1m])` with 1m scrape | Insufficient samples for accurate rate | Use `rate()` window >= 4x scrape interval |
| `{job=~".*"}` | Matches everything; expensive and noisy | Use explicit label matchers |
| Nested `label_replace()` | Complex, fragile, hard to debug | Simplify metric naming at emission time |
| `count(up == 1)` for health | Conflates "scrape works" with "service healthy" | Use dedicated health metrics or probes |

---

## 9. Dashboard Review Checklist

Use this checklist when reviewing a dashboard (new or modified):

- [ ] **Overview row exists** and answers "is anything broken?" in < 5 seconds
- [ ] **Four golden signals** (or RED/USE equivalent) are represented
- [ ] **Required filters** are present and apply to all panels
- [ ] **Filters cascade** (selecting one narrows the options for dependent filters)
- [ ] **Histograms show percentiles** (p50, p95, p99), not averages
- [ ] **Counters use rate()** or increase(), not raw values
- [ ] **Units are set** on every panel (seconds, bytes, percent, etc.)
- [ ] **Time range is shared** across all panels
- [ ] **Annotation layers** exist for deployments and alerts
- [ ] **Dashboard-as-code** definition exists in the repository
- [ ] **No unbounded label cardinality** in any query
- [ ] **Linked alerts** exist for critical conditions
- [ ] **Collapsible rows** used for detail sections
- [ ] **Legend labels** are readable and use `legendFormat` or equivalent
- [ ] **Datasource** uses a variable (e.g., `${DS_PROMETHEUS}`) for portability

---

## 10. Axiom Integration

### 10.1 Evidence Capture

When dashboards are part of verification evidence:
- Capture a screenshot of the relevant dashboard time range.
- Include the dashboard URL with time range parameters in the evidence bundle.
- Reference the dashboard-as-code file path in `axiom:trace` markers.

### 10.2 Spec Alignment

Dashboard implementations MUST align with the panel sets defined in
`specs/34-Observability-And-Metrics.md#dashboard-expectations`. When auditing:
- Verify every spec-defined panel exists in the dashboard definition.
- Verify required filters are implemented.
- Verify metric names match the metric contract in `specs/34-Observability-And-Metrics.md`.

### 10.3 Related Skills

| Skill | Relationship |
|-------|-------------|
| `alert-engineering-axiom` | Alert rules that trigger from dashboard metrics |
| `metrics-instrumentation-axiom` | Metric emission that feeds dashboard panels |
| `sre-ops-axiom` | SLO/SLI dashboards and error budget tracking |
| `distributed-tracing-axiom` | Trace-linked panels and drill-down from dashboard to traces |
| `observability-diagnosis-axiom` | Using dashboards as part of the diagnostic loop |
| `visual-observability-evidence-axiom` | Capturing dashboard screenshots as evidence |

---

axiom:trace work_item=observability-skills-collection-01 spec=specs/34-Observability-And-Metrics.md plan=phase-87-2/task-87-2-1/step-87-2-1-1 test= doc=.opencode/skills/dashboard-design-axiom/SKILL.md evidence= commit=
