# AC-017: Planning error transitions session to failed

## Mission
When the planning loop hits an unrecoverable error (not SQLITE_BUSY — that's transient), the session must transition to `status='failed'` instead of staying stuck in "planning" forever.

## In Scope
- `handlePlanningError` in `planning.go` already writes `status='failed'` — verify
- Create a test case: inject a planning error → assert session status=failed within timeout
- Ensure SQLITE_BUSY (transient) does NOT trigger failed (only returns error, lets heartbeat retry)

## Out of Scope
- Circuit breaker logic (AC-040 covers that)
- Permanent stuck session recovery outside the harness

## AC Mapping
| AC | Verification |
|----|-------------|
| AC-017 | `go test -run TestPlanningErrorTransitionsToFailed` |

## Steps
1. Read `planning.go` `handlePlanningError` — confirm status='failed' write
2. Read `executor.go` `pollAndDispatch` — confirm error routing
3. Write test: create session → force planning error → poll → assert status=failed
4. Run test, verify PASS

## Files touched
- `internal/harness/planning_test.go` (new test)
- Possibly `internal/harness/planning.go` if status write is missing

axiom:trace work_item=ac-017-planning-error-to-failed spec=SPEC-008 spec=SPEC-020 impl=internal/harness/planning.go
