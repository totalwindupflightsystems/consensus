---
name: performance-benchmark-axiom
description: >
  Load/stress/soak test generation, baseline establishment, regression detection, performance
  budgeting, and AI inference latency tracking. Load this skill when establishing performance
  baselines, running benchmarks, detecting regressions, or defining performance budgets for
  any service managed by Axiom.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-02-27"
  primary_spec: specs/34-Observability-And-Metrics.md
  secondary_specs:
    - specs/47-Cost-Tracking-And-Session-Analytics.md
    - specs/00-PRD.md
tags:
  vertical: [benchmarking, sre]
  category: performance
  core: false
---

# Performance Benchmark Skill (Portable)

> **"Never claim performance is acceptable without a measured baseline."**
>
> **"A benchmark without a regression threshold is just a number."**

This skill provides portable, production-grade guidance for performance testing, baseline
establishment, regression detection, and performance budgeting. It covers both traditional
service benchmarks and AI-specific performance concerns (inference latency, token throughput,
context window impact).

---

## Activation

Load this skill when:
- Establishing a performance baseline for a new service or endpoint
- Running load, stress, soak, spike, or volume tests
- Defining performance budgets (latency, throughput, error rate)
- Detecting performance regressions between releases
- Benchmarking AI inference latency or token throughput
- Benchmarking Axiom runtime (`axiom run`, `axiom serve`)
- Preparing a performance report for a release gate
- Investigating a performance degradation

---

## Non-Negotiables

1. **No performance claims without a measured baseline.** Every performance assertion must
   reference a specific baseline measurement with date, tool, configuration, and raw output.

2. **Fail-closed on regressions above threshold.** If p95 latency degrades >10% from baseline
   (configurable), the verdict is FAIL. No exceptions without documented approval.

3. **Reproducible benchmarks only.** Every benchmark must specify: tool, configuration, duration,
   concurrency, target, and environment. "It felt fast" is not a benchmark.

4. **AI inference latency is a first-class metric.** For AI-assisted systems, model inference
   latency, token throughput, and context window size impact must be tracked alongside
   traditional service metrics.

5. **Never benchmark in production without safeguards.** Load tests against production require
   explicit approval, rate limiting, and a kill switch.

---

## Performance Test Types

| Type | Purpose | Duration | Concurrency | When to Use |
|------|---------|----------|-------------|-------------|
| **Load** | Normal traffic simulation | 5-30 min | Expected concurrent users | Baseline establishment |
| **Stress** | Find breaking point | 10-30 min | Ramp beyond capacity | Capacity planning |
| **Soak** | Detect memory leaks, degradation | 1-8 hours | Sustained normal load | Pre-release validation |
| **Spike** | Sudden burst handling | 5-15 min | 10x normal, then drop | Resilience testing |
| **Volume** | Large data handling | Varies | Normal concurrency, large payloads | Data pipeline testing |

---

## Benchmark Template

Every benchmark MUST include these fields:

```yaml
benchmark:
  name: "<descriptive name>"
  target: "<service/endpoint/command>"
  type: "load | stress | soak | spike | volume"
  baseline:
    date: "<ISO 8601>"
    environment: "<description>"
    tool: "<tool name + version>"
    configuration:
      duration: "<e.g., 5m>"
      concurrency: "<e.g., 10 virtual users>"
      ramp_up: "<e.g., 30s>"
    results:
      p50_ms: <value>
      p95_ms: <value>
      p99_ms: <value>
      throughput_rps: <value>
      error_rate_pct: <value>
      total_requests: <value>
  target_budget:
    p50_ms: <max acceptable>
    p95_ms: <max acceptable>
    p99_ms: <max acceptable>
    throughput_rps: <min acceptable>
    error_rate_pct: <max acceptable>
  regression_threshold:
    p95_degradation_pct: 10  # FAIL if p95 degrades more than this
    throughput_degradation_pct: 15
    error_rate_increase_pct: 5
  success_criteria:
    - "p95 latency < <target>ms"
    - "error rate < <target>%"
    - "throughput > <target> req/s"
```

---

