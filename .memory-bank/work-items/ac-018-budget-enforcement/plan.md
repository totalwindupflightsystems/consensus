# AC-018: Budget enforcement pauses runaway sessions

## Mission
Sessions exceeding `budget_limit_cents` must transition to `paused` (not failed, not stuck). The `agent_billing` table exists, `RecordBilling` is wired into the harness — wire budget check into the planning loop.

## In Scope
- Read `billing/tracker.go` — confirm `RecordBilling` + `BudgetCheck` exist
- Wire `BudgetCheck` call into planning loop after each LLM call
- On budget exceeded: session → `paused`, error message in audit
- Test: create session with `budget_limit_cents=1` → send prompt → assert paused

## Out of Scope
- Multi-session budget pools
- Refund/credit logic

## AC Mapping
| AC | Verification |
|----|-------------|
| AC-018 | `go test -run TestBudgetEnforcement_PausesRunawaySession` |

## Steps
1. Read `internal/billing/tracker.go` — verify RecordBilling and BudgetCheck signatures
2. Read `internal/harness/planning.go` — find where LLM calls happen
3. Add budget check after each LLM response
4. Wire `BillingTracker.RecordBilling` call per LLM turn (use prompt_tokens/completion_tokens)
5. On budget exceeded: `UPDATE sessions SET status='paused'`, inject error into audit
6. Write test, verify PASS

## Files touched
- `internal/harness/planning.go` (budget check after LLM call)
- `internal/harness/planning_test.go` (new test)
- Possibly `internal/billing/tracker.go` (if wiring incomplete)

axiom:trace work_item=ac-018-budget-enforcement spec=SPEC-006 spec=SPEC-008 impl=internal/harness/planning.go,internal/billing/tracker.go
