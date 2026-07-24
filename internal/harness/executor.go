// Package harness: agent iteration executor (SPEC-008 core loop).
//
// The executor runs the full iteration cycle:
//  1. Read context from DB
//  2. Send Markdown + system prompt to LLM
//  3. Parse JSON response
//  4. Execute SQL in a transaction
//  5. Save audit + snapshot
//
// Reactive context truncation (§11 of SPEC-002): catches LLM 400 context-limit
// errors, truncates the largest context event, and retries with instructions
// for the agent to use chunking or semantic search.
//
// Page-fault handler (load_memory_event, §4.3): exposes single-event recall as
// an internal action via SQL, so agents can access full content behind pointers.
//
// axiom:trace work_item=spec-002-hardening-01 spec=specs/002-memory.md,specs/008-harness.md,specs/006-transactions.md plan=phase-3/task-1 impl=internal/harness/executor.go
package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/security"
	"github.com/wojons/consensus/internal/session"
)

// DefaultTrustLevel is the trust level used when a session doesn't have one set.
const DefaultTrustLevel = "high"

// contextOverflowMarker is the text appended when truncation occurs.
const contextOverflowMarker = "\n\n[SYSTEM: Context truncated — full data saved in memory. Use SQL to chunk into smaller pages, spawn a sub-agent, or use semantic search.]"

// maxContextRetryAttempts is the max number of retry-truncate cycles.
const maxContextRetryAttempts = 3

// ============================================================================
// RunAgentIteration — the main execution loop
// ============================================================================

// RunAgentIteration executes a single agent iteration.
//
// This is the core runtime method. It:
//  1. Reads context (session + memory + tools) from the database
//  2. Checks budget limits before calling LLM (AC-HARDEN-02)
//  3. Assembles the system prompt and Markdown context
//  4. Sends messages to the LLM
//  5. Parses and validates the JSON response
//  6. Records billing row (AC-HARDEN-01)
//  7. Executes SQL in a transaction with classification + policy enforcement
//  8. Commits (or rolls back) and saves audit + snapshot
func (h *Harness) RunAgentIteration(ctx context.Context, sessionID string) (*IterationResult, error) {
	startTime := time.Now()
	slog.Info("harness: starting iteration", "session_id", sessionID)

	// Step 1: Read context
	ic, err := h.ReadActiveContext(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("iteration: read context: %w", err)
	}

	// Step 1.5: Check budget before LLM call (AC-HARDEN-02)
	if h.BillingTracker != nil {
		exceeded, err := h.BillingTracker.BudgetCheck(ctx, sessionID, ic.BudgetLimitCents)
		if err != nil {
			slog.Error("harness: budget check failed", "session_id", sessionID, "error", err)
		}
		if exceeded {
			slog.Error("harness: budget exceeded, pausing session", "session_id", sessionID)
			h.db.Exec(ctx, `UPDATE sessions SET status = 'paused' WHERE id = $1`, sessionID)
			return &IterationResult{
				Status:        "blocked",
				NextStatus:    "paused",
				ErrorInjected: fmt.Sprintf("budget exceeded: %d/%d cents used", ic.BudgetUsedCents, ic.BudgetLimitCents),
			}, nil
		}
	}

	// Step 2+3: Call LLM
	slog.Info("harness: calling LLM", "session_id", sessionID, "iteration", ic.Iteration)
	llmStart := time.Now()

	llmResp, err := h.LLMClient.Call(ctx, ic.Messages)
	if err != nil {
		slog.Error("harness: LLM call failed", "session_id", sessionID, "error", err, "elapsed", time.Since(llmStart))
		// Record billing even on error paths (RT-M001 fix)
		if h.BillingTracker != nil {
			h.BillingTracker.RecordBilling(ctx, sessionID, ic.Iteration+1, ic.ModelID, "cognition",
				0, 0, 0, 0, 0, // Zero tokens on error; cost is zero but event is recorded
			)
		}
		return h.handleLLMError(ctx, sessionID, ic, err)
	}

	elapsed := time.Since(llmStart)
	slog.Info("harness: LLM response received", "session_id", sessionID, "elapsed", elapsed, "model", llmResp.ModelID)

	// Step 3.5: Record billing (AC-HARDEN-01)
	if h.BillingTracker != nil {
		h.BillingTracker.RecordBilling(ctx, sessionID, ic.Iteration+1, llmResp.ModelID, "cognition",
			llmResp.Usage.PromptTokens, llmResp.Usage.CompletionTokens,
			llmResp.Usage.CacheReadTokens, llmResp.Usage.CacheWriteTokens,
			h.calculateCostUSD(ctx, llmResp.ModelID, llmResp.Usage.PromptTokens, llmResp.Usage.CompletionTokens,
				llmResp.Usage.CacheReadTokens, llmResp.Usage.CacheWriteTokens),
		)
	}

	output := llmResp.Output

	// Marshal LLM response for snapshot
	llmResponseJSON, _ := json.Marshal(output)

	// Step 4: Execute in transaction
	result, err := h.executeInTransaction(ctx, sessionID, ic, output, llmResponseJSON)
	if err != nil {
		return result, err
	}

	slog.Info("harness: iteration complete", "session_id", sessionID, "elapsed", time.Since(startTime), "status", result.Status)
	return result, nil
}

