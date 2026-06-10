---
name: continuous-verification-axiom
description: >-
  Continuous verification playbook for critical infrastructure. Covers synthetic probe
  workloads, log-to-metric-to-alarm pipelines, fail-closed dead-man's-switch alarms,
  automated quality gates, evidence file formats, alarm round-trip testing, adversarial
  self-checks, and human trust-building over time. Load this skill when building or
  reviewing any system where "it probably works" isn't good enough — security boundaries,
  payment processing, data pipelines, API gateways, certificate rotation, or any property
  that must be continuously proven rather than assumed.
version: "1.0"
tags:
  vertical: [infrastructure, security, reliability, observability]
  category: verification
  core: false
---

# Continuous Verification Playbook

A reusable guide for any agent building verification infrastructure that continuously proves a system works — not just at deploy time, but every few minutes, forever.

Load this skill when:
- Building security enforcement probes (mTLS, authz policies, certificate rotation)
- Designing fail-closed alarm chains for critical properties
- Implementing quality gates that prevent entire bug classes structurally
- Creating evidence files for auditability
- Proving to humans that a system is trustworthy over time (not just "tests pass once")
- Reviewing observability infrastructure for continuous verification gaps

---

## Core Principle

**Verification is not a one-time event. It is a continuously-running system that produces evidence.**

A test that ran once proves nothing about right now. A probe that runs every 5 minutes and has been flowing clean metrics for 30 days — that proves the property holds continuously.

The patterns below form a layered verification stack:

```
┌─────────────────────────────────────────────┐
│  Human Trust Loop (weeks)                   │
│  Spot-checks results, audits evidence       │
├─────────────────────────────────────────────┤
│  Adversarial Self-Check (periodic)          │
│  Tries to disprove "done" claims            │
├─────────────────────────────────────────────┤
│  Alarm Round-Trip (weekly)                  │
│  Proves alarms actually fire                │
├─────────────────────────────────────────────┤
│  Fail-Closed Alarms (always on)            │
│  Silence = alarm, bypass = immediate fire   │
├─────────────────────────────────────────────┤
│  Log → Metric Pipeline (real-time)          │
│  Structured events → metrics → dashboards   │
├─────────────────────────────────────────────┤
│  Synthetic Probes (every N minutes)         │
│  Lightweight pods/scripts testing the       │
│  property from the OUTSIDE                  │
└─────────────────────────────────────────────┘
```

---

## Pattern 1: Synthetic Probe Workload

**What:** A lightweight process that continuously tests a boundary/property and logs structured results.

**Why:** Proves a property is enforced RIGHT NOW, not just that it was configured correctly at deploy time.

**Design principles:**
- The probe MUST NOT have valid credentials/permissions for the thing it's testing (otherwise it proves nothing)
- The probe runs on a fixed schedule (e.g., every 5 minutes) without human intervention
- Every test emits a structured JSON log event with a deterministic result field
- The probe is marked as synthetic test traffic so it can be filtered from production dashboards

**Kubernetes deployment pattern:**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: boundary-tester
  namespace: test-namespace
  labels:
    app: boundary-tester
    axiom/synthetic: "true"           # Mark as synthetic test traffic
    axiom/test-type: "negative"       # Tests what should be REJECTED
spec:
  replicas: 1
  selector:
    matchLabels:
      app: boundary-tester
  template:
    metadata:
      annotations:
        # CRITICAL: disable credentials/sidecar so the probe has NO valid identity
        # Adapt this annotation to your specific system:
        # - Istio: sidecar.istio.io/inject: "false"
        # - AWS IAM: no role annotation
        # - API gateway: no API key mounted
        sidecar.istio.io/inject: "false"
    spec:
      containers:
        - name: tester
          image: "python:3.12-slim"   # Minimal image — no dependencies needed
          command: ["python3", "-c", "<inline-test-script>"]
          resources:
            requests:
              cpu: 10m
              memory: 32Mi
            limits:
              cpu: 50m
              memory: 64Mi
