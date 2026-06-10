---
name: runtime-completeness-gate-axiom
description: >
  Prevent "looks built" from being confused with "actually works". Forces the last-mile
  runtime wiring, operator-path proof, and closure checks that catch nil executors,
  unregistered routes, stubbed adapters, demo-only completion claims, cross-path
  wiring gaps where subsystems work independently but fail when combined, and
  verification theater where tests prove component shape but not system behavior.
version: "1.5"
tags:
  vertical: [coding]
  category: testing
  core: false
---

# Runtime Completeness Gate (Axiom)

> **"A feature is not complete when the pieces exist. It is complete when a human can drive the real path and it works."**
>
> **"The missing je ne sais quoi is usually not more code. It is the final wiring, the real operator path, and the proof that the system behaves as claimed."**
>
> **"The fix is always trivial. One line, one field, one config change. The catastrophe is that nobody looked."**
>
> **"The most dangerous sentence an AI agent can produce is: 'All tests pass.' It means the engine runs on the bench. It does not mean the car drives."**

Use this skill when an app, CLI, workflow, admin surface, or major feature appears "mostly done" but you need to prove it is actually runnable and complete.

This skill is portable across projects. It is not Morty-specific; Morty is only one example of the completion error pattern.

This skill covers **three related but distinct failure classes**:

1. **Runtime incompleteness** — the operator-visible path is not wired, not reachable, or not stateful (nil executor, unregistered route, demo path, mock-data trap).
2. **Wiring gaps** — subsystems work independently but fail when combined because the connecting tissue between write paths and read paths was never built.
3. **Verification theater** — tests pass but prove component shape, not system behavior; the agent reports "done" while the user cannot accomplish the primary task.

All three share the same root cause: AI builders optimize for local success (passing tests, satisfying plan checkboxes, implementing individual components) while missing the **joins** that make the whole system work.

**Load trigger decision guide** — which sections to apply:

| Feature type | Apply Patterns 1–5? | Apply Patterns 6–17 + Step 2b + Section E? |
|---|---|---|
| CLI / server / API / worker | ✅ Yes | Only if it also writes to multiple tables or has multiple insertion paths |
| UI / frontend / dashboard | ✅ Yes | ✅ Yes — always check navigation (10–11), API-to-UI (12), action feedback (14), environment (15) |
| Feature with 2+ insertion paths | Optional | ✅ Yes |
| Feature reading from 2+ tables via JOIN or FK | Optional | ✅ Yes |
| Feature passing data across subsystem boundaries | Optional | ✅ Yes — check content fidelity (13) |
| Any feature nearing "done" with only unit/component tests | ✅ Yes | ✅ Yes — check verification theater (16–17) |
| Simple CRUD with one write path and one read path | ✅ Yes | ❌ Skip |
| Any feature where "no data" is observed but data is expected | ✅ Yes | ✅ Yes |

When in doubt, apply both. The cost of a false positive (running an extra check) is lower than the cost of a false negative (missing a silent wiring gap).