// ============================================================================
// Transaction Execution
// ============================================================================

type txExecutionResult struct {
	Result       string
	SQLExecuted  []string
	RowsAffected int
	Error        error
}

// executeInTransaction runs the agent's SQL inside a database transaction.
func (h *Harness) executeInTransaction(ctx context.Context, sessionID string, ic *IterationContext, output *AgentOutput, llmResponseJSON []byte) (*IterationResult, error) {
	tx, err := h.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("iteration: begin tx: %w", err)
	}

	defer func() {
		if tx.IsActive() {
			tx.Rollback()
		}
	}()

	// Set RLS context
	if err := tx.SetSessionContext(ctx, sessionID); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("iteration: set session context: %w", err)
	}

	var allSQL []string
	rowsAffected := 0

	// Determine trust level from session context (default to high for backward compat)
	trustLevel := ic.TrustLevel
	if trustLevel == "" {
		trustLevel = DefaultTrustLevel
	}

	// Classify + execute memory_state_changes
	for _, stmt := range SplitStatementsSemicolon(output.MemoryStateChanges) {
		if err := h.executeStatement(ctx, tx, stmt, sessionID, trustLevel); err != nil {
			tx.Rollback()
			return h.buildRollbackResult(sessionID, ic, output, err, allSQL, llmResponseJSON), nil
		}
		allSQL = append(allSQL, stmt)
	}

	// Classify + execute system_actions
	for _, stmt := range SplitStatementsSemicolon(output.SystemActions) {
		if err := h.executeStatement(ctx, tx, stmt, sessionID, trustLevel); err != nil {
			tx.Rollback()
			return h.buildRollbackResult(sessionID, ic, output, err, allSQL, llmResponseJSON), nil
		}
		allSQL = append(allSQL, stmt)
	}

	// Write tool_requests as pending rows
	for _, tr := range output.ToolRequests {
		stmt := fmt.Sprintf("INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status) VALUES ($1, $2, $3, $4, 'pending')")
		params, _ := json.Marshal(tr.Parameters)
		if err := tx.Exec(ctx, stmt, sessionID, ic.Iteration, tr.ToolName, string(params)); err != nil {
			tx.Rollback()
			return h.buildRollbackResult(sessionID, ic, output, err, allSQL, llmResponseJSON), nil
		}
		allSQL = append(allSQL, stmt)
	}

	// Write sub_agent_spawns as pending tasks
	for _, spawn := range output.SubAgentSpawns {
		stmt := `INSERT INTO tasks (session_id, title, description, status) VALUES ($1, $2, $3, 'pending')`
		if err := tx.Exec(ctx, stmt, sessionID, spawn.Goal, spawn.Goal); err != nil {
			tx.Rollback()
			return h.buildRollbackResult(sessionID, ic, output, err, allSQL, llmResponseJSON), nil
		}
		allSQL = append(allSQL, stmt)
	}

	// Update session status
	newStatus := h.determineNextStatus(output)
	if err := tx.Exec(ctx, `UPDATE sessions SET status = $1, heartbeat_at = datetime('now'), iteration = iteration + 1 WHERE id = $2`,
		newStatus, sessionID); err != nil {
		tx.Rollback()
		return h.buildRollbackResult(sessionID, ic, output, err, allSQL, llmResponseJSON), nil
	}

	// COMMIT
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("iteration: commit: %w", err)
	}

	// Save audit + snapshot (post-commit, best-effort)
	errs := h.FinalizeIteration(ctx, sessionID, ic.Iteration+1, output.InternalMonologue, allSQL, llmResponseJSON, rowsAffected, "committed", "")
	for _, e := range errs {
		slog.Error("harness: audit write failed", "session_id", sessionID, "error", e)
	}

	result := &IterationResult{
		Status:     "success",
		NextStatus: newStatus,
		AuditEntry: AuditEntry{
			SessionID:   sessionID,
			Iteration:   ic.Iteration + 1,
			Monologue:   output.InternalMonologue,
			SQLExecuted: allSQL,
			Result:      "committed",
		},
	}

	// Phase 2: Tool Execution (if any tools were requested)
	_ = len(output.ToolRequests)
	// Tool execution happens outside the main transaction — see tools.go

	return result, nil
}

