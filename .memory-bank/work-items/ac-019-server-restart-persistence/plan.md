# AC-019: Server survives restart — DB state persists

## Mission
Killing and restarting the conscience server must preserve sessions, memory events, and config. SQLite WAL mode already handles this — just needs a test.

## In Scope
- Integration test: start server → create session → post message → kill server → restart server → verify session + memory events intact
- Verify schema_versions count unchanged after restart

## Out of Scope
- In-flight planning state recovery (AC-016 covers that)
- Multi-node persistence

## AC Mapping
| AC | Verification |
|----|-------------|
| AC-019 | `go test -run TestServerSurvivesRestart` |

## Steps
1. Write integration test in `internal/harness/server_recovery_test.go`
2. Test: start server on random port → create session → record session ID + memory count → SIGTERM → wait → restart → GET session + memory → assert same data
3. Verify with `go test -count=1 -v`

## Files touched
- `internal/harness/server_recovery_test.go` (new)

axiom:trace work_item=ac-019-server-restart-persistence impl=internal/harness/server_recovery_test.go
