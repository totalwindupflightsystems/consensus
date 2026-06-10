---
name: predictive-observability-axiom
description: >
  Predictive observability guidance for trend analysis, anomaly detection, error budget
  burn-rate forecasting, capacity planning, cost projection, leading-vs-lagging indicator
  frameworks, and predictive alert contracts. Load this skill when designing predictive
  alerts, forecasting resource exhaustion, analyzing burn rates, planning capacity, or
  building anomaly detection for any service managed by Axiom.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-03-20"
  primary_spec: specs/66-Predictive-Observability.md
  secondary_specs:
    - specs/34-Observability-And-Metrics.md
    - specs/61-Logging-And-Observability-Overhaul.md
    - specs/47-Cost-Tracking-And-Session-Analytics.md
    - specs/65-Diagnostic-Workflows.md
    - specs/25-Structured-Logging-Events.md
    - specs/27-Evidence-Bundle-Schema.md
tags:
  vertical: [sre]
  category: observability
  core: false
---

# Predictive Observability Skill (Portable)

> **"Predict the breach, don't just detect it. The best incident is the one that never happens."**
>
> **"A prediction without a confidence interval is a guess. A guess without a recommended action is noise."**

This skill provides portable, production-grade guidance for predicting problems before they
happen — trend analysis, anomaly detection, error budget burn-rate forecasting, capacity
planning, cost projection, and predictive alerting. It is designed to be executable by both
human operators and AI agents.

**Spec grounding**: The predictive observability contract — requirements, invariants, and
acceptance criteria — is defined in `specs/66-Predictive-Observability.md` (REQ-PRED-001
through REQ-PRED-100). This skill operationalizes those requirements into actionable
procedures and portable guidance.

**Companion skills**: This skill works alongside:
- `observability-diagnosis-axiom` — the diagnostic loop that consumes predictive signals
- `alert-engineering-axiom` — reactive alert design that predictive alerts augment
- `dashboard-design-axiom` — dashboard panels that visualize predictions
- `metrics-instrumentation-axiom` — the metrics that feed predictive models
- `sre-ops-axiom` — SLO/SLI definitions that drive burn-rate calculations

axiom:trace work_item=observability-skills-collection-01 spec=specs/66-Predictive-Observability.md plan=phase-87-3/task-87-3-2/step-87-3-2-1

---

## 1. Predictive Observability Philosophy

### 1.1 Why Predict?

Reactive monitoring answers: "Is something broken right now?"
Predictive observability answers: "Will something break soon, and what should we do about it?"

The value hierarchy:

| Level | Question | Signal Type | Response Time |
|-------|----------|-------------|---------------|
| **Predictive** | "Will this break in 3 days?" | Trend projection, burn rate | Hours to days (preventive) |
| **Proactive** | "Is this trending toward failure?" | Leading indicators, anomalies | Minutes to hours (early warning) |
| **Reactive** | "Is this broken now?" | Threshold alerts, error spikes | Seconds to minutes (incident response) |
| **Forensic** | "What broke and why?" | Diagnostic loop, post-mortem | After the fact (learning) |

Predictive observability does NOT replace reactive alerting. It adds a defense-in-depth
layer that gives operators time to prevent incidents rather than just respond to them
(REQ-PRED-091).

### 1.2 Core Principles

1. **Predictions must include confidence.** A point estimate without a confidence interval
   is a guess. Always report R², confidence ranges, and lookback window sensitivity
   (REQ-PRED-010).

2. **Predictions must include recommended actions.** A prediction without a suggested
   mitigation is noise. Every predictive alert must say what to do, not just what will
   happen (REQ-PRED-090).

3. **Predictions must be tunable.** False positives erode trust. Every predictive threshold
   must be configurable, and every false positive must be recordable as feedback
   (REQ-PRED-022).

4. **Predictions augment, never replace.** Predictive alerts run alongside reactive alerts.
   If the prediction fails, the reactive alert is the safety net (REQ-PRED-091).

5. **Leading indicators over lagging indicators.** Monitor the signals that predict failure
   (queue depth growth, saturation, burn rate) rather than waiting for the signals that
   confirm it (error rate, SLO breach) (REQ-PRED-080).

---

## 2. Trend Analysis

Trend analysis projects future metric values based on historical patterns. It answers:
"At the current rate, when will metric X reach threshold Y?"

Spec ref: REQ-PRED-001, REQ-PRED-002, REQ-PRED-003, REQ-PRED-004, REQ-PRED-010.

### 2.1 Linear Regression on Time-Series Metrics (REQ-PRED-001)

Linear regression is the baseline projection method. For any Prometheus-format time-series
metric, the system must support:

1. **Trend line computation** over a configurable lookback window.
2. **Forward projection** to a configurable forecast horizon.
3. **Time-to-threshold calculation** — when will the metric reach a specified value?
4. **R-squared reporting** — how well does the linear model fit the data?

**PromQL example — linear projection of disk usage:**

```promql
# Predict disk usage 7 days from now using 14-day lookback
predict_linear(
  node_filesystem_avail_bytes{mountpoint="/"}[14d],
  7 * 24 * 3600
)
```

**PromQL example — time to disk exhaustion:**

```promql
# Hours until disk is full (available bytes reaches 0)
# Uses current rate of change over 7 days
-node_filesystem_avail_bytes{mountpoint="/"}
  / deriv(node_filesystem_avail_bytes{mountpoint="/"}[7d])
  / 3600
```

**Interpretation guidance:**

| R-squared | Confidence | Interpretation |
|-----------|------------|----------------|
| > 0.9 | High | Strong linear trend; projection is reliable |
| 0.7 - 0.9 | Medium | Moderate fit; projection is directionally useful but imprecise |
| < 0.7 | Low | Poor linear fit; data may be non-linear, seasonal, or noisy |

