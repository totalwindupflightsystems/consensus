---
work_item_id: spec-006-hardening-01
status: complete
repo: wojons/conscientiousness
created: 2026-05-05
updated: 2026-05-05
verifier_audit: 2026-05-05 — 7/7 ACs PASS. All acceptance criteria have passing tests.
trigger: idle-spec-conformance-sweep-01 sweep-003
spec_ref: specs/006-transactions.md
---

# Plan — SPEC-006 Transaction Hardening

Fix four gaps discovered by idle spec conformance sweep-003 against `specs/006-transactions.md`: billing write path, circuit breaker persistence, async tool execution, and stored procedure equivalents.

axiom:trace work_item=spec-006-hardening-01 spec=specs/006-transactions.md plan=phase-1/task-1 evidence=.memory-bank/work-items/spec-006-hardening-01/verification.md

## AC → Verification

| AC | Criterion | Verification Path | Status |
|---|---|---|---|
| AC-HARDEN-01 | LLM calls record billing rows with token counts + cost_usd | Integration test: mock LLM, verify agent_billing row written after Harness.RunAgentIteration | PASS (internal/billing/tracker_test.go: 3 tests; wired in executor.go) |
| AC-HARDEN-02 | Budget limit enforced before LLM call (pauses session if exceeded) | Integration test: set BudgetLimitCents=1, make 2 small calls, verify 2nd call pauses | PASS (internal/billing/tracker_test.go: 5 tests; wired in executor.go lines 50-65) |
| AC-HARDEN-03 | agent_circuit_breakers table created + persisted | Migration added; unit tests exercise write + read + upsert of count state | PASS (internal/harness/circuit_test.go: 8 tests; migration at migrations/003_circuit_breakers.sql + testdata/migration_test.sql) |
| AC-HARDEN-04 | checkCircuitBreaker persists tripped state to agent_circuit_breakers | Integration test: verify tripBreaker writes tripped_at; backward compat wrapper; reset clears state | PASS (internal/harness/circuit_test.go: TestCircuitBreaker_TripPersistsTimestamp, TestCircuitBreaker_ResetClearsTripState, TestCircuitBreaker_IndependentBreakerTypes) |
| AC-HARDEN-05 | Tool executor goroutine polls pending tool_requests and executes sandboxed | Integration test: write pending tool_request, call PollOnce, verify status→completed + tool_results written + session wakes | PASS (internal/harness/tool_executor_test.go: 13 tests; TestToolExecutor_PollOnce_ClaimsAndExecutes, TestToolExecutor_Integration_ProcessesHarnessProducedRequests) |
| AC-HARDEN-06 | complete_task() Go function for general-purpose task completion | Unit test: call complete_task for arbitrary task, verify state transition enforced | PASS (internal/session/complete_test.go: 5 tests) |
| AC-HARDEN-07 | cancel_task() Go function for general-purpose task cancellation | Unit test: call cancel_task for arbitrary task, verify state transition + cleanup trigger | PASS (internal/session/complete_test.go: 4 tests) |

## Tasks

### Task 1: Billing Write Path (AC-HARDEN-01, AC-HARDEN-02)
- Implement `RecordBilling(sessionID, iteration, modelID, category, promptTokens, completionTokens, costUsd)` in `internal/billing/`
- Add `BeforeLLMCall` budget check in harness that queries cumulative cost from agent_billing
- Wire into `RunAgentIteration` after LLM call returns
- **Touched:** `internal/billing/tracker.go`, `internal/harness/executor.go`, `internal/harness/harness.go`

### Task 2: Circuit Breaker Persistence (AC-HARDEN-03, AC-HARDEN-04)
- Add migration for `agent_circuit_breakers` table
- Update `checkCircuitBreaker` to INSERT tripped state into circuit_breakers table
- Add `ResetCircuitBreaker(sessionID, breakerType)` for admin manual reset
- **Touched:** `migrations/`, `internal/harness/executor.go`, `internal/harness/circuit.go`

### Task 3: Async Tool Execution Poller (AC-HARDEN-05)
- Implement tool executor goroutine in harness that polls `tool_requests.status='pending'`
- Dispatch to sandboxed subprocess (or go_native handler) with timeout
- Write `tool_results` rows; update `tool_requests.status`
- Handle tool_executor DB role for result writes
- **Touched:** `internal/harness/tool_executor.go`, `internal/tools/sandbox.go`, `internal/tools/executor.go`

### Task 4: General-Purpose Stored Procedure Equivalents (AC-HARDEN-06, AC-HARDEN-07)
- Extract `complete_task()` from sub-agent-specific `CompleteChild` into general `internal/session/complete.go`
- Extract `cancel_task()` similarly
- Register both as tools in `tools_registry` (hemisphere: internal, handler_type: go_native)
- **Touched:** `internal/session/complete.go`, `internal/session/cancel.go`, `internal/tools/tools.go`

## Expected Touched Files
- `internal/billing/tracker.go` (new) — billing record write
- `internal/harness/executor.go` — wire billing + budget checks
- `internal/harness/circuit.go` (new) — circuit breaker persistence
- `internal/harness/tool_executor.go` (new) — async tool polling loop
- `internal/tools/sandbox.go` — sandboxed subprocess execution
- `internal/tools/executor.go` — tool dispatch (sql_function/go_native/subprocess/http)
- `internal/session/complete.go` (new) — general-purpose complete_task
- `internal/session/cancel.go` (new) — general-purpose cancel_task
- `migrations/008_circuit_breakers.sql` (new) — agent_circuit_breakers table
- Tests for each new file

## Rollback
- Revert Go source changes. Billing rows in agent_billing are append-only — no cleanup needed.
- Circuit breaker rows can be manually cleared: `DELETE FROM agent_circuit_breakers WHERE session_id = $1`.

## Commit Message Template
```
fix(harness): add billing write path, circuit breaker persistence, and async tool executor

Remediate SPEC-006 gaps found by idle spec conformance sweep-003:
- Record billing rows with token counts + cost after each LLM call
- Enforce budget limits before LLM calls
- Persist circuit breaker tripped state in agent_circuit_breakers table
- Add async tool executor goroutine polling pending tool_requests
- Extract general-purpose complete_task/cancel_task from sub-agent variants

axiom:trace work_item=spec-006-hardening-01 spec=specs/006-transactions.md plan=phase-1 evidence=.memory-bank/work-items/spec-006-hardening-01/verification.md

Co-authored-by: {from .axiom/axiom.config.yaml}
```