```

**The test script pattern:**

```python
import json, time

TARGET = "http://your-service.namespace.svc.cluster.local:8080"
INTERVAL_SECONDS = 300  # Every 5 minutes
COMPONENT = "boundary-tester"

def test_property():
    """
    Attempt the thing that SHOULD be rejected.
    Returns "REJECTED" if enforcement works, "BYPASSED" if it doesn't.
    """
    try:
        # Replace with your protocol: HTTP, gRPC, raw TCP, WebSocket, etc.
        import urllib.request
        resp = urllib.request.urlopen(TARGET, timeout=10)
        # If we reach here, the boundary is BROKEN
        return "BYPASSED"  # ALARM CONDITION
    except Exception:
        # Connection refused / auth error = enforcement is working
        return "REJECTED"  # EXPECTED RESULT

while True:
    result = test_property()
    event = {
        "event": "enforcement_test",
        "level": "INFO" if result == "REJECTED" else "ERROR",
        "component": COMPONENT,
        "result": result,
        "target": TARGET,
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "synthetic": True,
    }
    print(json.dumps(event), flush=True)  # Structured JSON to stdout
    time.sleep(INTERVAL_SECONDS)
```

**Key design choices:**
- Credentials disabled on the probe — it MUST NOT have valid identity, otherwise it tests nothing
- `synthetic: True` in every event — allows filtering test traffic from real traffic
- `flush=True` — ensures logs reach the collector immediately (no buffering delays)
- JSON to stdout — container runtime captures it, log pipeline ships it to your system
- Minimal image + tiny resource limits — this runs forever, keep it cheap

**Adaptation guide:**

| What you're testing | Probe approach | Expected rejection |
|---|---|---|
| mTLS enforcement | Connect without client cert | TLS handshake failure |
| API key auth | Call without API key header | HTTP 401/403 |
| Network policy | TCP connect from wrong namespace | Connection timeout/refused |
| RBAC/IAM | Call with insufficient permissions | Permission denied |
| Rate limiting | Burst requests above limit | HTTP 429 |
| Input validation | Send malformed payload | HTTP 400 |
| Certificate rotation | Monitor cert serial number | Serial changes on schedule |

---

## Pattern 2: Log → Metric → Alarm Pipeline

**What:** A pipeline that turns structured log events into metrics, then fires alarms when the metrics go wrong.

**Why:** Turns every test probe into a continuously-monitored signal with automatic alerting.

### Step 1 — Structured log event (emitted by probe)

```json
{
  "event": "enforcement_test",
  "result": "REJECTED",
  "component": "boundary-tester",
  "target": "http://service:8080",
  "timestamp": "2026-05-14T10:00:00Z",
  "synthetic": true
}
```

### Step 2 — Metric filter (extracts metric from log)

The metric filter parses structured logs and emits a CloudWatch/Prometheus/Datadog metric.

**CloudWatch Metric Filter (Terraform):**

```hcl
resource "aws_cloudwatch_log_metric_filter" "enforcement_pass" {
  name           = "${var.service_name}-enforcement-pass"
  log_group_name = var.log_group_name

  # Match structured JSON where enforcement worked
  # NOTE: If using Fluent Bit on EKS, logs are JSON-wrapped.
  # Use substring matching on $.log field:
  pattern = "{ ($.log = \"*enforcement_test*\") && ($.log = \"*REJECTED*\") }"

  metric_transformation {
    name      = "EnforcementPass"
    namespace = "${var.service_name}/Verification"
    value     = "1"
    unit      = "Count"
    # Do NOT set default_value = "0" — it floods every period with zeros
    # and overwhelms the real signal. Let missing data be missing.
  }
}

resource "aws_cloudwatch_log_metric_filter" "enforcement_bypassed" {
  name           = "${var.service_name}-enforcement-bypassed"
  log_group_name = var.log_group_name

  pattern = "{ ($.log = \"*enforcement_test*\") && ($.log = \"*BYPASSED*\") }"

  metric_transformation {
    name      = "EnforcementBypassed"
    namespace = "${var.service_name}/Verification"
    value     = "1"
    unit      = "Count"
  }
}
```

**Prometheus alternative (probe exposes /metrics):**

```python
from prometheus_client import Counter, start_http_server

