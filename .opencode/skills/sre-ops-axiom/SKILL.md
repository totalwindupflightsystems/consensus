---
name: sre-ops-axiom
description: >
  SLO/SLI definition, error budget calculation, on-call runbook creation, deploy safety
  checklists, and observability setup. This skill is paired with the @sre-ops-axiom agent
  for deploy safety reviews, SLO definition, runbook creation, and incident response
  coordination. Load this skill for any production operations, reliability, or observability work.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-02-27"
  primary_spec: specs/34-Observability-And-Metrics.md
  secondary_specs:
    - specs/25-Structured-Logging-Events.md
    - specs/50-Runtime-Logging-Modes-And-Formats.md
    - specs/00-PRD.md
tags:
  vertical: [sre, ops]
  category: operations
  core: false
---

# SRE Ops Skill (Portable)

> **"No deploy without a rollback plan. No SLO without an error budget."**
>
> **"Observability is not optional for production services."**

This skill provides portable, production-grade guidance for Site Reliability Engineering
operations: SLO/SLI definition, error budget management, runbook creation, deploy safety,
and observability setup. It is the operational companion to `enterprise-release-quality`
(release gates) and `chaos-engineer-axiom` (resilience testing).

**Agent integration**: This skill is paired with the `@sre-ops-axiom` agent. When this
skill is loaded, invoke `@sre-ops-axiom` for: deploy safety reviews, SLO definition,
runbook creation, and incident response coordination.

---

## Activation

Load this skill when:
- Defining SLOs and SLIs for a service
- Calculating or reviewing error budgets
- Creating or updating on-call runbooks
- Preparing a deploy safety checklist
- Setting up observability (metrics, logs, traces) for a service
- Reviewing production readiness of a Axiom deployment
- Responding to an incident or post-incident review
- Monitoring `axiom serve`, `axiom run`, or OpenCode integration health

---

## Non-Negotiables

1. **No deploy without a rollback plan.** Every deployment must have a documented, tested
   rollback procedure. "We'll figure it out" is not a rollback plan.

2. **No SLO without an error budget.** An SLO without an error budget is just a wish.
   Error budget = 1 - SLO target. Track burn rate.

3. **Fail-closed on missing observability for production services.** A production service
   without structured logging, health checks, and basic metrics is not production-ready.

4. **Every alert must have a runbook.** An alert without a runbook is noise. Every alert
   in the alert table must link to a runbook with triage, diagnose, mitigate, verify, and
   rollback sections. (Source: `specs/34-Observability-And-Metrics.md` REQ-OBS-RUNBOOK-001)

5. **Never silence an alert without documenting why.** Alert suppression requires a
   documented reason, owner, and expiration date.

---

## SLO/SLI Definition Workflow

### Step 1: Identify User-Facing Signals

Start with the user's perspective. What do they care about?

| Signal Category | Example SLI | Measurement |
|----------------|-------------|-------------|
| **Availability** | Service is reachable | `successful_requests / total_requests` |
| **Latency** | Responses are fast | `p95 response time < threshold` |
| **Error Rate** | Responses are correct | `error_responses / total_responses` |
| **Throughput** | Service handles load | `requests_per_second >= minimum` |
| **Freshness** | Data is current | `time_since_last_update < threshold` |

### Step 2: Define SLIs

An SLI (Service Level Indicator) is a quantitative measure of a service aspect.

```yaml
# .axiom/slo/sli-definitions.yaml
slis:
  - name: "availability"
    description: "Percentage of successful HTTP responses (non-5xx)"
    measurement: "count(status < 500) / count(all requests)"
    good_event: "HTTP response with status < 500"
    total_event: "Any HTTP request to the service"
    data_source: "structured logs (event_type: opencode_response_received)"

  - name: "latency_p95"
    description: "95th percentile response latency"
    measurement: "p95(response_duration_ms)"
    unit: "milliseconds"
    data_source: "codeops_step_duration_seconds histogram"

  - name: "error_rate"
    description: "Percentage of requests resulting in errors"
    measurement: "count(status >= 500) / count(all requests)"
    data_source: "structured logs"

  - name: "run_success_rate"
    description: "Percentage of Axiom runs that complete successfully"
    measurement: "count(run_completed) / count(run_started)"
    data_source: "structured logs (event_type: run_completed, run_failed)"
```

