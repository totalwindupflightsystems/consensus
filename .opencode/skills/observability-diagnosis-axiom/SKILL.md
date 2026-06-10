---
name: observability-diagnosis-axiom
description: >
  Systematic diagnostic workflow for consuming and correlating observability signals
  (logs, metrics, traces, dashboards) to diagnose current, past, and predicted problems
  in Axiom-managed systems. Encodes the canonical 8-phase diagnostic loop, cross-pillar
  correlation rules, scenario-specific diagnostic playbooks, AI-specific diagnostics,
  visual diagnosis guidance, and evidence capture requirements. Load this skill when
  diagnosing production issues, performing incident investigation, correlating signals
  across pillars, or capturing diagnostic evidence for any service managed by Axiom.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-03-20"
  primary_spec: specs/65-Diagnostic-Workflows.md
  secondary_specs:
    - specs/34-Observability-And-Metrics.md
    - specs/25-Structured-Logging-Events.md
    - specs/61-Logging-And-Observability-Overhaul.md
    - specs/50-Runtime-Logging-Modes-And-Formats.md
    - specs/27-Evidence-Bundle-Schema.md
    - specs/47-Cost-Tracking-And-Session-Analytics.md
    - specs/21-Traceability-Doctrine.md
    - specs/66-Predictive-Observability.md
tags:
  vertical: [sre, ops]
  category: observability
  core: false
---

# Observability Diagnosis Skill (Portable)

> **"Diagnose systematically, not heroically. Follow the loop, not your gut."**
>
> **"Two pillars confirm; one pillar suggests. Never declare root cause from a single signal."**

This skill provides a portable, repeatable diagnostic workflow for consuming and correlating
observability signals to diagnose problems in Axiom-managed systems. It is designed to be
executable by both human operators and AI agents, step by step.

**Spec grounding**: The diagnostic loop, signal correlation requirements, scenario contracts,
visual diagnosis requirements, and evidence format are defined in
`specs/65-Diagnostic-Workflows.md` (REQ-DIAG-001 through REQ-DIAG-111). This skill
operationalizes those requirements into actionable procedures.

**Companion skills**: This skill works alongside:
- `dashboard-design-axiom` — how to build and read dashboards
- `alert-engineering-axiom` — how alerts trigger the diagnostic loop
- `distributed-tracing-axiom` — how to read trace waterfalls and propagate context
- `metrics-instrumentation-axiom` — how metrics are named, labeled, and queried

axiom:trace work_item=observability-skills-collection-01 spec=specs/65-Diagnostic-Workflows.md plan=phase-87-3/task-87-3-1/step-87-3-1-1

---

## 1. The Canonical Diagnostic Loop

*Spec ref: REQ-DIAG-001 through REQ-DIAG-010*

Every diagnostic investigation follows eight phases. Do not skip phases. Do not jump to
mitigation without confirming root cause. The loop is bounded: after 3 iterations of
Verify → Hypothesize without resolution, escalate.

```
Detect → Triage → Scope → Hypothesize → Investigate → Confirm → Mitigate → Verify
                                ↑                                          |
                                └──────── Issue persists ──────────────────┘
```

### 1.1 Phase 1: Detect (REQ-DIAG-002)

**Goal**: Recognize that something is wrong and create a detection record.

**Entry criteria**: An alert fires, a metric anomaly is observed, a user reports an issue,
or a scheduled health check fails.

**Required signals**:
- Alert payload (name, severity, condition, timestamp), OR
- Anomaly description (which metric, what changed, when), OR
- User report (what they observed, when, affected scope)

**Actions**:
1. Record the detection event: what was detected, when, by whom/what.
2. Assign initial severity based on available information.
3. Note the detection source (alert name, dashboard, user report, health check).

**Output**: A detection record with enough context to begin triage.

**Exit criteria**: Detection record exists. Proceed to Triage.

### 1.2 Phase 2: Triage (REQ-DIAG-003)

**Goal**: Assign priority, identify affected services, and decide whether to investigate.

**Entry criteria**: Detection record exists.

**Required signals**:
- Alert history — is this new or recurring?
- Service health dashboard — current state of affected services
- Recent deploy history — any changes in the last 24 hours?

**Actions**:
1. Check alert history: has this alert fired before? How often? When was it last resolved?
2. Assign priority:
   - **P1 (Critical)**: Service down, data loss risk, or security incident
   - **P2 (High)**: Significant degradation affecting users
   - **P3 (Medium)**: Partial degradation or non-critical service affected
   - **P4 (Low)**: Minor anomaly, no user impact
3. Identify affected service(s) and estimate initial blast radius.
4. Assign an owner for the investigation.
5. Decide: investigate now, or defer?

**Output**: Priority assignment, affected services, blast radius estimate, owner.

**Exit criteria**: Priority assigned, owner identified, proceed/defer decision made.

### 1.3 Phase 3: Scope (REQ-DIAG-004)

