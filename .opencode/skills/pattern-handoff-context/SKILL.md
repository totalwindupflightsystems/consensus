---
name: pattern-handoff-context
description: >-
  Park current context in a stash, generate a portable reference, and pass it
  to the next agent or session. Uses stash_push + stash_ref + stash_peek.
  Pure stash-based handoff — no session spawn required. Runtime-tested 2026-05-18.
version: "1.0"
tags:
  vertical: [handoff, coordination, workflow]
  category: pattern
  core: false
spec: specs/121-Pattern-Generator.md
trigger_conditions:
  - "You are about to hand off work to another agent"
  - "You need to park context and resume later in a fresh session"
  - "Passing investigation results to a downstream agent"
  - "Ending a step and leaving breadcrumbs for the next step"
tools_required:
  - stash_push
  - stash_ref
  - stash_peek
  - stash_search (optional — verify handoff was picked up)
  - stash_enter (receiving agent uses this)
estimated_steps: 3
estimated_duration: "5-10 seconds"
lifecycle:
  state: active
  created: "2026-05-18"
  last_validated: "2026-05-18"
  validation_count: 1
---

# Pattern: Handoff Context

Park current working context into a stash, generate a portable reference, and
surface it so the next agent or session can pick up exactly where you left off.

**Spec**: `specs/121-Pattern-Generator.md`
**Observed from**: 1 real execution on 2026-05-18

<!-- axiom:trace work_item=pattern-design-01 spec=specs/121-Pattern-Generator.md -->

---

## Prerequisites

| Requirement | How to Verify | Expected | If Missing |
|-------------|---------------|----------|------------|
| Context Stash available | Call `stash_list` | returns `count` field | Plugin auto-initializes; check `.memory-bank/stash/` |

> No daemon, no binary, no tree-memory required. This pattern works in any session.

---

## Tool Chain

| Step | Purpose | Tool | Key Input | Key Output | On Failure | Criticality |
|------|---------|------|-----------|------------|------------|-------------|
| 1 | Park context | `stash_push` | `{name, summary, detail, tags}` | `{stash_id, state: "suspended"}` | Retry once | Required |
| 2 | Get portable reference | `stash_ref` | `{id: stash_id}` | `{stash_id, name, summary, tags}` | Use stash_id directly | Enriching |
| 3 | Verify handoff readable | `stash_peek` | `{id: stash_id}` | `{summary_preview, state}` | WARN; handoff still works | Optional |

**Receiving agent** (separate step, new session):

| Step | Purpose | Tool | Key Input | Key Output |
|------|---------|------|-----------|------------|
| R1 | Find the stash | `stash_search` | `{query: "keyword"}` | matching stash |
| R2 | Enter it | `stash_enter` | `{id: stash_id}` | active context |
| R3 | Read the context | `stash_log` | `{id}` | entries |

---

## Pseudocode

```text
PATTERN handoff_context(summary, detail, tags?, name?):

  // Step 1: Park context
  stash_name = name OR "handoff-{agent_name}-{date()}"
  stash = CALL stash_push(
    name: stash_name,
    summary: summary,         // 1-3 sentences: what this is about
    detail: detail,           // full preserved context (structured or prose)
    tags: tags OR "handoff"
  )
  stash_id = stash.stash_id
  IF stash.error:
    RETURN { status: "PATTERN_FAILED", reason: stash.error }

  // Step 2: Get portable reference (for embedding in messages/logs)
  ref = CALL stash_ref(id: stash_id)
  // ref contains: stash_id, name, summary, tags — safe to pass to any agent

  // Step 3: Verify readable (optional but good practice)
  peek = CALL stash_peek(id: stash_id)
  IF NOT peek.summary_preview:
    WARN "Stash created but not readable — check .memory-bank/stash/"

  RETURN {
    status: "PATTERN_COMPLETE",
    stash_id: stash_id,
    stash_name: stash_name,
    ref: ref,                 // pass this to the receiving agent
    pickup_hint: "Call stash_enter(id: '{stash_id}') to resume"
  }

// ─── RECEIVING AGENT (different session) ───
PATTERN receive_handoff(stash_id? OR search_keyword?):

  IF stash_id given:
    CALL stash_enter(id: stash_id)
  ELSE:
    results = CALL stash_search(query: search_keyword)
    IF results.total == 0:
      RETURN { status: "PATTERN_FAILED", reason: "No matching stash found" }
    CALL stash_enter(id: results.results[0].stash_id)

  // Context is now active — read entries and proceed
  log = CALL stash_log(id: stash_id)
  RETURN { status: "CONTEXT_LOADED", entries: log.entries }
```

