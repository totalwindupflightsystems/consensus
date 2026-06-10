---
name: working-backwards-axiom
description: >
  Plan from the end-user experience backward to implementation details. Every plan step
  includes not just "what to build" but "how to verify it's connected end-to-end." Prevents
  the common AI failure mode where components are built in isolation and never wired together.
  Companion to runtime-completeness-gate-axiom: this skill PREVENTS wiring gaps at planning
  time; the gate DETECTS them after implementation.
version: "1.0"
tags:
  vertical: [coding, planning]
  category: methodology
  core: false
---

# Working Backwards (Axiom)

> **"Start with the customer experience and work backward to the technology."** — Jeff Bezos
>
> **"A plan that says 'implement X' without saying 'verify X is reachable from the user's entry point' is a plan that builds disconnected components."**
>
> **"The fuel line problem is never a coding problem. It's a planning problem. Nobody planned the step that says 'connect the fuel line and verify fuel flows.'"**

## Purpose

This skill forces AI builders to plan from the **end-user experience backward** to implementation details. The goal is to produce plans so detailed that the model's hand is held through every connection point — it literally cannot build a disconnected component because the plan tells it to verify the connection at every step.

This skill is the **prevention** half of a prevention + detection pair:
- **Working Backwards** (this skill) → prevents wiring gaps at planning time
- **`runtime-completeness-gate-axiom`** → detects wiring gaps after implementation

Load this skill BEFORE planning. Load the runtime-completeness-gate AFTER implementation to catch anything that slipped through.

## When to Load This Skill

Load when:
- Creating a new work item's `meta-planning.md` or `plan.md`
- Planning any feature that has a user-visible surface (CLI, API, UI, worker output)
- Planning any feature that crosses subsystem boundaries
- Planning any feature with multiple write paths or read paths
- A previous implementation had wiring gaps caught by the runtime-completeness-gate
- You want the first pass of building something to be more complete

## Core Principles

### 1. Start with the User's Destination

Before writing any plan step, answer: **"What does the user see when this is done?"**

Not "what module exists" or "what test passes" — what does a human experience? Describe the exact CLI command they run, the exact page they visit, the exact API call they make, and the exact output they see.

### 2. Work Backward from Destination to Foundation

Plan in reverse order:
1. **User experience** → what the user sees/does
2. **Presentation layer** → what renders/formats the output
3. **Business logic** → what processes the request
4. **Data access** → what reads/writes the data
5. **Infrastructure** → what stores/transports the data

Then BUILD in forward order (foundation → user experience), but VERIFY in reverse order (user experience → foundation) at every step.

### 3. Every Plan Step Has Two Halves

Every step in a plan MUST include:
- **Build**: What to implement (the component, the function, the route, the UI element)
- **Verify Connection**: How to prove it's connected to the user's entry point

A step that only has "Build" is incomplete. A step that only has "Verify" is premature. Both halves are required.

### 4. The Walking Skeleton Comes First

The first phase of any plan MUST be a walking skeleton — the thinnest possible end-to-end slice that proves the architecture works:
- User entry point → API → data store → response → user sees result
- Hardcode values, skip edge cases, ignore auth if not core
- The skeleton must be deployable/runnable
- Every subsequent feature is an addition to a working system, not a risky integration

### 5. Acceptance Tests Before Implementation

For every plan step, write the acceptance test FIRST (Given/When/Then):
- **Given**: The system is in state X (walking skeleton is running, data exists)
- **When**: The user does Y (clicks button, runs command, calls API)
- **Then**: The user sees Z (specific output, specific behavior)

The acceptance test defines "done." Implementation makes the test pass.

### 6. Cross-Path Verification Is Mandatory

Tests that write data and read it back through the SAME path prove nothing about integration. Require:
- Insert via path A, read via path B
- Write via API, verify via UI
- Write via seeder, read via search
- Write via webhook, read via dashboard

### 7. No "Implement X" Without "Verify X Is Reachable"

Every plan step that says "implement [component]" MUST also say:
- "Verify: user can reach [component] from [entry point]"
- "Verify: [component] produces [expected behavior] visible to user"
- "Verify: [component] connects to [upstream] and [downstream]"

If you can't write the verification, you don't understand the component well enough to plan it.

## The Working Backwards Plan Template

Use this template for every plan step in `plan.md`:

```markdown
### Step N.M: [Component/Feature Name]

**User story**: As a [user type], I [action] so that [outcome].

#### Build
- Implement [specific component/feature]
- [Technical details: files to create/modify, functions to write, routes to register]
- [Data model changes if any]

#### Verify Connection
- **User entry point**: [exact CLI command / URL / UI click path]
- **Expected behavior**: [exact output the user sees — not "returns 200" but "shows list of 3 items with titles and dates"]
- **Connection points**: [list every other component this connects to, with direction: upstream → this → downstream]
- **Integration test**: [specific test that writes via one path and reads via another]
- **Road test**: [manual or automated walkthrough from user entry point to expected behavior]

#### Acceptance Criteria
- [ ] Component implemented and compiles/passes lint
- [ ] Reachable from user entry point (road test passes)
- [ ] Produces expected user-visible behavior (not just status code)
- [ ] Integration test passes (cross-path: write via A, read via B)
- [ ] Connected to upstream: [component] receives data from [source]
- [ ] Connected to downstream: [component] sends data to [destination]

#### What Could Go Wrong (pre-mortem)
- [Wiring gap risk]: [component] might not be registered/routed — verify by [method]
- [Data gap risk]: [field] might be null because [write path] doesn't populate it — verify by [query]
- [Theater risk]: Test might pass but user can't actually [do the thing] — verify by [road test]
```

## The Walking Skeleton Phase Template