When R-squared is low, consider:
- Shorter lookback window (recent trend may differ from long-term).
- Seasonal decomposition (REQ-PRED-003) before projecting.
- Non-linear models (exponential, logarithmic) if growth is clearly non-linear.

### 2.2 Growth Rate Projection (REQ-PRED-002)

For each resource category, calculate growth rate and project time to exhaustion:

| Resource | Metric Source | Default Threshold | Projection Method |
|----------|--------------|-------------------|-------------------|
| Disk/storage | `node_filesystem_avail_bytes`, `codeops_snapshot_size_bytes` | 80% capacity | `predict_linear` over 14d |
| Queue depth | `codeops_queue_depth` | 100 items | `deriv` over 1h/6h/24h |
| Connection pool | Application connection metrics | Pool size limit | `deriv` over 1h |
| Memory | Container memory metrics (cAdvisor) | `runtime_resources.memory_limit_mib` | `predict_linear` over 7d |
| Token/cost budget | `codeops_opencode_token_usage_total` | Configured budget limit | `rate` over 24h extrapolated |

**Growth rate calculation pattern:**

```promql
# Current growth rate: bytes per second over the last 7 days
deriv(codeops_snapshot_size_bytes[7d])

# Convert to human-readable: MB per day
deriv(codeops_snapshot_size_bytes[7d]) * 86400 / 1048576
```

**Time-to-exhaustion alert rule:**

```yaml
# Alert when projected exhaustion is within 7 days
- alert: CodeOpsDiskExhaustionPredicted
  expr: |
    predict_linear(
      node_filesystem_avail_bytes{mountpoint="/"}[14d],
      7 * 24 * 3600
    ) < 0
  for: 1h
  labels:
    severity: warning
  annotations:
    summary: "Disk exhaustion predicted within 7 days"
    recommended_action: "Review workspace snapshot retention; consider increasing disk allocation"
```

### 2.3 Seasonal Pattern Detection (REQ-PRED-003)

Seasonal patterns cause false positives in trend analysis if not accounted for. Detect and
account for:

**Time-of-day patterns:**
- Compare current value against the same hour from previous days.
- Use `avg_over_time` with day-aligned offsets for baseline.

```promql
# Average CPU at this hour over the past 7 days
avg_over_time(
  container_cpu_usage_seconds_total[1h]
  offset 1d
) + avg_over_time(
  container_cpu_usage_seconds_total[1h]
  offset 2d
) # ... through 7d, then divide by 7
```

**Day-of-week patterns:**
- Compare weekday vs weekend baselines separately.
- Flag anomalies only when deviation exceeds the day-type baseline.

**Monthly cycles:**
- Track end-of-month processing spikes.
- Suppress predictive alerts during known high-load windows.

**Practical guidance:**
- Start with simple day-of-week and hour-of-day baselines.
- Use platform-native seasonal detection when available (Datadog Anomaly Detection,
  Grafana ML, Prophet).
- For PromQL-only environments, use `offset` comparisons and recording rules to build
  rolling baselines.

### 2.4 Change-Point Detection (REQ-PRED-004)

Change-point detection identifies when a metric's behavior fundamentally shifted — not a
temporary spike, but a persistent change in mean or growth rate.

**Step change detection (shift in mean):**

```promql
# Compare recent mean (1h) against historical mean (7d)
# Large deviation suggests a step change
abs(
  avg_over_time(metric_name[1h])
  - avg_over_time(metric_name[7d])
) / stddev_over_time(metric_name[7d])
```

A z-score > 3 against the 7-day baseline suggests a step change.

**Trend change detection (shift in growth rate):**

```promql
# Compare recent growth rate (1d) against historical growth rate (7d)
abs(
  deriv(metric_name[1d]) - deriv(metric_name[7d])
) / abs(deriv(metric_name[7d]))
```

A relative change > 50% in growth rate warrants investigation.

**Correlation with deployments:**
- When a change point is detected, correlate the timestamp with:
  - Recent deployments (check CI/CD timestamps).
  - Configuration changes (check config audit logs).
  - Infrastructure changes (scaling events, node additions).
- Record the correlation in the diagnostic evidence bundle.

### 2.5 Trend Analysis Limitations (REQ-PRED-010)

Every trend projection output MUST include:

1. **Confidence interval** — not just a point estimate.
2. **Lookback window** — different windows produce different trends; state which was used.
3. **Non-linearity caveat** — if data shows exponential or logarithmic growth, note that
   the linear model may be inaccurate.
4. **Staleness flag** — if the most recent data point is older than 2x the expected
   collection interval, flag the projection as potentially stale.

**Template for trend analysis output:**

```
Metric: codeops_snapshot_size_bytes
Current value: 3.2 GB
Growth rate: 50 MB/day (over 14-day lookback)
Projected value in 30 days: 4.7 GB
Time to 5 GB threshold: ~36 days
Confidence: HIGH (R-squared = 0.94)
Lookback window: 14 days
Caveat: Linear model; growth appears steady. If growth accelerates, threshold may be reached sooner.
Last data point: 2 minutes ago (fresh)
```

---

## 3. Anomaly Detection

Anomaly detection identifies metric values that deviate significantly from expected behavior.
It catches problems that fixed thresholds miss — gradual degradation, novel failure modes,
and subtle shifts.

Spec ref: REQ-PRED-020, REQ-PRED-021, REQ-PRED-022, REQ-PRED-023.

### 3.1 Required Statistical Methods (REQ-PRED-020)

