---
name: alert-engineering-axiom
description: >
  Alert design philosophy, threshold types (static, dynamic, burn-rate, composite),
  severity and routing/escalation patterns, noise-reduction rules, alert-as-code guidance,
  lifecycle practices, anti-patterns, and mandatory alert-to-runbook linkage. Load this
  skill when designing alerts, reviewing alert rules, tuning thresholds, or auditing
  alert-to-runbook coverage for any service managed by Axiom.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-03-20"
  primary_spec: specs/34-Observability-And-Metrics.md
  secondary_specs:
    - specs/25-Structured-Logging-Events.md
    - specs/61-Logging-And-Observability-Overhaul.md
    - specs/65-Diagnostic-Workflows.md
    - specs/66-Predictive-Observability.md
tags:
  vertical: [sre, ops]
  category: observability
  core: false
---

# Alert Engineering Skill (Portable)

> **"Every alert should be actionable, and every action should have a runbook."**
>
> **"If an alert fires and nobody acts, it is noise. If it fires and nobody can act, it is broken."**

This skill provides portable, production-grade guidance for designing, implementing, and
maintaining alert rules that surface real problems without drowning operators in noise.
It covers threshold types, severity classification, routing and escalation, noise reduction,
alert-as-code practices, lifecycle management, and the mandatory alert-to-runbook linkage
required by `specs/34-Observability-And-Metrics.md#REQ-OBS-RUNBOOK-001`.

**Spec grounding**: The Axiom required alerts table, severity definitions, and runbook
linkage contract are defined in `specs/34-Observability-And-Metrics.md#baseline-alerting-expectations`.
This skill encodes those requirements as reusable alert engineering patterns applicable to
any monitoring stack (Prometheus/Alertmanager, Datadog, CloudWatch, PagerDuty, OpsGenie).

---

## Activation

Load this skill when:
- Designing new alert rules for a service or infrastructure component
- Reviewing existing alert rules for completeness, noise, or coverage gaps
- Tuning alert thresholds to reduce false positives or catch regressions earlier
- Implementing alert-as-code (Prometheus rules, Terraform, Pulumi, CloudFormation)
- Auditing alert-to-runbook linkage per `REQ-OBS-RUNBOOK-001`
- Defining severity levels, routing policies, or escalation chains
- Investigating alert fatigue or on-call burnout

---

## 1. Alert Design Philosophy

### 1.1 Core Principles

| Principle | Description |
|-----------|-------------|
| **Actionable** | Every alert MUST have a clear action the responder can take. If no action exists, it is not an alert -- it is a metric to watch on a dashboard. |
| **Runbook-linked** | Every alert MUST have a linked runbook (`REQ-OBS-RUNBOOK-001`). The runbook provides triage, diagnosis, mitigation, verification, and rollback steps. |
| **Symptom-based** | Alert on user-visible symptoms (error rate, latency), not internal causes (CPU usage) unless the cause directly predicts user impact. |
| **Bounded** | Alerts MUST have bounded label cardinality. Do not create per-request or per-user alerts. |
| **Tunable** | Thresholds SHOULD be configurable without code changes (via config files, environment variables, or alert rule parameters). |
| **Tested** | Alert rules SHOULD be tested in CI (unit tests for PromQL expressions, integration tests for end-to-end firing). |

### 1.2 The Alert Pyramid

Structure alerts in layers, from broadest to most specific:

```
        /\
       /  \  SLO burn-rate alerts (highest signal, lowest volume)
      /    \
     /------\  Symptom-based alerts (error rate, latency, availability)
    /        \
   /----------\  Cause-based alerts (resource saturation, dependency failures)
  /            \
 /--------------\  Informational signals (dashboard-only, no paging)
```

**Rule**: Page on SLO burn-rate and symptom alerts. Use cause-based alerts for triage
context, not for paging. Informational signals belong on dashboards, not in alert rules.

---

## 2. Axiom Required Alerts Table

These alerts are defined in `specs/34-Observability-And-Metrics.md#baseline-alerting-expectations`
and MUST be implemented when Prometheus-based metrics are available (post-v1). In v1,
equivalent monitoring is achieved through log-based queries.