The first phase of every plan MUST follow this template:

```markdown
## Phase 0: Walking Skeleton

**Goal**: Prove the end-to-end path works with the thinnest possible slice.

### Step 0.1: Define the User's Primary Task
- **User**: [who]
- **Task**: [what they want to accomplish]
- **Entry point**: [CLI command / URL / API call]
- **Expected output**: [what they see when it works]

### Step 0.2: Build the Thinnest End-to-End Slice
- [Minimal implementation that connects entry point to output]
- Hardcode values where needed — the goal is proving the path, not the logic
- Skip: auth, error handling, edge cases, performance, styling
- Include: real data store (not in-memory mock), real entry point (not test harness)

### Step 0.3: Verify the Skeleton Works
- Run the exact command/URL from Step 0.1
- Verify the exact output from Step 0.1 appears
- If it doesn't work: fix the skeleton before adding any features
- Record evidence: command run, output captured, screenshot if UI

### Step 0.4: Acceptance Test for the Skeleton
- Write a Given/When/Then test that exercises the full path
- This test MUST remain green for the entire project — it's the integration canary
```

## How This Integrates with Runtime-Completeness-Gate

| Phase | This Skill (Prevention) | Runtime-Completeness-Gate (Detection) |
|-------|------------------------|--------------------------------------|
| Planning | Forces every step to include "verify connection" | Not loaded yet |
| Implementation | Builder follows the plan's verification instructions | Not loaded yet |
| Verification | Plan's acceptance criteria are checked | Loaded to catch anything the plan missed |
| Post-mortem | If gate finds gaps, update the plan template to prevent recurrence | Reports gaps for the next iteration |

The runtime-completeness-gate's patterns map directly to this skill's prevention:

| Gate Pattern | Prevention in Plan |
|---|---|
| Nil executor | Plan step says "verify [executor] is registered and non-nil" |
| Unregistered route | Plan step says "verify [route] is reachable from [entry point]" |
| Stubbed adapter | Plan step says "verify [adapter] uses real implementation, not stub" |
| Demo-only path | Plan step says "verify [path] works with real data, not hardcoded" |
| Mock-data trap | Plan step says "verify with data from a different write path" |
| Wiring gap | Plan step says "verify [write path] populates [field] that [read path] depends on" |
| Verification theater | Plan step says "road test from user entry point, not just unit test" |

## Anti-Patterns This Skill Prevents

### 1. Bottom-Up Component Building
**Bad**: "Step 1: Build the database layer. Step 2: Build the API layer. Step 3: Build the UI layer. Step 4: Connect them."
**Good**: "Step 0: Walking skeleton (UI → API → DB → response). Step 1: Add [feature] to the working skeleton. Verify: user can [do thing] from [entry point]."

### 2. Plan Steps Without Connection Verification
**Bad**: "Implement the search API endpoint."
**Good**: "Implement the search API endpoint. Verify: user can type a query in the search box, hit enter, and see results. Integration test: seed data via admin API, search via user API, verify results match."

### 3. Verification Theater in Plans
**Bad**: "Write unit tests for the search function."
**Good**: "Write acceptance test: Given 10 documents are indexed, When user searches for 'quarterly report', Then results include the Q3 report with title, date, and snippet. Road test: open browser, type query, verify results."

### 4. The Fuel Line Problem
**Bad**: "Step 1: Build the engine. Step 2: Build the fuel tank. Step 3: Test the engine. Step 4: Test the fuel tank."
**Good**: "Step 0: Connect engine to fuel tank with a fuel line. Verify: engine starts and idles using fuel from the tank. Step 1: Improve the engine. Verify: still starts from the tank. Step 2: Improve the tank. Verify: engine still idles."

### 5. "Implement X" Without "Reachable From Y"
**Bad**: "Add the /api/v1/alerts endpoint."
**Good**: "Add the /api/v1/alerts endpoint. Verify: `curl http://localhost:8080/api/v1/alerts` returns the alert list. Verify: the dashboard's alert panel fetches from this endpoint and displays alerts. Verify: creating an alert via POST shows up in the GET response within 1 second."

## Checklist for Plan Authors

Before finalizing any plan, verify:

- [ ] **Walking skeleton is Phase 0** — the thinnest end-to-end slice is built first
- [ ] **Every step has Build + Verify Connection** — no step is build-only
- [ ] **User entry point is specified** for every step that touches user-visible surfaces
- [ ] **Expected behavior is concrete** — not "returns 200" but "shows list of items with titles"
- [ ] **Cross-path tests are planned** — write via A, read via B
- [ ] **Road tests are planned** — user entry point → primary task → useful output
- [ ] **Connection points are listed** — every component names its upstream and downstream
- [ ] **Pre-mortem is done** — each step lists what could go wrong (wiring gap, data gap, theater)
- [ ] **Acceptance tests are defined BEFORE implementation steps**
- [ ] **The plan can be followed by someone who has never seen the codebase** — if it's ambiguous, it's incomplete

## Integration with Work Item Artifacts

When creating work-item plans (`.memory-bank/work-items/<ID>/plan.md`), this skill adds these requirements on top of the existing `_prompt.md` rules:

1. **Phase 0 is mandatory** — every plan starts with a walking skeleton phase
2. **Every task uses the Working Backwards Step Template** (Build + Verify Connection + AC + Pre-mortem)
3. **The AC → Verification Mapping table** must include "User Entry Point" and "Expected Behavior" columns
4. **Cross-path integration tests** must be listed in the verification mapping
5. **Road tests** must be listed in the verification mapping (not just unit/component tests)

axiom:trace spec=specs/48-Test-Quality-Gates.md,specs/27-Evidence-Bundle-Schema.md skill=runtime-completeness-gate-axiom
