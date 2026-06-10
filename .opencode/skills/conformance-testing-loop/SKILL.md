---
name: conformance-testing-loop
description: >
  Behavior-first conformance loop for any software testing surface (CLI, API,
  SDK/library, workers, jobs, UI flows, integration paths). Execute real behavior,
  compare against spec/contract, and for every mismatch: add a failing regression
  test, apply the smallest fix, and rerun until full scope is green.
version: "1.0"
tags:
  vertical: [coding]
  category: testing
  core: false
---

# Conformance Testing Loop (Portable)

> "Run real behavior first. If reality disagrees with contract, lock it with a failing test, fix minimally, rerun."

Use this skill for any testing loop where implementation must converge to documented behavior.

## When to Load This Skill

- You are validating behavior against specs/contracts across any surface
- You need a repeatable test -> fix -> retest loop for runtime or integration bugs
- You want full-scope conformance ("entire spectrum") without green theater
- You are working on code paths where unit-only proof is insufficient

## Core Rules

1. Behavior-first. Start with real execution on the target surface.
2. Contract-first decisions. Compare observed behavior to explicit spec/contract text.
3. Mismatch discipline. For each mismatch: failing regression test first, then fix.
4. Smallest meaningful change. One bug, one focused test, one focused fix.
5. Double verification after each fix:
   - Targeted regression test(s)
   - Real behavior probe that previously failed
6. Evidence required. Capture command/request, output/response, status/exit code, and state change.
7. Completion requires full matrix convergence (pass or explicit blocked reason for every row).

## Supported Testing Surfaces

- CLI command behavior and argument validation
- HTTP/gRPC/GraphQL/WebSocket/SSE API behavior
- SDK/library function behavior at public interfaces
- Worker/daemon/queue processing paths
- Adapter/integration behavior (third-party APIs, persistence, auth)
- UI flows and end-to-end product journeys

## Loop Algorithm

### Step 1: Define Conformance Matrix

Create rows of: surface -> scenario -> expected contract.

Suggested columns:
- Surface
- Scenario
- Contract reference
- Probe command/request/test
- Expected behavior
- Actual behavior
- Status (pass/fail/blocked)

### Step 2: Execute Real Probes

Run real probes first (avoid mocks here).

- CLI: execute actual commands
- APIs: run service and make real requests
- Libraries: call public API via realistic inputs
- Workers/jobs: run process and observe outputs/events

Capture outputs verbatim.

### Step 3: Identify Deltas

For each failure/unexpected result:
- Cite exact contract clause
- Record observed vs expected
- Assign severity

### Step 4: Add Failing Regression Test

Create regression coverage that fails before the fix and passes after.

- Test name references bug/symptom
- Test calls real product code path
- Test includes meaningful assertions

### Step 5: Apply Minimal Fix

Implement the smallest change that satisfies contract + regression test.

### Step 6: Re-run Verification

After each fix, rerun:
1. New regression test
2. Nearby relevant tests
3. Original failing probe

Repeat if mismatch remains.

### Step 7: Complete the Matrix

Proceed row-by-row until all in-scope rows are pass or explicitly blocked with evidence.

## Verification Ladder

- Tier 1: Regression test proof
- Tier 3+: Real execution path proof (minimum for done)
- Tier 4: Service integration proof when service paths are touched
- Tier 5: End-to-end proof when milestone/release closure is claimed

Do not claim completion on Tier 1 alone.

## Evidence Template (Per Mismatch)

```markdown
### Mismatch: <id>
- Contract: <path#section>
- Probe: `<exact command/request>`
- Observed: <actual>
- Expected: <contract behavior>
- Regression test: <path::test_name>
- Fix: <changed file(s)>
- Re-run:
  - test: PASS/FAIL
  - probe: PASS/FAIL
```

## Anti-Patterns

- Unit-only validation for runtime/integration claims
- Fix without regression coverage
- Assertionless/tautology/source-inspection tests
- Harness-only verification with no real probes
- Declaring done before matrix rows are closed

## Exit Criteria

Conformance run is complete only when:

- Every in-scope matrix row is pass or blocked with explicit rationale
- Every fixed mismatch has regression coverage
- Real probes for all fixed mismatches pass
- Evidence exists for each mismatch cycle

## References

- `specs/00-PRD.md#verification-signal-hierarchy`
- `specs/09-Baby-Steps-Methodology.md`
- `specs/48-Test-Quality-Gates.md`
- `.opencode/skills/regression-testing-bug-fixes/SKILL.md`
- `.opencode/skills/enterprise-testing-standard/SKILL.md`

## Trace

`axiom:trace work_item=cli-spectrum-01 spec=specs/00-PRD.md,specs/48-Test-Quality-Gates.md doc=.opencode/skills/conformance-testing-loop/SKILL.md`