## Tools and CLI Examples

### k6 (Recommended for HTTP)

```bash
# Install
brew install k6  # or: go install go.k6.io/k6@latest

# Basic load test
cat > /tmp/k6-load.js << 'EOF'
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '30s', target: 10 },  // ramp up
    { duration: '5m', target: 10 },   // sustain
    { duration: '30s', target: 0 },   // ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'],  // p95 < 500ms
    http_req_failed: ['rate<0.01'],    // error rate < 1%
  },
};

export default function () {
  const res = http.get('http://127.0.0.1:8100/health');
  check(res, {
    'status is 200': (r) => r.status === 200,
    'response has status field': (r) => JSON.parse(r.body).status !== undefined,
  });
  sleep(1);
}
EOF

k6 run /tmp/k6-load.js 2>&1 | tee /tmp/k6-results.txt
```

### locust (Python-native)

```bash
# Install
pip install locust

# Basic locustfile
cat > /tmp/locustfile.py << 'EOF'
from locust import HttpUser, task, between

class CodeOpsUser(HttpUser):
    wait_time = between(1, 3)

    @task(3)
    def health_check(self):
        self.client.get("/health")

    @task(1)
    def list_runs(self):
        self.client.get("/api/v1/runs")
EOF

# Run headless
locust -f /tmp/locustfile.py --host http://127.0.0.1:8100 \
  --users 10 --spawn-rate 2 --run-time 5m --headless \
  --csv /tmp/locust-results 2>&1 | tee /tmp/locust-output.txt
```

### wrk (High-throughput HTTP)

```bash
# Simple throughput test
wrk -t4 -c100 -d30s http://127.0.0.1:8100/health 2>&1 | tee /tmp/wrk-results.txt

# With Lua script for POST requests
wrk -t4 -c50 -d60s -s /tmp/post.lua http://127.0.0.1:8100/api/v1/runs
```

### hey (Simple HTTP benchmarking)

```bash
# 200 requests, 10 concurrent
hey -n 200 -c 10 http://127.0.0.1:8100/health 2>&1 | tee /tmp/hey-results.txt

# With POST body
hey -n 100 -c 5 -m POST \
  -H "Content-Type: application/json" \
  -d '{"intent":"benchmark test"}' \
  http://127.0.0.1:8100/api/v1/runs 2>&1 | tee /tmp/hey-post-results.txt
```

### hyperfine (CLI command benchmarking)

```bash
# Benchmark CLI commands
hyperfine --warmup 3 --runs 10 \
  'axiom run --help' \
  2>&1 | tee /tmp/hyperfine-results.txt

# Compare two implementations
hyperfine --warmup 3 \
  'axiom run --work-item "bench" --repo . --in-process --dry-run' \
  'axiom run --work-item "bench" --repo . --in-process --dry-run --new-engine' \
  2>&1 | tee /tmp/hyperfine-compare.txt
```

---

## AI-Specific Performance Concerns

### Model Inference Latency

AI-assisted systems have unique performance characteristics:

| Metric | What It Measures | Why It Matters |
|--------|-----------------|----------------|
| **Time to First Token (TTFT)** | Latency before streaming starts | User-perceived responsiveness |
| **Tokens per Second (TPS)** | Output generation speed | Throughput for batch operations |
| **Total Inference Time** | End-to-end LLM call duration | Step execution time |
| **Context Window Utilization** | % of context window used | Performance degrades near limits |
| **Cache Hit Rate** | % of tokens served from cache | Cost and latency reduction |

### Benchmarking AI Inference

