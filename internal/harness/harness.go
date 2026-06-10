// Package harness implements the agent iteration loop (SPEC-008, SPEC-020).
//
// The harness is a long-running Go process that:
//  1. Polls for ready tasks via heartbeat
//  2. Reads active context from the database
//  3. Formats Markdown for the LLM
//  4. Parses JSON responses
//  5. Executes SQL in transactions
//  6. Manages interactive multi-turn planning (SPEC-020)
//
// axiom:trace work_item=spec-006-hardening-01 spec=specs/008-harness.md,specs/006-transactions.md,specs/010-tools.md plan=phase-1/task-1 impl=internal/harness/harness.go
package harness

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wojons/conscientiousness/internal/db"
	"github.com/wojons/conscientiousness/internal/secrets"
)

// ============================================================================
// Core Iteration Types
// ============================================================================

// IterationContext holds all the data needed to run a single agent iteration.
// It is assembled from database queries (sessions, memory_events, tools_registry)
// and passed through the iteration loop.
type IterationContext struct {
	// Session identity
	SessionID   string
	AgentName   string
	ModelID     string
	TrustLevel  string // low, medium, high (SPEC-008 §5.4)

	// Task state
	Goal      string
	Status    string
	Iteration int64

	// LLM messages (system prompt + formatted Markdown context)
	Messages []Message

	// Budget & constraints
	ContextBudget         int
	TokensUsedIn          int64
	TokensUsedOut         int64
	BudgetLimitCents      int64
	BudgetUsedCents       int64
	MaxIterations         int
	MaxConsecutiveErrors  int

	// Circuit breaker state
	ConsecutiveErrors int

	// Reactive truncation (AC-MEM-H04)
	ContextRetries int // number of truncate-retry cycles for this iteration

	// Sub-agent mode (SPEC-012 §6)
	IsSubAgent bool // true if this session has a parent_id (filtered tool access)
}

// IterationResult holds the outcome of a single iteration.
type IterationResult struct {
	Status       string // "success" | "error" | "complete"
	NextStatus   string // the session status to transition to
	Error        error
	ErrorInjected string // error message injected into context for next iteration
	AuditEntry   AuditEntry
}

// ============================================================================
// LLM Message Types
// ============================================================================

// Message represents a single message in the LLM conversation.
type Message struct {
	Role    string `json:"role"`    // "system" | "user" | "assistant" | "tool"
	Content string `json:"content"` // message body
}

// ============================================================================
// Agent Output (LLM JSON Response) — SPEC-007, SPEC-008
// ============================================================================

// AgentOutput is the structured JSON that the LLM must return.
// This is validated against a strict JSON Schema before execution.
type AgentOutput struct {
	// InternalMonologue is the agent's private reasoning — it is NEVER shown
	// to the user. It is stored in audit_logs.monologue.
	InternalMonologue string `json:"internal_monologue"`

	// MemoryStateChanges are SQL statements that modify the agent's memory.
	// Each statement is classified and validated before execution.
	MemoryStateChanges []string `json:"memory_state_changes"`

	// SystemActions are session-level operations (status changes, etc).
	SystemActions []string `json:"system_actions"`

	// ToolRequests are external tool invocations requested by the agent.
	// These are written to the tool_requests table and executed outside the
	// main cognition transaction.
	ToolRequests []ToolRequest `json:"tool_requests"`

	// SubAgentSpawns are requests to fork a new sub-agent.
	SubAgentSpawns []SubAgentSpawn `json:"sub_agent_spawns"`
}

// ToolRequest describes a single external tool invocation.
type ToolRequest struct {
	ToolName string         `json:"tool_name"`
	Parameters map[string]any `json:"parameters"`
}

// SubAgentSpawn describes a sub-agent fork request.
type SubAgentSpawn struct {
	AgentName string `json:"agent_name"`
	Goal      string `json:"goal"`
	ModelID   string `json:"model_id,omitempty"`
	ParentGoal string `json:"parent_goal,omitempty"` // context about why this sub-agent was spawned
}

// ============================================================================
// Audit Entry
// ============================================================================

// AuditEntry is written to the audit_logs table after each iteration.
type AuditEntry struct {
	SessionID   string   `json:"session_id"`
	Iteration   int64    `json:"iteration"`
	Monologue   string   `json:"monologue"`
	SQLExecuted []string `json:"sql_executed"`
	Result      string   `json:"result"` // "committed" | "rolled_back"
	ErrorMessage string  `json:"error_message,omitempty"`
}

// ============================================================================
// Harness Core Interface
// ============================================================================

// Runner is the core harness execution interface.
// It abstracts the iteration loop for testability.
type Runner interface {
	// RunIteration executes a single agent iteration:
	// 1. Read context from DB
	// 2. Send to LLM
	// 3. Parse response
	// 4. Execute SQL in transaction
	// 5. Save audit
	RunIteration(ctx context.Context, sessionID string) (*IterationResult, error)
}

// ============================================================================
// Harness Implementation
// ============================================================================

// Harness is the concrete implementation of the agent runtime.
type Harness struct {
	db db.DB

	// secretStore scrubs secrets from LLM responses before audit storage.
	secretStore *secrets.Store

	// HeartbeatConfig controls task polling.
	HeartbeatConfig HeartbeatConfig

	// LLMClient is the abstract LLM caller (injected for testability).
	LLMClient LLMClient

	// BillingTracker records cost data after LLM calls (optional, may be nil for tests).
	BillingTracker BillingTracker

	// ToolExecutor runs sandboxed tool execution asynchronously (optional, may be nil).
	ToolExecutor ToolExecutor

	// inFlight tracks sessions currently inside RunInteractivePlanning.
	// Prevents the heartbeat loop from dispatching duplicate goroutines for
	// the same session (which causes SQLITE_BUSY).
	inFlight   map[string]bool
	inFlightMu sync.Mutex
}