**Persistence scope:** The wiring gap patterns (6–9) and the Data Path Matrix (Step 2b) are written for **relational/SQL systems** (tables, FKs, JOINs). For non-relational systems, see the [Non-Relational Adaptations](#non-relational-adaptations) section below.

## Definitions

- **write-path**: Any code path that creates or updates persistent state. Includes: primary pipelines, seeders, importers, admin endpoints, migrations, test fixtures, background jobs, webhooks, event consumers. Excludes: read-only queries, temporary in-memory state.
- **read-path**: Any code path that queries persistent state to return data to a user or downstream system. Includes: API endpoints, UI views, dashboards, reports, export functions.
- **insertion path**: A write-path that creates new records. Used interchangeably with write-path in this skill.
- **wiring gap**: A specific data-flow failure where write-paths and read-paths are mismatched — write-path A does not populate a field that read-path B depends on.
- **verification theater**: Tests that pass but prove component shape rather than system behavior. Bench tests, not road tests.
- **bench test**: A test that verifies a component in isolation (unit test, mock-based integration test, same-path write+read test). Necessary but not sufficient.
- **road test**: End-to-end verification through the actual user interface with real data, starting from the user's entry point. Required before claiming "done."
- **data-flow spec**: A spec that governs features with 2+ write-paths, 2+ tables, or reads via JOIN/FK from tables written by different paths.

## The Handbuilt Car vs The Production Car

This analogy is the fastest way to understand why AI-built systems fail in this specific way.

**A handbuilt car** is made by someone who spent years watching and learning. When the instructions are incomplete — and they always are — they fill in the gaps from experience. They know the fuel line connects to the intake, that the timing belt needs tension, that the brake fluid reservoir feeds the master cylinder. They don't need a step that says "connect the fuel line" because they'd never leave it disconnected.

**A production car** has documents and steps so detailed that people with a high school education can assemble it in a factory. Two experts cannot tell the handbuilt car from the production car. The difference is not quality — it's that the production car's completeness comes from the *process*, not from the builder's intuition.

**An AI-built system is neither.** It has the confidence of the handbuilt car ("I know what I'm doing") but not the experience. It has the documentation ambitions of the production car but not the completeness. The result is a car where:

- The engine is built and the tests say it runs
- The fuel tank is built and the tests say it holds fuel
- But nobody connected the fuel line between them
- When asked "does it start?", you carry a cup of fuel from the tank, pour it into the intake, and crank the engine — and even then it starts but can't idle

**This is what AI-built software looks like to a human user.** Every component passes its tests. The system "works" in the sense that each piece does what its unit test says. But the human sits down, clicks a button, and nothing happens — because the button was never connected to the API, or the API was never connected to the database field the UI needs, or the route was never linked from the navigation.

## Why AI Verification Is Theater

AI agents write tests. The tests pass. The agent reports "all tests pass." But the tests are often **verification theater** — they prove the shape of the code, not the behavior of the system.

### What AI tests actually verify

- "Does this function return the right type?" ✅
- "Does this handler return 200 when given valid input?" ✅
- "Does this query return results when the database has matching rows?" ✅
- "Does this component render without crashing?" ✅

### What AI tests almost never verify

- "If I insert data through Path A, can I read it through Path B?" ❌
- "If I click this button, does the user see a result?" ❌
- "If the API returns an error, does the UI show a message?" ❌
- "Can a user who has never seen this system accomplish the primary task?" ❌
- "Does the search return *useful* results, or just *any* results?" ❌
- "Is the content that reaches the LLM actually sufficient to answer the question?" ❌

The first list is **component verification** — does each piece work in isolation?
The second list is **integration verification** — does the whole system work when a human uses it?

AI agents are excellent at the first list and almost completely blind to the second. This is not a bug in any specific agent — it is a structural limitation of how AI agents approach verification. They test what they built, not what the user experiences.

### The "it works on my bench" problem

A mechanic can test an engine on a bench. It starts, it revs, it idles. All gauges read normal. The mechanic says "engine works." But the engine is not in a car. It's not connected to a transmission, a fuel system, a cooling system, an exhaust. The bench test proves the engine works. It does not prove the car drives.

AI tests are bench tests. They prove the component works. They do not prove the system works. And the agent reports "all tests pass" with the same confidence as if it had driven the car around the block.

**The most dangerous sentence an AI agent can produce is: "All tests pass."** It means the engine runs on the bench. It does not mean the car drives.

### What "actually verify" means

Real verification means:

1. **Start from the user's entry point** (the browser, the CLI, the API call a human would make)
2. **Perform the primary task** (search for something, view a document, ask a question)
3. **Check the actual output** (not "did it return 200?" but "did it return *useful content*?")
4. **Check the error paths** (not "does the error handler exist?" but "does the user *see* the error?")
5. **Check with real data** (not test fixtures designed to pass, but data that exercises edge cases)
6. **Check from a different path than you built** (if you built the write path, test the read path with data from a *different* write path)

If you skip any of these, you are doing verification theater. You are testing the engine on the bench and reporting that the car drives.

## The Core Gap This Skill Covers

AI systems often ship **code-complete but runtime-incomplete** work. Common symptoms:

- CLI accepts the right flags, but the real execution path still calls a `nil` dependency
- tests exist for endpoints, but the routes are not actually registered
- admin/server/UI slices are "done" in isolation, but the first real user path still 404s
- dry-run works, but actual run fails immediately
- docs, plans, and traces say "complete", but a fresh operator cannot make the thing work
- all tests pass, but a human clicking through the UI cannot accomplish the primary task

This is not just a bug class. It is a **completion error**.

## When to Load This Skill

Load this skill when:
- a work item is nearing completion
- a system has many green tests but no trusted end-to-end proof
- a CLI/server/app has multiple partially integrated subsystems
- the repo has evidence of "candidate technical proof" without accepted runtime proof
- you suspect the implementation is demo-shaped rather than operator-shaped
- a human says some version of: "does it actually work?"
- an agent reports "all tests pass" and you want to know if that means anything

Especially load it for:
- CLIs and orchestration tools
- apps with background jobs or adapters
- admin panels / dashboards
- systems with stubs, interfaces, executors, registries, or plugin wiring
- any feature where the agent has only run unit/component tests

## The Missing "Je Ne Sais Quoi"

What makes software feel complete is usually one or more of these:

1. **The real entrypoint is wired**
2. **The first operator path succeeds on a fresh run**
3. **Every claimed surface is reachable from that entrypoint**
4. **The system returns useful behavior, not placeholder behavior**
5. **A real runtime proof exists, not just component proof**
6. **A human can accomplish the primary task without help**

If any of those are missing, the software is often only **implementation-shaped**.

## Core Rules

1. **Entry-point first.** Start from the command, route, button, or workflow a human actually uses.
2. **Trace the whole path.** Follow control flow from entrypoint to the final side effect.
3. **Hunt for silent incompleteness.** Look for nil dependencies, unregistered routes, stubs, TODO placeholders, default fallbacks, no-op implementations, and fake-success paths.
4. **Fresh-operator proof beats internal confidence.** Prefer a new directory / clean config / real invocation over internal assumptions.
5. **Do not trust dry-run alone.** Dry-run proves shape; runtime proves substance.
6. **Do not trust tests alone.** Tests prove slices; operator path proves product.
7. **Do not trust "all tests pass."** Tests are bench tests. Road tests are required.
8. **Completion claims must be downgraded if the real path is broken.**

## The Runtime Completeness Checklist

Before calling a surface complete, answer all of these:

### A. Entry and Wiring
- What exact entrypoint does the human use?
- What concrete runtime object executes the work?
- Is every required dependency actually instantiated and wired?
- Are there any `nil`, `TODO`, placeholder, or "future wiring" seams on the hot path?

### B. Surface Reachability
- Are all claimed routes/commands actually registered?
- Does the entrypoint reach the real implementation rather than an inert stub?
- Do success and failure paths both return meaningful operator-visible behavior?

### C. Fresh-Run Reality
- Can a fresh operator run it in a clean directory or sandbox?
- Are config, prompts, context files, and relative paths resolved correctly?
- Does the first real invocation do useful work rather than fail immediately?

### D. Proof Quality
- Is there at least one Tier-3+ runtime proof for the claimed surface?
- Is there at least one negative-path proof for expected failure handling?
- Is the proof transcript-owned and not just summarized?
- Is the proof a road test (user entry point → primary task → useful output), not just a bench test?

### E. Wiring Gap Check (for multi-table, multi-path, UI, or multi-boundary features)
- Has a Data Path Matrix been built (or explicitly scoped/sampled) for every table the feature touches?
- Does every write-path (primary pipeline, seeders, importers, admin endpoints, migrations, test fixtures, background jobs, event consumers, webhooks) populate every field that every read-path depends on?
- Have async/event-driven insertion paths been enumerated and included in the matrix? (These are the most commonly missed.)
- Are there cross-path integration tests — not just "insert via pipeline, read via API" but also "insert via seeder, read via API" and "insert via admin tool, read via dashboard"?
- Is there at least one end-to-end test that exercises a non-primary insertion path and asserts the read-path returns correct data?
- If a page shows "no data": was a direct DB query run to confirm data exists before attributing to data absence? If data exists, was a wiring gap investigation opened?
- For nullable FK columns: is there a justification for nullability, and are all insertion paths that must set it confirmed to do so?
- What is the confidence level in path enumeration (HIGH / MEDIUM / LOW)? If MEDIUM or LOW, are the unknown paths documented as a risk?

**UI/Navigation checks** (for UI/frontend features):
- For every route in the router: is there at least one navigation element (link, button, menu item) that points to it?
- For every list or table: are the rows clickable when a detail view exists for those items?
- For every user-facing API endpoint: is there a UI component that calls it?

**Content fidelity checks** (for features with multi-subsystem data handoffs):
- At every data boundary (DB → API → UI, DB → API → LLM, API → export): is the full content passed, or just a summary/heading/snippet?
- Does the downstream consumer need the full content? If yes, is truncation explicitly tested?
- Are the results *useful* to a human, or just *present* for a test?

**Action feedback checks** (for UI features with API-calling buttons):
- For every button that calls an API: does the user see a success confirmation when it works?
- For every button that calls an API: does the user see an error message when it fails? Or is the error swallowed silently?

**Environment awareness checks** (for features with external service dependencies):
- For every action button: does it depend on a service that may be unavailable (devserver stubs, missing provider config)?
- If yes: is the button disabled, hidden, or does it show a tooltip when the dependency is unavailable?

**Verification theater checks**:
- Do the integration tests write through one path and read through a *different* path? Or do they use the same code path for both?
- For every API call in tests: does the test check the *content* of the response, or just the status code?
- Has someone performed the primary task through the actual UI with real data (road test)?

**Automated code-quality pre-check** (load `code-analysis-axiom` for details):
- Before claiming runtime completeness, run `axiom analyze --score` to surface dead code (vulture/deadcode), high-complexity functions (radon/gocyclo), and lint issues (ruff/biome) that tests may be silently masking.
- A low health score or dead-code findings are a signal that the "road test" path may be exercising less code than assumed. Treat `analyze` output as a fast pre-flight, not a substitute for runtime proof.

## Completion Error Patterns to Catch

### Pattern 1: The Nil Executor
The interfaces exist, the engine exists, the CLI exists, but the real runtime path never instantiates the executor.

Smell:
- runtime fails immediately on the first real action
- tests pass because they use mocks directly

### Pattern 2: The Unregistered Route
Handlers exist and tests reference endpoints, but the route table does not actually expose them.

Smell:
- route-specific tests 404
- component/handler code looks complete in isolation

### Pattern 3: The Demo Path
Dry-run, preview, mock server, or screenshot flow works, but the actual command/request path does not.

Smell:
- "looks done" in docs and demos
- first real invocation fails

### Pattern 4: The Candidate Evidence Trap
There is a pile of strong-looking candidate evidence, but no accepted runtime proof on the real path.

Smell:
- many runs marked candidate-only
- repeated process debt around the same supposedly-complete slice

### Pattern 5: The Mock-Data Trap
The system looks complete because tests, screenshots, or demos run against mocks, stubs, fixtures, or sample data, but the real backend or real side effect has never been proven.

Smell:
- `httptest` / mock server proof is treated as equivalent to live runtime proof
- UI is shown with sample data, but the real fetch path is unproven
- workflows are validated with fabricated state instead of a real operator path
- the only passing evidence uses fixtures, fake responses, or dry-run output

Rule:
- Runtime completeness proof MUST include at least one execution against a real backend, real adapter, or real side effect.
- Mock-only, stub-only, `httptest`-only, fixture-only, screenshot-only, and dry-run-only evidence are non-closing evidence classes.

---

## The Wiring Gap: A Second Class of Completion Failure

> **"Subsystem A works. Subsystem B works. A→B is broken."**

The five patterns above catch *runtime incompleteness* — the operator path is not wired or not reachable. This section covers a related but distinct failure: **the wiring gap**, where every subsystem passes its own tests but the connecting tissue between write paths and read paths was never built.

### Why It's Catastrophic

- **Silent failure.** No crash, no error log, no red banner. The user sees "no results" and blames the data, not the code.
- **Each piece passes its tests.** The write path test confirms data is stored. The read path test confirms queries work when data exists. Neither test checks that the write path stores what the read path needs.
- **The fix is trivial.** Usually one line: set a field, add a join, populate a config value. The cost is not in fixing — it's in the hours/days of "why is this empty?" before someone traces the actual data flow.

### Pattern 6: The Foreign Key Gap
Two tables. Table B has a nullable foreign key to Table A. The primary insertion pipeline sets it. A secondary path (seeder, import tool, migration, admin endpoint) doesn't. Queries that JOIN on the FK return nothing.

**Detection**: For every nullable FK column, ask: "What are ALL the code paths that INSERT into this table? Do all of them set this FK?"

### Pattern 7: The Secondary Table Gap
Feature X requires rows in both Table A and Table B. The primary pipeline populates both. A secondary path only populates Table A. The UI tab that reads Table B shows empty.

**Detection**: For every feature that reads from multiple tables, ask: "What are ALL the code paths that create the initial data? Do all of them populate ALL the tables this feature reads from?"

### Pattern 8: The Config/Credential Divergence
A process needs Config Set A for Subsystem 1 and Config Set B for Subsystem 2. Both are loaded from environment variables. Config Set A's values override Config Set B's because the runtime's priority chain picks the wrong one.

**Detection**: "If this process needs two different credential/config sets simultaneously, which one wins when both are present? Does the SDK/framework's priority order match our intent?"

### Pattern 9: The Aggregation Disconnect
An overview page shows "151 documents" (counts from Table A). A detail page shows "0 documents" (counts from Table B via FK). Both queries are correct. They just query different tables/paths.

**Detection**: "If the same number is shown in two places, do both places derive it from the same source? If not, what keeps them in sync?"

### Pattern 10: The Unreachable Route
A route is registered in the router (`/explore/*`). A component exists for it. But no navigation element (sidebar link, button, menu item) points to it. The feature is built but invisible to users who don't know the URL.

**Smell**:
- feature is "done" in the plan but nobody can find it in the UI
- only accessible by typing the URL directly

**Detection**: For every route in the router, ask: "Can a user reach this route by clicking through the UI starting from the home page?" Enumerate every `<Route>` or route registration, then for each one grep for a `<Link>`, `<a>`, or `navigate()` that points to it. Any route with no inbound navigation element is unreachable.

### Pattern 11: The Unlinked List Item
A list or table shows data (file names, repo names, user names). Each row has all the information needed to navigate to a detail view. But the rows are plain text — not links, not buttons, not clickable. The detail view exists. The list exists. The click handler was never added.

**Smell**:
- list renders correctly with real data
- clicking a row does nothing
- detail view exists at a known route

**Detection**: For every list or table in the UI, ask: "Can the user click a row to see more detail? If the detail view exists and the list has the ID/path needed to navigate there, why isn't the row a link?" Check if list items are `<tr>`, `<div>`, or `<span>` when they should be `<a>`, `<button>`, or wrapped in a `<Link>`.

### Pattern 12: The API Without a UI Surface
An API endpoint works perfectly — returns correct data, handles errors, has tests. But no UI component calls it. The feature is backend-complete but has no frontend. Users can only access it via curl or a developer tool.

**Smell**:
- backend tests all pass
- no user-visible surface for the feature
- "done" in the plan but invisible to users

**Detection**: For every API endpoint, ask: "What UI component calls this endpoint?" Map every `fetch()` / `axios` / API call in the frontend to the backend routes it hits. Any backend route with no frontend caller is an API-without-UI gap.

### Pattern 13: The Content Fidelity Gap
Data flows through the full pipeline — write, store, retrieve, display. But somewhere in the middle, the content is degraded. Maybe the search returns the right document but surfaces the least informative sections. Maybe the snippet shows a heading instead of the body. Maybe the LLM receives thin context and produces a weak answer. No error occurs — the pipeline "works" — but the quality is silently destroyed.

This is the subtlest gap because the system *does* return results. The tests pass because they check "did we get results?" not "are the results *good*?" An AI agent will report "search works — returns 5 results from the correct file" without noticing that all 5 results contain only a repeated heading with no useful content.

**Smell**:
- pipeline runs without errors
- output is degraded, incomplete, or low-quality
- tests check presence ("got results") not quality ("results are useful")
- no test checks the actual content at the downstream boundary

**Detection**: Don't just check "does it return results?" Check: "Are the results *useful*? Would a human reading this output get what they need? Is the content that reaches the downstream consumer (LLM, UI, export) actually sufficient for its purpose?" Trace the actual bytes from storage → API response → consumer input and check for truncation or quality degradation at each boundary.

### Pattern 14: The Silent Action Failure
A button triggers an API call. The API returns an error (503, 500, 400). But the UI swallows the error — no toast, no banner, no inline message. The dialog closes, the page looks the same, and the user has no idea the action failed. The only evidence is a console error or a network tab entry.

**Smell**:
- action button appears to work (dialog closes, page unchanged)
- user repeats the action, confused why nothing happened
- error only visible in browser devtools network tab or console

**Detection**: For every button or action in the UI that calls an API, ask: "What happens when the API returns an error? Does the user see a message? Or does the UI silently close the dialog and pretend nothing happened?" Click every action button with the network tab open and check for swallowed errors.

### Pattern 15: The Devserver-Only Happy Path
The system works in devserver mode because the devserver stubs out dependencies (SQS, S3, GitHub webhooks). But actions that depend on those real services fail silently or return 503 in devserver mode. The UI has buttons for these actions but no indication that they're unavailable in the current environment.

**Smell**:
- feature works in local dev but silently fails in staging or production
- button is clickable but the action has no effect
- no tooltip, disabled state, or environment indicator

**Detection**: For every action button, ask: "Does this action depend on a service that is stubbed or unavailable in the current environment? If so, should the button be disabled, hidden, or show a tooltip explaining why it won't work?"

---

## Verification Theater: A Third Class of Completion Failure

> **"All tests pass" is a bench test result. It is not a road test result.**

Patterns 1–15 describe gaps in the system itself. This section covers a gap in the *verification* of the system — where the agent's own tests create false confidence that the system works.

### Pattern 16: The Assumed-Working API
An AI agent calls an API, gets a 200 response, and reports "API works." But it never checked the *content* of the response. The API returned 200 with empty arrays, or with truncated data, or with the wrong fields populated. The status code passed. The actual behavior failed.

**Smell**:
- test asserts `response.status == 200` and nothing else
- agent reports "endpoint works" based on status code alone
- actual response body contains empty arrays, null fields, or truncated content

**Detection**: For every API call in tests, don't just check the status code. Parse the response body. Check that the fields you need are present, non-empty, and contain the expected data. If you're testing search, check that the results are *relevant*, not just *present*.

### Pattern 17: The Test That Tests Itself
An AI agent writes a test that inserts data, then reads it back through the same code path. The test passes because the same code that wrote the data also reads it. But the real system writes data through Path A and reads it through Path B. The test never exercises the actual integration.

**Smell**:
- integration test writes and reads through the same service/function
- test passes even when the real read path is broken
- no test ever writes through a secondary path (seeder, importer) and reads through the primary API

**Detection**: For every integration test, ask: "Does this test use the same code path for write and read? If so, it's testing the code path, not the integration. Write through one path, read through another."

### The Kill Question (Wiring Gap Edition)

For the entire feature, ask:

> **"I see how data gets IN. I see how data gets OUT. Show me the exact line of code where every IN-path populates every field that every OUT-path depends on."**

If you cannot point to that line for every combination, you have a wiring gap. See **Step 11** in the Workflow section for scope bounds and abstraction layer guidance.

---

## How to Prevent Wiring Gaps in Specs

These rules prevent wiring gaps from being introduced in the first place. Apply them when writing specs and plans for any feature with multiple tables, insertion paths, UI surfaces, or data boundaries.

1. **Require a Data Path Matrix** for any feature that touches multiple tables or has multiple insertion paths. Make it a required section in the spec, not an afterthought.

2. **Ban nullable foreign keys without justification.** If a FK is nullable, the spec must explain why and must list which insertion paths are allowed to leave it NULL.

3. **Spec every insertion path as a first-class citizen.** If the system has a seeder, an import tool, or an admin create endpoint, its contract must include "populates the same fields as the primary pipeline."

4. **Require cross-path integration tests.** Not just "insert via pipeline, read via API" — also "insert via seeder, read via API" and "insert via admin tool, read via dashboard."

5. **Treat "empty state" as a failure signal during verification.** If a page shows "no data" and you know data exists, that is a wiring gap until proven otherwise.

6. **Require a Route→Navigation audit.** Every route in the router must have at least one navigation element (link, button, menu item) that points to it. If a route has no inbound link, it is unreachable and the feature is invisible.

7. **Require list items to be interactive when a detail view exists.** If a list shows items and a detail view exists for those items, the list rows MUST be clickable links. Plain-text lists of navigable items are a wiring gap.

8. **Require an API→UI mapping.** Every user-facing API endpoint must have at least one UI component that calls it. If an API has no frontend caller, the feature is backend-only and invisible to users.

9. **Require content quality checks, not just content presence checks.** When data passes from one subsystem to another, the spec must state whether the full content or a summary is expected. Tests must check that the content is *useful*, not just *present*.

10. **Require visible error feedback for every user action.** Every button that calls an API must show a success or error message to the user. Silent failures are wiring gaps — the action→feedback loop was never closed.

11. **Require environment-aware UI.** If an action depends on a service that may be unavailable (SQS in devserver mode, GitHub webhooks without a provider config), the button should be disabled or show a tooltip. Don't let users click buttons that will silently fail.

12. **Require road test verification, not just bench test verification.** Before any feature is marked done, someone (or some agent) must perform the primary task through the UI with real data. Unit tests are bench tests. Integration tests that use the same code path are bench tests. Only end-to-end verification through the actual user interface with real data is a road test.

13. **Distrust "all tests pass."** When an AI agent reports all tests pass, ask: "What do the tests actually verify? Do they test the integration, or just the components? Do they test with data from a different path? Do they check content quality, or just content presence?"

---

## Non-Relational Adaptations

Patterns 6–9 and the Data Path Matrix assume SQL/relational persistence. For other persistence paradigms, adapt as follows:

| Paradigm | Equivalent of "FK Gap" | Equivalent of "Secondary Table Gap" | Kill Question adaptation |
|---|---|---|---|
| **Document DB** (MongoDB, Firestore) | Reference field not set in secondary write path | Embedded sub-document not populated by secondary path | "Show me where every write-path sets the reference field or embeds the sub-document" |
| **Event Sourcing** | Event published without required fields; read model not updated | Projection handler not subscribed to event type | "Show me the event handler that updates the read model when this event fires" |
| **Message Queue** (Kafka, SQS, NATS) | Producer omits field; consumer cannot process | Consumer not subscribed to topic/queue written by secondary path | "Show me the consumer that reads from this topic and the field it depends on" |
| **CQRS** | Command handler does not populate field needed by query model | Query model not updated when command executes | "Show me the query model update triggered by this command" |
| **Cache** (Redis, Memcached) | Cache warm-up job not triggered after seeder/importer runs | Cache key not invalidated after secondary write path | "Show me where every write-path invalidates or warms the cache" |

For all paradigms, the Data Path Matrix concept still applies — replace "table" with "collection / stream / topic / projection" and "field" with "attribute / event field / message field."

---

## Portability

Use this skill for any project where a human or operator interacts with a real entrypoint:

- CLI tools
- web apps and dashboards
- APIs and admin panels
- workers, jobs, and schedulers
- plugin systems and adapters

The exact entrypoint changes by project, but the completion question stays the same:

**Can a fresh human drive the real path and get the claimed behavior?**

## Verification Tier Definitions

- **Tier 2 / component proof**: mocks, stubs, `httptest`, isolated handler tests, screenshot demos, dry-run-only output
- **Tier 3+ runtime proof**: the real operator path executes and produces the claimed side effect
- **Tier 4 live server proof**: the actual compiled binary/process is running and an external client reaches it over a real port

If your repo already defines verification tiers, use the repo contract first. In Axiom repos, see `specs/00-PRD.md#verification-signal-hierarchy`.

## Workflow: Use This Every Time

### Step 1: Name the Human Path

Write one line:

`Human path: <exact command / click path / request path>`

Examples:
- `morty run --work-item-hint demo --git-mode none config.yaml`
- `GET /api/logs?follow=true`
- `Open app -> login -> create first project -> publish`

### Step 2: Build the Wiring Map

For the human path, map:

`entrypoint -> parser/router -> runtime object -> adapter/executor -> side effect`

If any box in that chain is unknown, stubbed, or not instantiated, stop calling the feature complete.

### Step 2b: Build the Data Path Matrix (for features touching multiple tables or insertion paths)

For each table the feature touches, create a matrix:

| Field | Write Path 1 | Write Path 2 | Write Path 3 | Read Path 1 needs it? | Read Path 2 needs it? |
|-------|-------------|-------------|-------------|----------------------|----------------------|
| id | ✅ auto | ✅ auto | ✅ auto | ✅ | ✅ |
| name | ✅ | ✅ | ✅ | ✅ | ❌ |
| parent_id | ✅ | ❌ NULL | ❌ NULL | ✅ JOIN | ❌ |

Any cell that is "❌ NULL" in a Write column AND "✅" in a Read column is a **wiring gap**.

**When to build this matrix**: Any time the feature has more than one insertion path OR reads from more than one table.

**Scope threshold**: Build the full matrix if tables ≤ 3 AND paths ≤ 3. For larger features, sample: identify the 3 most critical tables (highest traffic or most FK dependencies) and verify all paths against those. Document which tables were excluded and why.

**Insertion-path enumeration checklist** — before declaring the matrix complete, confirm you have checked for all of these:
- [ ] Primary pipeline (the main application write path)
- [ ] Seeders / fixtures (dev/test data population)
- [ ] Importers / bulk-load tools
- [ ] Admin endpoints (admin UI create/update actions)
- [ ] Database migrations (schema changes that also backfill data)
- [ ] Background jobs / scheduled tasks that write to these tables
- [ ] Event consumers / message queue handlers
- [ ] Webhooks / external integrations that write inbound data
- [ ] Manual SQL scripts run by ops (if any)

**Confidence annotation**: After completing the matrix, record your confidence level:
- **HIGH**: Static analysis covered all paths; no dynamic/reflective code or external writers
- **MEDIUM**: Dynamic code, DI containers, or plugin systems may create hidden paths
- **LOW**: External systems or manual processes may write to these tables; matrix is incomplete

**Secondary table check**: List every table the read paths query. For each — does every write path create the necessary rows in this table, or only the primary pipeline?

**Config/credential isolation check**: If the system needs multiple config contexts simultaneously, which one wins when both are present? Does the SDK/framework's priority order match intent?

### Step 3: Run Real Behavior Before Reading More Code

Do the real invocation first.

Capture:
- command or request
- exit code / HTTP status
- output/body
- resulting files/state

### Step 4: Explain the Gap in One Sentence

Use this template:

`The system is code-complete but runtime-incomplete because <entrypoint> does not reach <claimed behavior>.`

### Step 5: Add a Runtime Completeness Gate

Add or require a checklist item that proves:
- the human path works end-to-end
- the primary side effect happens
- the failure mode is meaningful when the side effect cannot happen

### Step 6: Check UI Navigation Completeness (for UI/frontend features)

For every route registered in the router:
- Is there a link, button, or menu item in the navigation that points to it?
- Can a user reach it by clicking from the home page?

For every list or table in the UI:
- Are the rows clickable? Do they link to the detail view?
- Does the detail view exist? Does the list have the ID/path needed to navigate there?

For every API endpoint:
- What UI component calls it?
- If the answer is "none" or "a page that is unreachable", the feature is invisible to users.

### Step 7: Check Content Fidelity and Quality (for features with multi-subsystem data handoffs)

For every data handoff between subsystems (DB → API → UI, DB → API → LLM, API → export):
- Is the full content being passed, or just a summary/heading/snippet?
- Does the downstream consumer need the full content to do its job?
- Are the results *useful* to a human, or just *present* for a test?
- Trace actual bytes at each boundary and check for silent truncation or quality degradation.

If the downstream consumer is an LLM or a renderer that needs full body text, a heading-only response is a wiring gap even if the API returns HTTP 200.

### Step 8: Check Action Feedback Loops (for UI features with API-calling buttons)

For every button or action in the UI that calls an API:
- What happens when the API returns **success**? Does the user see a confirmation (toast, banner, state change)?
- What happens when the API returns an **error** (4xx/5xx)? Does the user see an error message?
- Click every action button with the network tab open. If the API returns an error and the UI shows nothing, that is a swallowed error — a wiring gap in the action→feedback loop.

### Step 9: Check Environment-Aware UI (for features with external service dependencies)

For every action button that depends on an external service (SQS, S3, GitHub webhooks, OAuth provider, etc.):
- Is that service available in the current environment (devserver, staging, prod)?
- If not, is the button **disabled**? Does it show a **tooltip**? Or does it silently fail?
- List every button → API → external dependency chain and check if the UI adapts when the dependency is unavailable.

### Step 10: Actually Use The System

This is the step AI agents skip. Before claiming "done":

1. **Start from the user's entry point** (home page, login screen, CLI help)
2. **Perform every primary task** the feature enables (search, view, create, edit, delete)
3. **Check the actual output** — not "did it return 200?" but "is this useful?"
4. **Try the error paths** — what happens when you search for something that doesn't exist? When you click a button that depends on an unavailable service?
5. **Check from a different data path** — if you seeded data through a tool, can the UI find it?

If you cannot complete the primary task by clicking through the UI, the feature is not done. No matter what the tests say.

### Step 11: The Kill Question

For the entire feature, ask:

> **"I see how data gets IN. I see how data gets OUT. Show me the exact line of code where every IN-path populates every field that every OUT-path depends on."**

If you cannot point to that line for every combination, you have a wiring gap.

> **"Can a user who has never seen this system sit down and accomplish the primary task without help?"**

If the answer is no, the feature is not done. It is a handbuilt car that only the builder can start.

**Scope bound**: Apply to the top 3 insertion paths by expected traffic volume (or all synchronous paths if traffic is unknown). Flag async/event-driven paths as requiring separate verification.

**Abstraction layer guidance**:
- **ORMs** (Django, SQLAlchemy, Prisma, ActiveRecord): the "line" is the model field assignment or constructor argument (e.g., `Document(parent_id=parent.id, ...)`). If the field is not in the constructor or is not explicitly set before `save()`, it is a gap.
- **Event sourcing**: trace from the event publication to the event handler to the read-model update. The "line" is the handler that populates the read model.
- **Generated code**: trace to the generation template or the input that drives generation.
- **If no single line exists**: document the abstraction layer and verify at that level. A gap at the abstraction layer is still a gap.

## Required Output Format When You Use This Skill

When diagnosing a suspected completeness gap, report:

```markdown
Human path:
- `<exact path>`

Claimed behavior:
- <what the plan/spec/docs say should work>

Observed behavior:
- <what actually happened>

Wiring gap (runtime):
- <the missing instantiation / route / adapter / side effect>

Wiring gap (data path):
- <the write path that does not populate a field the read path depends on>
  (omit if not applicable)

Wiring gap (navigation/UI):
- <the route with no inbound link, the list with unclickable rows, the API with no UI caller>
  (omit if not applicable)

Wiring gap (content fidelity):
- <the boundary where content is truncated or degraded and the downstream consumer receives a shell>
  (omit if not applicable)

Wiring gap (action feedback):
- <the button whose API error is swallowed with no user-visible message>
  (omit if not applicable)

Wiring gap (environment):
- <the button that silently fails because its backend dependency is unavailable in this environment>
  (omit if not applicable)

Wiring gap (verification theater):
- <the test that uses the same code path for write and read, or checks status code but not content>
  (omit if not applicable)

Why the AI missed it:
- <how local proofs and implementation slices hid the system-level gap>

What must be added to the plan:
- <runtime gate, sandbox proof, route registration check, data path matrix, cross-path integration test,
  navigation audit, content quality check, action feedback test, environment check, road test, etc.>
```

## Why AI Misses This

AI builders commonly miss runtime completeness because they optimize for visible local success:

- "I implemented the handler"
- "I added the flags"
- "The engine type exists"
- "The test for the helper passes"
- "The TODO step is satisfied by the code diff"
- "All tests pass"

But software completeness lives in the **joins**:

- the wiring between CLI and executor
- the registration between handler and router
- the path from config to real side effect
- the proof that a human can use it from the outside

AI builders also miss **wiring gaps** and **verification theater** because they:

- validate components, flags, and engine behavior separately but never prove the default CLI path with a fresh sandbox
- write tests for the primary insertion path but never test secondary paths (seeders, importers, admin tools) against the same read paths
- treat nullable FK columns as "fine" without checking which insertion paths leave them NULL
- see "no results" as "no data" rather than as a wiring gap signal
- optimize for passing the test suite rather than tracing data flow across subsystem boundaries
- register routes without adding navigation links — the feature is built but invisible
- render lists as plain text when the rows should be clickable links to detail views
- build backend endpoints without a frontend caller — the API works but no user can reach it
- pass summaries or headings through data boundaries when the downstream consumer needs full content
- implement API calls without error handlers — the action "works" in the happy path but swallows all failures silently
- build features that depend on external services without checking whether those services are available in the current environment
- write tests that check status codes but not response content — "200 OK" is not "useful data"
- write integration tests that write and read through the same code path — testing the path, not the integration
- never perform the primary task through the actual UI with real data — they test the engine on the bench and report the car drives

The result: AI-built systems are often **component-complete but integration-incomplete**. Every piece works. The whole doesn't. And the agent's own verification confirms "everything passes" because the tests are bench tests, not road tests.

The connecting tissue is:
- **Data connections**: FK set, secondary tables populated, config isolated
- **Navigation connections**: routes linked, lists clickable, detail views reachable
- **API-to-UI connections**: every endpoint has a caller, every feature has a surface
- **Content connections**: full, useful data passes through every boundary without silent truncation or quality degradation
- **Feedback connections**: every user action produces visible success or error feedback
- **Environment connections**: buttons are disabled or explained when their backend is unavailable
- **Verification connections**: tests prove the system works end-to-end, not just that the components work in isolation

That join — between write paths and read paths, between CLI and executor, between route and navigation, between API and UI, between data and its downstream consumer, between action and feedback, between button and its environment, between test and actual system behavior — is the thing to hunt.

## Do / Don't

### Do
- run the real operator path early
- use fresh subdirectories/sandboxes when possible
- treat unregistered routes and nil dependencies as completion blockers
- add explicit runtime gates to plans/TODOs
- downgrade "done" claims when real execution is broken
- perform the primary task through the actual UI before claiming done
- check response content, not just status codes
- write integration tests that use different paths for write and read

### Don't
- accept dry-run as proof of runtime functionality
- accept unit green as proof of product completeness
- accept mock data, sample data, or fixture-only proof as proof of real behavior
- confuse "implemented pieces" with "working software"
- bury runtime enablement after UI polish or secondary features
- trust "all tests pass" without asking what the tests actually verify
- accept same-path write+read tests as integration proof
- accept status-code-only assertions as content verification

## Recommended Pairings

Load this skill together with:
- `runtime-spec-conformance-loop` when you are already in fix mode
- `conformance-testing-loop` for matrix-based behavior verification
- `test-quality-gates-axiom` so runtime gates are not replaced by green theater
- `evidence-bundle-schema` so the completeness proof is durable
- `enterprise-testing-standard` for cross-path integration test tier requirements
- `chrome-devtools-mcp` for browser-based road test evidence capture

## Example: Morty Orchestrator

Example diagnosis:

```markdown
Human path:
- `morty run --work-item-hint demo --git-mode none config.yaml`

Claimed behavior:
- Morty executes a real plan -> execute -> verify loop against OpenCode.

Observed behavior:
- Dry-run prints the workflow, but real run fails on cycle 0 with `stage executor is nil`.

Wiring gap (runtime):
- `cmdRun` constructs the engine with a nil stage executor, so the runtime path never reaches actual agent execution.

Why the AI missed it:
- It validated components, flags, engine behavior, and mocks separately, but never proved the default CLI path with a fresh sandbox.

What must be added to the plan:
- A top-priority runtime enablement step, a real sandbox conformance run, and a completion gate that requires the operator path to execute at least one real stage.
```

## Exit Criteria

You are done using this skill when:
- the missing runtime join is identified precisely
- the plan/TODO now includes a runtime completeness gate
- there is a real operator-path proof or an explicit runtime blocker
- the primary task has been performed through the actual UI with real data (road test), or a road test blocker is explicitly documented

## Trace

`axiom:trace doc=.opencode/skills/runtime-completeness-gate-axiom/SKILL.md spec=specs/00-PRD.md#verification-signal-hierarchy`

## Changelog

| Version | Date | Change |
|---------|------|--------|
| 1.5 | 2026-03-31 | Added Handbuilt Car analogy + Verification Theater section ("Why AI Verification Is Theater"); Patterns 16–17 (Assumed-Working API, Test That Tests Itself); Step 10 (Actually Use The System); Kill Question expanded with second question ("Can a user accomplish the primary task?"); spec prevention rules 12–13 (road test, distrust "all tests pass"); expanded "Why AI Misses This" with verification theater dimension; added Verification connections to connecting tissue taxonomy; updated Checklist Section D and E with verification theater checks; updated Do/Don't; added chrome-devtools-mcp pairing; updated Exit Criteria; added bench test / road test / verification theater to Definitions. Source: updated portable anti-pattern document from external Axiom instance. |
| 1.4 | 2026-03-31 | Added Patterns 14–15 (Silent Action Failure, Devserver-Only Happy Path); Steps 8–9 in workflow (Action Feedback Loops, Environment-Aware UI); expanded Checklist Section E with action feedback and environment awareness checks; added spec prevention rules 10–11; expanded "Why AI Misses This" and connecting tissue taxonomy with feedback/environment dimensions; updated load trigger table and output format. Source: updated portable anti-pattern document from external Axiom instance. |
| 1.3 | 2026-03-31 | Added Patterns 10–13 (Unreachable Route, Unlinked List Item, API Without UI Surface, Content Truncation Gap); Steps 6–7 in workflow (UI Navigation Completeness, Content Fidelity); expanded Checklist Section E with UI/navigation and content fidelity checks; added 9-rule spec prevention section; expanded "Why AI Misses This" with navigation/UI/content dimensions; updated load trigger table for UI features; updated output format with navigation and content fidelity gap fields; updated meta-lesson. Source: updated portable anti-pattern document from external Axiom instance. |
| 1.2 | 2026-03-31 | Added load-trigger decision table; Definitions block (write-path, read-path, wiring-gap, data-flow spec); SQL scope statement + Non-Relational Adaptations section; insertion-path enumeration checklist + scope threshold + confidence annotation in Step 2b; ORM/abstraction guidance on Kill Question with scope bound; expanded Checklist Section E (async paths, confidence level, direct-DB verification for "no data"); updated Recommended Pairings. Source: @devils-advocate-axiom + @assumption-buster-axiom review. |
| 1.1 | 2026-03-31 | Added Wiring Gap failure class (Patterns 6–9), Data Path Matrix protocol (Step 2b), Checklist Section E, expanded "Why AI Misses This", updated output format with data-path wiring gap field, added `enterprise-testing-standard` pairing. Source: portable anti-pattern finding from external Axiom instance. |
| 1.0 | — | Initial release: runtime incompleteness patterns 1–5, operator-path proof workflow. |