```bash
# Measure OpenCode session execution time
time curl -sf -X POST http://127.0.0.1:4096/session \
  -H "Content-Type: application/json" \
  -d '{}' > /tmp/session.json

SESSION_ID=$(python3 -c "import json; print(json.load(open('/tmp/session.json'))['id'])")

# Time a command execution
time curl -sf -X POST "http://127.0.0.1:4096/session/$SESSION_ID/message" \
  -H "Content-Type: application/json" \
  -d '{"parts":[{"type":"text","text":"What is 2+2?"}]}' > /tmp/response.json

# Extract token usage from cost.json after a run
python3 -c "
import json
cost = json.load(open('.memory-bank/work-items/<ID>/runs/<RUN_ID>/cost.json'))
print(f'Total tokens: {cost[\"total_tokens\"][\"input\"] + cost[\"total_tokens\"][\"output\"]}')
print(f'Total cost: \${cost[\"total_cost_usd\"]:.4f}')
print(f'Messages: {cost[\"message_count\"]}')
print(f'Avg tokens/message: {(cost[\"total_tokens\"][\"input\"] + cost[\"total_tokens\"][\"output\"]) / max(cost[\"message_count\"], 1):.0f}')
"
```

### Context Window Size Impact

As context grows, inference latency increases. Track this relationship:

```python
# Benchmark context window impact
import time

context_sizes = [1000, 5000, 10000, 25000, 50000, 100000]
results = []

for size in context_sizes:
    prompt = "x " * size  # Approximate token count
    start = time.monotonic()
    # ... send to model ...
    elapsed = time.monotonic() - start
    results.append({"context_tokens": size, "latency_ms": elapsed * 1000})

# Plot or tabulate results to identify the degradation curve
```

---

## Axiom-Specific Benchmarks

### `axiom run` Benchmark

```bash
# Benchmark the full run pipeline (dry-run mode if available)
hyperfine --warmup 1 --runs 5 \
  'axiom run --work-item "perf-bench" --repo . --in-process' \
  2>&1 | tee /tmp/axiom-run-bench.txt
```

### `axiom serve` Benchmark

```bash
# Start server
axiom serve --port 8100 &
sleep 3

# Health endpoint baseline
hey -n 1000 -c 20 http://127.0.0.1:8100/health 2>&1 | tee /tmp/serve-health-bench.txt

# Runs list endpoint
hey -n 200 -c 10 http://127.0.0.1:8100/api/v1/runs 2>&1 | tee /tmp/serve-runs-bench.txt

# SSE connection establishment
for i in $(seq 1 10); do
  time timeout 2 curl -sf -N http://127.0.0.1:8100/api/v1/events/stream > /dev/null 2>&1
done 2>&1 | tee /tmp/serve-sse-bench.txt

kill %1
```

### OpenCode Integration Benchmark

```bash
# Benchmark OpenCode health check latency
hey -n 100 -c 5 http://127.0.0.1:4096/global/health 2>&1 | tee /tmp/opencode-health-bench.txt

# Benchmark session creation
hey -n 50 -c 2 -m POST \
  -H "Content-Type: application/json" \
  -d '{}' \
  http://127.0.0.1:4096/session 2>&1 | tee /tmp/opencode-session-bench.txt
```

---

## Performance Budgets

### Defining Budgets

A performance budget sets the maximum acceptable values for key metrics:

```yaml
# .axiom/performance-budget.yaml
budgets:
  - name: "Health endpoint"
    target: "GET /health"
    p50_ms: 10
    p95_ms: 50
    p99_ms: 100
    throughput_rps: 500
    error_rate_pct: 0.1

  - name: "Run list endpoint"
    target: "GET /api/v1/runs"
    p50_ms: 50
    p95_ms: 200
    p99_ms: 500
    throughput_rps: 100
    error_rate_pct: 0.5

  - name: "SSE connection"
    target: "GET /api/v1/events/stream"
    connection_time_p95_ms: 100
    first_event_p95_ms: 500

  - name: "CLI startup"
    target: "axiom run --help"
    p95_ms: 2000

  - name: "AI inference (single step)"
    target: "OpenCode /session/{id}/message"
    p50_ms: 30000
    p95_ms: 120000
    p99_ms: 300000
```

---

## Regression Detection

### Comparing Current vs Baseline