Three statistical methods form the minimum required baseline. Each has different strengths;
choose based on the metric's distribution and behavior.

#### Z-Score Method

Best for: metrics with approximately normal distributions (latency percentiles, error rates).

```
z_score = (current_value - rolling_mean) / rolling_stddev
```

- Flag when |z_score| > threshold (default: 3.0).
- Rolling window: configurable (default: 7 days).

**PromQL approximation:**

```promql
# Z-score of current request latency vs 7-day baseline
(
  histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))
  - avg_over_time(
      histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))[7d:1h]
    )
) / stddev_over_time(
    histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))[7d:1h]
  )
```

**When to use:** Latency percentiles, error rates, request rates with stable baselines.
**When NOT to use:** Metrics with heavy skew (use IQR instead) or strong trends (use
moving average deviation instead).

#### IQR (Interquartile Range) Method

Best for: skewed distributions, metrics with outliers (response sizes, queue depths).

```
Q1 = 25th percentile over rolling window
Q3 = 75th percentile over rolling window
IQR = Q3 - Q1
Lower fence = Q1 - 1.5 * IQR
Upper fence = Q3 + 1.5 * IQR
```

- Flag values below lower fence or above upper fence.
- More robust to outliers than z-score.

**When to use:** Queue depths, response body sizes, token counts, any metric where the
distribution is not symmetric.
**When NOT to use:** Metrics with very narrow IQR (nearly constant values) — use z-score
or fixed thresholds instead.

#### Moving Average Deviation

Best for: trending metrics where absolute thresholds are inappropriate (growing storage,
increasing user counts).

```
deviation = current_value - moving_average
anomaly if deviation > k * moving_average_stddev
```

- Moving average type: simple (SMA) or exponential (EMA).
- EMA is more responsive to recent changes; SMA is more stable.
- Default k: 2.0 (configurable per metric).

**PromQL example (SMA-based):**

```promql
# Deviation from 24-hour moving average
abs(
  codeops_queue_depth
  - avg_over_time(codeops_queue_depth[24h])
) / stddev_over_time(codeops_queue_depth[24h])
```

**When to use:** Storage growth, user count growth, any metric with a natural upward or
downward trend.
**When NOT to use:** Metrics that should be stable (use z-score or fixed thresholds).

### 3.2 Advanced Anomaly Detection (REQ-PRED-021, Optional)

These methods are OPTIONAL and depend on additional infrastructure:

| Method | When to Use | Infrastructure Required |
|--------|-------------|------------------------|
| **Isolation Forest** | Multi-dimensional anomaly detection (correlating CPU + memory + latency) | ML model training pipeline |
| **Prophet / ARIMA** | Metrics with strong seasonal patterns and complex trends | Python/R runtime for model fitting |
| **Platform-native** | When using Datadog, Grafana Cloud, or New Relic | Platform subscription |

**Practical recommendation:** Start with the three required statistical methods. Graduate
to advanced methods only when false positive rates from statistical methods are unacceptable
despite tuning.

### 3.3 False Positive Management (REQ-PRED-022)

False positives erode operator trust. Manage them systematically:

**Threshold tuning:**
- Every anomaly sensitivity threshold MUST be configurable per metric.
- Start with conservative thresholds (z-score 3.0, IQR 1.5x) and tighten based on
  observed false positive rates.
- Document threshold changes with rationale.

**Feedback loops:**
- When an anomaly is investigated and determined false positive, record:
  - Metric name and timestamp.
  - Why it was a false positive (seasonal pattern, known maintenance, data artifact).
  - Suggested threshold adjustment.
- Review false positive feedback monthly to adjust thresholds.

**Suppression with audit trail:**
- Suppress anomaly alerts during known maintenance windows.
- Every suppression MUST record: who suppressed, why, and expiration time.
- Suppressions MUST auto-expire; permanent suppressions are forbidden.

**Cooldown periods:**
- After an anomaly is acknowledged, the same anomaly MUST NOT re-alert within the
  cooldown period (default: 1 hour).
- Cooldown is per-metric, not global.

### 3.4 Anomaly Severity Classification (REQ-PRED-023)

Classify detected anomalies by severity to route them appropriately:

| Severity | Criteria | Routing | Response SLA |
|----------|----------|---------|--------------|
| **Informational** | Deviation detected but within historical variance; no user impact expected | Log only; include in weekly review | Weekly |
| **Warning** | Significant deviation from baseline; potential user impact if trend continues | Standard channel (Slack) | 1 business day |
| **Critical** | Extreme deviation; active or imminent user impact | On-call channel (PagerDuty) | Immediate |

**Severity determination factors:**
1. **Magnitude** — how far from baseline? (z-score 3 vs 5 vs 10)
2. **Rate of change** — rapid deviation is more severe than gradual.
3. **SLO linkage** — metrics tied to SLOs are more severe than internal-only metrics.
4. **Blast radius** — anomalies affecting all users are more severe than single-tenant.

---

## 4. Error Budget Burn Rate Prediction

Error budget burn rate analysis predicts when SLO error budgets will be exhausted. It
distinguishes fast-burn incidents (immediate response needed) from slow-burn degradation
(investigate within a day).

Spec ref: REQ-PRED-040, REQ-PRED-041, REQ-PRED-042, REQ-PRED-043, REQ-PRED-050.

### 4.1 Burn Rate Fundamentals

**Error budget** = 1 - SLO target. For a 99.9% availability SLO over 30 days:
- Error budget = 0.1% of requests (or ~43 minutes of downtime).
- Sustainable burn rate = 1x (uses exactly the budget over the period).

