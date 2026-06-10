---
name: runtime-spec-conformance-loop
description: >
  Real runtime conformance loop for any surface (CLI, HTTP API, SSE, background jobs,
  worker daemons, adapters). Execute actual behavior first, compare against specs, and
  for every mismatch: add a failing regression test, apply the smallest fix, and rerun
  runtime plus tests until the full target spectrum is green.
version: "1.0"
tags:
  vertical: [coding]
  category: testing
  core: false
---

# Runtime Spec Conformance Loop (Portable)

> "Run real behavior first. If reality disagrees with spec, lock it with a failing test, fix minimally, rerun."

Use this skill when you want continuous, evidence-backed convergence between implemented behavior and documented contract.

## When to Load This Skill

- You are validating real behavior against specs (not just unit-level correctness)
- You need a repeatable fix loop for runtime bugs
- You want to cover a complete surface area ("entire spectrum") without green theater
- You are working across mixed surfaces, not only CLI

## Core Rules

1. Real runtime first. Start with real command/request execution on the target surface.
2. Specs are contracts. Compare observed behavior to explicit spec text before changing code.
3. For every mismatch: write failing regression test first, then fix.
4. Smallest fix only. One bug, one focused code change, one focused test.
5. Re-verify at two levels after each fix:
   - Targeted regression test(s)
   - Real runtime command/request that previously failed
6. Evidence required. Capture command, output, exit/status, and resulting state.
7. Stop only when the selected spectrum matrix is all pass or explicitly blocked with reason.

## Supported Surfaces (Not CLI-only)

- CLI commands/subcommands and argument validation
- HTTP endpoints and health/readiness routes
- SSE/event streams and event payload contract
- Background jobs/daemons/dispatch loops
- Integration adapters (GitHub, Jira, OpenCode, queues)
- End-to-end workflow paths spanning multiple components

## Loop Algorithm

### Step 1: Define Spectrum Matrix

Create a matrix of surface -> scenario -> expected contract.

Example columns:
- Surface
- Scenario
- Spec reference
- Runtime probe command/request
- Expected behavior
- Actual behavior
- Status (pass/fail/blocked)

### Step 2: Execute Real Probes

Run real runtime probes first (no mocks for this step).

- CLI: run actual command invocations
- HTTP/SSE: run real server + real requests
- Daemons/workers: run process and observe outcomes

Record outputs verbatim.

### Step 3: Identify Contract Deltas

For each failure or unexpected behavior:
- Cite exact spec clause
- State observed mismatch
- Classify severity (critical/high/medium/low)

### Step 4: Test-First Bug Lock

Add a regression test that fails on current code and passes with intended behavior.

- Name includes bug id/symptom
- Test must execute real product code path
- Test must contain meaningful assertions (no tautologies)

### Step 5: Minimal Fix

Implement smallest code change to satisfy spec and test.

### Step 6: Re-run Verification

After each fix, rerun:
1. New regression test
2. Closely related test slice
3. Original runtime probe command/request

If runtime still mismatches spec, repeat from Step 3.

### Step 7: Advance Through Entire Spectrum

Continue matrix row-by-row until all selected rows are pass or documented blocked.

## Verification Ladder

Apply tiered evidence discipline:

- Tier 1: Regression test passes
- Tier 3+: Runtime path probe passes
- Tier 4: Service startup + health when server path touched
- Tier 5: E2E path when milestone closure is claimed

Do not mark "done" on Tier 1 alone.

## Evidence Template (Per Mismatch)

```markdown
### Mismatch: <id>
- Spec: <path#section>
- Probe: `<exact command/request>`
- Observed: <actual output/result>
- Expected: <spec behavior>
- Regression test: <path::test_name>
- Fix: <file path(s)>
- Re-run result:
  - test: PASS/FAIL
  - probe: PASS/FAIL
```

## Anti-Patterns

- Only running unit tests and claiming runtime correctness
- Applying a fix without adding regression coverage
- Writing assertionless or source-inspection tests
- Running synthetic harness only, never real runtime probes
- Closing work without updating the spectrum matrix status

## Recommended Command Pattern

Use this command rhythm repeatedly:

1. Probe runtime behavior
2. Add failing test
3. Run failing test to confirm red
4. Implement minimal fix
5. Run targeted tests
6. Re-run runtime probe

## Exit Criteria

A spectrum run can be called complete only when all are true:

- Every in-scope spectrum row is pass or explicitly blocked with reason
- Every fixed mismatch has regression test coverage
- Runtime probes for all fixed mismatches pass
- Evidence is captured for each mismatch cycle

## References

- `specs/00-PRD.md#verification-signal-hierarchy`
- `specs/09-Baby-Steps-Methodology.md`
- `specs/48-Test-Quality-Gates.md`
- `.opencode/skills/regression-testing-bug-fixes/SKILL.md`
- `.opencode/skills/enterprise-testing-standard/SKILL.md`

## Trace

`axiom:trace work_item=cli-spectrum-01 spec=specs/00-PRD.md,specs/48-Test-Quality-Gates.md doc=.opencode/skills/runtime-spec-conformance-loop/SKILL.md`
