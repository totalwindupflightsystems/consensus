# ROUTE-FIX-044 / ROUTE-FIX-045 — Live Reprobe

ch:trace row=ROUTE-FIX-044 row=ROUTE-FIX-045
  spec=migrations/008_hitl_tables.sql
  wave=consensus-foreman-2026-10-03-00-46-49.json#task-2
  test=internal/shim/opencode/permission_realstore_test.go
  doc=docs/evidence/ROUTE-FIX-044-045-reprobe.md
  evidence=docs/evidence/ROUTE-FIX-044-045-reprobe.md
  witness=none:unattended worker session; probes are scripted curls against a locally built binary, transcripts preserved verbatim below

Date: 2026-10-02 (local, UTC-05) / 2026-10-03T01:20Z
Worktree: /home/kara/worktrees/consensus-ROUTE-FIX-044 (branch wt/ROUTE-FIX-044, base 5f9239f)

## What changed

`internal/shim/opencode/server.go` — getPermission and resolvePermission
referenced approval_requests columns that do not exist
(`sql_preview`, `decision_reason`, `resolved_at`, `resolved_by`).
migrations/008_hitl_tables.sql defines the real column set; the native
`hitl.Manager.ReviewApproval` (internal/hitl/hitl.go:292-296) writes the same
columns. Mapping applied (JSON keys unchanged, only column mapping fixed):

| Shim JSON field / SQL | Real column (008 schema) |
|---|---|
| `sql_preview` | `target_sql` |
| `decision_reason` | `review_notes` |
| `resolved_at` | `reviewed_at` |
| `resolved_by = 'opencode-shim'` | `reviewer_id = 'opencode-shim'` |

resolvePermission additionally gained the documented 404 arm: the db wrapper
(`internal/db/db.go`) exposes no RowsAffected, so existence and the
`status='pending'` guard are checked up front with a SELECT (same
read-then-write shape as `hitl.Manager.ReviewApproval`). An unknown id and an
already-resolved row both answer the declared 404 arm — the contract documents
only [200,400,401,404], so 409 is not available on this route.

The 401 arm needs no code: `/permission/{id}` sits behind the standard api-key
auth middleware (`authMiddleware` in server.go), and the T1 sweep itself
recorded `unauthenticated_status: 401, documents_401: true`.

## Method

Binary built from this worktree, scratch DB initialized and migrated, row
seeded with REAL columns (session FK + approval row `t1-perm-0002`,
status='pending'), then scripted curl probes. No sweep generator script ships
in `scripts/` for the T1 sweep (`docs/evidence/t1-openapi-sweep-2026-09-30.json`
is a foreman artifact), so the two affected operations were probed manually
with the same request shapes as the sweep.

```text
go build -o /tmp/consensus-t044 ./cmd/consensus
/tmp/consensus-t044 init --db-url sqlite:///tmp/t044.db   # fresh scratch DB, embedded migrations applied
sqlite3 /tmp/t044.db < seed.sql    # INSERT sessions row + approval_requests row t1-perm-0002 (status='pending')
/tmp/consensus-t044 serve --db-url sqlite:///tmp/t044.db --port 18090
# probes below, admin bearer = the first_admin_key minted by init
```

## Observed vs documented

### ROUTE-FIX-044 — GET /permission/{permissionId} (documented [200,401,404])

**T1-D4 reprobe — existing pending row:**

```text
GET /permission/t1-perm-0002  (admin bearer)
{"created_at":"2026-10-03 01:21:20","decision_reason":"","description":"t1 sweep pending approval","id":"t1-perm-0002","resolved_at":null,"risk_level":"high","session_id":"68ed0699-367d-4206-9268-25ebab753dcb","sql_preview":"DROP TABLE temp_cache","status":"pending","type":"destructive_action"}
HTTP_STATUS=200
```

Observed 200, documented 200. **T1-D4 gone.** (Sweep observed 404 NOT_FOUND
on this exact row.)

