---
name: didi-qa-harness-axiom
description: >
  Scenario-based QA harness extending AutoTune via Morty templates. Covers scenario registry,
  diagnostic sub-stages, triage reporting, pluggable runners (shell, pytest, go-test, http-probe,
  mcp-tool, agent), flakiness detection, per-stage model declaration, and cost-attributed QA runs.
  Named after Dee Dee from Dexter's Laboratory — the sister who always broke things.
version: "1.0"
synopsis: |
  Didi is the QA sibling of AutoTune. Same Morty template engine, same Rick supervision, same
  checkpoint/resume — but the payload is test scenarios + diagnostic stages instead of parameter
  sweeps. The genuinely new piece is the diagnostic sub-stage that produces structured triage
  records, turning failures into routed graph operations instead of exceptions.
  Source spec: specs/99-Didi-QA-Harness.md.
when-to-use: |
  Load when running /axiom-qa or any sub-command, when setting up QA scenarios for a project,
  when an agent needs to understand the QA harness architecture, when diagnosing test failures
  via the diagnostic stage, or when reviewing triage reports.
tags:
  vertical: [coding, testing, operations]
  category: testing
  core: false
---

# didi-qa-harness-axiom — Scenario-Based QA via Morty Templates

Named after Dee Dee from Dexter's Laboratory — the sister who always broke things.

Scenario-based QA harness that extends the AutoTune/Morty/Rick architecture. Load this skill when
running QA scenarios, reviewing triage reports, setting up the QA harness, or understanding how
Didi relates to AutoTune and the local dev test harness.

**Source spec**: `specs/99-Didi-QA-Harness.md`
**Design doc**: `.memory-bank/projects/axiom/didi-autotune.md`
**Work item**: `didi-qa-harness-01`

---

## Quick Reference

| Aspect | Detail |
|--------|--------|
| Scenario registry | `.axiom/qa-scenarios/*.yaml` |
| Run results | `.axiom/qa-runs/<qa_run_id>/` |
| Triage report | `.axiom/qa-runs/<qa_run_id>/triage-report.md` |
| JSON summary | `.axiom/qa-runs/<qa_run_id>/summary.json` |
| Morty template | `qa-sweep` (5 stages: provision → generate-scenarios → scenario-loop → collect-results → report) |
| Cost attribution | `qa_run_id` in RunCostRecord (same pattern as AutoTune's `sweep_id`) |
| Diagnostic model | Per-stage model declaration; defaults to more capable model for diagnosis |

---

## CLI Commands

```bash
# Run all scenarios
axiom qa run

# Run scenarios by tag
axiom qa run --scenario-set integration

# Run with budget ceiling
axiom qa run --budget 20-usd

# Check status of running QA sweep
axiom qa status

# List scenario sets
axiom qa list

# Show triage report
axiom qa report --run qa-2026-04-28

# Diff two runs
axiom qa diff --report qa-2026-04-28

# Establish baseline
axiom qa baseline

# Check staleness + flakiness
axiom qa stale

# Replay a single scenario from a past run
axiom qa replay --run qa-2026-04-28 --scenario happy-path-multi-server
```

---

## The 5-Stage Shape (Mirroring AutoTune)

| Stage | Purpose | AutoTune Analog |
|-------|---------|-----------------|
| `provision` | Bring up dependencies (servers, services, fixtures) | `baseline` |
| `generate-scenarios` | Enumerate test scenarios from the registry | `generate-combos` |
| `scenario-loop` | Run each scenario; on failure, route to `diagnose` | `benchmark-loop` |
| `collect-results` | Aggregate pass/fail + diagnostics | `collect-results` |
| `report` | Produce triage report with RCAs and proposed fixes | `select-profile` |

---

## Diagnostic Stage — The New Piece

When `scenario-loop` reports `STATUS: FAIL` and the scenario has `on_failure.route_to: "diagnose"`,
control routes to a diagnostic sub-stage that:

1. Receives the failure output + collected artifacts + provision state + graph context
2. Uses a capable model (configurable, defaults to Opus-class) for long-context log reasoning
3. Produces a structured **triage record** with:
   - **verdict**: `system_bug | test_bug | flaky | environmental | unknown`
   - **confidence**: 0.0–1.0
   - **root_cause_hypothesis**: plain-language explanation
   - **evidence**: log references, metric pointers
   - **proposed_fix**: type + details + file + line
   - **retry_decision**: `retry_once | skip | abort`
4. Respects per-scenario `diagnostic_budget_usd` ceiling

The single most useful thing this does: **distinguish "the system is broken" from "the test is broken."**

---

## Relationship to Other Skills

| Skill | Relationship |
|-------|-------------|
| `axiom-autotune` | Sibling — same Morty template engine, different payload |
| `rick-and-morty-axiom` | Rick supervises Didi runs the same way it supervises AutoTune sweeps |
| `axiom-local-dev-test-harness` | Consumer — Didi uses it as the `pytest` runner type |
| `performance-benchmark-axiom` | Peer — AutoTune uses it for benchmarks; Didi may use it for perf scenarios |

---

## Scenario YAML Schema (Quick Reference)

```yaml
version: 1
name: "scenario-name"
description: "What this scenario tests"

session:
  shared_provision: true
  session_id: "integration"

dependencies:
  - service: postgres
    health: "pg_isready"
    timeout_seconds: 30

setup:
  - "docker compose up -d"

execute:
  runner: shell          # shell | pytest | go-test | http-probe | mcp-tool | agent
  command: "./run-test.sh"
  timeout_seconds: 600

success_criteria:
  - type: exit_code
    expected: 0
  - type: log_pattern
    file: ".out/test.log"
    pattern: "all tests passed"
  - type: semantic
    prompt: "Does the output contain valid results?"
    evaluator_model: "claude-haiku"

on_failure:
  route_to: "diagnose"
  collect:
    - ".out/*.log"
    - "docker compose logs"
  diagnostic_model: "claude-opus"
  diagnostic_budget_usd: 2.00

flakiness:
  retries: 1
  consider_flaky_after: 2

tags:
  - integration
```

---

## Key Design Decisions

1. **Evaluator ≠ executor** — the model that runs the test must not grade its own output
2. **Per-stage model declaration is a Morty-level feature** — not Didi-specific; benefits AutoTune too
3. **Diagnostic stage is read-only** — it reads logs and produces triage records; it does not modify the system
4. **Success criteria are layered cheapest-first** — exit_code → log_pattern → file_exists → semantic
5. **Didi consumes `axiom-local-dev-test-harness`** — they're peers at different layers

axiom:trace work_item=didi-qa-harness-01 spec=specs/99-Didi-QA-Harness.md
