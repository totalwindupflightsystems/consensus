// Package api: request/response types for REST API endpoints (SPEC-015).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-1/step-2-1-1 impl=internal/api/types.go
package api

import "time"

// ============================================================================
// Session Endpoint Types (SPEC-015 §3.1)
// ============================================================================

// CreateSessionRequest is the request body for POST /api/v1/sessions.
type CreateSessionRequest struct {
	AgentName     string         `json:"agent_name"`
	Goal          string         `json:"goal"`
	ModelID       string         `json:"model_id,omitempty"`
	ContextBudget int            `json:"context_budget,omitempty"`
	HITLConfig    map[string]any `json:"hitl_config,omitempty"`
	ProjectID     string         `json:"project_id,omitempty"` // Project scope (empty = Global)
}

// CreateSessionResponse is the response body for POST /api/v1/sessions.
type CreateSessionResponse struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	APIKey    string    `json:"api_key"`
	ModelID   string    `json:"model,omitempty"`
	ProjectID string    `json:"project_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionResponse is the response body for GET /api/v1/sessions/:id.
type SessionResponse struct {
	ID            string  `json:"id"`
	ParentID      *string `json:"parent_id,omitempty"`
	AgentName     string  `json:"agent_name"`
	ModelID       string  `json:"model_id"`
	Status        string  `json:"status"`
	Goal          *string `json:"goal,omitempty"`
	ContextBudget int     `json:"context_budget"`
	TokensUsedIn  int64   `json:"tokens_used_in"`
	TokensUsedOut int64   `json:"tokens_used_out"`
	Iteration     int64   `json:"iteration"`
	ProjectID     *string `json:"project_id,omitempty"`
	HeartbeatAt   string  `json:"heartbeat_at"`
	CreatedAt     string  `json:"created_at"`
	CompletedAt   *string `json:"completed_at,omitempty"`
	LastMessage   *string `json:"last_message,omitempty"` // most recent assistant response
	LastError     *string `json:"last_error,omitempty"`   // most recent audit_logs error_message (DOGFOOD-004)
}

// UpdateSessionRequest is the request body for PATCH /api/v1/sessions/:id.
type UpdateSessionRequest struct {
	Status *string `json:"status,omitempty"` // "pause", "resume", "cancel"
}

// SendMessageRequest is the request body for POST /api/v1/sessions/:id/message.
type SendMessageRequest struct {
	Content string `json:"content"`
	Type    string `json:"type,omitempty"` // "user_instruction" — default
}

// ============================================================================
// Tool & Skill Endpoint Types (SPEC-015 §3.4)
// ============================================================================

// ToolResponse is the response body for GET /api/v1/tools.
type ToolResponse struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Hemisphere       string `json:"hemisphere"`
	HandlerType      string `json:"handler_type"`
	Status           string `json:"status"`
	Enabled          bool   `json:"enabled"`
	RequiresApproval bool   `json:"requires_approval"`
}

// SkillResponse is the response body for GET /api/v1/skills.
type SkillResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Metadata any    `json:"metadata"`
	Enabled  bool   `json:"enabled"`
}

// SkillDetailResponse is the response body for GET /api/v1/skills/:name.
type SkillDetailResponse struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Metadata      any      `json:"metadata"`
	Instructions  string   `json:"instructions"`
	LinkedToolIDs []string `json:"linked_tool_ids"`
	Enabled       bool     `json:"enabled"`
}

// ExecuteToolRequest is the request body for POST /api/v1/tools/:name/execute.
type ExecuteToolRequest struct {
	SessionID  string         `json:"session_id"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// ExecuteToolResponse is the response body for POST /api/v1/tools/:name/execute.
type ExecuteToolResponse struct {
	ToolName     string `json:"tool_name"`
	Result       any    `json:"result,omitempty"`
	RowsAffected int64  `json:"rows_affected,omitempty"`
	IsError      bool   `json:"is_error"`
	Error        string `json:"error,omitempty"`
}

// ============================================================================
// Config & Metrics Endpoint Types (SPEC-015 §3.6, §3.7)
// ============================================================================

// ConfigResponse is the response body for GET /api/v1/config.
type ConfigResponse struct {
	LLM            any            `json:"llm,omitempty"`
	HITL           any            `json:"hitl,omitempty"`
	Harness        any            `json:"harness,omitempty"`
	Database       any            `json:"database,omitempty"`
	Logging        any            `json:"logging,omitempty"`
	SystemSettings map[string]any `json:"system_settings,omitempty"`
}

// MetricsResponse is the response body for GET /api/v1/metrics.
type MetricsResponse struct {
	ActiveSessions   int     `json:"active_sessions"`
	PendingTasks     int     `json:"pending_tasks"`
	PendingApprovals int     `json:"pending_approvals"`
	TotalSessions    int     `json:"total_sessions"`
	TotalCostUSD     float64 `json:"total_cost_usd,omitempty"`
}

// ============================================================================
// Memory Endpoint Types (SPEC-015 §3.2)
// ============================================================================

// MemoryEventResponse is the response body for GET /api/v1/sessions/:id/memory and .../:mid.
type MemoryEventResponse struct {
	ID               int64   `json:"id"`
	Type             string  `json:"type"`
	Content          string  `json:"content,omitempty"`
	SummaryText      *string `json:"summary_text,omitempty"`
	SessionID        string  `json:"session_id"`
	IterationCreated int64   `json:"iteration_created"`
	DisplayMode      string  `json:"display_mode,omitempty"`
	CreatedAt        string  `json:"created_at"`
}

