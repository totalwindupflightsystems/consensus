---
name: chaos-engineer-axiom
description: >
  Fault injection patterns, resilience testing, runbook validation under failure conditions,
  and recovery time objective verification for AI-assisted development. Load this skill when
  designing chaos experiments, validating resilience, testing runbooks under failure, or
  verifying RTO/RPO for any project managed by Axiom. Produces a structured chaos review
  verdict (PASS|WARN|FAIL|BLOCKED).
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  primary_spec: specs/00-PRD.md
  supporting_specs:
    - specs/46-Broken-Arrow-Emergency-Swarm.md
    - specs/34-Observability-And-Metrics.md
    - specs/25-Structured-Logging-Events.md
    - specs/32-Security-Hardening-Roadmap.md
  agents:
    - chaos-engineer-axiom
    - sre-ops-axiom
    - sitrep-axiom
  integrates_with:
    - docs-runbooks-axiom
    - enterprise-release-quality
    - security-review-axiom
    - concurrent-client-server-testing
tags:
  vertical: [sre, ops]
  category: testing
  core: false
---

# Chaos Engineering Skill (Portable)

> **"Resilience is not the absence of failure. It is the presence of recovery."**

This skill provides structured chaos engineering workflows for any project managed by Axiom. It covers fault injection patterns, resilience testing for AI-assisted workflows, runbook validation under failure conditions, and recovery time objective verification. It is designed for the `@chaos-engineer-axiom` agent but can be loaded by any agent that needs to validate system resilience.

## When to Load This Skill

Load this skill when:
- Designing chaos experiments to validate system resilience
- Testing what happens when dependencies fail (OpenCode server, model API, external services)
- Validating runbooks under actual failure conditions
- Verifying recovery time objectives (RTO) and recovery point objectives (RPO)
- Preparing for production deployment (Gate 5 per `enterprise-release-quality`)
- Responding to an incident that revealed a resilience gap
- Testing Broken Arrow emergency swarm behavior under degraded conditions
- Evaluating AI-specific failure modes (context window overflow, model API unavailability)

## Core Principles

1. **Hypothesis-driven.** Every chaos experiment starts with a hypothesis about system behavior under failure. No random destruction.
2. **Bounded blast radius.** Every experiment must have a defined, limited scope. Unbounded experiments are forbidden.
3. **Steady-state baseline required.** You cannot measure degradation without knowing what "normal" looks like.
4. **Rollback before experiment.** The rollback procedure must be verified before the experiment begins, not after it fails.
5. **Runbook validation is a first-class output.** Every chaos experiment must validate (or invalidate) at least one runbook.
6. **AI-specific resilience is first-class.** What happens when the model API is down? When context windows overflow? When agents loop? These are not edge cases — they are expected failure modes.

---

## Chaos Experiment Types

### Type 1: Kill (Process/Container Termination)

**What**: Abruptly terminate a process, container, or pod.

**Targets in Axiom**:
- OpenCode server process
- Repo runner process
- Control plane process
- PR workspace container
- Database/cache process (if applicable)

**Expected resilient behavior**:
- Crash recovery per `specs/24-Runtime-State-Persistence.md` (checkpoint resume)
- Health check detects failure per `specs/31-OpenCode-Integration-Contract.md`
- Automatic restart or escalation
- No data loss beyond the last checkpoint

**Commands**:
```bash
# Kill OpenCode server
kill -9 $(pgrep -f "opencode")

# Kill repo runner
kill -9 $(pgrep -f "axiom")

# Kill Kubernetes pod
kubectl delete pod <pod-name> -n axiom --grace-period=0 --force
```

### Type 2: Delay (Network Latency Injection)

**What**: Add artificial latency to network calls.

**Targets in Axiom**:
- OpenCode HTTP API calls (runner -> OpenCode server)
- Model API calls (OpenCode -> model provider)
- Jira/GitHub API calls
- Inter-agent communication (inbox filesystem operations)

**Expected resilient behavior**:
- Timeouts fire correctly (not too early, not too late)
- Retry logic activates per `specs/12-Retry-And-Escalation.md`
- Structured log events capture the delay (`duration_ms` field)
- User-facing progress updates continue (SSE events)