```text
GET /permission/t1-perm-9999  (admin bearer)     → 404 {"error":{"code":"NOT_FOUND","message":"permission not found"}}
GET /permission/t1-perm-0002  (no credentials)   → 401 {"error":{"code":"UNAUTHENTICATED","message":"invalid credentials"}}
```

404 (unknown id) and 401 (unauthenticated) both within the documented set.

### ROUTE-FIX-045 — POST /permission/{permissionId}/resolve (documented [200,400,401,404])

**T1-D5 reprobe — well-formed body on the pending row:**

```text
POST /permission/t1-perm-0002/resolve  {"decision":"approved","reason":"reprobe: safe to proceed"}
{"id":"t1-perm-0002","resolved":true,"status":"approved","timestamp":"2026-10-03T01:22:07Z"}
HTTP_STATUS=200
```

Observed 200, documented 200, no sqlite error. **T1-D5 gone.** (Sweep observed
500 `no such column: decision_reason` on this exact request.) Persistence
verified by reading the row back through sqlite3:

```text
t1-perm-0002|approved|opencode-shim|reprobe: safe to proceed|2026-10-03T01:22:07Z
-- id | status | reviewer_id | review_notes | reviewed_at
```

```text
POST /permission/t1-perm-0002/resolve again {"decision":"rejected"}  → 404 {"error":{"code":"NOT_FOUND","message":"permission is not pending"}}
POST /permission/t1-perm-9999/resolve {"decision":"approved"}        → 404 {"error":{"code":"NOT_FOUND","message":"permission not found"}}
POST /permission/t1-perm-9999/resolve body "{not json"               → 400 {"error":{"code":"INVALID_REQUEST","message":"malformed request body"}}
POST /permission/t1-perm-9999/resolve {"decision":"maybe_later"}    → 400 {"error":{"code":"INVALID_REQUEST","message":"decision must be 'approved', 'rejected', or 'modified'"}}
```

400 (malformed body / invalid decision) and 404 (unknown id, non-pending row)
all within the documented set.

## Focused test evidence

```text
go test -short -count=1 -run 'Permission' ./internal/shim/opencode/   → ok (29.5s)
go test -short -count=1 ./internal/hitl/                              → ok (exit=0)
go build ./... && go vet ./... && gofmt -l internal/                  → clean
```

New/updated tests: `TestGetPermission`, `TestGetPermissionNotFound`,
`TestResolvePermission{Approve,Reject,MalformedBody,NotFound,AlreadyResolved}`
(server_test.go, mock-based) and `permission_realstore_test.go`
(real SQLite store with the 008 column set — GET 200/404, resolve 200 with
read-back of review_notes/reviewed_at/reviewer_id, pending-guard 404, unknown
404, malformed/invalid 400).

Note: the full `go test -short ./internal/shim/opencode/` package run hit go's
default 600s test timeout on an unrelated process-spawning test
(TestSessionCommandServesCommandAgainstSession) under host load (loadavg ~60-80,
sibling fleet workers). A re-run with `-timeout 2400s` got through the package
in 923s with a single failure: TestSendMessageReturnsResponseForSubmittedTurn
failed with `sqlite: exec: database is locked (5) (SQLITE_BUSY)` — a
load-induced busy timeout on that test's own scratch DB, in the message-route
surface this change does not touch. Re-run in isolation:
`go test -short -count=1 -run TestSendMessageReturnsResponseForSubmittedTurn
./internal/shim/opencode/` → ok (36.6s). All permission-route tests pass in
every run.

## Verdict

- ROUTE-FIX-044: PASS — GET /permission/{id} serves the documented 200 for an
  existing row; 404/401 arms intact.
- ROUTE-FIX-045: PASS — POST resolve serves the documented 200 on a pending row
  with the resolution persisted to real columns; 400/404 arms intact.

commit= see git footer of the fixing commit on wt/ROUTE-FIX-044.