// executeStatement classifies a SQL statement, checks tier-aware policy,
// injects secrets, and executes it (SPEC-008 §5.4).
//
// The execution tier is determined by the session's trust level:
//   - low    → Tier 1 (stored-procedure-only)
//   - medium → Tier 2 (parameterized SQL)
//   - high   → Tier 3 (raw SQL with classifier + whitelist)
func (h *Harness) executeStatement(ctx context.Context, tx interface{ Exec(context.Context, string, ...any) error }, stmt string, sessionID string, trustLevel string) error {
	// Sanitize
	stmt = security.Sanitize(stmt)
	// Fix common LLM SQL generation error: LLMs copy DEFAULT (datetime('now'))
	// from CREATE TABLE column definitions into INSERT VALUES clauses, where
	// it's invalid SQL. Replace with the bare expression.
	stmt = fixLLMSQLDefaults(stmt)

	if stmt == "" {
		return nil
	}

	// Parse trust level
	level, err := security.ParseTrustLevel(trustLevel)
	if err != nil {
		// Default to high for backward compatibility
		level = security.TrustHigh
	}

	// Enforce tier-aware execution policy
	whitelist := security.NewTableWhitelist()
	tierResult := security.EnforceTieredPolicy(stmt, level, whitelist, nil)
	if !tierResult.Allowed {
		return fmt.Errorf("tier %s policy: %s", tierResult.Tier, tierResult.Reason)
	}

	// Inject secrets from session-scoped store
	stmtWithSecrets, err := h.secretStore.Inject(stmt)
	if err != nil {
		// Unresolved aliases are logged but not fatal — the alias is left in-place
		// and the DB will reject it if it's invalid
		slog.Warn("harness: unresolved secret aliases", "session_id", sessionID, "error", err)
		stmtWithSecrets = stmt // fallback to original (uninjected) statement
	}

	// Execute
	if err := tx.Exec(ctx, stmtWithSecrets); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	return nil
}

// executeStatementLegacy is the original executeStatement signature preserved for
// callers that haven't been updated to pass trust level yet.
// It defaults to Tier 3 (high trust) for backward compatibility.
func (h *Harness) executeStatementLegacy(ctx context.Context, tx interface{ Exec(context.Context, string, ...any) error }, stmt string, sessionID string) error {
	return h.executeStatement(ctx, tx, stmt, sessionID, DefaultTrustLevel)
}

// SplitStatementsSemicolon splits a list of SQL strings on semicolons.
func SplitStatementsSemicolon(stmts []string) []string {
	return security.SplitStatements(stmts)
}

// fixLLMSQLDefaults fixes common LLM SQL generation errors before execution.
// LLMs frequently copy DEFAULT (datetime('now')) from CREATE TABLE column
// definitions into INSERT VALUES clauses, where DEFAULT (expr) is invalid.
func fixLLMSQLDefaults(stmt string) string {
	// Replace DEFAULT (datetime('now'[, args])) with bare datetime('now'[, args])
	// in INSERT VALUES contexts — the LLM copies DDL syntax into DML.
	re := regexp.MustCompile(`(?i)(VALUES\s*\([^)]*?)\bDEFAULT\s*\((\s*datetime\s*\(\s*'now'[^)]*\)\s*)\)`)
	for re.MatchString(stmt) {
		stmt = re.ReplaceAllString(stmt, `$1$2`)
	}
	return stmt
}

// determineNextStatus figures out what session status to transition to.
func (h *Harness) determineNextStatus(output *AgentOutput) string {
	if len(output.ToolRequests) > 0 {
		return string(session.StatusToolExec)
	}
	if len(output.SubAgentSpawns) > 0 {
		return string(session.StatusWaitingSub)
	}
	return string(session.StatusIdle)
}

// ============================================================================
// Error Handling
// ============================================================================