**Goal**: Bound the blast radius — which components, which time window, how many users.

**Entry criteria**: Triage complete with priority >= threshold for investigation.

**Required signals**:
- Metrics dashboards: error rate, latency percentiles, saturation
- Log volume changes: sudden spikes or drops
- Trace error rates: percentage of traces with errors
- Deployment timeline: recent deploys, config changes, dependency updates

**Actions**:
1. Identify the time window of impact (when did it start? is it ongoing?).
2. List affected components (which services, which endpoints, which pods).
3. Estimate user/request impact (error rate × traffic = affected requests).
4. Collect correlation IDs for affected requests (for later drill-down).

**Scoping queries** (tool-agnostic patterns):
- Error rate by component: `rate(codeops_verification_result_total{result="failed"}[5m])`
- Latency by step: `histogram_quantile(0.95, rate(codeops_step_duration_seconds_bucket[5m]))`
- Log volume: count of log events per component per minute
- Deployment events: filter by `event_type=deploy` in the scoping time window

**Output**: Affected components list, time window, user impact estimate, correlation IDs.

**Exit criteria**: Blast radius is bounded. Proceed to Hypothesize.

### 1.4 Phase 4: Hypothesize (REQ-DIAG-005)

**Goal**: Generate ranked hypotheses about root cause.

**Entry criteria**: Scope is bounded.

**Required signals**:
- Scoped metrics from Phase 3
- Recent changes: deploys, config changes, dependency updates
- Known failure patterns: has this component failed this way before?

**Actions**:
1. List candidate hypotheses, most likely first.
2. For each hypothesis, note:
   - **Description**: What could be causing this?
   - **Supporting evidence**: What signals point toward this hypothesis?
   - **Investigation plan**: What specific queries/checks would confirm or refute it?
3. Prioritize hypotheses by likelihood and ease of investigation.

**Hypothesis generation heuristics**:
- **Recent change?** If a deploy or config change happened in the scoping window, it is the
  top hypothesis until ruled out.
- **Resource exhaustion?** Check CPU, memory, disk, connections.
- **External dependency?** Check upstream API health, DNS, network.
- **Known pattern?** Check if this matches a previously diagnosed issue.
- **AI-specific?** Check model API latency, token budget, context overflow, agent loops.

**Output**: Ranked list of hypotheses with investigation plans.

**Exit criteria**: At least one testable hypothesis exists. Proceed to Investigate.

### 1.5 Phase 5: Investigate (REQ-DIAG-006)

**Goal**: Gather evidence to confirm or refute each hypothesis.

**Entry criteria**: At least one hypothesis with an investigation plan.

**Required signals**: Depends on hypothesis — may include:
- Detailed logs filtered by correlation ID
- Trace waterfalls for slow or erroring requests
- Flame graphs (when available)
- Metric drill-downs (by label, by time)
- Code diffs, config diffs

**Actions**:
1. Execute the investigation plan for the top hypothesis.
2. For each piece of evidence found, record:
   - What signal was consulted (log query, metric query, trace ID)
   - What was observed (specific values, error messages, timing)
   - Whether it supports or refutes the hypothesis
3. If the top hypothesis is refuted, move to the next one.
4. If all hypotheses are refuted, return to Hypothesize with updated context.

**Investigation discipline**:
- Always cite specific evidence: log line IDs, metric values at specific times, trace span IDs.
- Do not rely on memory or assumptions — query the actual signals.
- Record negative results too (what you checked and ruled out).

**Output**: Per-hypothesis evidence (supporting or refuting).

**Exit criteria**: At least one hypothesis confirmed, OR all refuted (loop back to Hypothesize).

### 1.6 Phase 6: Confirm (REQ-DIAG-007)

**Goal**: Validate root cause with corroborating evidence from multiple signal pillars.

**Entry criteria**: Investigation evidence points to a root cause.

**Required signals**: Corroborating evidence from **at least two signal pillars**:
- Logs AND metrics
- Traces AND logs
- Metrics AND traces
- Any combination — single-pillar confirmation is insufficient

**Actions**:
1. State the root cause clearly and concisely.
2. List corroborating evidence from each pillar:
   - Pillar 1: [specific evidence]
   - Pillar 2: [specific evidence]
3. Assign confidence level:
   - **High**: Multiple pillars agree, evidence is unambiguous
   - **Medium**: Two pillars agree but some ambiguity remains
   - **Low**: Evidence is suggestive but not conclusive
4. List mitigation options.

**Confirmation rule**: Two pillars confirm; one pillar suggests. Never declare root cause
from a single signal pillar. If only one pillar has evidence, return to Investigate to
find corroborating signals.

**Output**: Root cause statement, confidence level, mitigation options.

**Exit criteria**: Root cause stated with multi-pillar evidence. Proceed to Mitigate.

### 1.7 Phase 7: Mitigate (REQ-DIAG-008)

**Goal**: Apply a fix or workaround to resolve or reduce the impact.