| Alert Name | Condition | Severity | Recommended Action | Runbook |
|------------|-----------|----------|-------------------|---------|
| `CodeOpsStepDurationHigh` | p95 `codeops_step_duration_seconds` > 600s for 15 min | Warning | Investigate slow steps; check OpenCode response times | `.axiom/runbooks/axiom-step-duration-high.md` |
| `CodeOpsVerificationFailureRateHigh` | `verification_failed / total > 0.3` for 30 min | Warning | Investigate failing verifiers; check spec alignment | `.axiom/runbooks/axiom-verification-failure-rate-high.md` |
| `CodeOpsPodOOMKilled` | `pods_failed{reason="oom_killed"}` increase > 0 | Critical | Escalate; review `runtime_resources.memory_limit_mib` | `.axiom/runbooks/axiom-pod-oom-killed.md` |
| `CodeOpsQueueDepthHigh` | `codeops_queue_depth > 50` for 15 min | Warning | Scale control plane or investigate stuck work items | `.axiom/runbooks/axiom-queue-depth-high.md` |
| `CodeOpsWorkItemStuck` | Work item `in_progress` > 2h without `step_completed` | Critical | Escalate to Jira; check for stuck OpenCode sessions | `.axiom/runbooks/axiom-work-item-stuck.md` |
| `CodeOpsEscalationRate` | `escalation_total` increase > 5 in 1h | Warning | Review escalation reasons; check for systemic issues | `.axiom/runbooks/axiom-escalation-rate.md` |
| `CodeOpsOpenCodeHealthFailed` | `opencode_health_failed` event emitted | Critical | Check OpenCode server; review crash recovery | `.axiom/runbooks/axiom-opencode-health-failed.md` |
| `CodeOpsSnapshotCaptureFailed` | `increase(snapshot_capture{status="failed"}[15m]) > 2` | Warning | Investigate S3/IAM/disk | `.axiom/runbooks/axiom-snapshot-capture-failed.md` |
| `CodeOpsSnapshotRestoreFailed` | `increase(snapshot_restore{status="failed"}[15m]) > 0` | Warning | Investigate integrity/S3/storage | `.axiom/runbooks/axiom-snapshot-restore-failed.md` |
| `CodeOpsSnapshotSizeExceeded` | `snapshot_size_bytes > 5 GB` | Warning | Workspace bloat; review exclusions | `.axiom/runbooks/axiom-snapshot-size-exceeded.md` |
| `CodeOpsSnapshotGCNotRunning` | Snapshots created but no GC in 24h | Warning | Verify `gc_on_teardown` config | `.axiom/runbooks/axiom-snapshot-gc-not-running.md` |
| `CodeOpsSnapshotChecksumMismatch` | `increase(checksum_mismatch[1h]) > 0` | Critical | Snapshot integrity compromised | `.axiom/runbooks/axiom-snapshot-checksum-mismatch.md` |
| `CodeOpsBenchmarkScoreRegression` | Composite score drops > 10% between syncs | Warning | Investigate benchmark regressions | `.axiom/runbooks/axiom-benchmark-score-regression.md` |
| `CodeOpsSubscriptionHealthDegraded` | `subscription_health_status == 0` for > 15 min | Critical | Investigate subscription health drivers | `.axiom/runbooks/axiom-subscription-health-degraded.md` |

**Verification**: CI or trace-auditor confirms every alert in this table has a corresponding
runbook file in `.axiom/runbooks/`. Missing runbooks are reported as a gap.

---

## 3. Threshold Types

### 3.1 Static Thresholds

A fixed numeric boundary that fires when a metric crosses it.

**When to use**: Well-understood metrics with stable baselines and clear failure boundaries
(e.g., error rate > 30%, queue depth > 50, OOM kill count > 0).

**Prometheus example**:
```yaml
- alert: CodeOpsQueueDepthHigh
  expr: codeops_queue_depth > 50
  for: 15m
  labels:
    severity: warning
  annotations:
    summary: "Queue depth exceeds 50 for 15 minutes"
    runbook_url: ".axiom/runbooks/axiom-queue-depth-high.md"
```

**Strengths**: Simple, predictable, easy to reason about.
**Weaknesses**: Requires manual tuning; does not adapt to seasonal patterns or growth.

### 3.2 Dynamic Thresholds (Anomaly Detection)

Thresholds derived from historical baselines, typically using statistical methods
(standard deviation bands, percentile envelopes, or ML-based anomaly detection).