// BillingTracker records billing rows and enforces budget limits.
// Implemented by internal/billing.Tracker.
type BillingTracker interface {
	RecordBilling(ctx context.Context, sessionID string, iteration int64, modelID string, category string, promptTokens, completionTokens, cacheReadTokens, cacheWriteTokens int64, costUSD float64)
	BudgetCheck(ctx context.Context, sessionID string, budgetLimitCents int64) (exceeded bool, err error)
	GetCumulativeCost(ctx context.Context, sessionID string) (int64, error)
}

// ToolExecutor runs pending tool requests asynchronously.
// Implemented by internal/harness.ToolExecutor.
type ToolExecutor interface {
	// PollOnce checks for pending tool_requests and executes one batch.
	PollOnce(ctx context.Context) (int, error)
	// Start begins the continuous polling loop.
	Start(ctx context.Context)
	// Stop gracefully shuts down the executor.
	Stop()
}

// HeartbeatConfig controls the background task polling loop.
type HeartbeatConfig struct {
	Interval time.Duration // polling interval, e.g. 5 * time.Second
}

// New creates a new Harness with the given dependencies.
func New(database db.DB, llm LLMClient) *Harness {
	return &Harness{
		db:             database,
		LLMClient:      llm,
		secretStore:    secrets.New(),
		HeartbeatConfig: HeartbeatConfig{
			Interval: 5 * time.Second,
		},
		inFlight: make(map[string]bool),
	}
}

// LLMClient is the abstract LLM caller interface.
// Implementations: internal/llm/client.go (OpenAI, Anthropic).
type LLMClient interface {
	// Call sends messages to the LLM and returns parsed AgentOutput with usage metadata.
	Call(ctx context.Context, messages []Message) (*LLMResponse, error)
}

// LLMResponse wraps the AgentOutput with usage data for billing.
type LLMResponse struct {
	Output      *AgentOutput `json:"output"`
	ModelID     string       `json:"model_id"`
	Usage       LLMUsage     `json:"usage"`
	DurationMs  int64        `json:"duration_ms"`
}

// LLMUsage holds token usage statistics from an LLM provider response.
type LLMUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// StartHeartbeat begins the task polling loop.
// It claims pending tasks and dispatches them to RunAgentIteration.
func StartHeartbeat() {
	// Placeholder — will be implemented in subsequent steps after
	// the database backends are wired.
}

// ============================================================================
// Cost Calculation (AC-HARDEN-01, AC-MEM-H03)
// ============================================================================

// Model pricing per 1M tokens (USD). These are fallback defaults when
// model_registry is not available (test environments, SQLite without the
// model_registry table loaded). The primary source is model_registry table.
var modelPricing = map[string]struct{ Input, Output float64 }{
	"gpt-4o":          {Input: 2.50, Output: 10.00},
	"gpt-4o-mini":     {Input: 0.15, Output: 0.60},
	"claude-sonnet-4": {Input: 3.00, Output: 15.00},
	"claude-haiku":    {Input: 0.25, Output: 1.25},
	"claude-opus":     {Input: 15.00, Output: 75.00},
	"gemini-flash":    {Input: 0.075, Output: 0.30},
	"mock-model":      {Input: 0.00, Output: 0.00},
	"default-model":   {Input: 5.00, Output: 15.00},
	"unknown":         {Input: 5.00, Output: 15.00},
}

// readModelPricing queries model_registry for the given model's pricing.
// Falls back to the hardcoded modelPricing map if the table is unavailable.
// This implements AC-MEM-H03: model routing through the database registry.
func (h *Harness) readModelPricing(ctx context.Context, modelID string) (inputPerM, outputPerM float64) {
	// Try model_registry table first
	if h.db != nil {
		rows, err := h.db.Query(ctx, `
			SELECT cost_per_m_in, cost_per_m_out
			FROM model_registry
			WHERE model_id = $1 AND enabled = true
			LIMIT 1
		`, modelID)
		if err == nil && len(rows) > 0 {
			in, _ := toFloat64(rows[0]["cost_per_m_in"])
			out, _ := toFloat64(rows[0]["cost_per_m_out"])
			if in > 0 || out > 0 {
				return in, out
			}
		}
	}

	// Fall back to hardcoded map
	if pricing, ok := modelPricing[modelID]; ok {
		return pricing.Input, pricing.Output
	}
	return modelPricing["unknown"].Input, modelPricing["unknown"].Output
}

// toFloat64 converts any to float64 with numeric type handling.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case []byte:
		var f float64
		if _, err := fmt.Sscanf(string(n), "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

// calculateCostUSD estimates the cost of an LLM call in USD from token counts.
// Queries model_registry via readModelPricing(); falls back to hardcoded map if DB unavailable.
// Cache tokens: writes cost 10% of input, reads are cost-saved (no direct charge).
func (h *Harness) calculateCostUSD(ctx context.Context, modelID string, promptTokens, completionTokens, cacheReadTokens, cacheWriteTokens int64) float64 {
	inputPrice, outputPrice := h.readModelPricing(ctx, modelID)
	inputCost := float64(promptTokens) * inputPrice / 1_000_000.0
	outputCost := float64(completionTokens) * outputPrice / 1_000_000.0
	// Cache tokens: writes cost 10% of input, reads save 50% (net zero for reads, small cost for writes)
	cacheWriteCost := float64(cacheWriteTokens) * inputPrice * 0.1 / 1_000_000.0
	_ = cacheReadTokens // cache reads are cost-saved; no direct charge

	return inputCost + outputCost + cacheWriteCost
}