**Commands**:
```bash
# Add 5s latency to all outbound HTTP (Linux, requires tc)
tc qdisc add dev eth0 root netem delay 5000ms

# Add latency to specific host (macOS, requires pfctl)
# Use a proxy like toxiproxy for targeted latency injection

# Simulate slow filesystem (for inbox-based communication)
# Mount a FUSE filesystem with artificial delay
```

### Type 3: Corrupt (Data/Response Corruption)

**What**: Return malformed or unexpected data from a dependency.

**Targets in Axiom**:
- OpenCode API returning malformed JSON
- Model API returning truncated responses
- Webhook payloads with invalid signatures
- Memory bank files with corrupted content
- XML responses with missing required tags

**Expected resilient behavior**:
- Safe parsing rejects malformed data (REQ-INPUT-004)
- V2 variant mechanism recovers missing XML tags per `specs/28-V2-Variant-Mechanism.md`
- Webhook signature verification rejects invalid payloads (REQ-INPUT-006)
- Error classification is correct per REQ-LOG-ERR-001

**Commands**:
```bash
# Intercept and corrupt HTTP responses (using mitmproxy)
mitmproxy --mode reverse:http://localhost:4096 --script corrupt_response.py

# Write corrupted memory bank file
echo "CORRUPTED" > .memory-bank/work-items/<ID>/state.json

# Send webhook with invalid signature
curl -X POST http://localhost:8100/api/v1/webhooks/github \
  -H "X-Hub-Signature-256: sha256=invalid" \
  -d '{"action": "opened"}'
```

### Type 4: Resource Exhaustion (CPU/Memory/Disk)

**What**: Exhaust system resources to test behavior under pressure.

**Targets in Axiom**:
- Memory exhaustion in the runner process
- Disk space exhaustion (memory bank, logs, snapshots)
- CPU saturation (many concurrent agents)
- File descriptor exhaustion (many open files/sockets)
- Context window exhaustion (token limit reached)

**Expected resilient behavior**:
- OOM killer terminates the right process (not the host)
- Disk full errors are caught and reported (not silent corruption)
- Resource limits per `specs/06-Project-Configuration.md` prevent host impact
- Graceful degradation under CPU pressure (slower, not broken)

**Commands**:
```bash
# Memory stress (Linux)
stress-ng --vm 1 --vm-bytes 90% --timeout 60s

# Disk fill
dd if=/dev/zero of=/tmp/fill_disk bs=1M count=10000

# File descriptor exhaustion
ulimit -n 100  # Reduce FD limit before starting process

# CPU stress
stress-ng --cpu $(nproc) --timeout 60s
```

### Type 5: Dependency Failure (External API Down)

**What**: Make an external dependency completely unavailable.

**Targets in Axiom**:
- OpenCode server unreachable
- Model API (OpenAI, Anthropic, etc.) unreachable
- Jira API unreachable
- GitHub API unreachable
- S3/object storage unreachable (for snapshots)

**Expected resilient behavior**:
- Retry with backoff per `specs/12-Retry-And-Escalation.md`
- Escalation after retry budget exhausted
- Clear error messages identifying the failed dependency
- Partial functionality continues where possible (e.g., local operations work without Jira)

**Commands**:
```bash
# Block outbound to specific host (iptables)
iptables -A OUTPUT -d api.openai.com -j DROP

# Block via /etc/hosts
echo "127.0.0.1 api.openai.com" >> /etc/hosts

# Use toxiproxy to simulate connection refused
toxiproxy-cli toxic add -t reset_peer -a timeout:0 opencode_proxy

# Simulate DNS failure
# Configure a DNS server that returns NXDOMAIN for the target
```

---

## Chaos Experiment Template

Every chaos experiment MUST use this template.