**When to use**: Metrics with natural variability, seasonal patterns, or gradual drift
where a fixed threshold would either miss slow degradation or fire on normal variation
(e.g., request latency that varies by time of day, token usage that grows with adoption).

**Approaches**:
- **Z-score / standard deviation bands**: Fire when metric deviates > N standard deviations from a rolling mean.
- **Percentile envelopes**: Fire when metric exceeds the historical p99 for the same time window.
- **Forecast-based**: Fire when the metric diverges from a predicted trend (see `specs/66-Predictive-Observability.md`).

**Prometheus example** (using `predict_linear`):
```yaml
- alert: CodeOpsDiskFull4Hours
  expr: predict_linear(node_filesystem_avail_bytes[6h], 4*3600) < 0
  for: 30m
  labels:
    severity: warning
  annotations:
    summary: "Disk predicted to fill within 4 hours"
    runbook_url: ".axiom/runbooks/axiom-disk-full.md"
```

**Strengths**: Adapts to changing baselines; catches slow degradation.
**Weaknesses**: More complex; requires sufficient history; can produce false positives during legitimate traffic changes.

### 3.3 Burn-Rate Alerts (SLO-Based)

Alerts based on the rate at which an error budget is being consumed. Defined relative
to an SLO target and a time window.

**When to use**: Services with defined SLOs where you want to alert proportionally to
the severity of the impact on the error budget rather than on absolute metric values.

**Concept**: If a service has a 99.9% availability SLO (monthly error budget = 43.2 minutes),
a burn rate of 1x means the budget is consumed at exactly the expected rate. A burn rate
of 14.4x means the entire monthly budget will be exhausted in 1 hour.

**Multi-window burn-rate pattern** (recommended):

| Window | Burn Rate | Budget Consumed | Severity | Response |
|--------|-----------|-----------------|----------|----------|
| 1h | 14.4x | 2% in 1h | Critical (page) | Immediate investigation |
| 6h | 6x | 5% in 6h | Critical (page) | Urgent investigation |
| 3d | 1x | 10% in 3d | Warning (ticket) | Next business day |

**Prometheus example** (using recording rules):
```yaml
# Recording rule: 5m error rate
- record: axiom:sli_error_rate:5m
  expr: |
    rate(codeops_step_total{status="failed"}[5m])
    / rate(codeops_step_total[5m])

# Alert: fast burn (1h window, 14.4x burn rate)
- alert: CodeOpsSLOFastBurn
  expr: |
    axiom:sli_error_rate:5m > (14.4 * 0.001)
    and
    axiom:sli_error_rate:1h > (14.4 * 0.001)
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "Error budget burning at 14.4x rate"
    runbook_url: ".axiom/runbooks/axiom-slo-fast-burn.md"
```

**Strengths**: Directly tied to user impact; naturally prioritizes by severity; reduces noise for transient spikes.
**Weaknesses**: Requires well-defined SLOs; more complex to set up; needs recording rules for efficiency.

### 3.4 Composite Alerts

Alerts that combine multiple conditions or signals to reduce false positives and
increase confidence that a real problem exists.

**When to use**: When a single metric is noisy but the combination of two or more
signals strongly indicates a problem (e.g., high latency AND high error rate AND
low throughput together indicate a real outage, not just a slow query).

**Prometheus example**:
```yaml
- alert: CodeOpsServiceDegraded
  expr: |
    (
      histogram_quantile(0.95, rate(codeops_step_duration_seconds_bucket[5m])) > 300
    )
    and
    (
      rate(codeops_step_total{status="failed"}[5m])
      / rate(codeops_step_total[5m]) > 0.1
    )
  for: 10m
  labels:
    severity: critical
  annotations:
    summary: "Service degraded: high latency AND elevated error rate"
    runbook_url: ".axiom/runbooks/axiom-service-degraded.md"
```

**Strengths**: High confidence; low false-positive rate.
**Weaknesses**: Can miss problems where only one signal is affected; more complex to maintain.

---

## 4. Severity and Routing/Escalation Patterns

### 4.1 Severity Definitions

Per `specs/34-Observability-And-Metrics.md#alert-severity-definitions`:

| Severity | Response Time | Notification Channel | Escalation |
|----------|---------------|---------------------|------------|
| **Critical** | Immediate (< 15 min) | PagerDuty / on-call + Slack | Auto-escalate if unacknowledged in 15 min |
| **Warning** | Next business day | Slack channel | Create Jira ticket if unresolved in 24h |
| **Info** | Review in weekly ops review | Dashboard only | No escalation |