**Burn rate** = actual error rate / allowed error rate.
- Burn rate 1x = sustainable (budget lasts the full period).
- Burn rate 2x = consuming budget at double the sustainable rate.
- Burn rate 14.4x = consuming budget so fast it will be gone in ~5 days.

### 4.2 Fast-Burn Alerts (REQ-PRED-040)

Fast-burn alerts detect rapid SLO consumption requiring immediate response.

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| Burn rate threshold | 14.4x | Exhausts 100% of monthly budget in 5 days |
| Short window | 1 hour | Detects rapid degradation quickly |
| Long window | 5 minutes | Confirms burn rate is sustained, not a spike |
| Alert severity | Critical | Requires immediate response |

**Alert condition:** burn rate > 14.4x over BOTH 1 hour AND 5 minutes.

**PromQL example (availability SLO):**

```promql
# Fast-burn: 14.4x over 1 hour
(
  1 - (
    sum(rate(http_requests_total{code!~"5.."}[1h]))
    / sum(rate(http_requests_total[1h]))
  )
) / (1 - 0.999) > 14.4
AND
# Confirmed over 5 minutes
(
  1 - (
    sum(rate(http_requests_total{code!~"5.."}[5m]))
    / sum(rate(http_requests_total[5m]))
  )
) / (1 - 0.999) > 14.4
```

**Alert rule:**

```yaml
- alert: CodeOpsSLOFastBurn
  expr: |
    (
      1 - (sum(rate(http_requests_total{code!~"5.."}[1h])) / sum(rate(http_requests_total[1h])))
    ) / (1 - 0.999) > 14.4
    and
    (
      1 - (sum(rate(http_requests_total{code!~"5.."}[5m])) / sum(rate(http_requests_total[5m])))
    ) / (1 - 0.999) > 14.4
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "SLO fast-burn detected — error budget exhaustion in ~5 days at current rate"
    recommended_action: "Investigate immediately; check recent deployments and error logs"
    runbook_url: "https://runbooks.example.com/slo-fast-burn"
```

### 4.3 Slow-Burn Alerts (REQ-PRED-041)

Slow-burn alerts detect gradual SLO consumption requiring investigation within a day.

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| Burn rate threshold | 1x | Exhausts 100% of monthly budget in 30 days |
| Short window | 3 days | Detects sustained degradation |
| Long window | 6 hours | Confirms burn rate is sustained |
| Alert severity | Warning | Requires investigation within 1 business day |

**Alert condition:** burn rate > 1x over BOTH 3 days AND 6 hours.

### 4.4 Multi-Window Burn Rate (REQ-PRED-042)

The multi-window approach combines fast and slow windows to prevent:

1. **False positives from spikes** — a single spike triggers the short window but not the
   long window, so no alert fires.
2. **Missed slow burns** — gradual degradation triggers the long window even though no
   single short window exceeds the fast-burn threshold.

**Decision matrix:**

| Fast-Burn (1h + 5m) | Slow-Burn (3d + 6h) | Action |
|----------------------|----------------------|--------|
| Both exceeded | Either | Critical alert — immediate response |
| Neither | Both exceeded | Warning alert — investigate within 1 day |
| One window only | One window only | No alert — likely transient |
| Neither | Neither | Healthy — budget is sustainable |

### 4.5 Error Budget Exhaustion Forecasting (REQ-PRED-043)

Report the following for each SLO:

1. **Current burn rate** — expressed as a multiple of sustainable rate (e.g., "2.3x").
2. **Remaining error budget** — percentage and absolute value
   (e.g., "67% remaining, 4320 errors of 6480 budget").
3. **Projected exhaustion time** — "at current burn rate, error budget exhausts in X
   hours/days" or "budget is sustainable at current rate."
4. **Trend direction** — is the burn rate increasing, decreasing, or stable?

**Template for error budget status:**

```
SLO: API Availability (99.9% over 30 days rolling)
Current burn rate: 2.3x sustainable
Remaining budget: 67% (4,320 of 6,480 allowed errors)
Projected exhaustion: 19.6 days at current rate
Trend: STABLE (burn rate unchanged over last 24 hours)
Status: WARNING — budget will exhaust before period end at current rate
Recommended action: Investigate top error sources; prioritize fixes for highest-volume error paths
```

### 4.6 Integration with SLO Definitions (REQ-PRED-050)

Burn rate prediction requires well-defined SLOs. Each SLO must specify:

| Field | Description | Example |
|-------|-------------|---------|
| SLI | The metric being measured | Request success rate |
| SLO target | The target value | 99.9% |
| Error budget period | Time window for budget calculation | 30 days rolling |
| Error budget | Derived from target and period | 0.1% of requests over 30 days |

The `sre-ops-axiom` skill provides portable guidance for defining SLOs. This skill
consumes those definitions to calculate burn rates.

**SLO-to-burn-rate wiring checklist:**
- [ ] SLI metric exists and is being collected.
- [ ] SLO target is defined and documented.
- [ ] Error budget period is configured (default: 30 days rolling).
- [ ] Fast-burn alert rule is deployed with correct thresholds.
- [ ] Slow-burn alert rule is deployed with correct thresholds.
- [ ] Error budget dashboard panel exists (see `dashboard-design-axiom`).
- [ ] Burn rate runbook is linked to both alert rules.

---

## 5. Capacity Planning

Capacity planning uses utilization trends to project future resource needs and prevent
exhaustion.

Spec ref: REQ-PRED-060, REQ-PRED-061, REQ-PRED-062, REQ-PRED-063, REQ-PRED-070.

### 5.1 Resource Utilization Trend Monitoring (REQ-PRED-060)

Monitor utilization trends for each resource and project future utilization:

| Resource | Warning Threshold | Critical Threshold | Lookback | Projection |
|----------|-------------------|--------------------|----------|------------|
| CPU | 70% sustained 1h | 90% sustained 15m | 7 days | `predict_linear` |
| Memory | 75% of limit | 90% of limit | 7 days | `predict_linear` |
| Disk | 80% capacity | 90% capacity | 14 days | `predict_linear` |
| Network | 70% bandwidth | 90% bandwidth | 7 days | `deriv` extrapolation |

For each resource:
1. Track utilization as percentage of configured limit.
2. Calculate growth rate (units per time period).
3. Project time to warning and critical thresholds.
4. Account for seasonal patterns in projections.

**Alert rule pattern — resource exhaustion prediction:**

```yaml
- alert: CodeOpsMemoryExhaustionPredicted
  expr: |
    predict_linear(
      container_memory_usage_bytes{container="axiom"}[7d],
      7 * 24 * 3600
    ) > container_spec_memory_limit_bytes{container="axiom"}
  for: 1h
  labels:
    severity: warning
  annotations:
    summary: "Memory exhaustion predicted within 7 days"
    recommended_action: "Review memory usage patterns; consider increasing memory limit or optimizing"
```

### 5.2 Queue Depth Growth Analysis (REQ-PRED-061)

Queue depth is a critical leading indicator. Monitor across multiple windows:

| Window | Purpose | Alert Condition |
|--------|---------|-----------------|
| 1 hour | Short-term spike detection | Monotonic increase for > 30 minutes |
| 6 hours | Medium-term trend | Growth rate > 2x baseline |
| 24 hours | Long-term capacity signal | Projected to reach threshold within 7 days |

**Monotonic increase detection:**

```promql
# Alert when queue depth has been increasing for 30+ minutes
# (minimum value over 30m equals the value 30m ago, meaning no decreases)
min_over_time(codeops_queue_depth[30m]) > codeops_queue_depth offset 30m
```

**Consumer health correlation:**
When queue depth grows, check:
- Are consumers running? (pod status, process health)
- Are consumers slow? (consumer processing latency)
- Are consumers failing? (consumer error rate)
- Is the producer rate abnormally high? (ingestion rate vs baseline)

### 5.3 Connection Pool Saturation Forecasting (REQ-PRED-062)

| Metric | Alert Threshold | Projection |
|--------|-----------------|------------|
| Active connections / pool size | > 80% sustained 5m | Time to 100% at current rate |
| Wait queue length | > 0 sustained 1m | Pool is already saturated |
| Connection checkout latency | > 2x baseline | Pool contention increasing |

**Saturation is a leading indicator.** When a connection pool is saturated:
- Requests queue waiting for connections.
- Latency increases as requests wait.
- Eventually, timeouts and errors follow.

Monitor saturation BEFORE errors appear.

### 5.4 Storage Consumption Projection (REQ-PRED-063)

Project storage consumption for each storage category:

| Category | Metric | Projection Method |
|----------|--------|-------------------|
| Workspace snapshots | `codeops_snapshot_size_bytes` | `predict_linear` over 14d |
| Log volume | Log ingestion rate (bytes/day) | Linear extrapolation |
| Evidence bundles | Evidence directory size | Linear extrapolation |
| Database storage | `pg_database_size_bytes` (if PostgreSQL) | `predict_linear` over 30d |

**Storage projection template:**

```
Category: Workspace Snapshots
Current size: 3.2 GB
Growth rate: 50 MB/day
Storage limit: 10 GB
Time to limit: ~136 days
Confidence: HIGH (R-squared = 0.92, 14-day lookback)
Recommended action: No immediate action; review in 90 days
```

### 5.5 Cost Projection (REQ-PRED-070)

Project future costs based on usage trends, integrating with `specs/47-Cost-Tracking-And-Session-Analytics.md`:

**Token cost projection:**

```
Current daily token spend: $12.50/day
Weekly trend: +3% week-over-week
Projected monthly cost: $412 (at current rate) / $425 (with trend)
Budget limit: $500/month
Time to budget exhaustion: ~38 days at current rate
Confidence: MEDIUM (R-squared = 0.78, 30-day lookback)
```

**Infrastructure cost projection:**
- Based on resource utilization trends, project when scaling events will be needed.
- Each scaling event has a known cost impact (e.g., adding a node = $X/month).
- Project the number of scaling events needed over the next 30/90 days.

**Required cost projection outputs:**
1. Current daily/weekly/monthly spend rate.
2. Projected spend for next 7, 30, and 90 days.
3. Confidence interval based on historical variance.
4. Budget exhaustion forecast (if budget is configured).

---

## 6. Leading vs Lagging Indicators

Every monitored service must identify its leading and lagging indicators. Leading indicators
predict problems; lagging indicators confirm them.

Spec ref: REQ-PRED-080.

### 6.1 Indicator Classification

| Type | Definition | Response |
|------|-----------|----------|
| **Leading** | Signals that predict future problems before user impact | Proactive: investigate and mitigate before impact |
| **Lagging** | Signals that confirm problems after user impact has begun | Reactive: diagnose and mitigate active impact |

### 6.2 Axiom Leading Indicators