```python
def check_regression(current: dict, baseline: dict, thresholds: dict) -> dict:
    """Compare current benchmark against baseline. Return verdict."""
    results = {}
    for metric in ["p50_ms", "p95_ms", "p99_ms"]:
        if metric in current and metric in baseline:
            pct_change = ((current[metric] - baseline[metric]) / baseline[metric]) * 100
            threshold = thresholds.get(f"{metric.split('_')[0]}_degradation_pct", 10)
            results[metric] = {
                "baseline": baseline[metric],
                "current": current[metric],
                "change_pct": round(pct_change, 1),
                "threshold_pct": threshold,
                "status": "FAIL" if pct_change > threshold else "PASS",
            }
    return results
```

### CI Integration

```bash
# Run benchmark and compare against stored baseline
k6 run benchmark.js --out json=/tmp/k6-current.json

# Compare (pseudo-script)
python3 scripts/compare-benchmark.py \
  --baseline .axiom/benchmarks/baseline.json \
  --current /tmp/k6-current.json \
  --threshold-p95 10 \
  --threshold-throughput 15
```

---

## Performance Verdict

### Verdict Scale

| Verdict | Score | Meaning |
|---------|-------|---------|
| **PASS** | 80-100 | All metrics within budget; no regressions above threshold |
| **WARN** | 50-79 | Minor budget violations or regressions near threshold |
| **FAIL** | 1-49 | Significant budget violations or regressions above threshold |
| **BLOCKED** | 0 | Cannot benchmark (server won't start, tools unavailable, no baseline) |

### Scoring Rubric

| Check | Weight | Pass Criteria |
|-------|--------|---------------|
| Baseline exists | 10 | A dated baseline measurement exists for comparison |
| p95 within budget | 25 | p95 latency <= budget target |
| p99 within budget | 15 | p99 latency <= budget target |
| Throughput within budget | 20 | Throughput >= budget minimum |
| Error rate within budget | 15 | Error rate <= budget maximum |
| No regression vs baseline | 15 | p95 degradation <= threshold % |

### Verdict Template

```markdown
## Performance Benchmark Verdict

**Verdict**: PASS | WARN | FAIL | BLOCKED
**Score**: <0-100>
**Date**: <ISO 8601>
**Tool**: <tool + version>
**Target**: <service/endpoint>
**Duration**: <test duration>
**Concurrency**: <virtual users/threads>

### Results vs Budget
| Metric | Budget | Actual | Status |
|--------|--------|--------|--------|
| p50 | <budget>ms | <actual>ms | PASS/FAIL |
| p95 | <budget>ms | <actual>ms | PASS/FAIL |
| p99 | <budget>ms | <actual>ms | PASS/FAIL |
| Throughput | ><budget> rps | <actual> rps | PASS/FAIL |
| Error rate | <<budget>% | <actual>% | PASS/FAIL |

### Regression Check (vs baseline <date>)
| Metric | Baseline | Current | Change | Threshold | Status |
|--------|----------|---------|--------|-----------|--------|
| p95 | <base>ms | <curr>ms | +<X>% | 10% | PASS/FAIL |

### Evidence
- Raw output: <path>
- Baseline: <path>
```

---

## Integration

### Works With

| Skill/Agent | Integration Point |
|-------------|-------------------|
| `@sre-ops-axiom` | SLI/SLO definition uses benchmark baselines |
| `enterprise-release-quality` | Performance benchmarks are a release gate |
| `api-contract-validator-axiom` | API performance is part of contract compliance |
| `protocol-testing` | Protocol tests can include performance assertions |
| `chaos-engineer-axiom` | Chaos tests measure performance under failure |

---

## AI-Assisted Development Risks (2026)

| Risk | Mitigation |
|------|------------|
| AI claims "performance is good" without data | Require measured baseline with raw output |
| AI generates benchmarks with unrealistic parameters | Review concurrency, duration, and target |
| AI ignores AI-specific metrics (TTFT, TPS) | Include AI inference metrics in budget |
| AI benchmarks against localhost only | Document environment; note localhost != production |
| AI skips soak tests | Mandate soak test for pre-release validation |

---

## Trace

`axiom:trace work_item=performance-benchmark-axiom spec=specs/34-Observability-And-Metrics.md plan= prompt=.opencode/skills/performance-benchmark-axiom/SKILL.md evidence= doc= test= commit=`