### Step 3: Set SLO Targets

An SLO (Service Level Objective) is a target value for an SLI.

```yaml
# .axiom/slo/slo-targets.yaml
slos:
  - name: "API Availability"
    sli: "availability"
    target: 99.9  # 99.9% of requests succeed
    window: "30d"  # rolling 30-day window
    error_budget: 0.1  # 0.1% of requests can fail

  - name: "API Latency"
    sli: "latency_p95"
    target: 500  # p95 < 500ms
    window: "30d"
    unit: "milliseconds"

  - name: "Run Success Rate"
    sli: "run_success_rate"
    target: 95.0  # 95% of runs complete successfully
    window: "7d"
    error_budget: 5.0
```

### Step 4: Calculate Error Budget

```
Error Budget = 1 - SLO Target

Example:
  SLO: 99.9% availability over 30 days
  Error Budget: 0.1% = 43.2 minutes of downtime per 30 days
  
  30 days * 24 hours * 60 minutes = 43,200 minutes
  43,200 * 0.001 = 43.2 minutes of allowed downtime
```

### Error Budget Burn Rate

```
Burn Rate = (actual error rate) / (error budget rate)

Example:
  Error budget: 0.1% over 30 days = 0.00333% per day
  Actual errors today: 0.01% = 3x burn rate
  
  At this rate, error budget exhausted in 10 days (30 / 3)
```

### Burn Rate Alerts

| Burn Rate | Window | Severity | Meaning |
|-----------|--------|----------|---------|
| 14.4x | 1 hour | Critical | Budget exhausted in 2 hours |
| 6x | 6 hours | Critical | Budget exhausted in 5 days |
| 3x | 1 day | Warning | Budget exhausted in 10 days |
| 1x | 3 days | Info | On track to exhaust budget |

---

## On-Call Runbook Creation

### Runbook Template

Every alert MUST have a linked runbook. Store runbooks in `.axiom/runbooks/`.

```markdown
# Runbook: <Alert Name>

## Alert Details

**Alert**: <alert name>
**Severity**: Critical | Warning | Info
**Condition**: <Prometheus expression or log query>
**Dashboard**: <link to relevant dashboard>

## 1. Triage

**Goal**: Determine scope and severity.

- [ ] Check: Is this a single instance or widespread?
  ```bash
  <command to check scope>
  ```
- [ ] Check: Are users affected?
  ```bash
  <command to check user impact>
  ```
- [ ] Check: Is this a known issue? (Check recent incidents)

**Escalation**: If scope is widespread or user-facing, escalate to <team/person>.

## 2. Diagnose

**Goal**: Identify root cause.

### Common Causes

| Cause | How to Check | Likelihood |
|-------|-------------|------------|
| <cause 1> | `<diagnostic command>` | High |
| <cause 2> | `<diagnostic command>` | Medium |
| <cause 3> | `<diagnostic command>` | Low |

### Diagnostic Commands

```bash
# Check service health
curl -sf http://127.0.0.1:8100/health | python3 -m json.tool

# Check recent logs
axiom events stream --level ERROR --run-id <run_id> | head -20

# Check resource usage
kubectl top pods -l app=axiom-runner

# Check OpenCode server
curl -sf http://127.0.0.1:4096/global/health
```

## 3. Mitigate

**Goal**: Reduce impact immediately.

### Option A: <Mitigation 1>
```bash
<commands>
```
**Expected result**: <what should happen>
**Risk**: <any risks of this mitigation>

### Option B: <Mitigation 2>
```bash
<commands>
```

## 4. Verify

**Goal**: Confirm mitigation worked.

```bash
# Verify service is healthy
curl -sf http://127.0.0.1:8100/health

# Verify error rate is decreasing
# (check dashboard or logs)