| Leading Indicator | What It Predicts | Metric Source | Alert Threshold |
|-------------------|------------------|---------------|-----------------|
| Queue depth growth rate | Processing backlog; timeout failures | `codeops_queue_depth` trend | Monotonic increase > 30m |
| Memory utilization growth | OOM kills; pod restarts | Container memory metrics | > 75% sustained 1h |
| OpenCode request latency trend | Step duration increases; timeouts | `codeops_opencode_request_duration_seconds` | p99 > 2x baseline |
| Token usage acceleration | Budget exhaustion; cost overruns | `codeops_opencode_token_usage_total` rate | Rate > 2x 7-day average |
| Retry rate increase | Systemic instability; escalation | `codeops_retry_total` rate | Rate > 3x baseline |
| Snapshot size growth | Storage exhaustion; capture failures | `codeops_snapshot_size_bytes` trend | Projected exhaustion < 30d |
| Error budget burn rate | SLO breach | Derived from SLI metrics | > 1x sustainable rate |
| Connection pool utilization | Request queuing; latency increase | Pool active/total ratio | > 80% sustained 5m |

### 6.3 The USE Method as a Leading Indicator Framework

The USE method (Utilization, Saturation, Errors) provides a systematic framework:

| Signal | Type | What It Tells You |
|--------|------|-------------------|
| **Utilization** | Leading | How much of the resource capacity is being used |
| **Saturation** | Leading | Whether work is queuing because the resource is full |
| **Errors** | Lagging | Whether the resource is failing |

**Key insight:** Saturation is the strongest leading indicator. When a resource is saturated
(requests queued, connections waiting, CPU runqueue growing), errors and latency increases
WILL follow. Monitor saturation before errors.

### 6.4 Indicator Review Checklist

For each service, verify:
- [ ] At least 3 leading indicators are identified and monitored.
- [ ] Each leading indicator has a defined alert threshold.
- [ ] Each leading indicator alert has a linked runbook.
- [ ] Lagging indicators (error rate, SLO breach) have reactive alerts as safety nets.
- [ ] Leading indicator dashboards are accessible to on-call engineers.
- [ ] Leading indicators are reviewed in weekly operational reviews.

---

## 7. Predictive Alert Contract

Predictive alerts have additional requirements beyond standard reactive alerts. They must
communicate not just "something is wrong" but "something will be wrong, here's when, and
here's what to do."

Spec ref: REQ-PRED-090, REQ-PRED-091, REQ-PRED-092, REQ-PRED-100.

### 7.1 Predictive Alert Format (REQ-PRED-090)

Every predictive alert MUST include these fields beyond standard alert fields:

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `predicted_breach_time` | ISO 8601 | When threshold is predicted to be breached | `2026-03-24T00:00:00Z` |
| `time_to_breach` | duration | Time remaining until predicted breach | `7 days` |
| `confidence_level` | enum | `high` (R-sq > 0.9), `medium` (0.7-0.9), `low` (< 0.7) | `high` |
| `trend_direction` | enum | `increasing`, `decreasing`, `stable`, `accelerating` | `increasing` |
| `current_value` | number | Current metric value | `72%` |
| `threshold_value` | number | Threshold that will be breached | `90%` |
| `recommended_action` | string | Suggested mitigation | `"Scale up disk"` |
| `lookback_window` | duration | Historical window used for prediction | `14 days` |

**Example predictive alert:**

```yaml
alert: CodeOpsDiskExhaustionPredicted
severity: warning
predicted_breach_time: "2026-03-24T00:00:00Z"
time_to_breach: "7 days"
confidence_level: high
trend_direction: increasing
current_value: "72% disk utilization"
threshold_value: "90% disk utilization"
recommended_action: "Review workspace snapshot retention; consider increasing disk allocation"
lookback_window: "14 days"
```

### 7.2 Predictive Alerts Augment Reactive Alerts (REQ-PRED-091)

Predictive alerts are a defense-in-depth layer. They MUST NOT replace reactive alerts.

**Alert hierarchy:**

```
Predictive alert (days/hours before breach)
  → If acted on: problem prevented (best outcome)
  → If ignored: falls through to...
Warning alert (threshold approaching)
  → If acted on: problem mitigated (good outcome)
  → If ignored: falls through to...
Critical alert (threshold breached)
  → Must act: reactive response (worst outcome)
```

Both predictive and reactive alerts MUST be active simultaneously.

### 7.3 Predictive Alert Tuning Lifecycle (REQ-PRED-092)

Predictive alerts require ongoing tuning:

| Phase | Duration | Activity |
|-------|----------|----------|
| **Initial calibration** | Day 0 | Set thresholds based on historical data (minimum 7 days baseline) |
| **Monitoring period** | Days 1-14 | Run in informational mode; assess accuracy |
| **Threshold adjustment** | Day 14 | Adjust sensitivity based on false positive rate |
| **Production activation** | Day 14+ | Promote to warning/critical severity |
| **Ongoing review** | Monthly | Review accuracy; adjust as system behavior evolves |

**Tuning checklist:**
- [ ] Baseline data collected (minimum 7 days).
- [ ] Initial thresholds set based on baseline.
- [ ] Informational-mode alerts deployed.
- [ ] False positive rate measured after 7-14 days.
- [ ] Thresholds adjusted based on false positive feedback.
- [ ] Alerts promoted to production severity.
- [ ] Monthly review scheduled.

### 7.4 Predictive Alert Runbook Integration (REQ-PRED-100)

Every predictive alert MUST have a linked runbook. Predictive runbooks focus on
**prevention** rather than mitigation:

| Step | Action | Key Difference from Reactive Runbook |
|------|--------|--------------------------------------|
| **Assess** | Validate the prediction — is the trend real or an artifact? | Reactive: "Is the system broken?" / Predictive: "Will the system break?" |
| **Plan** | Determine preventive action (scale, optimize, budget adjustment) | Reactive: "How do we fix it?" / Predictive: "How do we prevent it?" |
| **Act** | Execute preventive action before predicted breach | Reactive: mitigate active impact / Predictive: prevent future impact |
| **Verify** | Confirm trend has changed after preventive action | Reactive: "Is it fixed?" / Predictive: "Has the trend reversed?" |
| **Document** | Record prediction, action, and outcome in evidence bundle | Same for both |