**Entry criteria**: Confirmed root cause with mitigation options.

**Required signals**:
- Current system state (is the issue still active?)
- Mitigation playbook or runbook (if available)

**Actions**:
1. Select the appropriate mitigation:
   - **Rollback**: If a recent deploy caused the issue
   - **Config change**: If a configuration error is the cause
   - **Restart**: If a transient state corruption is suspected
   - **Scale**: If resource exhaustion is the cause
   - **Hotfix**: If a code bug is identified and a quick fix is available
   - **Workaround**: If the root cause cannot be fixed immediately
2. Execute the mitigation.
3. Record: what action was taken, when, by whom, expected recovery time.

**Output**: Mitigation action, timestamp, expected recovery time.

**Exit criteria**: Mitigation executed and documented. Proceed to Verify.

### 1.8 Phase 8: Verify (REQ-DIAG-009)

**Goal**: Confirm the issue is resolved.

**Entry criteria**: Mitigation action executed.

**Required signals**:
- Same signals that triggered detection (are they back to baseline?)
- Error rate trend (decreasing?)
- Latency trend (returning to normal?)
- User-facing health checks (passing?)

**Actions**:
1. Wait for the expected recovery time.
2. Check the triggering signals: have they returned to baseline?
3. Check downstream effects: are dependent services recovering?
4. Run user-facing health checks.

**Decision**:
- **Resolved**: Signals return to baseline → record evidence, close investigation.
- **Not resolved**: Signals still abnormal → loop back to Hypothesize with updated context.

**Bounded iteration (REQ-DIAG-010)**: After 3 iterations of Verify → Hypothesize without
resolution, escalate to a higher-severity response or bring in additional expertise.

**Output**: Verification result (resolved / not resolved) with evidence.

---

## 2. Diagnostic Loop Invariants

*Spec ref: REQ-DIAG-010*

These invariants MUST hold across all eight phases:

| # | Invariant | Enforcement |
|---|-----------|-------------|
| 1 | **Immutability** | Once a phase produces outputs, those outputs are recorded and not modified. Corrections are additive (new entries, not edits). |
| 2 | **Traceability** | Every finding references specific signals: log line IDs, metric queries, trace IDs, dashboard screenshot paths. |
| 3 | **Time context** | Every observation includes the time window it covers (start and end timestamps). |
| 4 | **Tool agnosticism** | Phase definitions do not reference specific tools. Tool bindings are in this skill's appendix, not in the loop itself. |
| 5 | **Bounded iteration** | The Verify → Hypothesize loop is limited to 3 iterations. After 3, escalate. |

---

## 3. Cross-Pillar Signal Correlation

*Spec ref: REQ-DIAG-020 through REQ-DIAG-023*

### 3.1 Correlation Fields

Logs, metrics, and traces are linked via shared identifiers. Use these fields to navigate
between signal pillars:

| Field | Present In | Use For |
|---|---|---|
| `trace_id` | Traces, Logs (post-v1) | Link a distributed trace to its log events |
| `span_id` | Traces, Logs (post-v1) | Link a specific span to its log events |
| `correlation_id` | Logs, HTTP headers | Link a request across service boundaries (v1) |
| `run_id` | Logs, Metrics (label), Evidence | Link all events in a single Axiom run |
| `work_item_id` | Logs, Metrics (label), Evidence, Jira | Link all activity for a work item across runs |
| `controller_id` | Logs (controller mode) | Link events from a specific controller instance |
| `timestamp` | All signals | Time-based correlation when ID-based is unavailable |

Source: `specs/25-Structured-Logging-Events.md` (correlation fields),
`specs/34-Observability-And-Metrics.md` (metric labels).

### 3.2 Correlation Query Patterns (REQ-DIAG-021)

Five canonical patterns for navigating between signal pillars:

**Pattern 1: Alert → Trace**
- Given: An alert (metric condition)
- Find: Traces that contributed to the alert condition
- How: Filter traces by alert time window + metric labels (`work_item_id`, `repo`)

**Pattern 2: Trace → Logs**
- Given: A trace ID
- Find: All log events associated with that trace
- How: Filter logs by `trace_id` (post-v1) or `correlation_id` (v1)

**Pattern 3: Metric Anomaly → Correlated Events**
- Given: A metric anomaly (e.g., latency spike at time T)
- Find: Log events and traces in the same time window with matching labels
- How: Filter logs and traces by time window ± alignment buffer + shared labels

**Pattern 4: Log Error → Related Metrics**
- Given: A log error event
- Find: Metric time series for the same component and time window
- How: Filter metrics by `component` label + time window around the log timestamp

**Pattern 5: Work Item → Full Signal View**
- Given: A `work_item_id`
- Find: All logs, metrics, traces, and evidence across all runs
- How: Filter all signal stores by `work_item_id`

### 3.3 Time-Window Alignment Rules (REQ-DIAG-022)

