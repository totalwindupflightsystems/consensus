# WI-008 + WI-009 — Execution Plan

## Summary
Implement RBAC scope model (projects + project_id on sessions/tasks + scope inheritance) and wire the SSE EventStream with actual data feeds (Postgres LISTEN/NOTIFY bridge + SQLite polling + shim bridge).

## AC → Verification Mapping

| AC | Verification Path |
|----|------------------|
| AC-RBAC-01..03 | `go test ./internal/migrate/...` — migration applies cleanly |
| AC-RBAC-04 | `go test ./internal/api/...` — CreateSession accepts project_id |
| AC-RBAC-05 | `go test ./internal/subagent/...` — SpawnSubAgent inherits project_id |
| AC-RBAC-06..07 | Scope check unit test in `internal/api/` |
| AC-SSE-01 | Postgres notification listener → EventBus integration |
| AC-SSE-02 | SQLite polling goroutine → EventBus goroutine test |
| AC-SSE-03 | Session status change events published (integration) |
| AC-SSE-04 | Approval request events published (integration) |
| AC-SSE-05 | `shimEventBridge` emit/listen test |
| AC-SSE-06 | `go test ./...` passes |

## Phases

### Phase 1: Migration + Model Updates
- **Task 1.1**: Create `014_projects_and_scope.sql` migration
- **Task 1.2**: Add `ProjectID` field to Go structs (SessionResponse, CreateSessionInput, etc.)
- **Verify**: `go test ./...`

### Phase 2: Wire RBAC into Session Creation + Subagent Spawn
- **Task 2.1**: Update `SessionService.CreateSession` to accept `project_id`
- **Task 2.2**: Update `SessionResponse` and API types to include `project_id`
- **Task 2.3**: Update `subagent.SpawnSubAgent` to inherit parent's `project_id`
- **Task 2.4**: Add scope check helper in API layer
- **Verify**: `go test ./...`

### Phase 3: Wire Postgres LISTEN/NOTIFY → EventBus
- **Task 3.1**: Add `SetEventHandler` / `SetEventBus` to `NotificationListener`
- **Task 3.2**: Create `StartEventListener` in `api` package that bridges Postgres notifications to EventBus
- **Task 3.3**: Create SQL triggers in migration 014 for session/approval notification
- **Verify**: `go test ./...`

### Phase 4: Wire SQLite Polling Goroutine + Shim Bridge
- **Task 4.1**: Create SQLite event polling goroutine that queries recent changes
- **Task 4.2**: Wire `shimEventBridge` to use EventBus directly
- **Task 4.3**: Wire event sources into main.go
- **Verify**: `go test ./...`

### Phase 5: Commit
- **Task 5.1**: Conventional commit with Co-authored-by trailer

## Backend Behavior

| Operation | Postgres | SQLite |
|-----------|----------|--------|
| RBAC Enforcement | RLS policies (future) + Go scope checks | Go scope checks |
| Project storage | `project_id` column with FK → projects | `project_id` TEXT column |
| SSE events | LISTEN/NOTIFY triggers → NotificationListener → EventBus | Polling goroutine → EventBus |
| Event detection | DB triggers on sessions/approvals | 1s interval poll on audit_logs + approval_requests |
| Shim bridge | EventBus channel | EventBus channel |

## Trace Map

```
Work Item WI-008 + WI-009
  → Specs: SPEC-004 (§RBAC), SPEC-005 (§Contextual Permission), SPEC-015 (§4)
  → Migration: 014_projects_and_scope.sql
  → Implementation: internal/api/events.go, internal/api/service.go, internal/subagent/subagent.go, internal/db/postgres/notify.go, cmd/consensus/main.go
  → Tests: ./...
  → Evidence: .memory-bank/work-items/WI-008-WI-009/verification.md
```

**axiom:trace work_item=WI-008-WI-009 spec=specs/004-subagents.md,specs/005-security.md,specs/015-api-and-mcp.md plan=phase-1/task-1**