enforcement_pass = Counter('enforcement_test_pass_total', 'Enforcement correctly rejected')
enforcement_bypass = Counter('enforcement_test_bypass_total', 'Enforcement was bypassed')

# In your test loop:
if result == "REJECTED":
    enforcement_pass.inc()
else:
    enforcement_bypass.inc()

start_http_server(9090)  # Prometheus scrapes this
```

### Step 3 — Fail-closed alarms

Two alarms per property — one for silence (dead man's switch), one for bypass (immediate fire):

```hcl
# ALARM 1: Probe went silent (dead man's switch)
resource "aws_cloudwatch_metric_alarm" "enforcement_silent" {
  alarm_name          = "${var.service_name}-enforcement-silent"
  alarm_description   = "No enforcement test results for 15 min — probe may be down"
  namespace           = "${var.service_name}/Verification"
  metric_name         = "EnforcementPass"
  statistic           = "Sum"
  period              = 300            # 5-minute periods
  evaluation_periods  = 3              # 3 consecutive = 15 minutes
  threshold           = 0
  comparison_operator = "LessThanOrEqualToThreshold"

  # CRITICAL: silence = alarm (fail closed)
  treat_missing_data  = "breaching"

  alarm_actions = [var.alert_topic_arn]
  ok_actions    = [var.alert_topic_arn]
}

# ALARM 2: Bypass detected (immediate fire)
resource "aws_cloudwatch_metric_alarm" "enforcement_bypassed" {
  alarm_name          = "${var.service_name}-enforcement-bypassed"
  alarm_description   = "CRITICAL: Enforcement was bypassed"
  namespace           = "${var.service_name}/Verification"
  metric_name         = "EnforcementBypassed"
  statistic           = "Sum"
  period              = 60             # 1-minute period
  evaluation_periods  = 1              # Fire immediately on ANY bypass
  threshold           = 0
  comparison_operator = "GreaterThanThreshold"

  treat_missing_data  = "notBreaching"  # No bypass events = healthy

  alarm_actions = [var.critical_alert_topic_arn]
}
```

**Key design choices:**
- `treat_missing_data = "breaching"` on silence alarm — probe crash triggers alarm within 15 minutes
- `treat_missing_data = "notBreaching"` on bypass alarm — absence of bad events is fine
- No `default_value = "0"` on metric filters — zero-floods drown real signals
- Two separate SNS topics: normal alerts vs critical alerts (different routing/escalation)

---

## Pattern 3: Automated Quality Gate

**What:** A script that runs deterministic checks on every commit, making entire bug classes structurally impossible to reintroduce.

**Why:** Once you find a bug, prevent it structurally — don't rely on humans remembering.

**The quality gate runner:**

```python
#!/usr/bin/env python3
"""
Continuous quality gate — runs on every commit (CI) or on schedule.
Each check prevents a specific class of bug from being (re)introduced.
"""
import sys

def check_no_unsafe_patterns():
    """
    FAIL if any config file contains a known-dangerous pattern.
    Adapt: replace with YOUR dangerous pattern (SQL without params,
    shell=True, hardcoded secrets, overly-permissive IAM, etc.)
    """
    import glob, re
    dangerous_pattern = re.compile(r'your-dangerous-pattern-here')
    failures = []
    for f in glob.glob("**/*.yaml", recursive=True):
        content = open(f).read()
        if dangerous_pattern.search(content):
            failures.append(f)
    if failures:
        for f in failures:
            print(f"  FAIL  {f} — contains unsafe pattern")
        return False
    print("  PASS  No unsafe patterns found")
    return True

def check_required_fields_present():
    """
    FAIL if structured log events are missing required fields.
    """
    import glob, json
    # Check test fixtures or actual log samples
    # ...
    print("  PASS  All required fields present")
    return True