```markdown
---
mb:
  type: chaos-experiment
  title: "<Experiment Title>"
  created: YYYY-MM-DDTHH:MM:SSZ
  tags: [chaos, <type>, <target>, <work-item>]
  links:
    runbook: <runbook being validated>
    spec: <spec reference>
    alert: <alert being tested> (optional)
    work_item: <work item ID> (optional)
experiment_type: kill | delay | corrupt | resource-exhaustion | dependency-failure
target: <what is being disrupted>
blast_radius: <scope of impact>
---

# Chaos Experiment: <Title>

## Hypothesis

**Statement**: "When <failure condition>, the system will <expected behavior>, and recovery will complete within <RTO>."

**Null hypothesis**: "The system will NOT <expected behavior>, indicating a resilience gap."

## Blast Radius

**Scope**: <what is affected>
**Not affected**: <what is explicitly excluded>
**Maximum duration**: <hard time limit for the experiment>
**Rollback trigger**: <condition that triggers immediate rollback>

## Steady-State Baseline

Before running the experiment, capture the steady state:

| Metric | Baseline Value | Measurement Command |
|---|---|---|
| <metric> | <value> | `<command>` |
| Health check status | 200 OK | `curl -sf http://localhost:8100/health` |
| Active runs | <count> | `<command>` |
| Error rate | <rate> | `<command>` |

## Prerequisites

- [ ] Steady-state baseline captured
- [ ] Rollback procedure verified (tested independently before experiment)
- [ ] Blast radius confirmed (no production impact / explicit approval obtained)
- [ ] Monitoring in place to observe the experiment
- [ ] Team notified (if applicable)
- [ ] Time limit set: <duration>

## Method

### Step 1: Inject Failure

**Command**:
```bash
<exact command to inject the failure>
```

**Verification** (failure is active):
```bash
<command to verify the failure is in effect>
```

### Step 2: Observe Behavior

**What to observe**:
- [ ] Error detection: does the system detect the failure?
- [ ] Error reporting: are structured log events emitted?
- [ ] Retry behavior: does retry logic activate?
- [ ] Degradation: what functionality is lost?
- [ ] User impact: what does the user see?

**Observation commands**:
```bash
<commands to observe system behavior during failure>
```

### Step 3: Measure Recovery

**Recovery trigger**: <what initiates recovery — automatic or manual>

**Recovery commands** (if manual):
```bash
<commands to initiate recovery>
```

**Recovery verification**:
```bash
<commands to verify recovery is complete>
```

**Recovery time**: <measured time from failure to recovery>

### Step 4: Remove Failure

**Command**:
```bash
<exact command to remove the injected failure>
```

**Verification** (failure removed):
```bash
<command to verify normal operation restored>
```

## Rollback

If the experiment causes unexpected damage:

### Immediate Rollback Steps

1. <Step 1: remove injected failure>
2. <Step 2: restart affected services>
3. <Step 3: verify recovery>

### Rollback Verification

```bash
<command to verify rollback succeeded>
```

## Observations

### What Happened

<Narrative description of observed behavior>

### Metrics During Experiment

| Metric | Before | During | After | Expected |
|---|---|---|---|---|
| <metric> | <value> | <value> | <value> | <value> |

### Structured Log Events Captured

```json
<relevant log events captured during the experiment>
```

## Results

### Hypothesis Result

**Confirmed / Refuted / Partially Confirmed**

<Explanation>

### Recovery Time

| Metric | Target (RTO) | Actual | Status |
|---|---|---|---|
| Detection time | <target> | <actual> | PASS/FAIL |
| Recovery time | <target> | <actual> | PASS/FAIL |
| Data loss (RPO) | <target> | <actual> | PASS/FAIL |

### Runbook Validation

**Runbook tested**: <runbook path>
**Result**: PASS | FAIL | PARTIAL

| Runbook Step | Followed? | Worked? | Notes |
|---|---|---|---|
| Step 1 | Yes/No | Yes/No | <notes> |
| Step 2 | Yes/No | Yes/No | <notes> |

### Findings

| ID | Severity | Description | Recommendation |
|---|---|---|---|
| <ID> | Critical|High|Medium|Low | <what was found> | <what to do about it> |

## Verdict

