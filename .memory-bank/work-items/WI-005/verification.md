# WI-005: External Tool Execution Sandbox — Verification

**axiom:trace work_item=WI-005 spec=specs/010-tools.md,specs/014-hitl-interrupt-state.md plan=plan.md evidence=verification.md commit=fd4bde9**

## Summary

CS-GAP-004 (CRITICAL) resolved. External tool execution sandbox implemented with:
- `ExecuteExternalTool()` subprocess sandbox
- Rate limiting and HITL approval gating
- Harness wiring for subprocess handler type
- Schema migration for `exit_code`, `duration_ms`, `approval_request_id`

## Build & Test Results

```
$ go build ./...
Build: OK

$ go vet ./...
Vet: OK

$ go test ./... -count=1
All 26 packages: OK

Tools package: 42 tests PASS
Harness package: All tool executor tests PASS, including subprocess integration
```

## Verification Commands

```bash
# Full build
go build ./...

# Vet
go vet ./...

# Tools package tests (sandbox, rate limiting, approval gating)
go test ./internal/tools/... -v -count=1

# Harness tool executor tests (wired subprocess execution)
go test ./internal/harness/... -run TestToolExecutor -v -count=1

# Full test suite
go test ./... -count=1
```

## Files Created/Modified

### New Files
| File | Purpose |
|------|---------|
| `internal/tools/sandbox.go` | Sandbox configuration, env whitelist, semaphore, `ExecuteExternalTool()` |
| `internal/tools/execute.go` | High-level tool dispatch (`ExecuteTool()`, `executeSubprocess()`) |
| `internal/tools/rate_limiter.go` | `CheckToolRateLimit()` — Go-level rate limit enforcement |
| `internal/tools/approval.go` | `ToolRequiresApproval()`, `CreateToolApprovalRequest()` — HITL gating |
| `internal/tools/execute_test.go` | 14 sandbox execution tests |
| `internal/tools/rate_limiter_test.go` | 8 rate limit tests |
| `internal/tools/approval_test.go` | 7 approval gating tests |
| `migrations/011_tool_sandbox.sql` | Schema migration for new columns |
| `.memory-bank/work-items/WI-005/meta-planning.md` | Meta-planning document |
| `.memory-bank/work-items/WI-005/plan.md` | Execution plan |
| `.memory-bank/work-items/WI-005/plan.yaml` | Structured plan |

### Modified Files
| File | Changes |
|------|---------|
| `internal/tools/tools.go` | `toBool()` handles int64 (SQLite compatibility) |
| `internal/harness/tool_executor.go` | `handleApprovalGated()`, updated `executeTool()` with subprocess dispatch, rate limit check |
| `internal/harness/tool_executor_test.go` | Updated schema columns in test DB, added subprocess integration tests |
| `internal/harness/testdata/migration_test.sql` | Added `exit_code`, `duration_ms`, `approval_request_id`, `awaiting_approval` |
| `.memory-bank/work-items/_index.md` | Added WI-005 entry |

## Trace Map

```
SPEC-010 (§External Hemisphere, §Tool Execution) ──→ internal/tools/sandbox.go
                                                      internal/tools/execute.go
SPEC-010 (§Registry Schema) ──────────────────────→ internal/tools/rate_limiter.go
                                                     migrations/011_tool_sandbox.sql
SPEC-014 (§Tool-Required Interrupts) ─────────────→ internal/tools/approval.go
SPEC-003 (§4.3 tool_results) ─────────────────────→ migrations/011_tool_sandbox.sql
Harness (SPEC-008) ───────────────────────────────→ internal/harness/tool_executor.go
```

axiom:trace work_item=WI-005 spec=specs/010-tools.md,specs/014-hitl-interrupt-state.md plan=plan.md evidence=verification.md commit=fd4bde9