// handleLLMError handles an LLM call failure.
// Implements AC-MEM-H04: reactive context truncation when the LLM returns a
// context-limit error (400 with "context window limit" or "too many tokens").
//
// The truncation flow (§11.2 of SPEC-002):
//  1. Detect context-limit error in the LLM response
//  2. Identify the largest message (usually the user context)
//  3. Truncate it, keeping first ~70% of tokens
//  4. Append instructions for the agent to use chunking/search
//  5. Retry with truncated context (up to maxContextRetryAttempts)
func (h *Harness) handleLLMError(ctx context.Context, sessionID string, ic *IterationContext, err error) (*IterationResult, error) {
	errMsg := err.Error()

	// Check if this is a context-limit error (AC-MEM-H04)
	if isContextLimitError(errMsg) {
		if ic.ContextRetries >= maxContextRetryAttempts {
			slog.Error("harness: reactive truncation exhausted retries", "session_id", sessionID, "retries", ic.ContextRetries)
		} else if truncated := h.truncateContext(ic); truncated {
			slog.Warn("harness: context truncated, retrying LLM call", "session_id", sessionID, "retry", ic.ContextRetries+1)
			ic.ContextRetries++

			// Retry LLM call with truncated context
			llmResp, retryErr := h.LLMClient.Call(ctx, ic.Messages)
			if retryErr == nil {
				slog.Info("harness: retry succeeded with truncated context", "session_id", sessionID)
				output := llmResp.Output
				llmResponseJSON, _ := json.Marshal(output)
				result, resultErr := h.executeInTransaction(ctx, sessionID, ic, output, llmResponseJSON)
				if resultErr != nil {
					return result, resultErr
				}
				result.ErrorInjected = "Note: previous context was truncated due to token limit. Full data in memory."
				return result, nil
			}
			slog.Error("harness: retry also failed", "session_id", sessionID, "error", retryErr)
		}
	}

	// Record the error
	audit := AuditEntry{
		SessionID:    sessionID,
		Iteration:    ic.Iteration + 1,
		Monologue:    "",
		Result:       "rolled_back",
		ErrorMessage: errMsg,
	}

	// Try to write audit
	if werr := h.WriteAuditLog(ctx, &audit); werr != nil {
		slog.Error("harness: audit write failed during llm error handling", "error", werr)
	}

	return &IterationResult{
		Status:        "error",
		Error:         err,
		ErrorInjected: fmt.Sprintf("LLM call failed: %v", err),
		AuditEntry:    audit,
	}, nil
}

// ============================================================================
// AC-MEM-H04: Reactive Context Truncation (SPEC-002 §11)
// ============================================================================

// isContextLimitError checks if an error message indicates a context window limit.
// Matches common patterns from OpenAI, Anthropic, and generic LLM providers.
func isContextLimitError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	indicators := []string{
		"context length", "context window", "too many tokens",
		"token limit", "maximum context length", "exceeds the model",
		"content exceeds", "request too large", "reduce the length",
		"prompt is too long", "400", "413",
	}
	for _, indicator := range indicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
}

// truncateContext truncates the largest message in the iteration context
// (typically the user context message). Keeps approximately 60-70% of content
// and appends instructions for the agent.
//
// Returns true if truncation was applied (at least one message modified).
func (h *Harness) truncateContext(ic *IterationContext) bool {
	if len(ic.Messages) == 0 {
		return false
	}

	// Find the largest message (typically the user context)
	largestIdx := -1
	largestLen := 0
	for i, msg := range ic.Messages {
		if len(msg.Content) > largestLen {
			largestLen = len(msg.Content)
			largestIdx = i
		}
	}

	if largestIdx < 0 || largestLen < 100 {
		return false
	}

	content := ic.Messages[largestIdx].Content
	// Keep ~65% of content (rough approximation — characters, not tokens)
	keepChars := int(float64(len(content)) * 0.65)
	if keepChars < 100 {
		keepChars = 100
	}

	// Find a natural break point near the target
	breakPoint := keepChars
	for i := keepChars; i < len(content) && i < keepChars+500; i++ {
		if content[i] == '\n' {
			breakPoint = i + 1
			break
		}
	}

	truncated := content[:breakPoint] + contextOverflowMarker
	ic.Messages[largestIdx].Content = truncated

	slog.Warn("harness: context truncated", "original_len", len(content), "truncated_len", len(truncated))
	return true
}