**Predictive runbook template:**

```markdown
# Runbook: [Alert Name]

## Assess
1. Open the [metric] dashboard panel.
2. Verify the trend is real (not a data artifact or seasonal pattern).
3. Check R-squared / confidence level — is the prediction reliable?
4. Check for recent deployments or config changes that might explain the trend.

## Plan
- If trend is real and confidence is HIGH: proceed to Act.
- If trend is real but confidence is LOW: extend monitoring period; re-assess in 24h.
- If trend is an artifact: suppress alert with audit trail; investigate data quality.

## Act
- [Specific preventive action for this alert]
- Example: "Increase disk allocation from 10 GB to 20 GB"
- Example: "Review and clean up old workspace snapshots"

## Verify
1. After action, monitor the metric for 1-2 hours.
2. Confirm the trend has changed (growth rate decreased or reversed).
3. If trend unchanged, escalate to [team/person].

## Document
- Record in evidence bundle: prediction timestamp, action taken, outcome.
```

---

## 8. Practical Implementation Patterns

### 8.1 Recording Rules for Prediction

Use Prometheus recording rules to pre-compute expensive prediction queries:

```yaml
groups:
  - name: codeops_predictions
    interval: 5m
    rules:
      # Pre-compute disk exhaustion prediction
      - record: axiom:disk_predicted_full_days
        expr: |
          -node_filesystem_avail_bytes{mountpoint="/"}
          / deriv(node_filesystem_avail_bytes{mountpoint="/"}[14d])
          / 86400

      # Pre-compute error budget remaining percentage
      - record: axiom:error_budget_remaining_pct
        expr: |
          1 - (
            sum(increase(http_requests_total{code=~"5.."}[30d]))
            / (sum(increase(http_requests_total[30d])) * (1 - 0.999))
          )

      # Pre-compute queue depth growth rate (items per hour)
      - record: axiom:queue_depth_growth_rate_per_hour
        expr: deriv(codeops_queue_depth[6h]) * 3600

      # Pre-compute token cost daily rate
      - record: axiom:token_cost_daily_rate
        expr: rate(codeops_opencode_token_usage_total[24h]) * 86400
```

### 8.2 Dashboard Panels for Predictions

Add these panels to operational dashboards (see `dashboard-design-axiom`):

| Panel | Visualization | Data Source |
|-------|---------------|-------------|
| Error budget burn rate | Gauge (green/yellow/red) | `axiom:error_budget_remaining_pct` |
| Disk exhaustion countdown | Stat (days remaining) | `axiom:disk_predicted_full_days` |
| Queue depth trend | Time series with trend line | `codeops_queue_depth` + `predict_linear` |
| Cost projection | Bar chart (actual vs projected) | `axiom:token_cost_daily_rate` |
| Leading indicator summary | Table (indicator, status, trend) | Multiple recording rules |

### 8.3 Grafana-Specific Guidance

**Trend line overlay:**
- Use the "Transform > Regression analysis" feature to overlay trend lines on time series.
- Configure lookback and projection windows in the transform settings.

**Anomaly bands:**
- Use the "Transform > Config from query results" to create dynamic upper/lower bounds.
- Display as a shaded band behind the actual metric line.

**Forecast panel:**
- Grafana Cloud ML provides native forecasting panels.
- For self-hosted Grafana, use recording rules to pre-compute predictions and display
  as separate series.

### 8.4 Datadog-Specific Guidance

**Anomaly detection:**
- Use the `anomalies()` function with algorithm selection (basic, agile, robust).
- `robust` is recommended for metrics with seasonal patterns.

**Forecast monitors:**
- Use Datadog Forecast Monitors with linear or seasonal algorithms.
- Configure alert threshold as "metric will reach X within Y time."

**Cost tracking:**
- Use Datadog Cost Management for infrastructure cost projection.
- Integrate with `specs/47` token cost tracking for application-level cost projection.

---

## 9. Anti-Patterns

### 9.1 Prediction Anti-Patterns

| Anti-Pattern | Problem | Fix |
|--------------|---------|-----|
| **Point estimate without confidence** | Operators trust a single number that may be wildly inaccurate | Always include R-squared and confidence interval |
| **Prediction without action** | Alert fires but nobody knows what to do | Every predictive alert must include `recommended_action` |
| **Replacing reactive with predictive** | If prediction fails, no safety net exists | Keep both layers active simultaneously |
| **Over-fitting to recent data** | Very short lookback produces volatile predictions | Use minimum 7-day lookback for trend analysis |
| **Ignoring seasonality** | Business-hours patterns cause false positives every morning | Apply seasonal decomposition before anomaly detection |
| **Permanent suppression** | Suppressed alerts are forgotten and never re-enabled | All suppressions must have expiration times |
| **Prediction on noisy metrics** | Low R-squared predictions generate constant false positives | Filter predictions by confidence level; suppress low-confidence alerts |
| **Single-method anomaly detection** | One method doesn't fit all metric distributions | Use z-score for normal, IQR for skewed, moving average for trending |
| **Burn rate without SLO** | Cannot calculate burn rate without a defined error budget | Define SLOs first (see `sre-ops-axiom`), then add burn rate alerts |
| **Cost projection without budget** | Projection is meaningless without a threshold to compare against | Define cost budgets before enabling cost exhaustion alerts |

### 9.2 Capacity Planning Anti-Patterns