# Verify SLO is recovering
# (check error budget burn rate)
```

## 5. Rollback

**Goal**: Revert if mitigation fails.

```bash
<rollback commands>
```

**Verification after rollback**:
```bash
<verification commands>
```

## Post-Incident

- [ ] Create incident report
- [ ] Update this runbook with lessons learned
- [ ] Add regression test if applicable
- [ ] Review SLO targets if budget was exhausted

## Trace

axiom:trace work_item=<ID> doc=.axiom/runbooks/<alert-name>.md
```

### Axiom-Specific Runbooks

Required runbooks per `specs/34-Observability-And-Metrics.md`:

| Alert | Runbook Path |
|-------|-------------|
| `CodeOpsStepDurationHigh` | `.axiom/runbooks/axiom-step-duration-high.md` |
| `CodeOpsVerificationFailureRateHigh` | `.axiom/runbooks/axiom-verification-failure-rate-high.md` |
| `CodeOpsPodOOMKilled` | `.axiom/runbooks/axiom-pod-oom-killed.md` |
| `CodeOpsQueueDepthHigh` | `.axiom/runbooks/axiom-queue-depth-high.md` |
| `CodeOpsWorkItemStuck` | `.axiom/runbooks/axiom-work-item-stuck.md` |
| `CodeOpsEscalationRate` | `.axiom/runbooks/axiom-escalation-rate.md` |
| `CodeOpsOpenCodeHealthFailed` | `.axiom/runbooks/axiom-opencode-health-failed.md` |
| `CodeOpsSnapshotCaptureFailed` | `.axiom/runbooks/axiom-snapshot-capture-failed.md` |
| `CodeOpsSnapshotRestoreFailed` | `.axiom/runbooks/axiom-snapshot-restore-failed.md` |
| `CodeOpsSnapshotSizeExceeded` | `.axiom/runbooks/axiom-snapshot-size-exceeded.md` |
| `CodeOpsSnapshotGCNotRunning` | `.axiom/runbooks/axiom-snapshot-gc-not-running.md` |
| `CodeOpsSnapshotChecksumMismatch` | `.axiom/runbooks/axiom-snapshot-checksum-mismatch.md` |

---

## Deploy Safety Checklist

### Pre-Deploy

```markdown
## Deploy Safety Checklist: <service> <version>

### Pre-Deploy
- [ ] All tests pass (unit, integration, e2e)
- [ ] Performance benchmarks within budget (no regressions)
- [ ] API contract validation passes (openapi.json in sync)
- [ ] Security review completed (if applicable)
- [ ] Migration guide prepared (if breaking changes)
- [ ] Rollback procedure documented and tested
- [ ] Runbooks updated for new alerts/behaviors
- [ ] Error budget has sufficient remaining budget
- [ ] Feature flags configured (if applicable)
- [ ] Canary/blue-green strategy defined

### Deploy Strategy
- [ ] Strategy: Canary | Blue-Green | Rolling | Feature Flag
- [ ] Canary percentage: <X>%
- [ ] Canary duration: <duration>
- [ ] Rollback trigger: <condition>
- [ ] Kill switch: <how to disable>

### Post-Deploy
- [ ] Health check passes
- [ ] Smoke tests pass
- [ ] Error rate within SLO
- [ ] Latency within SLO
- [ ] No new alerts firing
- [ ] Canary promoted (or rolled back)
```

### Deploy Strategies

| Strategy | When to Use | Rollback Speed | Risk |
|----------|------------|----------------|------|
| **Canary** | Most deploys; gradual rollout | Fast (route traffic away) | Low |
| **Blue-Green** | Zero-downtime required | Instant (switch back) | Medium (2x resources) |
| **Rolling** | Stateless services | Moderate (roll back pods) | Medium |
| **Feature Flag** | Risky features; A/B testing | Instant (toggle flag) | Low |

### Rollback Triggers

Define automatic rollback triggers:

```yaml
rollback_triggers:
  - condition: "error_rate > 5%"
    window: "5m"
    action: "automatic rollback"
  - condition: "p95_latency > 2x baseline"
    window: "10m"
    action: "automatic rollback"
  - condition: "health_check_failures > 3"
    window: "1m"
    action: "automatic rollback"
```

---

## Observability Setup

### Metrics (Prometheus/OTEL)

