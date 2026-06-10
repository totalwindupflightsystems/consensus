# WI-005: External Tool Execution Sandbox — Meta-Planning

**axiom:trace work_item=WI-005 spec=specs/010-tools.md,specs/014-hitl-interrupt-state.md plan=meta-planning.md**

## Gap Reference
CS-GAP-004 (CRITICAL): `custom_agent_tools` table exists but `ExecuteExternalTool()` is not implemented. External tools are registrable but not executable.

## Spec Coverage
| Spec | Section | Relevance |
|------|---------|-----------|
| SPEC-010 | §External Hemisphere, §Tool Execution, §Registry Schema | Core sandbox design |
| SPEC-014 | §Tool-Required Interrupts, §Schema | Approval gating for requires_approval tools |
| SPEC-003 | §4.3 tool_results | Result storage schema |

## Scope

### In Scope
1. `ExecuteExternalTool()` in `internal/tools/` — subprocess execution with timeout, output limit, temp dir, env whitelist
2. Wire subprocess handler in harness `tool_executor.go` `executeTool()` method
3. Capture results → `tool_results` with exit_code, duration_ms, output
4. Per-tool rate limiting (`rate_limit_per_min` column enforcement)
5. Tool approval gating (`requires_approval=true` → HITL approval_request)
6. Max 10 concurrent executions (semaphore)
7. No network access for untrusted tools (configurable per-tool via env)
8. Migration: add `exit_code`, `duration_ms` to `tool_results`; add `approval_request_id` to `tool_requests`

### Out of Scope
- Deno/TypeScript JIT runtime (future work)
- WASM sandbox (future work)
- `http_endpoint` handler type (separate task)
- Autonomous CI/CD pipeline for tool testing (separate task)

## Architecture

```
Agent Output (JSON) → executeInTransaction() → INSERT INTO tool_requests
                                                          ↓
                      ToolExecutor.PollOnce() → claims pending requests
                                                          ↓
                      executeTool() → dispatches by handler_type
                          ├── sql_function → e.database.Query(...)
                          ├── subprocess  → tools.ExecuteExternalTool()
                          ├── go_native   → function registry lookup
                          └── http_endpoint → stub (future)
                                                          ↓
                      Writes tool_results → wake session
```

## Dependencies
- `internal/tools/tools.go` — Tool types, Registry
- `internal/tools/execute.go` — **NEW**: ExecuteExternalTool()
- `internal/tools/rate_limiter.go` — **NEW**: Rate limit enforcement
- `internal/tools/approval.go` — **NEW**: Approval gating
- `internal/harness/tool_executor.go` — Updated executeTool()
- `internal/harness/tool_executor_test.go` — Updated tests
- `internal/db/db.go` — DB interface
- `internal/hitl/hitl.go` — Manager.RequestApproval() for gating
- `migrations/011_tool_sandbox.sql` — **NEW**: Schema additions

## Risk Assessment
| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Sandbox escape via shell injection | Low | Critical | Use exec.CommandContext (no shell), validate params |
| Resource exhaustion (fork bomb) | Low | High | Semaphore cap (10 concurrent), timeout enforcement |
| Temp dir leakage | Medium | Low | defer os.RemoveAll on temp dir |
| Rate limit bypass | Low | Medium | Go-level check + DB trigger (two layers) |
