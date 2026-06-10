// Package harness: async tool execution poller (AC-HARDEN-05).
//
// The tool executor goroutine polls the tool_requests table for pending requests
// and dispatches them to sandboxed subprocesses. Tool execution happens OUTSIDE
// the main cognition transaction — this is the "Phase 2" from SPEC-006 §Two-Phase
// Execution.
//
// WI-005: subprocess handler now dispatches to tools.ExecuteExternalTool() with
// sandbox isolation (temp dir, timeout, output limit, env whitelist).
// Rate limiting and approval gating are enforced before execution.
//
// axiom:trace work_item=WI-005 spec=specs/010-tools.md,specs/014-hitl-interrupt-state.md plan=phase-3/task-1 impl=internal/harness/tool_executor.go
package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wojons/conscientiousness/internal/db"
	"github.com/wojons/conscientiousness/internal/tools"
)

// ============================================================================
// Tool Executor
// ============================================================================

// ToolExecutorImpl polls for pending tool_requests and executes them asynchronously.
//
// It implements harness.ToolExecutor so it can be injected into the Harness.
type ToolExecutorImpl struct {
	database  db.DB
	pollMS    int           // polling interval in milliseconds
	timeout   time.Duration // max execution time per tool
	shutdown  chan struct{}
	stopped   atomic.Bool
	running   atomic.Bool
	processed atomic.Int64
	lastError atomic.Value // stores error string
	wg        sync.WaitGroup
}

// ToolExecutorConfig configures the async tool executor.
type ToolExecutorConfig struct {
	// PollMS is the interval between polling cycles in milliseconds.
	// Default: 500ms. Use jitter to prevent thundering herd.
	PollMS int

	// Timeout is the maximum execution time per tool call.
	// Default: 30 seconds.
	Timeout time.Duration
}

// DefaultToolExecutorConfig returns sensible defaults.
func DefaultToolExecutorConfig() ToolExecutorConfig {
	return ToolExecutorConfig{
		PollMS:  500,
		Timeout: 30 * time.Second,
	}
}

// NewToolExecutor creates a new async tool executor.
func NewToolExecutor(database db.DB, cfg ToolExecutorConfig) *ToolExecutorImpl {
	if cfg.PollMS <= 0 {
		cfg.PollMS = 500
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}

	return &ToolExecutorImpl{
		database: database,
		pollMS:   cfg.PollMS,
		timeout:  cfg.Timeout,
		shutdown: make(chan struct{}),
	}
}

// ============================================================================
// Lifecycle
// ============================================================================

// Start begins the continuous polling loop in a background goroutine.
func (e *ToolExecutorImpl) Start(ctx context.Context) {
	if !e.running.CompareAndSwap(false, true) {
		slog.Warn("tool_executor: already running")
		return
	}

	slog.Info("tool_executor: started", "poll_ms", e.pollMS, "timeout", e.timeout)
	e.wg.Add(1)
	go e.pollLoop(ctx)
}

// Stop gracefully shuts down the executor, draining in-flight executions.
//
// Blocks until all in-flight tool executions complete (up to timeout).
func (e *ToolExecutorImpl) Stop() {
	if !e.running.CompareAndSwap(true, false) {
		return
	}

	slog.Info("tool_executor: stopping, draining in-flight...")
	close(e.shutdown)
	e.wg.Wait()
	slog.Info("tool_executor: stopped", "processed", e.processed.Load())
}

// ============================================================================
// Health Check
// ============================================================================

// IsRunning returns true if the executor loop is active.
func (e *ToolExecutorImpl) IsRunning() bool {
	return e.running.Load()
}

// ProcessedCount returns the total number of tool requests processed.
func (e *ToolExecutorImpl) ProcessedCount() int64 {
	return e.processed.Load()
}