| Anti-Pattern | Problem | Fix |
|--------------|---------|-----|
| **Reactive scaling only** | Scale up only after performance degrades | Use utilization trend projections to scale proactively |
| **Ignoring saturation signals** | Monitor utilization but not saturation (queuing) | Add saturation metrics for every resource (USE method) |
| **Linear projection for exponential growth** | Underestimates time to exhaustion | Check R-squared; if low, try exponential model |
| **Projecting without seasonal adjustment** | Weekend lulls make weekday projections too optimistic | Account for day-of-week patterns in capacity projections |

---

## 10. Open Decisions from Spec

The following decisions from `specs/66-Predictive-Observability.md` are currently open.
When resolved, update both the spec and this skill.

| Decision | Current Default | Impact |
|----------|-----------------|--------|
| Anomaly detection infrastructure (sidecar vs service vs platform) | Platform-delegated where available; statistical methods for platform-agnostic | Affects deployment architecture |
| Error budget period (30d rolling vs calendar month) | 30 days rolling | Affects burn rate calculation and reset behavior |
| R-squared confidence thresholds configurable per metric? | Fixed (high > 0.9, medium 0.7-0.9, low < 0.7) | Affects prediction confidence reporting |
| Cost projection granularity (per-work-item, per-repo, per-org) | Per-repo | Affects cost dashboard and alert scope |

---

## 11. Operational Checklists

### 11.1 Predictive Observability Setup Checklist

Use this checklist when enabling predictive observability for a new service:

- [ ] **Baseline data**: Minimum 7 days of metric history collected.
- [ ] **SLOs defined**: At least one SLO with SLI, target, and error budget period.
- [ ] **Leading indicators identified**: At least 3 leading indicators per service.
- [ ] **Recording rules deployed**: Pre-computed prediction queries for dashboard performance.
- [ ] **Fast-burn alert**: Deployed in informational mode for initial calibration.
- [ ] **Slow-burn alert**: Deployed in informational mode for initial calibration.
- [ ] **Resource exhaustion alerts**: Disk, memory, queue depth projections configured.
- [ ] **Cost projection**: Token and infrastructure cost projections configured (if applicable).
- [ ] **Dashboard panels**: Error budget, trend lines, and leading indicator summary added.
- [ ] **Runbooks linked**: Every predictive alert has a linked runbook.
- [ ] **Tuning schedule**: Monthly review of prediction accuracy and false positive rates.

### 11.2 Monthly Prediction Accuracy Review

- [ ] Review false positive rate for each predictive alert.
- [ ] Review false negative rate (did any incidents occur that predictions missed?).
- [ ] Adjust thresholds based on false positive/negative feedback.
- [ ] Review R-squared values for trend projections — are they still reliable?
- [ ] Check for new seasonal patterns that need to be accounted for.
- [ ] Update runbooks if recommended actions have changed.
- [ ] Document review findings in the operational evidence bundle.

### 11.3 Incident Prevention Verification

After taking preventive action based on a predictive alert:

- [ ] Confirm the metric trend has changed (growth rate decreased or reversed).
- [ ] Verify the predictive alert has cleared or downgraded in severity.
- [ ] Record the prediction, action taken, and outcome in the evidence bundle.
- [ ] If the action was ineffective, escalate and update the runbook.

---

## 12. Requirement Coverage Matrix

This skill covers the following requirements from `specs/66-Predictive-Observability.md`:

| Requirement | Section | Coverage |
|-------------|---------|----------|
| REQ-PRED-001 | 2.1 | Linear regression on time-series metrics |
| REQ-PRED-002 | 2.2 | Growth rate projection for 5 resource categories |
| REQ-PRED-003 | 2.3 | Seasonal pattern detection (time-of-day, day-of-week, monthly) |
| REQ-PRED-004 | 2.4 | Change-point detection (step changes, trend changes, deployment correlation) |
| REQ-PRED-010 | 2.5 | Trend analysis limitations (confidence, lookback, non-linearity, staleness) |
| REQ-PRED-020 | 3.1 | Statistical anomaly detection (z-score, IQR, moving average deviation) |
| REQ-PRED-021 | 3.2 | Advanced anomaly detection (optional: Isolation Forest, Prophet, platform-native) |
| REQ-PRED-022 | 3.3 | False positive management (tuning, feedback, suppression, cooldown) |
| REQ-PRED-023 | 3.4 | Anomaly severity classification (informational, warning, critical) |
| REQ-PRED-040 | 4.2 | Fast-burn alerts (14.4x, 1h + 5m windows) |
| REQ-PRED-041 | 4.3 | Slow-burn alerts (1x, 3d + 6h windows) |
| REQ-PRED-042 | 4.4 | Multi-window burn rate analysis |
| REQ-PRED-043 | 4.5 | Error budget exhaustion forecasting |
| REQ-PRED-050 | 4.6 | Integration with SLO definitions |
| REQ-PRED-060 | 5.1 | Resource utilization trend monitoring (CPU, memory, disk, network) |
| REQ-PRED-061 | 5.2 | Queue depth growth analysis |
| REQ-PRED-062 | 5.3 | Connection pool saturation forecasting |
| REQ-PRED-063 | 5.4 | Storage consumption projection |
| REQ-PRED-070 | 5.5 | Cost projection (token, infrastructure, budget exhaustion) |
| REQ-PRED-080 | 6 | Leading vs lagging indicator framework |
| REQ-PRED-090 | 7.1 | Predictive alert format (8 required fields) |
| REQ-PRED-091 | 7.2 | Predictive alerts augment (not replace) reactive alerts |
| REQ-PRED-092 | 7.3 | Predictive alert tuning lifecycle |
| REQ-PRED-100 | 7.4 | Predictive alert runbook integration |