When correlating signals across pillars, account for timing differences:

| Factor | Adjustment |
|---|---|
| **Clock skew** | Allow ±5 seconds alignment buffer for cross-component queries |
| **Metric aggregation lag** | Extend metric query windows by one scrape interval (typically 15-60s) |
| **Log ingestion delay** | Extend log query windows by 1-30 seconds (depends on pipeline) |
| **Trace completion delay** | Allow 30-60 seconds after last span for trace assembly |

**Practical rule**: When querying across pillars, extend the time window by at least
60 seconds on each side of the event of interest.

### 3.4 Minimum Correlation Coverage (REQ-DIAG-023)

Every Axiom component that emits logs MUST include at least `run_id` and `work_item_id`
in its log events (when operating within a run context). Components that make HTTP calls
MUST propagate `correlation_id` (v1) or W3C Trace Context headers (post-v1).

**Verification check**: If a component's logs lack `run_id` or `work_item_id`, that is a
diagnostic prerequisite gap — file a remediation work item before attempting cross-pillar
correlation for that component.

---

## 4. Scenario-Specific Diagnostic Playbooks

*Spec ref: REQ-DIAG-040 through REQ-DIAG-053*

Each playbook follows the canonical loop but provides scenario-specific guidance for the
Hypothesize, Investigate, and Confirm phases.

### 4.1 Latency Spike Investigation (REQ-DIAG-040)

**Trigger**: `codeops_step_duration_seconds` p95 exceeds threshold
(alert `CodeOpsStepDurationHigh` from `specs/34`)

**Scoping queries**:
- Which steps are slow? Filter `codeops_step_duration_seconds` by `step_id`, `command`
- Is it OpenCode API latency? Check `codeops_opencode_request_duration_seconds`
- Is it local processing? Compare step duration minus OpenCode request duration

**Hypothesis checklist**:
1. Model API latency degradation (check provider status, request durations)
2. Large context window (check token usage per request)
3. Resource contention (check CPU, memory, I/O metrics)
4. Network issues (check DNS resolution, connection timeouts)
5. Slow dependency (check external API response times)

**Confirmation requires**: Evidence from at least two of: step duration metrics,
OpenCode request metrics, trace waterfalls, resource utilization metrics.

**Dashboard panels**: Step Duration (panel 1), OpenCode Request Duration (panel 5),
Token Usage (panel 6).

### 4.2 Error Rate Increase After Deploy (REQ-DIAG-041)

**Trigger**: `codeops_verification_result_total{result="failed"}` rate increase
correlated with a deployment timestamp.

**Scoping queries**:
- When did the error rate increase start? Overlay with deployment timeline.
- Which verifier types are failing? Filter by `verifier_type`.
- Are all repos affected or just one? Filter by `repo`.

**Hypothesis checklist**:
1. Deploy introduced a regression (compare error types before/after deploy)
2. Deploy changed configuration (check config diffs)
3. Deploy changed dependencies (check dependency versions)
4. Coincidental external failure (check external dependency health)

**Confirmation requires**: Correlation between deploy timestamp and error rate change,
plus specific failing verification logs showing the regression.

**Dashboard panels**: Verification Pass Rate (panel 2), Retry and Escalation Rate (panel 7).

### 4.3 Memory Leak Detection (REQ-DIAG-042)

**Trigger**: Container memory usage monotonically increasing without corresponding
workload increase.

**Scoping queries**:
- Is memory growth monotonic? Plot container memory over 24+ hours.
- Is growth proportional to workload? Compare with run count / step count.
- Any OOM kills? Check `codeops_pods_failed_total{reason="oom_killed"}`.

**Hypothesis checklist**:
1. Object accumulation (growing caches, unbounded lists)
2. Connection leak (connections opened but not closed)
3. File handle leak (temp files not cleaned up)
4. Third-party library leak (check known issues for dependencies)

**Confirmation requires**: Monotonic growth pattern in memory metrics AND identification
of the leaking component (controller vs workspace vs OpenCode).

**Dashboard panels**: Resource Usage (control plane panel 6), Active Pods (control plane panel 5).

### 4.4 Cascading Failure Diagnosis (REQ-DIAG-043)

**Trigger**: Multiple alerts firing across different components within a short time window.

**Scoping queries**:
- Build alert timeline: sort all alerts by timestamp.
- Which component failed first? The earliest alert is the likely root cause.
- What is the dependency graph? Trace failure propagation.

**Hypothesis checklist**:
1. Root cause in the first-failing component (investigate that component)
2. Shared dependency failure (database, message queue, network)
3. Resource exhaustion cascade (one component consuming resources, starving others)
4. Configuration propagation error (bad config deployed to multiple services)

**Confirmation requires**: Timeline showing failure propagation order AND evidence that
downstream failures are consequences of the root cause (not independent issues).

**Dashboard panels**: All panels — full dashboard view with time range aligned to incident.

### 4.5 Intermittent / Flaky Failures (REQ-DIAG-044)

