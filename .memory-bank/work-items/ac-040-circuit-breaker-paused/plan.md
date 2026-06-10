# AC-040: Circuit breaker — 3 consecutive errors → paused

## Mission
After 3 consecutive errors, the circuit breaker must trigger `status='paused'` (not failed). The circuit breaker table and code exist in `internal/harness/circuit_test.go` — wire it to session status.

## In Scope
- Read `internal/harness/circuit_breaker.go` — confirm CheckCircuitBreaker + TripCircuitBreaker exist
- Wire circuit breaker check into harness iteration loop
- On 3 errors: `UPDATE sessions SET status='paused'`, inject HITL approval context
- Test: create session with always-failing tool → 3 iterations → assert paused

## Out of Scope
- HITL approval review UI (AC-039)
- Circuit breaker reset logic (exists, just need to test)

## AC Mapping
| AC | Verification |
|----|-------------|
| AC-040 | `go test -run TestCircuitBreaker_ThreeErrorsPausesSession` |

## Steps
1. Read `internal/harness/circuit_breaker.go` and `circuit_test.go` for existing API
2. Wire breaker check in `internal/harness/harness.go` RunIteration or planning loop
3. On trip: set session status='paused', write audit entry with error context
4. Write test, verify PASS

## Files touched
- `internal/harness/harness.go` (wire circuit breaker)
- `internal/harness/planning.go` (status transition on trip)
- `internal/harness/circuit_test.go` (new test)

axiom:trace work_item=ac-040-circuit-breaker-paused spec=SPEC-014 impl=internal/harness
