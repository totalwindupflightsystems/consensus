# AC-042: Multi-session isolation — session A cannot see session B's memory

## Mission
Session-scoped API keys must enforce that session A can only read/write its own data. API key scope enforcement exists (admin/session/readonly/webhook) — verify session_key scope actually filters queries.

## In Scope
- Read API key auth code — confirm session_key scopes queries with `WHERE session_id = $1`
- Read memory API endpoint — verify it checks API key scope before returning data
- Test: create sessions A and B → use A's key to GET B's memory → assert 403 → use admin key → assert 200
- Ensure query-level RLS: session key cannot read other sessions even via direct ID injection

## Out of Scope
- Row-level SQL triggers for RLS (already handled by API layer)
- Postgres RLS policies (AC-056 deferred)

## AC Mapping
| AC | Verification |
|----|-------------|
| AC-042 | `go test -run TestMultiSessionIsolation` |

## Steps
1. Read `internal/api/` — find auth middleware and memory/query handlers
2. Verify session_key scope appends `WHERE session_id = <own>`
3. Write API-level test: create A+B, query cross-session, assert 403
4. Write bypass test: direct DB query with A's key trying to read B's data → assert blocked
5. Run test, verify PASS

## Files touched
- `internal/api/sessions_test.go` or `internal/api/quarantine_test.go` (new test)
- Possibly `internal/api/auth.go` if scope enforcement missing

axiom:trace work_item=ac-042-multi-session-isolation spec=SPEC-015 impl=internal/api