Per `specs/34-Observability-And-Metrics.md`, Axiom defines these metric categories:

| Category | Key Metrics | Source |
|----------|------------|--------|
| Step execution | `codeops_step_duration_seconds`, `codeops_step_total` | Structured logs |
| Verification | `codeops_verification_result_total` | Structured logs |
| Confidence | `codeops_confidence_score` | Run completion |
| OpenCode | `codeops_opencode_request_duration_seconds` | HTTP events |
| Tokens/Cost | `codeops_opencode_token_usage_total` | Cost collector |
| Retries | `codeops_retry_total`, `codeops_escalation_total` | Structured logs |

v1 uses log-derived metrics. Post-v1 adds Prometheus `/metrics` endpoint.

### Logs (Structured JSONL)

Per `specs/25-Structured-Logging-Events.md`:

- Format: JSON Lines (one JSON object per line)
- Required fields: `timestamp`, `level`, `event_type`, `component`
- Correlation: `run_id`, `work_item_id`, `repo`, `phase_id`, `task_id`, `step_id`
- Redaction: Sensitive data redacted as `[REDACTED]`
- Profiles: `minimal`, `standard`, `verbose`, `debug` (per `specs/50-Runtime-Logging-Modes-And-Formats.md`)

### Traces (Distributed Tracing)

v1: Correlation-based tracing via `correlation_id` field.
Post-v1: OpenTelemetry trace spans with W3C Trace Context propagation.

### Health Checks

```bash
# Axiom server health
curl -sf http://127.0.0.1:8100/health | python3 -m json.tool

# OpenCode server health
curl -sf http://127.0.0.1:4096/global/health | python3 -m json.tool

# Combined health check script
python3 << 'EOF'
import urllib.request, json, sys

checks = [
    ("Axiom server", "http://127.0.0.1:8100/health"),
    ("OpenCode server", "http://127.0.0.1:4096/global/health"),
]

all_healthy = True
for name, url in checks:
    try:
        resp = urllib.request.urlopen(url, timeout=5)
        data = json.loads(resp.read())
        status = data.get("status", data.get("healthy", "unknown"))
        print(f"  {name}: {status}")
    except Exception as e:
        print(f"  {name}: UNHEALTHY ({e})")
        all_healthy = False

sys.exit(0 if all_healthy else 1)
EOF
```

---

## Axiom-Specific Monitoring

### `axiom serve` Monitoring

```bash
# Monitor server health continuously
while true; do
  STATUS=$(curl -sf -o /dev/null -w "%{http_code}" http://127.0.0.1:8100/health)
  if [ "$STATUS" != "200" ]; then
    echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) ALERT: axiom serve unhealthy (HTTP $STATUS)"
  fi
  sleep 10
done
```

### `axiom run` Monitoring

Key events to watch during a run:

| Event | Level | What to Watch For |
|-------|-------|-------------------|
| `run_started` | INFO | Run begins; note `plan_step_count` |
| `step_failed` | ERROR | Step failure; check `error_class` and `reason` |
| `escalation_triggered` | ERROR | Human intervention needed |
| `opencode_health_failed` | ERROR | OpenCode server down |
| `xml_recovery_exhausted` | ERROR | All XML recovery attempts failed |
| `run_blocked` | WARN | Waiting for human input |

### OpenCode Integration Health

```bash
# Check OpenCode session health
curl -sf http://127.0.0.1:4096/session | python3 -c "
import json, sys
sessions = json.load(sys.stdin)
active = [s for s in sessions if s.get('status', {}).get('type') == 'busy']
print(f'Total sessions: {len(sessions)}')
print(f'Active sessions: {len(active)}')
if len(active) > 5:
    print('WARNING: Many active sessions; possible leak')
"
```

---

## SRE Review Verdict

### Verdict Scale