### 4.2 Severity Selection Guide

| Signal | Severity | Rationale |
|--------|----------|-----------|
| Data loss or corruption | Critical | Irreversible user impact |
| Service completely unavailable | Critical | All users affected |
| SLO burn rate > 14.4x (1h window) | Critical | Monthly budget exhausted in < 1h |
| OOM kill or pod crash | Critical | Immediate capacity impact |
| Elevated error rate (> 30%) | Warning | Degraded but not down |
| High latency (p95 > threshold) | Warning | User experience degraded |
| Queue depth growing | Warning | Capacity pressure building |
| SLO burn rate > 1x (3d window) | Warning | Budget erosion over days |
| Metric anomaly (no user impact yet) | Info | Early signal for investigation |
| Capacity forecast (days to exhaustion) | Info | Planning signal |

### 4.3 Routing Patterns

**Route by severity and team ownership**:

```
Critical alerts
  --> PagerDuty (on-call rotation)
  --> #incidents Slack channel
  --> Auto-create Jira incident ticket

Warning alerts
  --> #ops-alerts Slack channel
  --> Auto-create Jira task (if not resolved in 24h)

Info alerts
  --> Dashboard annotation only
  --> Weekly ops review agenda
```

**Route by component**:

| Component | Primary Team | Escalation Team |
|-----------|-------------|-----------------|
| Repo Runner | Platform Engineering | SRE |
| Control Plane | Platform Engineering | SRE |
| OpenCode Integration | Platform Engineering | OpenCode vendor support |
| Snapshot/Restore | Infrastructure | SRE |
| CI/CD Pipeline | DevOps | Platform Engineering |

### 4.4 Escalation Chains

Define escalation chains with increasing scope and authority:

1. **L1 -- On-call engineer** (0-15 min): Triage, run runbook, mitigate if possible.
2. **L2 -- Team lead / senior engineer** (15-30 min): Deeper investigation, coordinate if multi-component.
3. **L3 -- Engineering manager + SRE** (30-60 min): Incident commander role, cross-team coordination.
4. **L4 -- VP Engineering / CTO** (60+ min): Business impact communication, resource allocation.

**Auto-escalation rules**:
- Unacknowledged Critical alert after 15 min --> escalate to L2.
- Unresolved Critical alert after 30 min --> escalate to L3.
- Unresolved Critical alert after 60 min --> escalate to L4.
- Unresolved Warning alert after 24h --> create Jira ticket and assign to team lead.

---

## 5. Noise Reduction

### 5.1 Common Noise Sources and Fixes

| Noise Source | Symptom | Fix |
|-------------|---------|-----|
| **Flapping alerts** | Alert fires and resolves repeatedly in short intervals | Add `for` duration (e.g., `for: 15m`); increase hysteresis |
| **Transient spikes** | Brief metric spikes trigger alerts that auto-resolve | Use `for` clause; use `avg_over_time()` or `rate()` over longer windows |
| **Deployment noise** | Alerts fire during every deploy due to restart latency | Add deployment annotation; suppress alerts during deploy windows; use `absent_over_time()` |
| **Low-traffic false positives** | Error rate spikes to 100% on 1 request out of 1 | Add minimum request volume guard (e.g., `rate(total[5m]) > 1`) |
| **Duplicate alerts** | Same problem fires multiple alerts across related metrics | Use inhibition rules to suppress lower-severity alerts when higher-severity fires |
| **Stale alerts** | Alerts for decommissioned services or renamed metrics | Regular alert hygiene reviews; CI validation of metric references |

### 5.2 Alertmanager Noise Reduction Features

| Feature | Purpose | Example |
|---------|---------|---------|
| **Grouping** | Combine related alerts into a single notification | `group_by: ['alertname', 'repo']` |
| **Inhibition** | Suppress alerts when a higher-priority alert is active | Inhibit `Warning` when `Critical` fires for same `repo` |
| **Silencing** | Temporarily mute alerts during maintenance | Silence `CodeOpsStepDurationHigh` during planned migration |
| **Repeat interval** | Control how often a firing alert re-notifies | `repeat_interval: 4h` for Warning; `repeat_interval: 15m` for Critical |
| **Group wait** | Buffer time before sending the first notification for a group | `group_wait: 30s` (collect related alerts before notifying) |
| **Group interval** | Minimum time between notifications for the same group | `group_interval: 5m` |

