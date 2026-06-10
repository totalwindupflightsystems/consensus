---
name: middle-out-planning-axiom
description: >
  Middle-Out Implementation Planning for AI agents. Start from the critical integration
  boundary — the place where the most risk and uncertainty lives — prove it works first,
  then expand outward in both directions. Prevents the two classic AI failure modes:
  top-down isolation (components built separately, never wired) and bottom-up avoidance
  (easy parts built first, hard integration deferred until context runs out).
version: "1.0"
spec: specs/94-Middle-Out-Implementation-Planning.md
tags:
  vertical: [coding, planning]
  category: methodology
  core: false
---

# Middle-Out Implementation Planning (Axiom)

> **"Start where the risk is highest. Prove the hard part works first. Expand from a working center."**
>
> **"Top-down builds components. Bottom-up avoids the hard parts. Middle-out starts at the join."**
>
> **"The boundary is not the scary part. The scary part is discovering the boundary doesn't work after you've built everything around it."**

## Purpose

This skill guides AI agents to plan and execute implementation starting from the **critical integration boundary** — the point where two or more systems meet, where the most assumptions are being made, and where failure invalidates the most downstream work.

Middle-out is the answer to two failure modes that plague AI-built systems:

| Failure Mode | What Happens | Middle-Out Fix |
|---|---|---|
| **Top-down isolation** | Agent builds components in isolation (spec → plan → code), discovers they don't wire together at integration time. Wasted work. | Start at the boundary. Prove the wire works before building the components. |
| **Bottom-up avoidance** | Agent builds what's easy first, avoids the hard integration problems, runs out of context before reaching them. | Start at the hard part. The easy parts are built after the hard part is proven. |

## When to Load This Skill

Load when:
- Planning a feature that crosses system boundaries (API ↔ DB, frontend ↔ backend, agent ↔ runtime)
- Starting a new work item where the integration point is unclear or risky
- A previous implementation had wiring gaps caught by the runtime-completeness-gate
- The plan has multiple phases and you're deciding where to start
- The request involves two or more systems that must exchange data

Do NOT load when:
- Pure refactoring (no new boundaries)
- Documentation-only changes
- Config changes with no behavioral impact
- Single-component work that doesn't cross a boundary

## The Four-Step Method

### Step 1: Identify the Critical Integration Boundary

Before writing any code, answer: **"Where is the join?"**

The boundary is where:
- Two or more systems meet (API ↔ DB, frontend ↔ backend, agent ↔ runtime)
- The most assumptions are being made
- Failure here invalidates the most downstream work
- The contract is least well-defined

**Boundary identification questions:**
- What are the two (or more) systems involved?
- What data flows between them?
- What contract governs the exchange (schema, protocol, format)?
- What happens when the boundary fails?
- Which side owns the contract?

**Common boundary types:**

| Feature Type | Critical Boundary |
|---|---|
| New API endpoint | Route → handler → DB |
| New agent capability | Agent → runtime → tool |
| New UI feature | Component → API → backend |
| Cross-service integration | Service A → message → Service B |
| DB migration with app changes | Migration → ORM → API |
| CLI tool | CLI entrypoint → executor → side effect |

### Boundary Ownership

Before building the boundary proof, classify the boundary:

- **Owned boundary** (both sides under your control): Real proof required — no mocks, no stubs.
- **Partially-owned boundary** (one side is a third-party API): Use a contract test against a recorded fixture. Regenerate the fixture against the real API before Phase 5 (Harden).
- **Fully-external boundary** (both sides are third-party): Middle-out does not apply — use integration testing instead.

This classification is required by `specs/94-Middle-Out-Implementation-Planning.md#boundary-ownership`.

**Recorded fixture requirements (Partially-owned boundaries):** Fixtures used as boundary proofs must include provenance metadata (date + real endpoint recorded against), live in a version-controlled repo location (not `/tmp` or outside the repo), and the plan must include a Phase 5 freshness gate that re-records and diffs against the live API. Any recording tool is acceptable. Full requirements: `specs/94-Middle-Out-Implementation-Planning.md#recorded-fixture-requirements-partially-owned-boundaries`.

