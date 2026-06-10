---
name: idle-spec-conformance-sweep
description: >
  Portable idle-time spec conformance sweep policy for any spec-driven project.
  When no unblocked work items remain (all TODO items are complete, credential-gated,
  or explicitly deferred), the loop does NOT stop. Instead it picks a random spec,
  audits the codebase for alignment, and either confirms conformance or creates
  remediation work items for discovered gaps. This keeps agents productively
  verifying and hardening the system during otherwise idle cycles.
version: "1.0"
license: MIT
compatibility: agentskills
metadata:
  workflow: idle-sweep
  spec_refs:
    - specs/49-Loop-Self-Unblocking.md
    - specs/00-PRD.md#verification-signal-hierarchy
  related_skills:
    - ralph-wiggum-loop
    - conformance-testing-loop
    - runtime-spec-conformance-loop
    - runtime-completeness-gate-axiom
    - axiom-todo
    - enterprise-testing-standard
tags:
  vertical: [coding, planning]
  category: methodology
  core: false
---

# Idle-Time Spec Conformance Sweep (Portable)

> "Never idle. When all planned work is done or blocked, audit specs against reality. Gaps found are real work; conformance confirmed is real evidence."

Use this skill to keep any spec-driven loop productive when no explicit TODO items are actionable. Instead of stopping, the agent audits one spec at a time against the live codebase and runtime, discovers alignment gaps, and either confirms conformance or creates remediation work.

## When to Load This Skill

- All TODO checkboxes are checked, credential-gated, or explicitly deferred
- No active steering packet or work item is unblocked
- The loop would otherwise declare `BLOCKED` or `stop`
- You want continuous spec-to-system alignment verification during quiet periods
- You are scaffolding a new Ralph loop and want to include idle-time productivity

## Why This Matters

Without idle sweep, autonomous loops stop when planned work runs out or gets blocked by external dependencies (credentials, approvals, infrastructure). This wastes cycles. With idle sweep:

1. **Continuous hardening**: Every idle cycle discovers and closes spec alignment gaps
2. **Drift detection**: Specs that were once conformant may drift as code evolves
3. **Gap discovery**: Specs written ahead of implementation get audited for readiness
4. **Evidence accumulation**: Each sweep produces immutable conformance evidence
5. **Loop persistence**: The loop never stops while unswept specs remain

## Core Rules

1. **Idle sweep takes precedence over credential-gated stop.** When the only unchecked TODO items are credential-gated or explicitly deferred, the loop MUST NOT stop. Run a spec conformance sweep instead.

2. **One spec per sweep iteration.** Depth over breadth. Audit one spec file thoroughly rather than skimming many.

3. **Random selection with exclusions.** Pick one spec from `specs/` at random (excluding `README.md`, `_index.md`, `_prompt.md`, and `_inputs/`). Use a non-deterministic selection method (e.g., timestamp-based modulo over the spec file list) to avoid always auditing the same files.

4. **Real verification required.** Tier 3+ runtime evidence is required per the verification signal hierarchy. Module imports and unit tests alone are insufficient.

5. **Conformance or remediation — no middle ground.** Either the spec is conformant (with evidence) or concrete gaps are found and turned into work items.

6. **Gaps create real work.** When a sweep discovers spec-required implementation gaps, stop rotating to new specs. Create/update a dedicated remediation work item and route the next iteration there.

7. **Evidence is immutable.** Record which spec was audited and the outcome in an immutable run snapshot.

## Sweep Algorithm

### Step 1: Determine Eligibility

Check these conditions (ALL must be true to enter idle sweep):

```
eligibility_check:
  - No unchecked, unblocked TODO items remain
  - No active steering packet with DECISION: continue/steer
  - No active work item in IN_PROGRESS state with unblocked steps
  - At least one spec file in specs/ has not been recently swept
```

If any unblocked work exists, do that work instead. Idle sweep is the fallback, never the priority.

### Step 2: Select a Spec

```
spec_selection:
  source_dir: specs/
  exclude:
    - README.md
    - _index.md
    - _prompt.md
    - _inputs/
    - any spec already swept in the current rotation (track in work item)
  method: non-deterministic (timestamp modulo, random, etc.)
  output: one spec file path
```

Track which specs have been swept to avoid re-auditing the same spec repeatedly. When all specs have been swept once, reset the rotation.

### Step 3: Audit Spec-to-System Alignment

For the selected spec, verify every requirement (`REQ-*`), acceptance criterion (`AC-*`), and behavioral contract:

| Check | Method | Evidence Required |
|-------|--------|-------------------|
| Required modules/functions/endpoints exist | `Grep`, `Read` source code | File paths + line numbers |
| Not stubbed (real implementation) | Read function bodies | Code snippets showing real logic |
| Tests cover acceptance criteria | `Grep` test files for spec refs | Test file paths + assertion counts |
| Runtime behavior matches spec | Execute commands (Tier 3+) | Command output transcripts |
| No contradictions between spec and impl | Compare spec text vs behavior | Diff or mismatch description |

If the audited spec governs a **runnable surface** (CLI, HTTP API/server, admin panel, UI/operator flow, worker/job, plugin/operator command), also load and apply `runtime-completeness-gate-axiom` so the sweep checks the real human/operator path instead of stopping at code-shape or mock-only proof.

If the audited spec governs a **data flow**, also apply the **Wiring Gap** protocol from `runtime-completeness-gate-axiom` (Step 2b: Data Path Matrix, Checklist Section E). Treat "page shows no data but data exists" as a wiring gap signal, not a data absence. The Kill Question applies: *"Show me the exact line where every IN-path populates every field every OUT-path depends on."*

A spec **governs a data flow** if it describes any of the following (check the spec text for these signals):
- Writes to 2 or more tables / collections / projections
- 2 or more distinct insertion paths (any combination of: primary pipeline, seeder, importer, migration, admin endpoint, background job, event consumer, webhook)
- Reads via JOIN, FK, or reference from tables/collections written by different paths
- An aggregation or count that appears in more than one UI surface

**Positive examples** (trigger wiring gap protocol):
1. A spec for a document import feature that writes to `documents` and `document_metadata` via both the API and a bulk importer
2. A spec for a dashboard that shows counts derived from a `reports` table populated by a background job
3. A spec for a user onboarding flow that creates rows in `users`, `profiles`, and `subscriptions` via both the signup API and an admin provisioning tool

**Negative examples** (do not trigger wiring gap protocol):
1. A spec for a single-table CRUD endpoint with one write path (the API itself)
2. A spec for a CLI command that reads config and prints output (no persistence)
3. A spec for an authentication flow that writes only to a `sessions` table via one path

### Step 4: Record Outcome

**If conformant:**
```
outcome: CONFORMANT
record_in:
  - work item run snapshot (verification.md)
  - brief note in _current.md or rolling verification
action: rotate to next random spec in next idle cycle
```

**If gaps found:**
```
outcome: GAPS_FOUND
for_each_gap:
  1. Create/update work item under .memory-bank/work-items/<WORK_ITEM_ID>/
     with meta-planning.md, plan.md, and plan.yaml
  2. Create/update implementation plan under
     .memory-bank/implementation-plans/P-XX-<descriptive-name>.md
  3. Add tasks to .memory-bank/TODO.md with checkboxes mapped to spec REQs
  4. Update _index.md files for discoverability
  5. Begin executing highest-priority gap immediately
action: STOP rotating specs; route next iteration to remediation
```

### Step 5: Remediation Handoff

When gaps are found:

1. **Stop random rotation.** Pin the sweep to the current spec until gaps are resolved or explicitly deferred.
2. **Create a dedicated work item.** Do not mix remediation into the sweep work item.
3. **Route the next loop iteration** to the remediation work item, not back to idle sweep.
4. **Return to idle sweep** only after remediation is complete or blocked.

## Integration Points

### With Ralph Loop (`ralph-wiggum-loop`)

The idle sweep policy should be included in every Ralph loop's `PROMPT.md`:

```markdown
## Idle-Time Spec Conformance Sweep

When no unchecked TODO items, unblocked work items, or active steering packets
remain, do NOT stop. Instead, run a random spec conformance sweep per the
idle-spec-conformance-sweep skill.

Loop persistence rule: do not declare terminal blocked/stop while non-credential
alignment work remains. Idle-time spec conformance sweeps are always available
as non-credential alignment work.
```

### With TODO.md (`axiom-todo`)

The idle sweep work item should appear in TODO.md as a recurring phase:

```markdown
## Phase XX — Idle-Time Spec Conformance Sweep (`idle-spec-conformance-sweep-01`)

- Status: ACTIVE (recurring)
- Work item: `idle-spec-conformance-sweep-01`
- Governing specs: all specs in specs/ (rotating audit target)

### Phase XX.N — Recurring Sweep Step

- [ ] `idle-spec-conformance-sweep-01` `phase-XX-N/task-XX-N-1/step-XX-N-1-1`
  — Audit one random spec from specs/ for alignment with codebase and runtime.
  Done evidence: immutable run snapshot with conformance verdict or gap list.
```

### With Loop Self-Unblocking (`specs/49-Loop-Self-Unblocking.md`)

The sitrep unblock pass MUST check for idle-time sweep eligibility before declaring terminal BLOCKED:

1. Check if unswept specs remain in `specs/`
2. If yes: select a random unswept spec
3. Set `NEXT_BUILDER_STEP` to the idle-time sweep task
4. Set `NEXT_BUILDER_PROMPT` to the spec audit instruction
5. Emit `DECISION: continue` (not `stop`)

### With Verifier Captain (`ralph-wiggum-verify`)

The verifier MUST recognize idle sweep iterations and apply appropriate gates:

- Verify the selected spec was actually audited (not just claimed)
- Verify Tier 3+ runtime evidence exists
- Verify gaps found were converted to work items (not just noted)
- Verify conformance claims have supporting evidence

### With Conformance Testing Loop (`conformance-testing-loop`)

When gaps are found, the remediation phase uses the conformance testing loop:

1. Define conformance matrix from spec requirements
2. Execute real probes against each requirement
3. For each mismatch: failing regression test first, then fix
4. Rerun until full scope is green

## Anti-Patterns

| Anti-Pattern | Why It's Wrong | Correct Approach |
|-------------|----------------|------------------|
| Stopping when TODO is empty | Wastes idle cycles; specs may have drifted | Enter idle sweep |
| Skimming many specs shallowly | Misses real gaps; produces false conformance | One spec, full depth |
| Claiming conformance without runtime evidence | Green theater; Tier 0-2 is insufficient | Require Tier 3+ evidence |
| Rotating to new spec when gaps are found | Leaves gaps unresolved; creates debt | Pin to remediation until resolved |
| Mixing remediation into sweep work item | Muddies evidence; hard to track | Create dedicated remediation work item |
| Re-auditing same spec repeatedly | Wastes cycles; no new information | Track swept specs; rotate |
| Treating idle sweep as lower priority than process debt | Process debt replay is rabbit-hole churn | Real spec gaps > process shape repair |

## Worked Example

**Scenario**: All TODO items are credential-gated. Loop would normally stop.

```
Iteration N:
  1. Check eligibility: all TODO items credential-gated ✓
  2. Select random spec: specs/11-Confidence-Scoring.md
  3. Audit:
     - REQ-CONF-INTAKE-001: Read confidence.py → function exists, not stubbed ✓
     - REQ-CONF-OUTPUT-001: Read output model → field present ✓
     - REQ-CONF-MIN-SIGNALS-001: Run `python -m pytest tests/test_confidence.py` → 12/12 PASS ✓
     - Tier 3: `axiom run --work-item "smoke-test" --repo . --in-process` → exit 2 (dirty worktree guard, expected)
     - Tier 4: `axiom serve --port 8100 &` + `curl -sf http://127.0.0.1:8100/health` → 200 OK ✓
  4. Verdict: CONFORMANT
  5. Record in run snapshot, rotate to next spec

Iteration N+1:
  1. Check eligibility: still credential-gated ✓
  2. Select random spec: specs/10-Lifecycle-State-Machine.md
  3. Audit:
     - REQ-DEP-002: dependency coordination → PARTIAL (missing approval chain)
     - REQ-DEP-003: state transition validation → MISSING (no test coverage)
  4. Verdict: GAPS_FOUND
  5. Create work item: lifecycle-dependency-conformance-01
  6. Create plan: P-89-lifecycle-dependency-conformance.md
  7. Add to TODO.md
  8. Route next iteration to remediation (NOT back to idle sweep)
```

## Configuration (Portable Defaults)

These defaults can be overridden per-project in `PROMPT.md` or `.axiom/axiom.config.yaml`:

| Setting | Default | Description |
|---------|---------|-------------|
| `sweep.enabled` | `true` | Enable idle-time sweep |
| `sweep.min_tier` | `3` | Minimum verification tier for conformance claims |
| `sweep.max_specs_per_rotation` | `all` | How many specs before rotation resets |
| `sweep.exclude_patterns` | `README.md, _index.md, _prompt.md, _inputs/` | Specs to skip |
| `sweep.work_item_id` | `idle-spec-conformance-sweep-01` | Default work item ID |
| `sweep.step_id_pattern` | `phase-XX-N/task-XX-N-1/step-XX-N-1-1` | Step ID template |
| `sweep.gap_threshold` | `0` | Number of gaps before creating remediation work item (0 = any gap) |

## Spec References

- `specs/49-Loop-Self-Unblocking.md` — Loop self-unblocking and idle sweep eligibility
- `specs/00-PRD.md#verification-signal-hierarchy` — Tier 3+ evidence requirements
- `specs/09-Baby-Steps-Methodology.md` — One step per iteration discipline
- `specs/45-TODO-Lifecycle-And-Archive.md` — TODO management for sweep work items

## Trace Marker

```
axiom:trace work_item=idle-spec-conformance-sweep-01 spec=<audited-spec> plan=phase-XX-N/task-XX-N-1/step-XX-N-1-1 test= doc= evidence=<run-snapshot-path> commit=
```