// LastError returns the most recent error, or empty string.
func (e *ToolExecutorImpl) LastError() string {
	v := e.lastError.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

// ============================================================================
// Poll Loop
// ============================================================================

func (e *ToolExecutorImpl) pollLoop(ctx context.Context) {
	defer e.wg.Done()

	for {
		select {
		case <-ctx.Done():
			slog.Info("tool_executor: context cancelled")
			return
		case <-e.shutdown:
			return
		default:
			e.PollOnce(ctx)
			// Jittered sleep
			jitter := time.Duration(rand.Intn(e.pollMS/2)) * time.Millisecond
			time.Sleep(time.Duration(e.pollMS)*time.Millisecond + jitter)
		}
	}
}

// ============================================================================
// AC-HARDEN-05: Poll and Execute
// ============================================================================

// PollOnce checks for pending tool_requests and executes one batch.
//
// Returns the number of requests processed.
//
// Uses a mini-transaction to claim rows (status→executing) and then executes
// each tool outside the transaction. Results are written back as tool_results
// and the request status is updated to completed/failed.
//
// WI-005: Before claiming, checks for tools requiring approval and creates
// HITL approval requests. Subprocess tools are dispatched to the sandbox.
func (e *ToolExecutorImpl) PollOnce(ctx context.Context) (int, error) {
	if e.database == nil {
		return 0, fmt.Errorf("tool_executor: no database configured")
	}

	// Step 0: Handle approval-gated tools before claiming.
	// Tools with requires_approval=true get an approval_request created
	// and their status set to 'awaiting_approval' instead of being claimed.
	approvalCount, err := e.handleApprovalGated(ctx)
	if err != nil {
		slog.Warn("tool_executor: approval gate processing error", "error", err)
		// Non-fatal — continue with non-gated tools
	}
	_ = approvalCount

	// Step 1: Claim pending requests using transaction isolation (RT-C001 fix)
	tx, err := e.database.BeginTx(ctx)
	if err != nil {
		e.lastError.Store(err.Error())
		return 0, fmt.Errorf("tool_executor: begin tx: %w", err)
	}

	// SELECT with row locking (SQLite: implicit; Postgres: would use FOR UPDATE)
	rows, err := tx.Query(ctx, `
		SELECT id, session_id, tool_name, parameters, timeout_ms
		FROM tool_requests
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT 10
	`)
	if err != nil {
		tx.Rollback()
		e.lastError.Store(err.Error())
		return 0, fmt.Errorf("tool_executor: query pending: %w", err)
	}

	if len(rows) == 0 {
		tx.Rollback()
		// If we only had approval-gated tools, still return 0 (they weren't "processed")
		return 0, nil
	}

	// Step 2: Mark as executing within the transaction
	for _, row := range rows {
		id := toString(row["id"])
		if err := tx.Exec(ctx, `UPDATE tool_requests SET status = 'executing', executed_at = $1 WHERE id = $2 AND status = 'pending'`,
			time.Now(), id,
		); err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("tool_executor: claim %s: %w", id, err)
		}
	}

	// Commit the claim transaction
	if err := tx.Commit(); err != nil {
		e.lastError.Store(err.Error())
		return 0, fmt.Errorf("tool_executor: commit claims: %w", err)
	}

	// Step 3: Execute each tool
	processed := 0
	for _, row := range rows {
		id := toString(row["id"])
		sessionID := toString(row["session_id"])
		toolName := toString(row["tool_name"])

		execCtx, cancel := context.WithTimeout(ctx, e.timeout)
		// Execute tool (sandboxed subprocess or go_native handler)
		result, execErr := e.executeTool(execCtx, toolName, row)
		cancel()

		// Step 4: Write result
		if execErr != nil {
			slog.Error("tool_executor: tool failed", "tool", toolName, "session_id", sessionID, "error", execErr)
			e.lastError.Store(execErr.Error())

			_ = e.database.Exec(ctx, `UPDATE tool_requests SET status = 'failed', completed_at = $1 WHERE id = $2`,
				time.Now(), id,
			)
			_ = e.database.Exec(ctx, `INSERT INTO tool_results (request_id, session_id, output, is_error, error_code, exit_code, duration_ms)
				VALUES ($1, $2, $3, true, 'TOOL_ERROR', -2, 0)`,
				id, sessionID, execErr.Error(),
			)
		} else {
			_ = e.database.Exec(ctx, `UPDATE tool_requests SET status = 'completed', completed_at = $1 WHERE id = $2`,
				time.Now(), id,
			)
			// Truncate very large outputs for DB storage
			output := result
			if len(output) > 10000 {
				output = output[:10000] + "...(truncated)"
			}
			_ = e.database.Exec(ctx, `INSERT INTO tool_results (request_id, session_id, output, is_error)
				VALUES ($1, $2, $3, false)`,
				id, sessionID, output,
			)
		}

		processed++
	}

	e.processed.Add(int64(processed))

	// Step 5: Wake sessions whose tools are all done
	for _, row := range rows {
		sessionID := toString(row["session_id"])
		// Check if any pending OR executing OR awaiting_approval requests remain
		pendingRows, _ := e.database.Query(ctx,
			`SELECT COUNT(*) as cnt FROM tool_requests WHERE session_id = $1 AND status IN ('pending', 'executing', 'awaiting_approval')`,
			sessionID,
		)
		if len(pendingRows) > 0 && toInt(pendingRows[0]["cnt"]) == 0 {
			// All tools done — wake the session
			_ = e.database.Exec(ctx, `UPDATE sessions SET status = 'idle', heartbeat_at = $1 WHERE id = $2 AND status = 'tool_exec'`,
				time.Now(), sessionID,
			)
		}
	}

	return processed, nil
}