---

## On-Track / Off-Track Signals

| Signal | Type | After Step | Indicator | Response |
|--------|------|-----------|-----------|----------|
| SIG-01 | on_track | 1 | `stash_push` returns `{stash_id, state: "suspended"}` | Continue |
| SIG-02 | off_track | 1 | `stash_push` returns error | Check `.memory-bank/stash/` exists; retry |
| SIG-03 | on_track | 2 | `stash_ref` returns `{stash_id, summary}` | ref is now portable |
| SIG-04 | on_track | 3 | `stash_peek` returns `{summary_preview}` | PATTERN_COMPLETE |
| SIG-05 | off_track | 3 | `stash_peek` returns empty | WARN; stash still exists (peek is optional) |
| SIG-R1 | on_track | R1 | `stash_search` returns ≥1 result | Enter the first match |
| SIG-R2 | off_track | R1 | `stash_search` returns 0 results | Try broader keyword; check stash was not popped already |

---

## Adjustment Protocol

```
tags type error: "tags.split is not a function"
  → tags must be a comma-separated STRING, not an array
  → FIX: tags: "handoff,analysis" (not tags: ["handoff", "analysis"])

Receiving agent can't find stash:
  → Was it stash_pop'd? Check stash_list() — pop removes from suspended list
  → Use stash_peek first to confirm it exists before stash_enter
  → Pass the stash_id explicitly rather than relying on search

Session spawn not needed for most handoffs:
  → stash_push + stash_ref is sufficient for context parking
  → Only spawn a new session if the receiving agent needs to run in parallel
  → For sequential handoffs (A finishes, B starts), stash is all you need
```

---

## Example Execution Trace (Observation #1: Work item handoff)

```
─── Pattern Instance: handoff-context ───
Input: {
  summary: "shellops-zod-migration-01 complete. 26 MCP tools unblocked. Next: pattern-design-01.",
  tags: "handoff,test,pattern-design"
}

[Step 1] stash_push
→ stash_push({ name: "handoff-pattern-test-payload", summary: "...", tags: "handoff,test,pattern-design" })
← { stash_id: "handoff-pattern-test-payload", state: "suspended" }
✓ SIG-01

[Step 2] stash_ref
→ stash_ref({ id: "handoff-pattern-test-payload" })
← {
    stash_id: "handoff-pattern-test-payload",
    name: "handoff-pattern-test-payload",
    tags: ["handoff","test","pattern-design"],
    summary: "# handoff-pattern-test-payload\n\n## Summary\n..."
  }
✓ SIG-03: portable reference ready

[Step 3] stash_peek
→ stash_peek({ id: "handoff-pattern-test-payload" })
← { summary_preview: "...", state: "suspended", last_agent: "dispatch-axiom" }
✓ SIG-04

RESULT: PATTERN_COMPLETE
  stash_id: handoff-pattern-test-payload
  pickup_hint: "Call stash_enter(id: 'handoff-pattern-test-payload') to resume"
```

---

## When NOT to Use This Pattern

- **The next agent needs real-time results from a sub-task**: use `conductor_spawn` + `conductor_collect` instead (fire-and-forget with result collection)
- **You just need a temporary note**: use `stash_append` to an existing stash
- **Cross-session read of a long transcript**: use `session_read` instead