### 5.3 Minimum Volume Guards

Always pair rate-based alerts with a minimum volume check to avoid false positives
on low-traffic services:

```yaml
# BAD: fires on 1 error out of 1 request
- alert: HighErrorRate
  expr: rate(errors[5m]) / rate(total[5m]) > 0.3

# GOOD: requires minimum 10 requests per 5 minutes
- alert: HighErrorRate
  expr: |
    (rate(errors[5m]) / rate(total[5m]) > 0.3)
    and
    (rate(total[5m]) > 0.033)  # ~10 requests per 5 min
```

---

## 6. Alert-as-Code

### 6.1 Principles

- **Version control**: Alert rules MUST live in the repository, not only in the monitoring UI.
- **Reproducibility**: Any alert configuration MUST be recreatable from its code definition.
- **Review**: Alert rule changes go through the same PR review process as code changes.
- **Testing**: Alert rules SHOULD be tested in CI (syntax validation, unit tests for expressions).
- **Idempotency**: Applying the same definition twice produces the same result.

### 6.2 File Locations

| Tool | Format | Location |
|------|--------|----------|
| Prometheus | YAML (rule groups) | `.axiom/alerts/<group-name>.rules.yml` |
| Alertmanager | YAML (routing config) | `.axiom/alerts/alertmanager.yml` |
| Datadog | Terraform HCL | `infra/monitoring/alerts/<name>.tf` |
| CloudWatch | JSON (CloudFormation) | `infra/monitoring/alerts/<name>.json` |
| PagerDuty | Terraform HCL | `infra/monitoring/pagerduty/<name>.tf` |

### 6.3 Prometheus Alert Rule Structure

Every alert rule MUST include these fields:

```yaml
groups:
  - name: axiom-repo-runner
    interval: 30s  # evaluation interval
    rules:
      - alert: AlertName
        expr: <PromQL expression>
        for: <duration>  # REQUIRED: prevents flapping
        labels:
          severity: critical | warning | info
          team: <owning-team>
          component: <component-name>
        annotations:
          summary: "Human-readable one-line summary"
          description: "Detailed description with {{ $labels.repo }} template variables"
          runbook_url: ".axiom/runbooks/<alert-name-kebab-case>.md"  # REQUIRED per REQ-OBS-RUNBOOK-001
          dashboard_url: "<link to relevant dashboard panel>"
```

**Required annotations** (per `REQ-OBS-RUNBOOK-001`):
- `runbook_url`: Path to the runbook file. MUST be present on every alert.
- `summary`: One-line human-readable description.

**Recommended annotations**:
- `description`: Detailed description with template variables for context.
- `dashboard_url`: Link to the relevant dashboard for visual investigation.

### 6.4 Alert Rule Testing

**Prometheus unit tests** (using `promtool`):

```yaml
# tests/alerts_test.yml
rule_files:
  - ../.axiom/alerts/axiom-repo-runner.rules.yml

evaluation_interval: 1m

tests:
  - interval: 1m
    input_series:
      - series: 'codeops_queue_depth{queue="default"}'
        values: '60 60 60 60 60 60 60 60 60 60 60 60 60 60 60'
    alert_rule_test:
      - eval_time: 15m
        alertname: CodeOpsQueueDepthHigh
        exp_alerts:
          - exp_labels:
              severity: warning
              queue: default
```

**CI validation checklist**:
- [ ] `promtool check rules .axiom/alerts/*.rules.yml` passes
- [ ] `promtool test rules tests/alerts_test.yml` passes
- [ ] Every alert has a `runbook_url` annotation
- [ ] Every `runbook_url` points to an existing file in `.axiom/runbooks/`
- [ ] No unbounded label selectors (`{job=~".*"}`)
- [ ] All referenced metrics exist in the metric catalog

---

## 7. Alert Lifecycle

### 7.1 Lifecycle Stages