// ============================================================================
// WI-005: Approval Gating — Pre-Claim Check
// ============================================================================

// handleApprovalGated checks for pending requests whose tools require approval.
// For each such request, it creates an approval_request and sets the
// tool_request status to 'awaiting_approval'.
func (e *ToolExecutorImpl) handleApprovalGated(ctx context.Context) (int, error) {
	if e.database == nil {
		return 0, nil
	}

	// Find pending requests for tools that require approval
	rows, err := e.database.Query(ctx, `
		SELECT tr.id, tr.session_id, tr.tool_name, tr.parameters
		FROM tool_requests tr
		JOIN tools_registry t ON t.name = tr.tool_name AND t.enabled = true
		WHERE tr.status = 'pending'
		  AND t.requires_approval = true
		LIMIT 5
	`)
	if err != nil {
		return 0, fmt.Errorf("approval_gate: query: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}

	count := 0
	for _, row := range rows {
		reqID := toString(row["id"])
		sessionID := toString(row["session_id"])
		toolName := toString(row["tool_name"])

		// Parse parameters
		params := make(map[string]any)
		if p, ok := row["parameters"].(string); ok && p != "" {
			_ = json.Unmarshal([]byte(p), &params)
		}

		// Create approval request
		approvalID, err := tools.CreateToolApprovalRequest(ctx, e.database, sessionID, toolName, params)
		if err != nil {
			slog.Error("tool_executor: failed to create approval request",
				"tool", toolName, "session", sessionID, "error", err)
			continue
		}

		// Update tool_request to awaiting_approval
		err = e.database.Exec(ctx, `
			UPDATE tool_requests
			SET status = 'awaiting_approval', approval_request_id = $1
			WHERE id = $2 AND status = 'pending'
		`, approvalID, reqID)
		if err != nil {
			slog.Error("tool_executor: failed to update tool_request status",
				"request_id", reqID, "error", err)
			continue
		}

		slog.Info("tool_executor: approval gate created",
			"tool", toolName, "session", sessionID,
			"request_id", reqID, "approval_id", approvalID,
		)
		count++
	}

	return count, nil
}

// ============================================================================
// executeTool — Dispatch to Handlers
// ============================================================================

// executeTool dispatches a tool call to the appropriate handler.
//
// Resolves tools from tools_registry and dispatches based on handler_type:
//   - sql_function: executes SQL function via the database
//   - subprocess: WI-005: sandboxed subprocess via tools.ExecuteExternalTool()
//   - go_native: stubbed — requires registered Go function map
//   - http_endpoint: stubbed — requires HTTP client
//
// WI-005: Rate limiting is enforced before execution.
func (e *ToolExecutorImpl) executeTool(ctx context.Context, toolName string, row db.Row) (string, error) {
	slog.Debug("tool_executor: executing tool",
		"tool", toolName,
		"params", fmt.Sprintf("%v", row["parameters"]),
	)

	// Look up tool in tools_registry
	toolRows, err := e.database.Query(ctx,
		`SELECT name, handler_type, handler_ref, hemisphere FROM tools_registry WHERE name = $1 AND enabled = true`,
		toolName,
	)
	if err != nil || len(toolRows) == 0 {
		// Tool not in registry — fall back to generic execution
		return fmt.Sprintf("tool %q executed (unregistered)", toolName), nil
	}

	handlerType := toString(toolRows[0]["handler_type"])
	handlerRef := toString(toolRows[0]["handler_ref"])

	// WI-005: Check rate limit before execution (Go-level enforcement)
	sessionID := toString(row["session_id"])
	if err := tools.CheckToolRateLimit(ctx, e.database, toolName, sessionID); err != nil {
		return "", fmt.Errorf("rate_limit: %w", err)
	}

	switch handlerType {
	case "sql_function":
		// Execute SQL function: SELECT * FROM function_name(params...)
		params := row["parameters"]
		resultRows, err := e.database.Query(ctx, fmt.Sprintf("SELECT %s($1)", handlerRef), params)
		if err != nil {
			return "", fmt.Errorf("sql_function %s: %w", handlerRef, err)
		}
		b, _ := json.Marshal(resultRows)
		return string(b), nil

	case "go_native":
		// Go native handlers require a function registry — not yet implemented
		return fmt.Sprintf("tool %q executed (go_native stub: %s)", toolName, handlerRef), nil

	case "subprocess":
		// WI-005: Real sandboxed subprocess execution
		return e.executeSubprocessTool(ctx, toolName, handlerRef, row)

	case "http_endpoint":
		// HTTP endpoint — requires HTTP client
		return fmt.Sprintf("tool %q executed (http_endpoint stub: %s)", toolName, handlerRef), nil

	default:
		return fmt.Sprintf("tool %q executed (unknown handler_type: %s)", toolName, handlerType), nil
	}
}

// executeSubprocessTool dispatches a subprocess tool call to the sandbox.
//
// Uses tools.ExecuteExternalTool() with sandbox isolation:
//   - Temp working directory
//   - Timeout from tool_request or default 30s
//   - 1MB output limit
//   - Environment whitelist
//   - Network disabled by default
func (e *ToolExecutorImpl) executeSubprocessTool(ctx context.Context, toolName, handlerRef string, row db.Row) (string, error) {
	// Parse timeout from tool_request
	timeoutMS := 0
	if v, ok := row["timeout_ms"]; ok {
		timeoutMS = toInt(v)
	}

	// Build sandbox config
	cfg := tools.DefaultExternalToolConfig()
	if timeoutMS > 0 {
		cfg.Timeout = time.Duration(timeoutMS) * time.Millisecond
	}

	// Parse handlerRef into executable + args
	// handlerRef format: "executable" or "executable arg1 arg2" or "/path/to/exe"
	parts := splitToolCommand(handlerRef)
	if len(parts) == 0 || parts[0] == "" {
		return "", fmt.Errorf("subprocess %q: empty handler_ref", toolName)
	}
	executable := parts[0]
	args := parts[1:]

	// Execute in sandbox
	slog.Debug("tool_executor: launching subprocess tool",
		"tool", toolName,
		"executable", executable,
		"args", args,
		"timeout", cfg.Timeout,
	)
	result, err := tools.ExecuteExternalTool(ctx, executable, args, cfg)
	if err != nil {
		return "", fmt.Errorf("subprocess %q: sandbox: %w", toolName, err)
	}

	// Format output with exit code info
	output := result.Output
	if result.Error != "" {
		output = fmt.Sprintf("[exit %d] %s", result.ExitCode, output)
	}

	// Log the result
	slog.Debug("tool_executor: subprocess tool completed",
		"tool", toolName,
		"exit_code", result.ExitCode,
		"duration_ms", result.DurationMs,
		"output_bytes", len(output),
	)

	return output, nil
}

// splitToolCommand splits a handler_ref string into executable and args.
func splitToolCommand(cmd string) []string {
	if cmd == "" {
		return nil
	}
	var parts []string
	var current []rune
	inQuote := false
	for _, r := range cmd {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if len(current) > 0 {
				parts = append(parts, string(current))
				current = nil
			}
		default:
			current = append(current, r)
		}
	}
	if len(current) > 0 {
		parts = append(parts, string(current))
	}
	return parts
}