<!-- axiom:trace work_item=middle-out-planning-01 spec=specs/94-Middle-Out-Implementation-Planning.md plan=phase-mo-2/task-mo-2-3/step-mo-2-3-1 jira_ref=DEX-476 -->

### Step 2: Build the Thinnest Possible Vertical Slice

Not a mock. Not a stub. A **real, working, minimal path** through the integration point.

The slice must:
- Use **real** data (not hardcoded test fixtures)
- Handle **real** errors (not swallowed)
- Validate **real** contracts (not assumed)
- Cover **one** use case (the simplest happy path)

The slice must NOT:
- Include auth (unless auth IS the boundary)
- Include edge cases
- Include error handling beyond the boundary itself
- Include performance optimization
- Include UI polish

**The slice is done when:** You can demonstrate that data flows through the boundary in both directions for one real use case.

### Step 3: Expand Outward in Both Directions

Once the boundary works for one case, expand:

- **Expand upward** — add more callers, more entry points, more UI surfaces
- **Expand downward** — add more storage, more adapters, more backends
- **Expand sideways** — add error cases, edge cases, negative tests

Each expansion is validated against the working boundary. If an expansion breaks the boundary, fix the boundary before continuing.

**Expansion order (default):**
1. Happy path through boundary (Step 2)
2. Error path through boundary (what happens when it fails?)
3. Additional callers (more entry points to the same boundary)
4. Additional backends (more storage/service options)
5. Edge cases and negative tests
6. Performance, security, chaos hardening

### Step 4: Harden

Once the full surface is covered:
- Performance testing against the boundary
- Security review of the boundary contract
- Chaos testing (what happens when the boundary breaks?)
- Documentation of the boundary contract

## The Boundary Proof

**REQ-MOP-004**: Every plan MUST include a "boundary proof" step — a test or demonstration that data flows through the integration point correctly — before any expansion work begins.

The boundary proof is:
- A test that sends real data through the boundary and asserts the correct response
- A CLI command that exercises the full path end-to-end
- A curl/HTTP call that hits the real endpoint and returns real data

The boundary proof is NOT:
- A unit test of one side of the boundary in isolation
- A mock-based test that simulates the other side
- A dry-run that doesn't actually execute the path

**If the boundary proof fails:** Fix the boundary. Do not expand. Do not add features. The boundary is the foundation — everything else is built on it.

## Plan Template: Middle-Out Structure

Use this structure for any plan that crosses a boundary:

```markdown
## Phase 0: Boundary Identification

**Critical boundary**: [System A] ↔ [System B]
**Data flowing through**: [what data, what format, what direction]
**Contract**: [schema/protocol/format that governs the exchange]
**Failure mode**: [what happens when the boundary breaks]
**Effort estimate**: [total estimated effort for this work item, e.g., "2 days"] *(optional but enables REQ-MOP-006 20% rule; if omitted, 4-hour Phase 1 time-box applies)*

## Phase 1: Vertical Slice (Boundary Proof)

**Goal**: Prove data flows through [boundary] for one real use case.

### Step 1.1: Build the thinnest real path
- Implement [minimal component on side A]
- Implement [minimal component on side B]
- Wire them together at [boundary point]
- Skip: auth, error handling, edge cases, performance, styling

### Step 1.2: Boundary proof
- **Test**: [exact test or command that proves data flows through]
- **Expected**: [what you see when it works]
- **Evidence**: [where to capture the proof]

### Step 1.3: Acceptance gate
- [ ] Data flows from [A] to [B] for one real use case
- [ ] Data flows from [B] to [A] (if bidirectional)
- [ ] Real errors are surfaced (not swallowed)
- [ ] Boundary proof test is green and committed

**If this phase fails**: Fix the boundary. Do not proceed to Phase 2.

**If Phase 1 is blocked** (external dependency unavailable, contract undefined, or taking >20% of total estimated effort, or >4 hours if no estimate exists):
1. **Reduce scope** — narrow the slice to a smaller boundary
2. **Contract-test proxy** — accept a recorded-fixture contract test as temporary proof; add Phase 1b to replace with real proof before Phase 5
3. **Spike first** — reframe as a spike work item to define the contract; plan implementation separately
See REQ-MOP-006 in `specs/94-Middle-Out-Implementation-Planning.md`.

## Phase 2: Expand Upward (More Callers)

[Add more entry points, more UI surfaces, more callers to the working boundary]

## Phase 3: Expand Downward (More Backends)

[Add more storage options, more adapters, more service backends]

## Phase 4: Expand Sideways (Error Cases + Edge Cases)

[Add error handling, edge cases, negative tests]

## Phase 5: Harden

[Performance, security, chaos, documentation]
```

