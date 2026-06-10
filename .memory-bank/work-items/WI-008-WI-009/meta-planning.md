# WI-008 + WI-009 — RBAC Scope Model + SSE Event Stream

**Gaps**: CS-GAP-009 (HIGH), CS-GAP-013 (MEDIUM)
**Specs**: SPEC-004 §RBAC, SPEC-005 §Contextual Permission, SPEC-015 §4
**Status**: Planning

---

## Why This Matters

**RBAC (CS-GAP-009)**: Currently all agents have equal access. No project boundaries exist. SPEC-004 §RBAC defines a 3-tier scope model (Global > Project > Sub-Agent) that is entirely unimplemented. Without it, multi-project deployments are unsafe — sub-agents cannot be isolated, and cross-project memory leaks are possible.

**SSE (CS-GAP-013)**: The `EventBus` exists and SSE handlers are wired, but nothing pushes events into it. SPEC-015 §4 defines real-time event streams via Postgres LISTEN/NOTIFY and SQLite polling goroutines, but the data feed is a no-op. The shim `shimEventBridge` returns a no-op stop function. Without this, monitoring UIs cannot receive real-time updates.

---

## Scope Fences

### In Scope
- `projects` table (id, name, description, created_at)
- `project_id` column on `sessions` and `tasks`
- Scope hierarchy enforcement in session creation, task claiming, memory access, subagent spawning
- Sub-agent inherits parent's `project_id`
- Postgres LISTEN/NOTIFY bridge: `NotificationListener` → `EventBus.Publish()`
- SQLite polling goroutine: periodically query `audit_logs` / `approval_requests` → `EventBus.Publish()`
- SQL triggers: `notify_session_change()`, `notify_approval_request()`
- Wire `shimEventBridge` to use `EventBus` directly instead of no-op

### Out of Scope
- Full RLS policy rewriting (CS-GAP-010 is separate)
- `project_members` / `user_project_access` table (SPEC-005 §Contextual Permission — will be added later)
- UI dashboard changes
- WebSocket support (SSE only per SPEC-015 §4)

---

## Acceptance Criteria

| ID | Criterion | Verification |
|----|-----------|-------------|
| AC-RBAC-01 | `projects` table exists with id, name, description | Migration check + go test |
| AC-RBAC-02 | `sessions` table has `project_id` column referencing `projects` | Migration check + go test |
| AC-RBAC-03 | `tasks` table has `project_id` column | Migration check + go test |
| AC-RBAC-04 | Session creation accepts optional `project_id` | API handler test |
| AC-RBAC-05 | Sub-agent inherits parent's `project_id` | subagent.SpawnSubAgent test |
| AC-RBAC-06 | Global scope (NULL project_id) can access all | Scope check logic test |
| AC-RBAC-07 | Project-scoped agent restricted to its project | Scope check logic test |
| AC-SSE-01 | Postgres LISTEN/NOTIFY feeds into EventBus | Integration test |
| AC-SSE-02 | SQLite polling goroutine feeds EventBus | Unit test with mock DB |
| AC-SSE-03 | Session status changes publish SSE events | Event published on status transition |
| AC-SSE-04 | Approval requests publish SSE events | Event published on INSERT |
| AC-SSE-05 | `shimEventBridge` uses EventBus for real delivery | Direct test of bridge |
| AC-SSE-06 | All `go test ./...` pass | CI run |

---

## Assumptions

1. **A#01**: The `api_keys` scope column already exists — no migration needed for keys.
   - How to verify: check migration 001 for `api_keys` table.
   - Impact if wrong: small fix — add scope column.

2. **A#02**: SQLite polling can use a 1-second interval goroutine for event detection.
   - How to verify: acceptable latency for SSE updates.
   - Impact if wrong: tune interval or use SQLite hook mechanism.

3. **A#03**: The existing `EventBus.Publish()` is goroutine-safe and non-blocking for slow subscribers.
   - How to verify: code review of `events.go`.
   - Impact if wrong: subscribers block publishers — add goroutine wrapper.

---

## Decision Points

| Decision | Options | Recommendation |
|----------|---------|---------------|
| Project ID type | UUID vs TEXT | UUID (matches existing FK pattern) |
| Default project | NULL = global vs mandatory default | NULL = global (SPEC-004 §RBAC) |
| SSE polling interval | 500ms vs 1s vs 2s | 1s (good balance for real-time feel) |

---

## Open Questions

None — spec is clear on scope hierarchy and SSE wiring.

**axiom:trace work_item=WI-008-WI-009 spec=specs/004-subagents.md,specs/005-security.md,specs/015-api-and-mcp.md plan=meta-planning**
