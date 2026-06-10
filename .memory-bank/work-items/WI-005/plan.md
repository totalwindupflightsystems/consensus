# WI-005: External Tool Execution Sandbox — Execution Plan

**axiom:trace work_item=WI-005 spec=specs/010-tools.md,specs/014-hitl-interrupt-state.md plan=plan.md**

## Step 1: Migration (011_tool_sandbox.sql)
Add columns to `tool_results` and `tool_requests`:
- `tool_results.exit_code INT` — exit code from subprocess (NULL for non-subprocess handlers)
- `tool_results.duration_ms BIGINT` — execution wall clock time
- `tool_requests.approval_request_id UUID REFERENCES approval_requests(id)` — link to HITL gate
- Add `'awaiting_approval'` to `tool_requests.status` CHECK constraint

## Step 2: Sandbox Core (internal/tools/sandbox.go)
Types and configuration for the sandbox:
- `SandboxConfig` struct with Timeout, MaxOutputBytes, WorkDir, AllowedEnvVars, NoNetwork
- `DefaultSandboxConfig()` with 30s timeout, 1MB limit, whitelisted env vars
- `ExecuteExternalTool(ctx, executable string, args []string, cfg SandboxConfig) (*ExternalToolResult, error)`
- Concurrent execution semaphore (max 10)

## Step 3: Execute External Tool (internal/tools/execute.go)
Core execution logic:
1. Create temp dir
2. Build exec.CommandContext with timeout
3. Set whitelisted env vars
4. Set working dir to temp dir
5. Capture stdout + stderr
6. Enforce output size limit (truncate at 1MB)
7. Return ExternalToolResult{Output, ExitCode, DurationMs, Error}

## Step 4: Rate Limiting (internal/tools/rate_limiter.go)
- `CheckRateLimit(ctx, db.DB, toolName, sessionID) error`
- Query: `SELECT COUNT(*) FROM tool_requests WHERE tool_name=$1 AND session_id=$2 AND created_at > now() - 1 minute`
- Compare against tools_registry.rate_limit_per_min for the tool
- Return error if exceeded

## Step 5: Approval Gating (internal/tools/approval.go)
- `RequiresApproval(ctx, db.DB, toolName) (bool, error)` — check tools_registry
- `CreateApprovalRequest(ctx, db.DB, sessionID, toolName, params) (string, error)` — insert approval_request, pause session

## Step 6: Harness Wiring (internal/harness/tool_executor.go)
In `executeTool()`:
- For `subprocess` handler_type: call `tools.ExecuteExternalTool()` instead of returning stub
- Check rate limit before execution (returns rate-limit error to tool_results)
- Check requires_approval before execution (creates approval_request, skips execution)
- Write exit_code + duration_ms to tool_results

## Step 7: Tests

### Sandbox Tests
- `TestExecuteExternalTool_Success` — runs `echo hello`, verifies output
- `TestExecuteExternalTool_Timeout` — runs `sleep 10` with 100ms timeout, verifies timeout error
- `TestExecuteExternalTool_OutputLimit` — generates 2MB output, verifies truncation to 1MB
- `TestExecuteExternalTool_ExitCode` — runs `exit 42`, verifies exit_code=42
- `TestExecuteExternalTool_NotFound` — runs nonexistent command, verifies error
- `TestExecuteExternalTool_EnvWhitelist` — verifies only whitelisted env vars pass through
- `TestConcurrentSemaphore` — verifies max 10 concurrent executions

### Rate Limiter Tests
- `TestCheckRateLimit_NoLimit` — no rate_limit_per_min set, returns nil
- `TestCheckRateLimit_UnderLimit` — under rate limit, returns nil
- `TestCheckRateLimit_Exceeded` — exceeds rate limit, returns error

### Approval Gating Tests
- `TestRequiresApproval_NotRequired` — tool without requires_approval=true
- `TestRequiresApproval_Required` — tool with requires_approval=true

### Harness Integration Tests
- Update `setupToolExecutorTestDB` for new columns
- `TestSubprocessToolExecution` — register subprocess tool, execute, verify tool_results

## Verification Commands
```bash
go build ./...
go vet ./...
go test ./internal/tools/... -v -count=1
go test ./internal/harness/... -run TestToolExecutor -v -count=1
go test ./... -count=1
```

axiom:trace work_item=WI-005 spec=specs/010-tools.md,specs/014-hitl-interrupt-state.md plan=plan.md