## Decision Tree: Where Is the Boundary?

```
Is there a new API endpoint?
  YES → Boundary: route registration → handler → DB write/read
  NO ↓

Is there a new agent capability?
  YES → Boundary: agent invocation → runtime executor → tool call
  NO ↓

Is there a new UI feature?
  YES → Boundary: UI component → API call → backend response
  NO ↓

Is there a cross-service integration?
  YES → Boundary: producer → message/event → consumer
  NO ↓

Is there a DB migration with app changes?
  YES → Boundary: migration → ORM model → API response
  NO ↓

Is there a CLI tool with a new execution path?
  YES → Boundary: CLI entrypoint → executor instantiation → side effect
  NO ↓

Single-component work with no boundary crossing?
  → Middle-out does not apply. Use baby-steps-methodology instead.
```

## Integration with Other Methodologies

Middle-out is designed to compose with the other Axiom methodologies:

### With Baby Steps (spec 09)

Each middle-out expansion is a baby step. The boundary proof is the first baby step. Every subsequent expansion is another baby step validated against the working boundary.

**Rule**: Baby steps apply within each middle-out phase. Middle-out determines the ORDER of phases (boundary first, then expand).

### With Working Backwards (skill)

Working backwards identifies **what** to build (from the user's experience backward). Middle-out identifies **where to start building** (at the integration boundary).

**Combined workflow**:
1. Working backwards: define the user's destination and the full path from user → presentation → logic → data → infrastructure
2. Middle-out: identify the riskiest boundary in that path and start there
3. Baby steps: implement the boundary proof in the smallest possible increment

### With TDD (skill)

TDD and middle-out are fully compatible. The boundary proof IS the first failing test.

**Combined workflow**:
1. Identify the boundary (middle-out Step 1)
2. Write a failing test for the boundary proof (TDD RED)
3. Implement the thinnest slice to make it pass (TDD GREEN)
4. Refactor while keeping the boundary proof green
5. Expand outward, writing a failing test for each expansion before implementing

### With Runtime Completeness Gate (skill)

Middle-out **prevents** the gaps that the runtime-completeness-gate **detects**.

| Gap Type | Middle-Out Prevention | Gate Detection |
|---|---|---|
| Nil executor | Boundary proof requires real executor instantiation | Detects nil executor at runtime |
| Unregistered route | Boundary proof requires real route registration | Detects 404 on real path |
| Stubbed adapter | Boundary proof requires real adapter, not stub | Detects mock-data trap |
| Wiring gap | Boundary proof requires data to flow through | Detects write-path/read-path mismatch |
| Verification theater | Boundary proof is a road test, not a bench test | Detects same-path write+read tests |

**Load order**: Load middle-out BEFORE planning. Load runtime-completeness-gate AFTER implementation to catch anything that slipped through.

## Anti-Patterns This Skill Prevents

### ❌ Top-Down Isolation
**Bad plan**:
```
Phase 1: Build the database layer
Phase 2: Build the API layer
Phase 3: Build the UI layer
Phase 4: Connect them
```
**Why it fails**: Phase 4 is where all the risk lives. By the time you get there, you've built three phases of work that may not wire together. The agent often runs out of context before Phase 4.

**Good plan**:
```
Phase 1: Vertical slice (UI → API → DB → response) — boundary proof
Phase 2: Expand UI (more components calling the working API)
Phase 3: Expand API (more endpoints on the working DB layer)
Phase 4: Expand DB (more tables, more queries)
```

### ❌ Bottom-Up Avoidance
**Bad plan**:
```
Phase 1: Build the data models (easy)
Phase 2: Build the utility functions (easy)
Phase 3: Build the helper classes (easy)
Phase 4: Build the integration (hard — deferred until context runs out)
```
**Why it fails**: The hard part is always last. The agent runs out of context, or the session ends, before the integration is proven.

**Good plan**:
```
Phase 1: Prove the integration works (hard — do it first)
Phase 2: Build the data models (now you know what the integration needs)
Phase 3: Build the utility functions (now you know what the models need)
Phase 4: Build the helper classes (now you know what the utilities need)
```

### ❌ Mock-Based Boundary Proof
**Bad boundary proof**: "I wrote a unit test that mocks the database and verifies the handler returns 200."

**Why it fails**: The mock is not the database. The boundary proof must use the real system.

**Good boundary proof**: "I wrote an integration test that inserts a row into the real database via the real API endpoint and verifies the row exists in the database."

### ❌ Deferred Wiring
**Bad**: "We'll wire it up in Phase 4."

**Why it fails**: Wiring is the boundary. If you defer the wiring, you've deferred the hardest part.

**Good**: "Phase 1 IS the wiring. Everything else is built on top of the working wire."

### ❌ Premature Hardening
**Bad**: "Step 1: Build the boundary. Step 2: Add performance tests. Step 3: Add security review. Step 4: Add chaos tests. Step 5: Expand to more callers."

**Why it fails**: Hardening a boundary that isn't fully expanded yet is wasted effort. Performance tests for a boundary that only handles one use case don't reflect production load. Security review of a half-built surface misses the attack vectors that appear when the surface is complete.

**Good**: "Phase 5 (Harden) comes after Phase 3 (Expand Sideways). The full surface is covered before hardening begins. Hardening validates the complete system, not a partial one."

## Checklist for Plan Authors

Before finalizing any plan that crosses a boundary:

- [ ] **Boundary identified** — the critical integration point is named explicitly
- [ ] **Phase 1 is the boundary proof** — the first deliverable is a working vertical slice, not a component in isolation
- [ ] **Boundary proof is real** — uses real data, real systems, not mocks or stubs
- [ ] **Expansion order is correct** — each phase expands from the working boundary
- [ ] **No "we'll wire it up later"** — wiring is Phase 1, not Phase N
- [ ] **Boundary proof test exists** — there is a specific test or command that proves data flows through
- [ ] **Failure mode is documented** — the plan says what happens if the boundary proof fails
- [ ] **Gate is explicit** — Phase 2 cannot start until Phase 1's boundary proof is green

## Checklist for Builders

Before claiming a phase complete:

- [ ] **Boundary proof is green** — the test/command that proves data flows through is passing
- [ ] **Evidence captured** — the boundary proof output is recorded (command + output)
- [ ] **Real data used** — for Owned boundaries: the proof uses real data, not hardcoded fixtures; for Partially-owned boundaries: a recorded fixture with provenance metadata is acceptable (see `specs/94-Middle-Out-Implementation-Planning.md#recorded-fixture-requirements-partially-owned-boundaries`)
- [ ] **Real errors handled** — the boundary surfaces errors, not swallows them
- [ ] **Expansion validated against boundary** — each new feature is tested against the working boundary, not in isolation

## Examples

### Example 1: New API Endpoint with DB

**Boundary**: HTTP route → handler → PostgreSQL

**Phase 1 (Vertical Slice)**:
```
Step 1.1: Register the route (real route, not mock)
Step 1.2: Implement the handler (real handler, not stub)
Step 1.3: Write to the real DB (real connection, not in-memory)
Step 1.4: Boundary proof: curl POST → verify row in DB
```

**Boundary proof command**:
```bash
# Insert via API
curl -X POST http://localhost:8080/api/v1/alerts \
  -H "Content-Type: application/json" \
  -d '{"message": "test alert", "severity": "high"}'

# Verify in DB
psql -c "SELECT * FROM alerts WHERE message = 'test alert'"
```

**Phase 2 (Expand Upward)**: Add more callers (dashboard UI, CLI tool)
**Phase 3 (Expand Downward)**: Add more storage options (archive table, S3 export)
**Phase 4 (Expand Sideways)**: Add error cases, validation, negative tests

---

### Example 2: New Agent Capability

**Boundary**: Agent invocation → runtime executor → tool call

**Phase 1 (Vertical Slice)**:
```
Step 1.1: Register the executor (real registration, not nil)
Step 1.2: Wire the agent to the executor (real wiring, not mock)
Step 1.3: Execute one real tool call through the boundary
Step 1.4: Boundary proof: invoke agent → verify tool was called with real args
```

**Boundary proof**:
```bash
# Run the agent with a real work item
axiom run --work-item "boundary-test" --repo . --in-process 2>&1 | grep "tool_call"
# Expected: at least one real tool call logged
```

**Phase 2 (Expand Upward)**: Add more agent capabilities using the working executor
**Phase 3 (Expand Downward)**: Add more tool adapters
**Phase 4 (Expand Sideways)**: Add error handling, retry logic, timeout handling

---

### Example 3: UI Feature with Backend

**Boundary**: React component → fetch() → API endpoint → DB

**Phase 1 (Vertical Slice)**:
```
Step 1.1: Create the API endpoint (real route, real handler)
Step 1.2: Create the minimal UI component (real fetch, not mock)
Step 1.3: Wire the component to the endpoint
Step 1.4: Boundary proof: open browser → click button → verify data appears
```

**Boundary proof**:
```
1. Start the dev server
2. Navigate to the feature page
3. Perform the primary action (click button, submit form)
4. Verify the expected data appears in the UI
5. Verify the data exists in the DB
```

**Phase 2 (Expand Upward)**: Add more UI surfaces (list view, detail view, export)
**Phase 3 (Expand Downward)**: Add more backend logic (validation, business rules)
**Phase 4 (Expand Sideways)**: Add error states, loading states, empty states

## Relationship to Specs

This skill implements the methodology contract defined in `specs/94-Middle-Out-Implementation-Planning.md`.

Requirements satisfied by this skill:
- **REQ-MOP-001**: Identify the critical integration boundary before writing any code ✓
- **REQ-MOP-002**: First deliverable is a working vertical slice through the boundary ✓
- **REQ-MOP-003**: Each subsequent phase expands from the working boundary ✓
- **REQ-MOP-004**: Plan includes a "boundary proof" step before expansion ✓
- **REQ-MOP-005**: If boundary proof fails, fix boundary before expanding ✓
- **REQ-MOP-006**: If Phase 1 cannot be completed within 20% of total estimated effort, escalate (reduce scope / contract-test proxy / spike first) ✓

## Cross-References

- `specs/94-Middle-Out-Implementation-Planning.md` — governing spec (this skill's contract)
- `specs/09-Baby-Steps-Methodology.md` — compatible: each expansion is a baby step
- `.opencode/skills/working-backwards-axiom/SKILL.md` — complementary: identifies WHAT to build; middle-out identifies WHERE to start
- `.opencode/skills/tdd-ai-axiom/SKILL.md` — compatible: boundary proof is the first failing test
- `.opencode/skills/runtime-completeness-gate-axiom/SKILL.md` — detection pair: middle-out PREVENTS gaps; gate DETECTS them

`axiom:trace work_item=middle-out-planning-01 spec=specs/94-Middle-Out-Implementation-Planning.md jira_ref=DEX-476`
