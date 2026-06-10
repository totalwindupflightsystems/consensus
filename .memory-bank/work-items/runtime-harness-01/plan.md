---
work_item_id: runtime-harness-01
status: substantially-complete
repo: wojons/conscientiousness
created: 2026-05-03
updated: 2026-05-04
amended_by: spec-alignment-scan-01 — expanded plan to cover all acceptance criteria (AC-001 through AC-038)
verifier_note: 2026-05-04 — Core harness is proven (263 tests, 0 failures across all packages). SQLite integration tests cover AC-001/005(partial)/006/007/008. Deferred: AC-023 (pg_cron), AC-028-031 (sandbox/RLS → deployment-ops-01), AC-032/033 heartbeat integration (code exists, needs real-DB test).
---

# Plan — Runtime Harness

Build the runtime only after schema primitives are present. The first slice proves one minimal iteration can read context, accept structured output, commit database changes, and enqueue tool work. The second slice layers in multi-turn interactive transaction planning per SPEC-020.

axiom:trace work_item=runtime-harness-01 spec=specs/006-transactions.md,specs/008-harness.md,specs/010-tools.md,specs/012-system-prompt-and-discovery.md,specs/020-multi-turn-planning.md plan=phase-1/task-1/step-1 evidence=.memory-bank/work-items/runtime-harness-01/verification.md

## AC → Verification Mapping

| AC | Criterion | Verification Path | Notes |
|---|---|---|---|
| AC-001 | Harness loop reads context → LLM → SQL transaction | Integration test: real DB + mock LLM, assert COMMIT and state change | — |
| AC-002 | SQL classifier + policy enforcement | Unit test: feed DANGEROUS/DML_WRITE statements, assert blocked; feed allowed statements, assert passed | — |
| AC-003 | Multi-statement split + per-statement classify | Unit test: semicolon-delimited input, verify each part classified independently | — |
| AC-004 | Secret injection + scrubbing | Unit test: inject known secret, verify SQL contains real value and response is scrubbed | — |
| AC-005 | memory_events append-only enforcement | Integration test: attempt UPDATE/DELETE as agent_role, assert permission denied | Depends on schema-memory-01 |
| AC-006 | Audit log written per iteration | Integration test: after successful iteration, assert audit_log row exists with correct fields | — |
| AC-007 | Iteration snapshot saved | Integration test: after COMMIT, assert iteration_commits row with llm_response + sql_executed | — |
| AC-008 | Dynamic system prompt assembly | Unit test: mock session + tools + skills + constraints, verify formatted prompt includes all layers | — |
| AC-009 | Schema discovery — core + dynamic tables | Unit test: mock information_schema, verify core whitelist match and dynamic table detection | — |
| AC-010 | JSON schema enforcement | Integration test: send structurally invalid LLM output, assert parser rejects with clear error | — |
| AC-011 | Prompt cache breakpoints | Unit test: verify cache_control markers on system + user messages; dynamic layer excluded | — |
| AC-012 | Skills progressive disclosure | Unit test: verify system prompt includes metadata only; `load_skill` returns full instructions | — |
| AC-013 | Sub-agent prompt filtering | Unit test: verify tools are filtered to internal + approved-external; parent memory excluded | — |
| AC-014 | Interactive multi-turn transaction | Integration test: run 4-turn scenario (stage→execute→tool→commit), verify state at each turn | — |
| AC-015 | Staging buffer lifecycle | Unit + integration test: verify status transitions staged→executed→committed; also rolled_back and failed paths | — |
| AC-016 | stage_and_execute action | Integration test: stage 2 SQL commands, verify both execute and results in buffer | — |
| AC-017 | stage_only action | Unit test: stage commands only, verify buffer populated but executed=false | — |
| AC-018 | commit action | Integration test: staged commands → commit → verify all buffer entries are 'committed', session is 'idle' | — |
| AC-019 | rollback action | Integration test: stage+execute → rollback → verify buffer is 'rolled_back', DB state unchanged | — |
| AC-020 | Rollback-retry cap | Unit test: trigger 4 rollbacks in one iteration, verify iteration ends with error on 4th | — |
| AC-021 | Turn context formatting | Unit test: mock staging buffer with executed+staged commands, verify formatted output per SPEC-020 §6 | — |
| AC-022 | Transaction timeout | Integration test: set timeout to 1s, sleep 1.5s, verify session → 'failed', buffer → 'failed' | — |
| AC-023 | Crash recovery | Integration test: kill process mid-planning, verify pg_cron reaps session, orphan cleanup marks buffer failed | Depends on pg_cron; manual test acceptable |
| AC-024 | Max turns enforcement | Unit test: simulate 10 turns with executed work, verify auto-commit; 10 turns with no work, verify error | — |
| AC-025 | tool_call_ref in staging buffer | Integration test: tool_call action during planning, verify buffer entry type=tool_call_ref, tool executes outside tx | — |
| AC-026 | Tool registry resolution | Unit test: lookup tool by name, verify handler_type + handler_ref returned | — |
| AC-027 | Tool resolution order | Unit test: verify internal first, then skill-linked, then JIT, then built-in | — |
| AC-028 | Sandboxed subprocess execution | Integration test: execute a test tool in Deno sandbox, verify subprocess isolation and timeout | — |
| AC-029 | tool_results writes | Integration test: after tool execution, verify tool_results row exists with correct is_error + output | — |
| AC-030 | tool_requests status transitions | Integration test: pending → completed; verify completed_at; verify session → 'thinking' after all done | — |
| AC-031 | Tool ownership RLS | Integration test: Agent A attempts UPDATE on Agent B's tool, verify blocked | — |
| AC-032 | Heartbeat loop poll + claim | Integration test: insert pending task, wait for heartbeat, verify task is claimed (in_progress) | — |
| AC-033 | ClaimNextReadyTask atomicity | Unit test: concurrent claims, verify FOR UPDATE SKIP LOCKED prevents double-claim | — |
| AC-034 | display_modes JOIN in active_context_view | Integration test: set display_mode for an event, verify view renders compressed/hidden correctly | Depends on schema-memory-01 |
| AC-035 | memory_pages expansion | Integration test: create page with target_ids, query view, verify pointers expanded and deduplicated | Depends on schema-memory-01 |
| AC-036 | RLS scoping in active_context_view | Integration test: query view as session A, verify only session A's events visible | Depends on schema-memory-01 |
| AC-037 | compression_queue table present | Schema check: verify table exists with expected columns | Depends on schema-memory-01 |
| AC-038 | model_registry readable | Unit test: query model_registry, verify harness can read model capabilities | Depends on schema-memory-01 |