// ============================================================================
// AC-MEM-H05: Page-Fault Handler — load_memory_event (SPEC-002 §4.3)
// ============================================================================

// LoadMemoryEvent retrieves the full content of a specific memory event.
// This is the page-fault handler — agents issue this as a MemoryStateChange SQL
// to pull full content behind a pointer into their working context.
//
// The agent writes: SELECT load_memory_event(<memory_id>) AS content
// The harness executes this within the transaction and the result becomes
// visible in the next iteration's context.
//
// In practice, agents use standard SELECT on memory_events directly:
//   SELECT content FROM memory_events WHERE id = <memory_id>
// This is the page-fault pattern documented in SPEC-002 §4.3.
//
// This function provides a convenience wrapper with error handling and
// session-scoped isolation enforcement.
func (h *Harness) LoadMemoryEvent(ctx context.Context, sessionID string, memoryID int64) (*MemoryEventInfo, error) {
	if h.db == nil {
		return nil, fmt.Errorf("load_memory_event: no database")
	}

	row, err := h.db.QueryRow(ctx, `
		SELECT me.id, me.type,
		       COALESCE(me.content, '') as content,
		       COALESCE(me.summary_text, '') as summary_text,
		       COALESCE(dm.mode, 'full') as display_mode,
		       me.iteration_created
		FROM memory_events me
		LEFT JOIN display_modes dm ON dm.memory_id = me.id
		WHERE me.id = $1 AND me.session_id = $2
	`, memoryID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load_memory_event: %w", err)
	}
	if row == nil {
		return nil, fmt.Errorf("load_memory_event: event %d not found for session %s", memoryID, sessionID)
	}

	return &MemoryEventInfo{
		ID:               toInt64(row["id"]),
		Type:             toString(row["type"]),
		Content:          toString(row["content"]),
		SummaryText:      toString(row["summary_text"]),
		DisplayMode:      toString(row["display_mode"]),
		IterationCreated: toInt64(row["iteration_created"]),
	}, nil
}

// buildRollbackResult creates an IterationResult for a rolled-back transaction.
func (h *Harness) buildRollbackResult(sessionID string, ic *IterationContext, output *AgentOutput, execErr error, allSQL []string, llmResponseJSON []byte) *IterationResult {
	slog.Warn("harness: transaction rolled back", "session_id", sessionID, "error", execErr)

	// Scrub secrets from the error message and monologue
	errMsg := execErr.Error()
	monologue := output.InternalMonologue
	if h.secretStore != nil {
		errMsg = h.secretStore.Scrub(errMsg)
		monologue = h.secretStore.Scrub(monologue)
	}

	audit := AuditEntry{
		SessionID:    sessionID,
		Iteration:    ic.Iteration + 1,
		Monologue:    monologue,
		SQLExecuted:  allSQL,
		Result:       "rolled_back",
		ErrorMessage: errMsg,
	}

	return &IterationResult{
		Status:        "error",
		Error:         execErr,
		ErrorInjected: errMsg,
		AuditEntry:    audit,
	}
}

// ============================================================================
// Heartbeat — task polling loop (SPEC-008 §Heartbeat)
// ============================================================================

// StartHeartbeatLoop begins the task polling loop. It continuously polls the
// database for pending tasks and dispatches them to RunAgentIteration.
//
// This is designed to be called as a goroutine from cmd/consensus/main.go.
func (h *Harness) StartHeartbeatLoop(ctx context.Context) {
	slog.Info("harness: heartbeat loop started", "interval", h.HeartbeatConfig.Interval)
	ticker := time.NewTicker(h.HeartbeatConfig.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("harness: heartbeat loop stopped")
			return
		case <-ticker.C:
			h.pollAndDispatch(ctx)
		}
	}
}

