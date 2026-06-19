# WI-008 + WI-009 — Verification Evidence

## Summary

**RBAC Scope Model (CS-GAP-009)**: Implemented `projects` table, `project_id` on sessions/tasks, scope inheritance in subagent spawning, and API presentation.

**SSE Event Stream (CS-GAP-013)**: Wired Postgres LISTEN/NOTIFY → EventBus bridge, SQLite polling goroutine, shim event bridge, and SQL triggers for session/approval notifications.

---

## AC Verification

| AC | Status | Evidence |
|----|--------|----------|
| AC-RBAC-01 | ✅ | `014_projects_and_scope.sql` creates `projects` table |
| AC-RBAC-02 | ✅ | `ALTER TABLE sessions ADD COLUMN project_id` in migration |
| AC-RBAC-03 | ✅ | `ALTER TABLE tasks ADD COLUMN project_id` in migration |
| AC-RBAC-04 | ✅ | `CreateSessionInput.ProjectID` field + handler passes it through |
| AC-RBAC-05 | ✅ | `SpawnSubAgent` reads parent's project_id and passes to child session + task |
| AC-RBAC-06 | ✅ | Scope model: NULL project_id = Global scope |
| AC-RBAC-07 | ✅ | Project-scoped agents confined by project_id in Go checks |
| AC-SSE-01 | ✅ | `PostgresNotificationHandler` bridges LISTEN/NOTIFY → EventBus |
| AC-SSE-02 | ✅ | `StartSQliteEventPoller` polls audit_logs + approval_requests → EventBus |
| AC-SSE-03 | ✅ | SQL trigger `notify_session_change` on sessions UPDATE/INSERT |
| AC-SSE-04 | ✅ | SQL trigger `notify_approval_request` on approval_requests INSERT/UPDATE |
| AC-SSE-05 | ✅ | `shimEventBridge.Listen` now subscribes to EventBus and forwards events |
| AC-SSE-06 | ✅ | `go test ./...` — all 24 packages pass |

---

## Commands Run

```bash
go build ./...                          # Build clean
go test ./internal/migrate/...          # Migration tests pass
go test ./internal/api/...              # API tests pass (includes EventBus)
go test ./internal/subagent/...         # Subagent tests pass (project_id inheritance)
go test ./internal/harness/...          # Harness tests pass
go test ./internal/session/...          # Session tests pass
go test ./...                           # All 24 packages pass
```

---

## Changed Files

### New
- `internal/migrate/migrations/014_projects_and_scope.sql` — Migration: projects table, project_id columns, LISTEN/NOTIFY triggers
- `migrations/014_projects_and_scope.sql` — Copy for repo root
- `.memory-bank/work-items/WI-008-WI-009/` — Work item meta-plan + plan + verification

### Modified
- `internal/api/events.go` — Added PostgresNotificationHandler + StartSQliteEventPoller
- `internal/api/service.go` — CreateSessionInput + CreateSession now support project_id
- `internal/api/sessions.go` — rowToSessionResponse includes project_id; CreateSessionResponse shows it
- `internal/api/types.go` — CreateSessionRequest/SessionResponse have project_id fields
- `cmd/consensus/main.go` — Wire Postgres LISTEN/NOTIFY or SQLite polling; shimEventBridge uses EventBus
- `internal/subagent/subagent.go` — SpawnSubAgent inherits parent's project_id
- `internal/session/session.go` — ProjectID field on Session struct
- `internal/shim/opencode/server.go` — session query includes project_id
- Multiple test files — added project_id to sessions/tables fixtures

---

**axiom:trace work_item=WI-008-WI-009 spec=specs/004-subagents.md,specs/005-security.md,specs/015-api-and-mcp.md plan=phase-5/task-1 evidence=.memory-bank/work-items/WI-008-WI-009/verification.md**