def check_evidence_freshness():
    """
    WARN if evidence files are older than 7 days.
    """
    import os, time, glob
    stale = []
    for f in glob.glob("**/verification.md", recursive=True):
        age_days = (time.time() - os.path.getmtime(f)) / 86400
        if age_days > 7:
            stale.append((f, int(age_days)))
    if stale:
        for f, days in stale:
            print(f"  WARN  {f} — {days} days old")
        return True  # WARN, not FAIL
    print("  PASS  All evidence files are fresh")
    return True

# Add checks here as you find bugs — each check prevents a class of bug
checks = [
    ("No unsafe patterns in configs", check_no_unsafe_patterns),
    ("Required log fields present", check_required_fields_present),
    ("Evidence freshness", check_evidence_freshness),
]

overall = True
for name, fn in checks:
    print(f"\n── {name} ──")
    if not fn():
        overall = False

print(f"\n{'='*40}")
print(f"OVERALL: {'PASS' if overall else 'FAIL'}")
sys.exit(0 if overall else 1)
```

**How to add a new check after finding a bug:**
1. Find a bug (config pattern, missing validation, unsafe default)
2. Write a regex/AST/grep check that detects that pattern
3. Add it to the `checks` list
4. That bug class can NEVER be reintroduced — the gate blocks it on every commit

**Integration with CI:**

```yaml
# .github/workflows/quality-gate.yml
- name: Run quality gate
  run: python3 scripts/quality_gate.py
  # Non-zero exit = PR blocked
```

---

## Pattern 4: Evidence File Format

**What:** A committed file that proves a property was verified at a specific time with specific commands.

**Why:** Humans can independently verify any claim by re-running the commands.

```markdown
---
mb/type: evidence
title: "<property being verified>"
created: <ISO 8601 timestamp>
status: PASS | FAIL | PARTIAL
work_item: <work-item-id>
---

## What was verified

<1-2 sentence description of the property being proven>

## Commands used

    <exact command — copy-paste-able>

## Raw output

    <unfiltered output from the command>
    <include enough lines to prove the claim>

## Corroboration (optional — second source)

    <a different command that confirms the same property>
    <output>

## Verdict

<PASS | FAIL | PARTIAL> — <brief justification referencing the raw output>
```

**Key properties of good evidence:**
- Commands are copy-paste-able — anyone can re-run them
- Raw output included — not summarized, filtered, or interpreted
- Multiple sources corroborate when possible (logs + metrics, or two different tools)
- Timestamp recorded — so you know WHEN it was verified
- Status is honest — PARTIAL is better than a false PASS

**Anti-patterns to avoid:**
- Evidence that says "verified" but shows no output
- Commands that require credentials the reader doesn't have (include how to get access)
- Output from weeks ago with no re-verification plan
- Summarized results that hide failures ("3/4 passed" without showing which failed)

---

## Pattern 5: Continuous Verification Schedule

**What:** Overlapping probe schedules that ensure no verification gap exceeds a defined threshold.

**Design rule:** The maximum undetected failure window is `probe_interval × evaluation_periods`. Design your schedule so this window is acceptable for the property's criticality.

| Property Criticality | Probe Interval | Alarm Fires After | Max Undetected Window |
|---|---|---|---|
| Critical (auth bypass, data leak) | 1 minute | 3 minutes | 3 minutes |
| High (cert rotation, rate limiting) | 5 minutes | 15 minutes | 15 minutes |
| Medium (log delivery, backup freshness) | 15 minutes | 45 minutes | 45 minutes |
| Low (cleanup jobs, cache warming) | 60 minutes | 3 hours | 3 hours |

**Example multi-probe schedule:**

```
┌─ Every 1 minute ───────────────────────────────────────┐
│  Auth boundary probes (highest criticality)            │
│  Alarm: 3 minutes of silence = FIRE                   │
└────────────────────────────────────────────────────────┘