**Trigger**: Verification failures that do not reproduce consistently; step retries
succeeding after initial failure.

**Scoping queries**:
- What is the flakiness rate? (failures / total attempts)
- Is there a temporal pattern? (time of day, day of week, concurrent load)
- Which steps are flaky? Filter retry metrics by `step_id`.

**Hypothesis checklist**:
1. External API rate limiting (check for HTTP 429 responses)
2. Network instability (check connection timeouts, DNS failures)
3. Race condition (check for ordering dependencies between steps)
4. Resource contention under load (check concurrent run count vs failures)
5. Non-deterministic test (check if the same input produces different outputs)

**Confirmation requires**: Either a reproducible trigger condition OR statistical evidence
of the flakiness pattern with environmental correlation.

**Dashboard panels**: Retry and Escalation Rate (panel 7), Verification Pass Rate (panel 2).

### 4.6 Resource Exhaustion (REQ-DIAG-045)

**Trigger**: Resource utilization approaching limits; pod evictions; connection pool
exhaustion; disk space alerts.

**Scoping queries**:
- Which resource is exhausted? (CPU, memory, disk, connections)
- Is it a sudden spike or gradual growth?
- Does it correlate with workload changes?

**Hypothesis checklist**:
1. Workload growth exceeding provisioned capacity
2. Resource leak (connections, file handles, temp files)
3. Misconfigured resource limits (too low for actual needs)
4. Noisy neighbor (another workload on shared infrastructure)

**Confirmation requires**: Identification of the exhausted resource AND its growth pattern
(sudden vs gradual) AND correlation with workload or leak evidence.

**Dashboard panels**: Resource Usage, Queue Depth (control plane panel 3), Active Pods.

---

## 5. AI-Specific Diagnostic Playbooks

*Spec ref: REQ-DIAG-050 through REQ-DIAG-053*

These scenarios are unique to AI-powered systems like Axiom and require specialized
diagnostic approaches.

### 5.1 Model API Latency Degradation (REQ-DIAG-050)

**Trigger**: `codeops_opencode_request_duration_seconds` p95 increasing; step durations
increasing without local resource contention.

**Key diagnostic question**: Is the latency in our code or in the model API?

**Investigation steps**:
1. Confirm latency is in model API calls, not local processing:
   - Compare `codeops_step_duration_seconds` with `codeops_opencode_request_duration_seconds`
   - If OpenCode request duration accounts for most of the step duration, the model API is slow
2. Check if latency correlates with request size (token count):
   - Plot request duration vs `codeops_opencode_token_usage_total{token_type="prompt"}`
   - If correlated, large prompts may be the cause
3. Check external model API status for known incidents
4. Compare latency across different models (if multiple configured)
5. Check for rate limiting (HTTP 429 responses in logs)

**Mitigation options**: Switch model provider, reduce context size, implement backoff,
cache repeated queries.

### 5.2 Token Budget Exhaustion (REQ-DIAG-051)

**Trigger**: `codeops_opencode_token_usage_total` approaching or exceeding configured
budget; cost tracking alerts from `specs/47`.

**Key diagnostic question**: Which runs/steps consume the most tokens, and is the
consumption justified?

**Investigation steps**:
1. Identify top token consumers: filter by `work_item_id`, `step_id`
2. Check for prompt bloat: are context windows growing unboundedly?
3. Check for unnecessary retries: are failed steps consuming tokens on retry?
4. Review model selection: is an expensive model used where a cheaper one would suffice?
5. Check for agent loops: are agents generating excessive API calls without progress?

**Mitigation options**: Reduce context window, switch to cheaper model for simple tasks,
fix agent loops, adjust budget if consumption is justified.

### 5.3 Context Window Overflow (REQ-DIAG-052)

**Trigger**: Model API errors indicating context length exceeded; truncated responses;
degraded output quality.

**Key diagnostic question**: What is consuming the context window?

**Investigation steps**:
1. Identify which requests exceed context limits (check error logs)
2. Analyze context composition:
   - System prompt size
   - Conversation history length
   - Tool output verbosity
   - Included file contents
3. Check for unbounded conversation history accumulation
4. Check for large tool outputs included verbatim
5. Review context management strategy (summarization, truncation, sliding window)

**Mitigation options**: Implement context management, reduce tool output verbosity,
summarize conversation history, switch to larger-context model.

### 5.4 Agent Loop Detection (REQ-DIAG-053)

**Trigger**: Step retry count exceeding threshold; same step executing repeatedly without
progress; escalation events from `specs/12-Retry-And-Escalation.md`.

**Key diagnostic question**: Why is the agent stuck, and is the step achievable?

**Investigation steps**:
1. Identify the stuck step and its retry history
2. Examine conversation logs for repetitive patterns:
   - Same error message repeated
   - Same fix attempted multiple times
   - Confidence score not improving