| Verdict | Score | Meaning |
|---------|-------|---------|
| **PASS** | 80-100 | Production-ready; all SRE requirements met |
| **WARN** | 50-79 | Mostly ready; minor gaps in observability or runbooks |
| **FAIL** | 1-49 | Not production-ready; missing critical SRE requirements |
| **BLOCKED** | 0 | Cannot assess (service won't start, no observability) |

### Scoring Rubric

| Check | Weight | Pass Criteria |
|-------|--------|---------------|
| Health check exists and works | 10 | `GET /health` returns 200 with status |
| Structured logging enabled | 15 | JSON Lines with required correlation fields |
| SLOs defined | 15 | At least availability and latency SLOs |
| Error budget tracked | 10 | Error budget calculated and monitored |
| Alerts defined with runbooks | 15 | Every alert has a linked runbook |
| Deploy rollback documented | 15 | Rollback procedure exists and is tested |
| Observability dashboard exists | 10 | Key metrics visible in dashboard |
| Incident response plan exists | 10 | Escalation path and on-call rotation defined |

### Verdict Template

```markdown
## SRE Review Verdict

**Verdict**: PASS | WARN | FAIL | BLOCKED
**Score**: <0-100>
**Date**: <ISO 8601>
**Service**: <service name>
**Reviewer**: @sre-ops-axiom

### Checks
| Check | Score | Status | Notes |
|-------|-------|--------|-------|
| Health check | 10/10 | PASS | GET /health -> 200 |
| Structured logging | 15/15 | PASS | JSONL with correlation fields |
| SLOs defined | 15/15 | PASS | Availability 99.9%, latency p95 < 500ms |
| Error budget | 10/10 | PASS | 43.2 min/month; currently at 80% remaining |
| Alerts + runbooks | 12/15 | WARN | 2 alerts missing runbooks |
| Deploy rollback | 15/15 | PASS | Tested rollback procedure |
| Dashboard | 8/10 | WARN | Missing token usage panel |
| Incident response | 10/10 | PASS | Escalation path defined |

**Total**: 95/100
```

---

## Integration

### Works With

| Skill/Agent | Integration Point |
|-------------|-------------------|
| `@sre-ops-axiom` | **Primary agent** for this skill |
| `docs-runbooks-axiom` | Runbook creation and maintenance |
| `chaos-engineer-axiom` | Resilience testing validates SLOs |
| `enterprise-release-quality` | SRE review is a release gate |
| `performance-benchmark-axiom` | Benchmarks feed SLI baselines |
| `api-contract-validator-axiom` | API health is an SLI |
| `incident-commander-axiom` | Incident response coordination |

### Agent Invocation

When this skill is loaded, invoke `@sre-ops-axiom` for:

- **Deploy safety review**: Before any production deployment
- **SLO definition**: When setting up a new service
- **Runbook creation**: When adding new alerts
- **Incident response**: During active incidents
- **Post-incident review**: After incident resolution

---

## AI-Assisted Development Risks (2026)

| Risk | Mitigation |
|------|------------|
| AI deploys without rollback plan | Non-negotiable: rollback required |
| AI defines SLOs without error budgets | Non-negotiable: error budget required |
| AI creates alerts without runbooks | Non-negotiable: runbook required (REQ-OBS-RUNBOOK-001) |
| AI claims "production-ready" without observability | Fail-closed on missing observability |
| AI silences alerts without documentation | Require documented reason and expiration |
| AI generates runbooks without diagnostic commands | Template enforces concrete commands |
| AI skips canary/blue-green for "simple" deploys | Deploy strategy required for all production deploys |

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|-------------|---------|-----|
| SLO without error budget | No way to make trade-off decisions | Calculate error budget from SLO |
| Alert without runbook | Alert is noise; on-call can't act | Create runbook per REQ-OBS-RUNBOOK-001 |
| Deploy without rollback | No recovery path | Document and test rollback |
| "We'll monitor it" without metrics | No data to monitor | Set up structured logging + metrics |
| Silencing alerts permanently | Hides real problems | Time-bound suppression with owner |
| Health check that always returns 200 | Useless; hides failures | Check actual dependencies |
| Observability added after incident | Reactive, not proactive | Observability is a launch requirement |

---

## Trace

`axiom:trace work_item=sre-ops-axiom spec=specs/34-Observability-And-Metrics.md plan= prompt=.opencode/skills/sre-ops-axiom/SKILL.md evidence= doc= test= commit=`