| Stage | Description | Owner |
|-------|-------------|-------|
| **Design** | Define alert condition, severity, routing, and runbook | Service team + SRE |
| **Implement** | Write alert rule, create runbook, add to alert-as-code | Service team |
| **Review** | PR review of alert rule and runbook | SRE + peer review |
| **Deploy** | Apply alert configuration to monitoring stack | CI/CD pipeline |
| **Tune** | Adjust thresholds based on false positive/negative data | Service team + SRE |
| **Retire** | Remove alert when the monitored condition is no longer relevant | Service team |

### 7.2 Alert Hygiene Practices

**Regular review cadence** (recommended quarterly):

1. **Coverage audit**: Are all critical paths covered by alerts? Cross-reference with the required alerts table.
2. **Noise audit**: Which alerts fired most often? Were they actionable? Calculate signal-to-noise ratio.
3. **Staleness check**: Do all alert rules reference metrics that still exist? Run CI validation.
4. **Runbook freshness**: Are runbooks still accurate? Do the diagnostic commands still work?
5. **Threshold review**: Have baselines shifted? Do static thresholds need adjustment?
6. **Ownership review**: Is the routing still correct? Has team structure changed?

**Metrics to track for alert health**:

| Metric | Target | Action if Missed |
|--------|--------|-----------------|
| Alert-to-runbook coverage | 100% | Create missing runbooks |
| False positive rate | < 5% per alert | Tune threshold or add volume guard |
| Mean time to acknowledge (MTTA) | < 15 min for Critical | Review routing and escalation |
| Mean time to resolve (MTTR) | < 1h for Critical | Improve runbooks; add automation |
| Alerts per on-call shift | < 10 actionable | Reduce noise; consolidate alerts |

### 7.3 Post-Incident Alert Review

After every incident, review the alerting chain:

1. **Did the right alert fire?** If not, add or fix the alert.
2. **Did it fire fast enough?** If not, tighten the threshold or reduce the `for` duration.
3. **Was the runbook helpful?** If not, update it with lessons learned.
4. **Were there too many alerts?** If so, add inhibition rules or consolidate.
5. **Was the routing correct?** If not, update the routing configuration.

---

## 8. Alert-to-Runbook Linkage (REQ-OBS-RUNBOOK-001)

### 8.1 Contract

Per `specs/34-Observability-And-Metrics.md#REQ-OBS-RUNBOOK-001`:

> Every alert defined in the "Required Alerts" table MUST have a linked runbook.

**Runbook location**: `.axiom/runbooks/`

**Naming convention**: `<alert-name-kebab-case>.md`
(e.g., `CodeOpsStepDurationHigh` --> `.axiom/runbooks/axiom-step-duration-high.md`)

### 8.2 Required Runbook Sections

Every runbook MUST contain at minimum:

| Section | Purpose |
|---------|---------|
| **Triage** | How to determine the scope and severity of the alert condition |
| **Diagnose** | Specific commands or queries to identify the root cause |
| **Mitigate** | Immediate actions to reduce impact (restart, scale, disable feature) |
| **Verify** | How to confirm the mitigation worked |
| **Rollback** | Steps to revert if mitigation fails |

### 8.3 Verification

**CI check** (or trace-auditor):
1. Extract all alert names from `.axiom/alerts/*.rules.yml`.
2. For each alert, verify `annotations.runbook_url` is present.
3. Verify the referenced runbook file exists in `.axiom/runbooks/`.
4. Verify the runbook contains all 5 required sections.
5. Report missing runbooks as a gap.

**v1 approach**: In v1 (log-based monitoring), runbooks are still required as markdown
files in `.axiom/runbooks/`. Log-based alert configurations (e.g., CloudWatch alarm
definitions) SHOULD reference the runbook path in their description.

---

## 9. Anti-Patterns

### 9.1 Alert Design Anti-Patterns (MUST avoid)