3. Check if verification criteria are achievable:
   - Are the acceptance criteria well-defined?
   - Is the required behavior implementable?
4. Check for environmental blockers:
   - Missing dependency or tool
   - Permission issue
   - Network restriction
5. Review the plan for circular dependencies

**Mitigation options**: Skip step with justification, fix environment, revise plan,
escalate to human operator.

---

## 6. Visual Diagnosis Guidance

*Spec ref: REQ-DIAG-070 through REQ-DIAG-080*

### 6.1 Dashboard Reading Requirements (REQ-DIAG-070)

For each diagnostic scenario, know which dashboard panels to consult:

| Scenario | Required Panels |
|---|---|
| Latency spike | Step Duration (1), OpenCode Request Duration (5), Token Usage (6) |
| Error rate after deploy | Verification Pass Rate (2), Retry and Escalation Rate (7) |
| Memory leak | Resource Usage (CP-6), Active Pods (CP-5) |
| Cascading failure | All panels — full dashboard, time-aligned to incident |
| Resource exhaustion | Resource Usage, Queue Depth (CP-3), Active Pods |
| AI latency degradation | OpenCode Request Duration (5), Step Duration (1) |
| Token budget exhaustion | Token Usage (6), Confidence Score (3) |

Panel numbers reference `specs/34-Observability-And-Metrics.md#dashboard-expectations`.

### 6.2 Trace Waterfall Interpretation (REQ-DIAG-072)

When distributed traces are available (post-v1, via OpenTelemetry):

1. **Read the span hierarchy**: Follow the Axiom span hierarchy defined in
   `specs/34-Observability-And-Metrics.md#post-v1-opentelemetry-trace-spans`.
2. **Find the slowest span**: Sort spans by duration; the longest span is often the bottleneck.
3. **Find error spans**: Look for spans with error status or error tags.
4. **Correlate with logs**: Use `trace_id` and `span_id` to find log events for specific spans.

### 6.3 Flame Graph Interpretation (REQ-DIAG-071)

When flame graphs are available (post-v1, via profiling tools):

1. **Identify hot paths**: Functions consuming the most wall-clock or CPU time appear as
   the widest bars.
2. **Compare across time**: Overlay flame graphs from before and after a change to identify
   new hot paths.
3. **Filter by component**: Narrow the flame graph to a specific module or package.

### 6.4 Heatmap Interpretation (REQ-DIAG-073)

When latency heatmaps are available (from Prometheus histograms):

1. **Identify distribution shifts**: A bimodal distribution (two clusters) indicates two
   populations of requests — investigate what differentiates them.
2. **Detect percentile changes**: Watch for the p95/p99 band widening over time.
3. **Correlate with changes**: Overlay deployment markers on the heatmap timeline.

### 6.5 Evidence Capture for Visual Diagnostics (REQ-DIAG-080)

When capturing diagnostic evidence from dashboards, flame graphs, trace waterfalls,
or heatmaps, include:

| Required Element | Description |
|---|---|
| **Time context** | Exact time range displayed (start and end timestamps) |
| **Filters applied** | All active filters (e.g., `work_item_id`, `repo`, `environment`) |
| **Annotations** | Markers indicating the relevant observation |
| **Format** | Screenshots as PNG with descriptive filenames, or query results as JSON/CSV |

File naming convention:
```
.memory-bank/work-items/<WORK_ITEM_ID>/diagnostics/
  screenshot-<scenario>-<YYYY-MM-DDTHH-MM-SSZ>.png
  query-result-<scenario>-<YYYY-MM-DDTHH-MM-SSZ>.json
```

---

## 7. Diagnostic Evidence Contract

*Spec ref: REQ-DIAG-090 through REQ-DIAG-100*

### 7.1 Evidence Integration (REQ-DIAG-090)

Diagnostic findings are stored alongside verification evidence in the work item's
evidence bundle, following `specs/27-Evidence-Bundle-Schema.md`.

**Storage location**:
```
.memory-bank/work-items/<WORK_ITEM_ID>/diagnostics/
  diagnostic-<YYYY-MM-DDTHH-MM-SSZ>.md
```

### 7.2 Required Fields (REQ-DIAG-091)

Every diagnostic evidence record MUST include:

```yaml
---
type: diagnostic
timestamp: "<ISO 8601>"           # When investigation started
trigger: "<string>"               # What triggered the investigation
severity: "<P1|P2|P3|P4>"        # Priority level
signals_consulted:                # Which pillars were queried
  - "<pillar>: <query description>"
hypotheses_tested:                # Each hypothesis with evidence
  - hypothesis: "<description>"
    evidence: "<what was found>"
    result: "<confirmed|refuted|inconclusive>"
root_cause: "<statement>"         # Or "unresolved"
confidence: "<high|medium|low>"   # Confidence in root cause
mitigation_applied: "<action>"    # Or "none"
verification_result: "<resolved|mitigated|unresolved|escalated>"
time_to_detect: "<duration>"      # Issue start → detection
time_to_mitigate: "<duration>"    # Detection → mitigation
related_work_items:               # Affected work item IDs
  - "<work_item_id>"
---
```