┌─ Every 5 minutes ──────────────────────────────────────┐
│  Network policy probes, encryption probes              │
│  Alarm: 15 minutes of silence = FIRE                  │
└────────────────────────────────────────────────────────┘

┌─ Every 15 minutes ─────────────────────────────────────┐
│  Certificate freshness checks, log pipeline health     │
│  Alarm: 45 minutes of silence = FIRE                  │
└────────────────────────────────────────────────────────┘

┌─ Every 60 minutes ─────────────────────────────────────┐
│  Backup validation, retention policy, cleanup jobs     │
│  Alarm: 3 hours of silence = FIRE                     │
└────────────────────────────────────────────────────────┘
```

---

## Pattern 6: Alarm Round-Trip Testing

**What:** Prove that your alarms actually fire by deliberately triggering them.

**Why:** An alarm that has never fired is indistinguishable from a broken alarm.

**The round-trip test:**

```bash
# 1. Inject a synthetic metric that crosses the alarm threshold
aws cloudwatch put-metric-data \
  --namespace "MyService/Verification" \
  --metric-name "EnforcementBypassed" \
  --value 1 --unit Count

# 2. Wait for alarm evaluation (period + buffer)
sleep 65

# 3. Verify alarm transitioned to ALARM state
STATE=$(aws cloudwatch describe-alarms \
  --alarm-names "my-service-enforcement-bypassed" \
  --query 'MetricAlarms[0].StateValue' --output text)
echo "Alarm state: $STATE"  # Expected: ALARM

# 4. Wait for recovery (no more events → returns to OK)
sleep 120
STATE=$(aws cloudwatch describe-alarms \
  --alarm-names "my-service-enforcement-bypassed" \
  --query 'MetricAlarms[0].StateValue' --output text)