// ActiveContextResponse is the response body for GET /api/v1/sessions/:id/context.
// Represents a single row from the active context view (rendered).
type ActiveContextResponse struct {
	ID               int64   `json:"id"`
	IterationCreated int64   `json:"iteration_created"`
	Type             string  `json:"type"`
	DisplayMode      string  `json:"display_mode"`
	RenderedText     *string `json:"rendered_text,omitempty"`
}

// IterationCommitResponse is the response body for GET /api/v1/sessions/:id/iterations.
type IterationCommitResponse struct {
	IterationID    int64    `json:"iteration_id"`
	SessionID      string   `json:"session_id"`
	ActivePointers []int64  `json:"active_pointers"`
	DisplayRules   any      `json:"display_rules"`
	LLMResponse    any      `json:"llm_response,omitempty"`
	SQLExecuted    []string `json:"sql_executed,omitempty"`
	RowsAffected   int      `json:"rows_affected"`
	CreatedAt      string   `json:"created_at"`
}

// ============================================================================
// Task Endpoint Types (SPEC-015 §3.3)
// ============================================================================

// CreateTaskRequest is the request body for POST /api/v1/sessions/:id/tasks.
type CreateTaskRequest struct {
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	Priority        int      `json:"priority,omitempty"`
	PrerequisiteIDs []string `json:"prerequisite_ids,omitempty"`
}

// TaskResponse is the response body for task CRUD endpoints.
type TaskResponse struct {
	ID              string   `json:"id"`
	SessionID       string   `json:"session_id"`
	ParentTaskID    *string  `json:"parent_task_id,omitempty"`
	Title           string   `json:"title"`
	Description     *string  `json:"description,omitempty"`
	Status          string   `json:"status"`
	Priority        int      `json:"priority"`
	LockedByAgent   *string  `json:"locked_by_agent,omitempty"`
	PrerequisiteIDs []string `json:"prerequisite_ids"`
	ResultMemoryID  *int64   `json:"result_memory_id,omitempty"`
	CreatedAt       string   `json:"created_at"`
	ClaimedAt       *string  `json:"claimed_at,omitempty"`
	CompletedAt     *string  `json:"completed_at,omitempty"`
}

// UpdateTaskRequest is the request body for PATCH /api/v1/tasks/:tid.
type UpdateTaskRequest struct {
	Status *string `json:"status,omitempty"`
}

// ============================================================================
// Health Endpoint Types
// ============================================================================

// HealthResponse is the response body for GET /api/v1/health.
type HealthResponse struct {
	Status            string            `json:"status"`
	Version           string            `json:"version"`
	UptimeSeconds     int64             `json:"uptime_seconds"`
	APILatencyMs      float64           `json:"api_latency_ms"`
	DBLatencyMs       float64           `json:"db_latency_ms"`
	LLMLatencyMs      float64           `json:"llm_latency_ms"`
	ErrorRatePct      float64           `json:"error_rate_pct"`
	DBBackend         string            `json:"db_backend"`
	DBPath            string            `json:"db_path"`
	DBSizeMB          float64           `json:"db_size_mb"`
	DBTables          int               `json:"db_tables"`
	DBMigrations      int               `json:"db_migrations"`
	SchemaVersion     int               `json:"schema_version"`
	ActiveConnections ActiveConnections `json:"active_connections"`
	SystemLog         []string          `json:"system_log"`
}

// ActiveConnections describes current connection counts.
type ActiveConnections struct {
	WebSocket          int `json:"websocket"`
	DBPoolActive       int `json:"db_pool_active"`
	DBPoolMax          int `json:"db_pool_max"`
	LLMActive          int `json:"llm_active"`
	APIRequestsLastMin int `json:"api_requests_last_min"`
}

// ============================================================================
// Approval Endpoint Types (SPEC-015 §3.5)
// ============================================================================

// ApprovalResponse is the response body for GET /api/v1/approvals, /api/v1/approvals/:id,
// and GET /api/v1/sessions/:id/approvals.
type ApprovalResponse struct {
	ID          string  `json:"id"`
	SessionID   string  `json:"session_id"`
	Iteration   int64   `json:"iteration"`
	RequestType string  `json:"request_type"`
	Description string  `json:"description"`
	RiskLevel   string  `json:"risk_level"`
	Context     any     `json:"context,omitempty"`
	TargetTool  *string `json:"target_tool,omitempty"`
	TargetSQL   *string `json:"target_sql,omitempty"`
	Status      string  `json:"status"`
	ReviewerID  *string `json:"reviewer_id,omitempty"`
	ReviewNotes *string `json:"review_notes,omitempty"`
	ModifiedSQL *string `json:"modified_sql,omitempty"`
	CreatedAt   string  `json:"created_at"`
	ReviewedAt  *string `json:"reviewed_at,omitempty"`
	ExpiresAt   *string `json:"expires_at,omitempty"`
}

// ApprovalReviewRequest is the request body for POST /api/v1/approvals/:id/review.
type ApprovalReviewRequest struct {
	Decision    string `json:"decision"` // "approved", "rejected", "modified"
	Notes       string `json:"notes,omitempty"`
	ModifiedSQL string `json:"modified_sql,omitempty"`
}