| Anti-Pattern | Problem | Fix |
|-------------|---------|-----|
| **Alert without runbook** | Responder has no guidance; wastes time during incident | Create runbook before deploying alert (`REQ-OBS-RUNBOOK-001`) |
| **Cause-based paging** | Paging on CPU/memory instead of user-visible symptoms | Alert on symptoms (error rate, latency); use cause metrics for triage |
| **Missing `for` clause** | Transient spikes trigger pages; alert flaps | Always include `for` duration (minimum 2m for Critical, 10m for Warning) |
| **Average-based thresholds** | Averages hide tail latency; miss p99 problems | Use percentile-based thresholds (`histogram_quantile`) |
| **Unbounded cardinality** | Alert rule matches too many label combinations; evaluation explodes | Use explicit label matchers; aggregate before alerting |
| **Copy-paste alerts** | Duplicated rules drift apart; maintenance burden grows | Use templates, recording rules, or alert rule generators |
| **Alert on absence without guard** | `absent()` fires during scrape gaps or restarts | Use `absent_over_time()` with a window longer than scrape interval |
| **Percentage alerts without volume** | 100% error rate on 1 request triggers page | Add minimum volume guard (see section 5.3) |
| **Too many severities** | 5+ severity levels confuse responders | Use exactly 3: Critical, Warning, Info |
| **UI-only alerts** | Alert rules exist only in the monitoring UI; no version control | Use alert-as-code (see section 6) |

### 9.2 Operational Anti-Patterns

| Anti-Pattern | Problem | Fix |
|-------------|---------|-----|
| **Alert fatigue** | Too many alerts; responders ignore them all | Audit noise; add inhibition; raise thresholds; consolidate |
| **Stale runbooks** | Runbook commands no longer work; wrong service names | Review runbooks quarterly; test commands in staging |
| **No escalation path** | Critical alert fires but nobody responds | Define auto-escalation chains with timeouts |
| **Alert-and-forget** | Alert deployed but never tuned after initial deployment | Schedule quarterly alert hygiene reviews |
| **Silencing as fix** | Noisy alert silenced permanently instead of fixed | Silences MUST have expiration; fix the root cause |

---

## 10. v1 Log-Based Alerting

In v1, monitoring is log-based per `specs/34-Observability-And-Metrics.md#v1-monitoring-approach`.
The same alert design principles apply, but the implementation uses log queries instead
of metric expressions.

### 10.1 v1 Critical Log-Based Alerts

| Alert Equivalent | Log Query | Tool |
|-----------------|-----------|------|
| `CodeOpsOpenCodeHealthFailed` | `event_type = "opencode_health_failed"` | CloudWatch Logs Insights / ELK |
| `CodeOpsEscalationRate` | `event_type = "escalation_triggered"` count > 5 in 1h | CloudWatch Metric Filter |
| `CodeOpsPodOOMKilled` | `event_type = "run_failed"` AND `reason = "oom_killed"` | CloudWatch Logs Insights |

### 10.2 v1 Runbook Requirement

Even in v1, runbooks are required as markdown files in `.axiom/runbooks/`. Log-based
alert configurations SHOULD reference the runbook path in their description field.

---

## 11. Axiom Integration

### 11.1 Evidence Capture

When alert rules are part of verification evidence:
- Include the alert rule YAML in the evidence bundle.
- Capture a screenshot of the alert firing in the monitoring UI (if applicable).
- Reference the alert-as-code file path in `axiom:trace` markers.

### 11.2 Spec Alignment

Alert implementations MUST align with the required alerts table in
`specs/34-Observability-And-Metrics.md#baseline-alerting-expectations`. When auditing:
- Verify every spec-defined alert exists in `.axiom/alerts/`.
- Verify every alert has a `runbook_url` annotation pointing to `.axiom/runbooks/`.
- Verify every referenced runbook exists and contains the 5 required sections.
- Verify severity levels match the spec definitions.

### 11.3 Related Skills

| Skill | Relationship |
|-------|-------------|
| `dashboard-design-axiom` | Dashboards that visualize the metrics alerts fire on |
| `sre-ops-axiom` | SLO/SLI definitions that drive burn-rate alerts |
| `runbook-writing-axiom` | Style guide for the runbooks linked from alerts |
| `metrics-instrumentation-axiom` | Metric emission that feeds alert expressions |
| `observability-diagnosis-axiom` | Diagnostic workflows triggered by alert notifications |
| `predictive-observability-axiom` | Forecast-based and anomaly-detection alert thresholds |
| `chaos-engineer-axiom` | Fault injection to validate alert rules fire correctly |

---

axiom:trace work_item=observability-skills-collection-01 spec=specs/34-Observability-And-Metrics.md#baseline-alerting-expectations,specs/34-Observability-And-Metrics.md#REQ-OBS-RUNBOOK-001 plan=phase-87-2/task-87-2-2/step-87-2-2-1 test= doc=.opencode/skills/alert-engineering-axiom/SKILL.md evidence= commit=