### 7.3 Immutability (REQ-DIAG-092)

Diagnostic evidence records are immutable once created. If a diagnosis is later revised
(e.g., root cause was wrong), create a new record referencing the original — do not edit
the original.

### 7.4 Incident Postmortem Link (REQ-DIAG-093)

For P1 or P2 incidents, the diagnostic record MUST include a link to the incident
postmortem document (when one is created). Add the link as a follow-up diagnostic record.

### 7.5 Example Diagnostic Record (REQ-DIAG-100)

```yaml
---
type: diagnostic
timestamp: "2026-03-15T14:32:00Z"
trigger: "Alert: CodeOpsStepDurationHigh (p95 > 600s for 15 min)"
severity: P2
signals_consulted:
  - "metrics: codeops_step_duration_seconds filtered by repo=acme/api"
  - "logs: step_completed events for work_item_id=ACME-456, 14:00-14:30 UTC"
  - "traces: trace waterfall for correlation_id=abc-123-def"
hypotheses_tested:
  - hypothesis: "Model API latency degradation"
    evidence: "OpenCode request p95 increased from 8s to 45s starting 14:15 UTC"
    result: confirmed
  - hypothesis: "Resource contention in workspace container"
    evidence: "CPU and memory utilization normal (< 60%)"
    result: refuted
root_cause: >
  Upstream model API (provider X) experiencing degraded performance;
  confirmed via provider status page and request latency metrics.
confidence: high
mitigation_applied: "Switched to fallback model provider Y via config change"
verification_result: resolved
time_to_detect: "17 minutes"
time_to_mitigate: "42 minutes"
related_work_items:
  - "ACME-456"
---
```

---

## 8. Tooling Abstraction Layer

*Spec ref: REQ-DIAG-110 through REQ-DIAG-111*

### 8.1 Required Tooling Capabilities (REQ-DIAG-110)

Any tooling stack used for Axiom diagnostics MUST provide:

| Capability | What It Does | Example Tools |
|---|---|---|
| **Query logs** | Filter structured log events by correlation field, time range, event type | CloudWatch Logs Insights, Elasticsearch, Loki, Datadog Logs |
| **Query metrics** | Query time-series by label, compute rate/percentile/sum | Prometheus, Datadog Metrics, CloudWatch Metrics |
| **View traces** | Display trace waterfalls with span hierarchy and timing | Jaeger, Zipkin, Tempo, Datadog APM |
| **Correlate signals** | Navigate between pillars using shared identifiers | Grafana (data source linking), Datadog (unified view) |
| **Capture evidence** | Export query results, screenshots, data snapshots | Any tool with export/screenshot capability |
| **Alert management** | View active alerts, history, alert-to-runbook links | Alertmanager, PagerDuty, Datadog Monitors |

### 8.2 Tool Bindings Are Skill-Level (REQ-DIAG-111)

Tool-specific procedures (e.g., "how to query Prometheus for latency percentiles",
"how to navigate Grafana dashboards") belong in this skill's appendix or in companion
skills — not in the spec. The spec defines WHAT to do; this skill defines HOW.

---

## 9. Quick-Reference Checklists

### 9.1 Diagnostic Loop Checklist

Use this checklist for every diagnostic investigation:

- [ ] **Detect**: Detection record created (what, when, by whom, initial severity)
- [ ] **Triage**: Priority assigned (P1-P4), owner identified, proceed/defer decided
- [ ] **Scope**: Blast radius bounded (components, time window, user impact, correlation IDs)
- [ ] **Hypothesize**: At least one testable hypothesis with investigation plan
- [ ] **Investigate**: Evidence gathered for each hypothesis (supporting or refuting)
- [ ] **Confirm**: Root cause stated with multi-pillar corroboration, confidence assigned
- [ ] **Mitigate**: Action taken and documented (what, when, by whom, expected recovery)
- [ ] **Verify**: Resolution confirmed (signals back to baseline) OR loop back to Hypothesize

### 9.2 Cross-Pillar Correlation Checklist

Before declaring root cause, verify:

- [ ] Evidence from at least two signal pillars (logs, metrics, traces)
- [ ] Time windows aligned with ±60s buffer for cross-pillar queries
- [ ] Correlation IDs used where available (`run_id`, `work_item_id`, `correlation_id`)
- [ ] Negative results recorded (what was checked and ruled out)

### 9.3 AI-Specific Diagnostic Checklist

When diagnosing AI/model-related issues:

- [ ] Model API latency checked (is the slowness in our code or the API?)
- [ ] Token usage checked (are we approaching budget limits?)
- [ ] Context window checked (are requests exceeding model limits?)
- [ ] Agent loop checked (is the same step retrying without progress?)
- [ ] Model selection reviewed (is the right model being used for the task?)