echo "Recovery state: $STATE"  # Expected: OK
```

**Document the round-trip as evidence:**
- Record: OK → ALARM transition timestamp
- Record: ALARM → OK recovery timestamp
- Record: notification delivery confirmation (SNS/PagerDuty/Slack message received)
- This proves: metric → alarm → notification → recovery — end-to-end

**Schedule:** Run alarm round-trips weekly or after any alarm infrastructure change.

---

## Pattern 7: Adversarial Self-Check (The Score Goes Down)

**What:** Periodically challenge your own "done" claims with adversarial intent.

**Why:** The agent that built something believes its own claims. An independent adversarial review catches gaps between "claimed verified" and "actually proven."

**What the adversarial check examines:**

| Claim | Adversarial Question | Red Flag |
|---|---|---|
| "Tests pass" | Is there raw output? Or just "PASS" with no evidence? | No raw output = unverified |
| "Pod is running" | Is it the real workload or a placeholder image? | Image tag is `latest` or `placeholder` |
| "Alarm is configured" | Has it ever actually fired? | No state transitions in history |
| "Metric is flowing" | Are there actual datapoints in the last hour? | Metric registered but 0 datapoints |
| "Property holds" | Can I construct a scenario where it fails? | No negative tests exist |

**The scoring pattern:**

```
Before adversarial review:  97/100 (self-reported by builder)
After adversarial review:   62/100 (found: placeholder claiming PASS, untested alarm)
After fixes applied:        84/100 (real evidence, alarm round-trip proven)
```

**If the score only goes UP, the review isn't adversarial enough.** A score that drops means the system is catching real gaps. A score that never drops means the review is rubber-stamping claims.

---

## Pattern 8: Structured Log Schema for Verification Events

**What:** A consistent JSON schema for all verification events, enabling automated parsing and cross-correlation.

```json
{
  "event": "<test_type>_test",
  "level": "INFO",
  "component": "<probe-name>",
  "result": "REJECTED | BYPASSED | PASS | FAIL | TIMEOUT",
  "target": "<what was tested — URL, service, endpoint>",
  "timestamp": "<ISO 8601>",
  "synthetic": true,
  "correlation_id": "<uuid — links related events>",
  "environment": "<cluster/region/account identifier>",
  "properties": {
    "<property-specific fields>"
  }
}
```

**Required fields for every verification event:**
- `event` — machine-parseable type (metric filters key on this)
- `result` — deterministic outcome (enables metric extraction)
- `synthetic: true` — distinguishes test traffic from production traffic
- `timestamp` — enables time-correlation across sources
- `component` — identifies which probe produced the event

**Why `synthetic` matters:** Without this field, your verification probes pollute production dashboards and error rates. With it, you can filter: "show me only real traffic" or "show me only test results."

---

## How A Human Builds Trust Over Time

The continuous verification system produces a trust signal that strengthens over time:

| Time | Human Action | Trust Level |
|---|---|---|
| Day 1 | Deploy probes and alarms. Check metrics start flowing. | Low — just deployed |
| Day 2-3 | Spot-check evidence files. Re-run commands manually. Results match? | Growing — corroborated |
| Week 1 | Dashboard review: all metrics flowing continuously? Any unexplained gaps? | Medium — consistent signal |
| Week 2 | Run alarm round-trip tests. Do they fire and recover correctly? | Medium-high — alarms proven |
| Week 3+ | Metrics flowing 24/7, no gaps, alarms fire only when expected | High — system is proven |

**What breaks trust:**
- Metric gap with no explanation (probe crashed — should have triggered silence alarm)
- Alarm fires but nobody investigates (operational gap in response)
- Evidence files with timestamps from weeks ago, never refreshed
- Score that only goes up (adversarial review is too soft)

**What builds trust:**
- Metrics flowing continuously for weeks with no unexplained gaps
- Alarms firing exactly when deliberately triggered, and not firing otherwise
- Evidence files with fresh timestamps from automated verification runs
- Score that dropped when a real gap was found, then recovered after the fix with new evidence

---

## Integration with Axiom

### When to load this skill

Load `continuous-verification-axiom` when:
- `@sre-ops-axiom` is setting up monitoring for a new service
- `@chaos-engineer-axiom` is designing resilience probes
- `@security-engineer-axiom` is building enforcement verification
- `@cloud-engineer-axiom` is deploying infrastructure with security boundaries
- `@qa-axiom` is designing continuous integration verification beyond unit tests
- Any agent needs to prove a property holds continuously (not just at test time)

### Evidence bundle integration

Evidence files produced by continuous verification follow `specs/27-Evidence-Bundle-Schema.md`. Store them at:
```
.memory-bank/work-items/<id>/runs/<run_id>/verification.md
```

### Alarm-to-runbook linkage

Every alarm created via this playbook MUST have a corresponding runbook (per `specs/34-Observability-And-Metrics.md#dashboard-expectations`). The runbook answers: "This alarm fired — what do I do first?"

### Trace markers

Verification probes and alarms MUST include trace markers linking back to the spec requirement they verify:
```
axiom:trace work_item=<id> spec=<spec-ref> plan=<phase/task/step>
```

---

## Anti-Patterns

| Anti-Pattern | Why It's Wrong | Fix |
|---|---|---|
| Probe with valid credentials | Tests nothing — of course it connects | Remove credentials from probe |
| Single test run as "verification" | Proves it worked once, not continuously | Deploy as always-running probe |
| `default_value = "0"` on metric filter | Zero-floods drown real signals | Let missing data be missing |
| `treat_missing_data = "notBreaching"` on silence alarm | Probe crash goes undetected | Use `"breaching"` — silence = alarm |
| Alarm never tested end-to-end | Indistinguishable from broken alarm | Run alarm round-trip monthly |
| Evidence without raw output | Unverifiable claims | Always include copy-paste-able commands + output |
| Score that never drops | Adversarial review is too soft | If review finds no gaps, the review is wrong |
| Probe runs in same network/identity as the service | Tests network path, not enforcement | Probe must be in a DIFFERENT identity/namespace |

<!-- axiom:trace work_item=continuous-verification-skill-01 spec=specs/34-Observability-And-Metrics.md -->