// pollAndDispatch checks for ready tasks and dispatches them.
func (h *Harness) pollAndDispatch(ctx context.Context) {
	// Check for sessions in planning/tool_exec that need to continue
	sessions, err := h.findActiveSessions(ctx)
	if err == nil && len(sessions) > 0 {
		for _, sid := range sessions {
			// Skip sessions already being processed in another goroutine.
			// Prevents duplicate RunInteractivePlanning calls that cause SQLITE_BUSY.
			h.inFlightMu.Lock()
			if h.inFlight[sid] {
				h.inFlightMu.Unlock()
				continue
			}
			h.inFlight[sid] = true
			h.inFlightMu.Unlock()

			slog.Info("harness: found active session", "session_id", sid)
			go func(sessionID string) {
				defer func() {
					h.inFlightMu.Lock()
					delete(h.inFlight, sessionID)
					h.inFlightMu.Unlock()
				}()

				result, err := h.RunInteractivePlanning(ctx, sessionID, nil)
				if err != nil {
					slog.Error("harness: planning failed", "session_id", sessionID, "error", err)

					// Circuit breaker: count failures and trip at 3 (AC-040).
					rows, qErr := h.db.Query(ctx,
						`SELECT COUNT(*) AS cnt FROM audit_logs WHERE session_id = $1 AND result = 'rolled_back'`,
						sessionID)
					errorCount := 1
					if qErr == nil && len(rows) > 0 {
						if v, ok := rows[0]["cnt"]; ok {
							if cv, ok2 := v.(int64); ok2 {
								errorCount = int(cv)
							}
						}
					}
					tripped, cbErr := h.CheckCircuitBreaker(ctx, sessionID, BreakerConsecutiveErrors, errorCount, 3)
					if cbErr != nil {
						slog.Error("harness: circuit breaker check failed", "session_id", sessionID, "error", cbErr)
					}
					if tripped {
						slog.Error("harness: circuit breaker tripped, pausing session", "session_id", sessionID)
						if pauseErr := h.db.Exec(ctx, `UPDATE sessions SET status = 'paused', heartbeat_at = datetime('now') WHERE id = $1`, sessionID); pauseErr != nil {
							slog.Error("harness: failed to pause session", "session_id", sessionID, "error", pauseErr)
						}
					}
				} else if result == nil {
					slog.Debug("harness: planning returned nil (session already claimed)", "session_id", sessionID)
				} else if result.NextStatus != "" {
					if err := h.db.Exec(ctx, `UPDATE sessions SET status = $1, heartbeat_at = datetime('now') WHERE id = $2`, result.NextStatus, sessionID); err != nil {
						slog.Error("harness: failed to apply session status transition", "session_id", sessionID, "next_status", result.NextStatus, "error", err)
					}
				}
			}(sid)
		}
	}

	// Only claim tasks when no planning sessions are in-flight.
	// Task claims on the same DB collide with open planning transactions.
	h.inFlightMu.Lock()
	hasInflight := len(h.inFlight) > 0
	h.inFlightMu.Unlock()

	if !hasInflight {
		task, err := h.ClaimNextReadyTask(ctx)
		if err != nil {
			slog.Error("harness: task claim failed", "error", err)
			return
		}
		if task == nil {
			return // no ready tasks
		}

		slog.Info("harness: dispatched task", "session_id", task.SessionID, "task_id", task.ID)
		go h.RunAgentIteration(ctx, task.SessionID)
	}
}

// findActiveSessions queries for sessions that need harness attention:
// thinking (just received a message), planning (multi-step), or tool_exec (external tool running).
func (h *Harness) findActiveSessions(ctx context.Context) ([]string, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id FROM sessions
		WHERE status IN ('thinking', 'planning', 'tool_exec')
		LIMIT 5
	`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, r := range rows {
		ids = append(ids, toString(r["id"]))
	}
	return ids, nil
}

// ClaimedTask is a task that has been claimed for execution.
type ClaimedTask struct {
	ID        string
	SessionID string
}

// ClaimNextReadyTask atomically claims the next pending task.
//
// On Postgres, this uses FOR UPDATE SKIP LOCKED for concurrency safety.
// On SQLite, single-writer semantics provide natural mutual exclusion.
func (h *Harness) ClaimNextReadyTask(ctx context.Context) (*ClaimedTask, error) {
	// Use FOR UPDATE SKIP LOCKED on Postgres for concurrent safety.
	// SQLite has single-writer semantics so SKIP LOCKED is unnecessary.
	lockClause := ""
	if h.db.Backend() == db.BackendPostgres {
		lockClause = " FOR UPDATE SKIP LOCKED"
	}

	query := `
		UPDATE tasks
		SET status = 'in_progress'
		WHERE status = 'pending'
		  AND id = (
		    SELECT id FROM tasks
		    WHERE status = 'pending'
		    ORDER BY priority DESC, created_at ASC
		    LIMIT 1` + lockClause + `
		  )
		RETURNING id, session_id
	`

	rows, err := h.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("claim: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	return &ClaimedTask{
		ID:        toString(rows[0]["id"]),
		SessionID: toString(rows[0]["session_id"]),
	}, nil
}