**PASS | WARN | FAIL | BLOCKED**
```

---

## Resilience Testing for AI-Assisted Workflows

These are failure modes specific to AI-assisted development that traditional chaos engineering does not cover.

### Experiment: OpenCode Server Down

**Hypothesis**: When the OpenCode server crashes mid-step, the runner detects the failure within 30s, retries per the health check protocol, and resumes from the last checkpoint.

**Method**:
1. Start a Axiom run with a multi-step plan.
2. After step 2 completes, kill the OpenCode server process.
3. Observe: does the runner detect the failure? Does it retry? Does it resume?

**Expected behavior** (per `specs/31-OpenCode-Integration-Contract.md`):
- Health check fails within `health_poll_interval_seconds`
- Runner logs `opencode_health_failed` event
- Runner attempts restart (if `--spinup-opencode` mode)
- After restart, runner resumes from last checkpoint

### Experiment: Model API Unavailable

**Hypothesis**: When the model API returns 503 for all requests, the runner retries with backoff, escalates after retry budget, and does not lose progress.

**Method**:
1. Configure a proxy that returns 503 for all model API requests.
2. Start a Axiom run.
3. Observe retry behavior, escalation, and state persistence.

**Expected behavior**:
- Retry with exponential backoff per `specs/12-Retry-And-Escalation.md`
- `opencode_request_failed` events logged with `retry_attempt` field
- Escalation after retry budget exhausted
- Checkpoint preserved; run can resume when API recovers

### Experiment: Context Window Exceeded

**Hypothesis**: When a step's context exceeds the model's token limit, the system handles the error gracefully without crashing or losing state.

**Method**:
1. Create a work item with an extremely large ticket description (> 100K tokens).
2. Start a Axiom run.
3. Observe: does the system truncate, error gracefully, or crash?

**Expected behavior** (per REQ-INPUT-002):
- Input truncated to configured limits with `...[TRUNCATED <n_bytes> bytes]` marker
- Structured warning logged
- Step proceeds with truncated input (or fails gracefully with clear error)
- No crash, no data corruption

### Experiment: Agent Infinite Loop

**Hypothesis**: When an agent enters an infinite loop (e.g., verification always fails, injection always adds more steps), the system detects and breaks the loop within the configured retry budget.

**Method**:
1. Create a verification gate that always returns FAIL.
2. Start a Axiom run with that gate.
3. Observe: does the system stop retrying after the budget? Does it escalate?

**Expected behavior**:
- Retry budget exhausted per `specs/12-Retry-And-Escalation.md`
- `escalation_triggered` event logged
- Run transitions to `blocked` state
- No unbounded resource consumption

### Experiment: Concurrent Agent Conflict

**Hypothesis**: When multiple agents in a Broken Arrow swarm write to the same inbox directory simultaneously, no messages are lost or corrupted.

**Method** (per `specs/46-Broken-Arrow-Emergency-Swarm.md`):
1. Launch a Broken Arrow with maximum agents.
2. All agents write findings simultaneously.
3. Verify: are all findings present and uncorrupted?

**Expected behavior**:
- All findings files are present in the incident inbox
- No file corruption (each file is a complete, valid markdown document)
- `sitrep-axiom` can read and synthesize all findings

---

## Runbook Validation Under Failure

Every chaos experiment MUST validate at least one runbook. This is the bridge between chaos engineering and operational readiness.

### Validation Workflow

1. **Select runbook**: Choose the runbook that should be followed when this failure occurs.
2. **Inject failure**: Run the chaos experiment's failure injection.
3. **Follow runbook**: Follow the runbook steps exactly as written (human or AI agent).
4. **Record results**: For each runbook step, record whether it was followable and whether it worked.
5. **Measure recovery**: Record the time from failure detection to full recovery.
6. **Update runbook**: If any step was unclear, incorrect, or missing, update the runbook.

### Validation Criteria

A runbook passes validation when:
- [ ] Every step can be followed without additional context or tribal knowledge
- [ ] Every command produces output that matches the documented expected output
- [ ] The rollback procedure works when tested
- [ ] Recovery completes within the documented RTO
- [ ] An AI agent (`@docs-runbooks-axiom`) can follow the runbook without human interpretation

A runbook fails validation when:
- Any step requires undocumented knowledge to follow
- Any command produces unexpected output not covered by the "if this fails" section
- The rollback procedure does not restore the system to a known good state
- Recovery exceeds the documented RTO by more than 50%

---

## Recovery Time Objective (RTO) Verification

### RTO Categories for Axiom

| Component | Target RTO | Measurement |
|---|---|---|
| OpenCode server restart | < 30s | Time from crash detection to health check passing |
| Run checkpoint resume | < 60s | Time from runner restart to step execution resuming |
| Broken Arrow swarm launch | < 30s | Time from command to first agent investigation |
| API server restart | < 15s | Time from crash to health endpoint responding |
| Snapshot restore | < 120s | Time from restore start to workspace ready |

### RTO Measurement Protocol

1. Capture steady-state timestamp (`T0`).
2. Inject failure.
3. Record detection timestamp (`T1` — when the system first logs the failure).
4. Record recovery start timestamp (`T2` — when recovery action begins).
5. Record recovery complete timestamp (`T3` — when steady state is restored).
6. Calculate:
   - Detection time: `T1 - T0`
   - Recovery time (RTO): `T3 - T1`
   - Total downtime: `T3 - T0`

---

## Safety Guardrails

These guardrails are non-negotiable for all chaos experiments.

### Before Any Experiment

- [ ] **Blast radius is bounded.** The experiment affects only the intended target. Document what is NOT affected.
- [ ] **Rollback is verified.** The rollback procedure has been tested independently before the experiment.
- [ ] **Steady-state baseline is captured.** You know what "normal" looks like.
- [ ] **Time limit is set.** The experiment has a hard maximum duration.
- [ ] **Monitoring is active.** You can observe the experiment in real time.
- [ ] **Approval obtained.** For production environments, explicit approval from the environment owner.

### During Any Experiment

- [ ] **Abort on unexpected impact.** If the blast radius exceeds the defined scope, abort immediately.
- [ ] **Capture all observations.** Log everything — you cannot re-observe a chaos experiment.
- [ ] **Do not stack experiments.** One failure injection at a time unless explicitly testing cascading failures.

### After Any Experiment

- [ ] **Verify full recovery.** Steady state must be restored to baseline values.
- [ ] **Document findings.** Every observation, metric, and runbook validation result.
- [ ] **Update runbooks.** If the experiment revealed gaps, update the relevant runbook immediately.
- [ ] **File work items.** For any resilience gaps found, create work items with clear acceptance criteria.

### Absolute Rules

1. **Never run chaos in production without explicit approval.** "Production" includes staging environments that serve real users.
2. **Always have rollback ready.** If you cannot roll back, you cannot experiment.
3. **Blast radius must be bounded.** Unbounded experiments are forbidden. If you cannot define the blast radius, you cannot run the experiment.
4. **No chaos without steady-state baseline.** You cannot measure degradation without knowing normal.
5. **No chaos without monitoring.** If you cannot observe the experiment, you cannot learn from it.

---

## Chaos Review Verdict

Every chaos engineering review MUST produce a structured verdict.

### Verdict Format

```markdown
## Chaos Engineering Verdict