## Phases

### Phase 1: Single-shot cognition iteration (SPEC-006, SPEC-008 core loop)

Prove the harness can run one iteration: read context, call LLM (mocked), parse structured JSON, execute SQL in a transaction, commit, save audit log + snapshot.

**Expected touched areas:** `internal/harness/`, `internal/harness/parser.go`, `internal/harness/executor.go`, `internal/harness/context.go`, `internal/harness/secrets.go`

### Phase 2: System prompt & schema discovery (SPEC-012)

Assemble the system prompt dynamically. Query session info, tools_registry, skills_registry, core/dynamic tables, and constraints. Inject secrets; scrub responses.

**Expected touched areas:** `internal/harness/prompt.go`, `internal/harness/discovery.go`, `internal/harness/schema.go`

### Phase 3: Tool execution boundary (SPEC-010, SPEC-008 §ToolExecutionPhase)

Implement tool registry resolution, sandboxed subprocess execution, tool_results writes, and tool ownership RLS.

**Expected touched areas:** `internal/harness/tools.go`, `internal/harness/tools/sandbox.go`, `internal/harness/tools/registry.go`

### Phase 4: Interactive multi-turn planning (SPEC-020)

Replace single-shot loop with multi-turn interactive transaction. Implement staging buffer, action dispatch, commit/rollback handlers, timeout, crash recovery.

**Expected touched areas:** `internal/harness/planning.go`, `internal/harness/planning/buffer.go`, `internal/harness/planning/actions.go`, `internal/harness/planning/recovery.go`

### Phase 5: Heartbeat & task claiming (SPEC-008 §Heartbeat)

Implement heartbeat loop, atomic task claiming (Postgres FOR UPDATE SKIP LOCKED / SQLite alternative).

**Expected touched areas:** `internal/harness/heartbeat.go`, `internal/harness/heartbeat/claimer.go`

### Phase 6: Integration evidence

Wire full end-to-end: heartbeat polls → claims task → runs multi-turn iteration → commits → audits. Collect evidence bundle.

**Expected touched areas:** `internal/harness/integration_test.go`, `.memory-bank/work-items/runtime-harness-01/runs/<RUN_ID>/`

## Rollback Strategy

- Every phase commit is reversible by reverting the Go source, but DB state changes (staging_buffer entries, session statuses) are forward-only.
- Test-only changes: use `go test -short` to skip integration tests during rollback verification.
- Staging buffer entries are scoped to session+iteration — no cross-work-item contamination.

## Commit Message Template

```
feat(harness): implement multi-turn cognition runtime

Implement harness core loop with interactive transaction staging per
SPEC-006, SPEC-008, SPEC-010, SPEC-012, and SPEC-020. This covers
single-shot iteration, dynamic prompt assembly, tool sandbox boundary,
and multi-turn planning with staging buffer lifecycle.

axiom:trace work_item=runtime-harness-01 spec=specs/006-transactions.md,specs/008-harness.md,specs/010-tools.md,specs/012-system-prompt-and-discovery.md,specs/020-multi-turn-planning.md plan=phase-1..phase-5 evidence=.memory-bank/work-items/runtime-harness-01/verification.md

Co-authored-by: {from .axiom/axiom.config.yaml}
```