### 9.4 Evidence Capture Checklist

Before closing a diagnostic investigation:

- [ ] Diagnostic record created with all required fields (REQ-DIAG-091)
- [ ] Time context included for every observation
- [ ] Specific signals cited (log line IDs, metric values, trace IDs)
- [ ] Screenshots/exports saved with descriptive filenames
- [ ] Record is immutable (corrections are additive, not edits)
- [ ] P1/P2 incidents linked to postmortem (when created)

---

## 10. Anti-Patterns

| # | Anti-Pattern | Why It Fails | Instead |
|---|---|---|---|
| 1 | **Jumping to mitigation** | Skipping Confirm means you might fix the wrong thing | Always confirm root cause with multi-pillar evidence before mitigating |
| 2 | **Single-pillar diagnosis** | One signal can mislead; correlation reduces false positives | Require evidence from at least two signal pillars |
| 3 | **Unbounded investigation** | Spending hours investigating without a hypothesis | Generate hypotheses first, then investigate systematically |
| 4 | **Tribal knowledge diagnosis** | "I just know it's the database" without evidence | Follow the loop; cite specific signals |
| 5 | **Ignoring negative results** | Not recording what was checked and ruled out | Record all investigation results, positive and negative |
| 6 | **Time-window mismatch** | Comparing metrics from 2pm with logs from 3pm | Always align time windows with ±60s buffer |
| 7 | **Missing correlation IDs** | Trying to correlate signals without shared identifiers | Ensure all components emit `run_id` and `work_item_id` |
| 8 | **Mutable diagnostic records** | Editing a diagnostic record after the fact | Records are immutable; corrections are new records |
| 9 | **Infinite investigation loop** | Verify → Hypothesize cycling without escalation | Bound to 3 iterations, then escalate |
| 10 | **Hero debugging** | One person debugging alone for hours | Follow the loop, document findings, escalate when stuck |

---

## 11. Spec Requirement Coverage Map

This skill covers the following requirements from `specs/65-Diagnostic-Workflows.md`:

| Requirement | Skill Section | Coverage |
|---|---|---|
| REQ-DIAG-001 (Canonical Phases) | §1 | Full — all 8 phases with entry/exit criteria |
| REQ-DIAG-002 (Detect) | §1.1 | Full |
| REQ-DIAG-003 (Triage) | §1.2 | Full |
| REQ-DIAG-004 (Scope) | §1.3 | Full |
| REQ-DIAG-005 (Hypothesize) | §1.4 | Full |
| REQ-DIAG-006 (Investigate) | §1.5 | Full |
| REQ-DIAG-007 (Confirm) | §1.6 | Full |
| REQ-DIAG-008 (Mitigate) | §1.7 | Full |
| REQ-DIAG-009 (Verify) | §1.8 | Full |
| REQ-DIAG-010 (Invariants) | §2 | Full |
| REQ-DIAG-020 (Correlation Fields) | §3.1 | Full |
| REQ-DIAG-021 (Query Patterns) | §3.2 | Full — all 5 patterns |
| REQ-DIAG-022 (Time Alignment) | §3.3 | Full |
| REQ-DIAG-023 (Min Correlation) | §3.4 | Full |
| REQ-DIAG-040 (Latency Spike) | §4.1 | Full |
| REQ-DIAG-041 (Error After Deploy) | §4.2 | Full |
| REQ-DIAG-042 (Memory Leak) | §4.3 | Full |
| REQ-DIAG-043 (Cascading Failure) | §4.4 | Full |
| REQ-DIAG-044 (Flaky Failures) | §4.5 | Full |
| REQ-DIAG-045 (Resource Exhaustion) | §4.6 | Full |
| REQ-DIAG-050 (AI Latency) | §5.1 | Full |
| REQ-DIAG-051 (Token Budget) | §5.2 | Full |
| REQ-DIAG-052 (Context Overflow) | §5.3 | Full |
| REQ-DIAG-053 (Agent Loop) | §5.4 | Full |
| REQ-DIAG-070 (Dashboard Reading) | §6.1 | Full |
| REQ-DIAG-071 (Flame Graphs) | §6.3 | Full |
| REQ-DIAG-072 (Trace Waterfalls) | §6.2 | Full |
| REQ-DIAG-073 (Heatmaps) | §6.4 | Full |
| REQ-DIAG-080 (Evidence Capture) | §6.5 | Full |
| REQ-DIAG-090 (Evidence Integration) | §7.1 | Full |
| REQ-DIAG-091 (Required Fields) | §7.2 | Full |
| REQ-DIAG-092 (Immutability) | §7.3 | Full |
| REQ-DIAG-093 (Postmortem Link) | §7.4 | Full |
| REQ-DIAG-100 (Example) | §7.5 | Full |
| REQ-DIAG-110 (Tool-Agnostic) | §8.1 | Full |
| REQ-DIAG-111 (Skill Bindings) | §8.2 | Full |