**Verdict**: PASS | WARN | FAIL | BLOCKED
**Reviewer**: @chaos-engineer-axiom
**Date**: YYYY-MM-DDTHH:MM:SSZ
**Work Item**: <work_item_id>
**Scope**: <what was tested>

### Experiments Run

| Experiment | Type | Target | Hypothesis | Result | RTO Met? |
|---|---|---|---|---|---|
| <name> | <type> | <target> | Confirmed/Refuted | PASS/FAIL | Yes/No |

### Runbook Validations

| Runbook | Experiment | Result | Gaps Found |
|---|---|---|---|
| <path> | <experiment> | PASS/FAIL | <description> |

### Findings Summary

| ID | Severity | Description | Recommendation | Work Item |
|---|---|---|---|---|
| <ID> | Critical|High|Medium|Low | <description> | <recommendation> | <work item ID> |

### Verdict Rules

- **PASS**: All experiments confirmed hypotheses, all RTOs met, all runbooks validated.
- **WARN**: Most experiments passed, minor RTO misses (< 50% over target), runbook gaps are non-critical.
- **FAIL**: Any experiment refuted its hypothesis with no mitigation, OR any critical RTO missed, OR any runbook completely failed.
- **BLOCKED**: Cannot run experiments (missing infrastructure, missing approval, missing baseline).
```

---

## Integration with Other Agents and Skills

### With `@sre-ops-axiom`

- SRE defines RTO/RPO targets and identifies critical failure modes
- Chaos engineer designs and runs experiments to validate those targets
- Both agents review results together

### With `@docs-runbooks-axiom`

- Every chaos experiment validates at least one runbook
- Runbook gaps found during chaos experiments are immediately reported to `@docs-runbooks-axiom`
- Updated runbooks are re-validated in the next chaos experiment cycle

### With `enterprise-release-quality`

- Gate 5 (Production Readiness Gate) includes "Disaster recovery tested"
- This skill provides the experiments and evidence for that gate
- Chaos experiment results are included in the release evidence bundle

### With `@security-review-axiom`

- Security-focused chaos experiments (e.g., auth bypass under failure, secret exposure during crash)
- Security review may inject chaos experiments to validate security controls under stress

### With Broken Arrow (`specs/46-Broken-Arrow-Emergency-Swarm.md`)

- Chaos experiments can test Broken Arrow itself: what happens when the swarm launches under degraded conditions?
- Broken Arrow findings may trigger new chaos experiments to validate fixes

### With `concurrent-client-server-testing`

- Chaos experiments during concurrent testing validate resilience under load
- Fault injection during API testing reveals error handling gaps

---

## Non-Negotiables

These rules are absolute and cannot be overridden:

1. **Fail closed on unbounded blast radius.** If you cannot define and bound the blast radius, the experiment is BLOCKED.
2. **No chaos without steady-state baseline.** Measuring degradation requires knowing normal. No baseline = no experiment.
3. **No chaos in production without explicit approval.** This includes staging environments serving real users.
4. **Rollback must be verified before experiment.** An untested rollback is not a rollback.
5. **No fabricated results.** Every observation must be captured from actual experiment execution. "The system would probably recover" is not evidence.
6. **Every experiment validates a runbook.** Chaos without runbook validation is destruction without learning.
7. **AI-specific failure modes are first-class.** A resilience review that ignores model API failures, context window overflow, and agent loops is incomplete.

---

## How to Use This Skill

### As `@chaos-engineer-axiom`

1. Load this skill at the start of every resilience review.
2. Identify the failure modes to test (use the 5 experiment types as a starting guide).
3. For each failure mode: design an experiment using the template.
4. Verify safety guardrails before running any experiment.
5. Run experiments, capture observations, validate runbooks.
6. Produce the chaos review verdict.
7. File findings and work items for resilience gaps.

### As Any Other Agent

1. Load this skill when you encounter a resilience concern.
2. Use the experiment types to identify relevant failure modes.
3. If you cannot run the experiment yourself, inject a step for `@chaos-engineer-axiom`.

### Injecting Chaos Engineering Steps

When another agent identifies a resilience gap, inject:

```yaml
injected_step:
  title: "Chaos experiment required for <failure mode>"
  agent: "@chaos-engineer-axiom"
  objective: "Design and run chaos experiment for <failure mode>; validate <runbook>; verify RTO < <target>"
  verification: "Chaos review verdict is PASS or WARN; runbook validated; RTO met"
  trace_refs:
    spec: specs/00-PRD.md
    plan: <current plan step>
```

---

## References

- `specs/00-PRD.md` — Product requirements, reliability NFR, crash recovery
- `specs/46-Broken-Arrow-Emergency-Swarm.md` — Emergency concurrent agent swarm
- `specs/34-Observability-And-Metrics.md` — Metrics, alerts, monitoring
- `specs/25-Structured-Logging-Events.md` — Structured logging events
- `specs/12-Retry-And-Escalation.md` — Retry budgets and escalation triggers
- `specs/24-Runtime-State-Persistence.md` — Checkpoint persistence and crash recovery
- `specs/31-OpenCode-Integration-Contract.md` — OpenCode server lifecycle and health checks
- `.opencode/skills/docs-runbooks-axiom/SKILL.md` — Runbook creation and validation
- `.opencode/skills/enterprise-release-quality/SKILL.md` — Release quality gates (Gate 5)

---

axiom:trace work_item=chaos-engineer-axiom spec=specs/00-PRD.md plan= prompt=.opencode/skills/chaos-engineer-axiom/SKILL.md evidence= doc= test= commit=
