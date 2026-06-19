// Package hitl implements human-in-the-loop approvals and notifications for
// Consensus (SPEC-014).
//
// HITL is the mechanism for humans to inject themselves into the agent
// execution loop. Agents pause and request approval for risky operations;
// humans review and approve/reject/modify via CLI, API, or dashboard.
//
// Key properties:
//   - No auto-approval — every pending request requires explicit human action
//   - Expiry, not auto-approval — expired requests become 'expired', never 'approved'
//   - Reviewer auth — only alt_mode_role users can review
//   - Scope precedence — session config overrides global defaults
//   - Six request types — tool_execution, destructive_action, budget_override,
//     schema_change, sub_agent_spawn, custom
//
// axiom:trace work_item=deployment-ops-01 spec=specs/014-hitl-interrupt-state.md plan=phase-3/task-3-1/step-3-1-1
package hitl

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/session"
)

// ============================================================================
// Types
// ============================================================================

// RequestType enumerates the six types of HITL approval requests.
type RequestType string

const (
	RequestToolExecution     RequestType = "tool_execution"
	RequestDestructiveAction RequestType = "destructive_action"
	RequestBudgetOverride    RequestType = "budget_override"
	RequestSchemaChange      RequestType = "schema_change"
	RequestSubAgentSpawn     RequestType = "sub_agent_spawn"
	RequestCustom            RequestType = "custom"
)

// IsValidRequestType checks if the given string is a known HITL request type.
func IsValidRequestType(t string) bool {
	switch RequestType(t) {
	case RequestToolExecution, RequestDestructiveAction, RequestBudgetOverride,
		RequestSchemaChange, RequestSubAgentSpawn, RequestCustom:
		return true
	}
	return false
}

// RiskLevel indicates the severity of a requested action.
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

func IsValidRiskLevel(r string) bool {
	switch RiskLevel(r) {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
		return true
	}
	return false
}

// Decision enumerates the possible outcomes of an approval review.
type Decision string

const (
	DecisionApproved Decision = "approved"
	DecisionRejected Decision = "rejected"
	DecisionModified Decision = "modified"
)

// ApprovalStatus is the lifecycle status of an approval request.
type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusRejected ApprovalStatus = "rejected"
	ApprovalStatusExpired  ApprovalStatus = "expired"
	ApprovalStatusModified ApprovalStatus = "modified"
)

// ============================================================================
// Approval Request
// ============================================================================

// ApprovalRequest represents a pending human approval decision.
type ApprovalRequest struct {
	ID           string         `json:"id"`
	SessionID    string         `json:"session_id"`
	Iteration    int64          `json:"iteration"`
	RequestType  RequestType    `json:"request_type"`
	Description  string         `json:"description"`
	RiskLevel    RiskLevel      `json:"risk_level"`
	Context      string         `json:"context,omitempty"`
	TargetTool   string         `json:"target_tool,omitempty"`
	TargetSQL    string         `json:"target_sql,omitempty"`
	Status       ApprovalStatus `json:"status"`
	ReviewerID   string         `json:"reviewer_id,omitempty"`
	ReviewNotes  string         `json:"review_notes,omitempty"`
	ModifiedSQL  string         `json:"modified_sql,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	ReviewedAt   *time.Time     `json:"reviewed_at,omitempty"`
	ExpiresAt    time.Time      `json:"expires_at"`
}

// ============================================================================
// HITL Configuration
// ============================================================================

// Scope determines whether configuration is global or session-specific.
type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeSession Scope = "session"
)

// Configuration holds HITL behavior settings.
type Configuration struct {
	Scope                            Scope  `json:"scope"`
	SessionID                        string `json:"session_id,omitempty"`
	AutoPauseOnErrorThreshold        int    `json:"auto_pause_on_error_threshold"`
	RequireApprovalForDestructive    bool   `json:"require_approval_for_destructive"`
	RequireApprovalForSchemaChanges  bool   `json:"require_approval_for_schema_changes"`
	RequireApprovalForExternalTools  bool   `json:"require_approval_for_external_tools"`
	ApprovalTimeoutMinutes           int    `json:"approval_timeout_minutes"`
	NotifyOnPause                    bool   `json:"notify_on_pause"`
}

// DefaultConfiguration returns sensible global defaults.
func DefaultConfiguration() Configuration {
	return Configuration{
		Scope:                            ScopeGlobal,
		AutoPauseOnErrorThreshold:        3,
		RequireApprovalForDestructive:    true,
		RequireApprovalForSchemaChanges:  true,
		RequireApprovalForExternalTools:  false,
		ApprovalTimeoutMinutes:           60,
		NotifyOnPause:                    true,
	}
}

// ============================================================================
// Manager
// ============================================================================

// Manager is the core HITL orchestration component.
type Manager struct {
	database      db.DB
	subscriptions map[string][]chan Notification
	mu            sync.RWMutex
}

// New creates a new HITL manager backed by the given database.
func New(database db.DB) *Manager {
	return &Manager{
		database:      database,
		subscriptions: make(map[string][]chan Notification),
	}
}

// ============================================================================
// AC-HITL-01: approval_requests creation (all 6 types)
// ============================================================================

// RequestApproval creates a new approval request and pauses the session.
// This is the primary entry point for agents to request human approval.
func (m *Manager) RequestApproval(ctx context.Context, sessionID string, requestType RequestType, description string, riskLevel RiskLevel, opts ...RequestOption) (*ApprovalRequest, error) {
	if !IsValidRequestType(string(requestType)) {
		return nil, fmt.Errorf("hitl: invalid request type: %q", requestType)
	}
	if !IsValidRiskLevel(string(riskLevel)) {
		return nil, fmt.Errorf("hitl: invalid risk level: %q", riskLevel)
	}
	if description == "" {
		return nil, fmt.Errorf("hitl: description is required")
	}

	// Get the effective HITL configuration (session overrides global)
	cfg, err := m.GetEffectiveConfiguration(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("hitl: get config: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(cfg.ApprovalTimeoutMinutes) * time.Minute)
	requestID := uuid.New().String()

	req := &ApprovalRequest{
		ID:          requestID,
		SessionID:   sessionID,
		RequestType: requestType,
		Description: description,
		RiskLevel:   riskLevel,
		Status:      ApprovalStatusPending,
		ExpiresAt:   expiresAt,
	}

	// Apply options (target_tool, target_sql, context)
	for _, opt := range opts {
		opt(req)
	}

	// Insert into database
	err = m.database.Exec(ctx, `
		INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, context, target_tool, target_sql, status, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, requestID, sessionID, 0, string(requestType), description, string(riskLevel), req.Context, req.TargetTool, req.TargetSQL, string(ApprovalStatusPending), expiresAt, time.Now())
	if err != nil {
		return nil, fmt.Errorf("hitl: insert approval request: %w", err)
	}

	// Pause the session
	if err := m.PauseSession(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("hitl: pause session: %w", err)
	}

	// Send notifications
	if cfg.NotifyOnPause {
		m.notify(ctx, Notification{
			SessionID:    sessionID,
			ApprovalType: string(requestType),
			RiskLevel:    string(riskLevel),
			Description:  description,
		})
	}

	return req, nil
}

// RequestOption modifies an approval request during creation.
type RequestOption func(*ApprovalRequest)

// WithTargetTool sets the target tool for the approval request.
func WithTargetTool(tool string) RequestOption {
	return func(r *ApprovalRequest) { r.TargetTool = tool }
}

// WithTargetSQL sets the target SQL for the approval request.
func WithTargetSQL(sql string) RequestOption {
	return func(r *ApprovalRequest) { r.TargetSQL = sql }
}

// WithContext sets additional context for the approval request.
func WithContext(ctxJSON string) RequestOption {
	return func(r *ApprovalRequest) { r.Context = ctxJSON }
}

// ============================================================================
// AC-HITL-03, AC-HITL-04: Review approval
// ============================================================================

// ReviewApproval processes a human decision on a pending approval request.
// Only users with alt_mode_role should call this (enforced at API/auth layer).
func (m *Manager) ReviewApproval(ctx context.Context, approvalID string, decision Decision, reviewerID string, reviewNotes string, modifiedSQL string) error {
	if !isValidDecision(decision) {
		return fmt.Errorf("hitl: invalid decision: %q", decision)
	}

	// Get the approval request
	rows, err := m.database.Query(ctx, `SELECT session_id, status FROM approval_requests WHERE id = $1`, approvalID)
	if err != nil {
		return fmt.Errorf("hitl: query approval: %w", err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("hitl: approval request %q not found", approvalID)
	}

	currentStatus := rows[0]["status"].(string)
	sessionID := rows[0]["session_id"].(string)

	if currentStatus != string(ApprovalStatusPending) {
		return fmt.Errorf("hitl: approval %q is not pending (current: %s)", approvalID, currentStatus)
	}

	now := time.Now()

	// Update the approval request
	err = m.database.Exec(ctx, `
		UPDATE approval_requests
		SET status = $1, reviewer_id = $2, review_notes = $3, modified_sql = $4, reviewed_at = $5
		WHERE id = $6
	`, string(decision), reviewerID, reviewNotes, modifiedSQL, now, approvalID)
	if err != nil {
		return fmt.Errorf("hitl: update approval: %w", err)
	}

	// Resume the session
	if err := m.ResumeSession(ctx, sessionID); err != nil {
		return fmt.Errorf("hitl: resume session: %w", err)
	}

	return nil
}

// ============================================================================
// AC-HITL-03: No auto-approval — expiry handling
// ============================================================================

// ExpirePendingApprovals marks all expired pending requests as 'expired'.
// This runs on a cron schedule (every 5 minutes by default).
// Returns the number of requests expired.
func (m *Manager) ExpirePendingApprovals(ctx context.Context) (int, error) {
	// Count first for reliable return value across backends
	rows, err := m.database.Query(ctx, `SELECT COUNT(*) as cnt FROM approval_requests WHERE status = 'pending' AND expires_at < $1`, time.Now())
	if err != nil {
		return 0, fmt.Errorf("hitl: expire approvals count: %w", err)
	}
	count := 0
	if len(rows) > 0 {
		count = toInt(rows[0]["cnt"])
	}

	if count == 0 {
		return 0, nil
	}

	err = m.database.Exec(ctx, `
		UPDATE approval_requests
		SET status = 'expired'
		WHERE status = 'pending' AND expires_at < $1
	`, time.Now())
	if err != nil {
		return 0, fmt.Errorf("hitl: expire approvals: %w", err)
	}

	// Fail sessions that are paused with only expired approvals
	err = m.database.Exec(ctx, `
		UPDATE sessions SET status = 'failed', completed_at = $1
		WHERE id IN (
			SELECT DISTINCT session_id FROM approval_requests
			WHERE status = 'expired'
		) AND status = 'paused'
	`, time.Now())
	if err != nil {
		return count, fmt.Errorf("hitl: fail expired sessions: %w", err)
	}

	return count, nil
}

// ============================================================================
// AC-HITL-02: hitl_configuration with scope precedence
// ============================================================================

// SetConfiguration creates or updates HITL configuration for a given scope.
func (m *Manager) SetConfiguration(ctx context.Context, cfg Configuration) error {
	if cfg.Scope == ScopeGlobal {
		// Upsert global config — use DELETE+INSERT for database portability
		// rather than ON CONFLICT which varies between Postgres and SQLite
		_ = m.database.Exec(ctx, `DELETE FROM hitl_configuration WHERE scope = 'global'`)
		return m.database.Exec(ctx, `
			INSERT INTO hitl_configuration (scope, auto_pause_on_error_threshold, require_approval_for_destructive, require_approval_for_schema_changes, require_approval_for_external_tools, approval_timeout_minutes, notify_on_pause, created_at)
			VALUES ('global', $1, $2, $3, $4, $5, $6, $7)
		`, cfg.AutoPauseOnErrorThreshold, toBoolInt(cfg.RequireApprovalForDestructive),
			toBoolInt(cfg.RequireApprovalForSchemaChanges), toBoolInt(cfg.RequireApprovalForExternalTools),
			cfg.ApprovalTimeoutMinutes, toBoolInt(cfg.NotifyOnPause),
			time.Now())
	}

	if cfg.Scope == ScopeSession {
		if cfg.SessionID == "" {
			return fmt.Errorf("hitl: session_id required for session-scoped config")
		}
		_ = m.database.Exec(ctx, `DELETE FROM hitl_configuration WHERE scope = 'session' AND session_id = $1`, cfg.SessionID)
		return m.database.Exec(ctx, `
			INSERT INTO hitl_configuration (scope, session_id, auto_pause_on_error_threshold, require_approval_for_destructive, require_approval_for_schema_changes, require_approval_for_external_tools, approval_timeout_minutes, notify_on_pause, created_at)
			VALUES ('session', $1, $2, $3, $4, $5, $6, $7, $8)
		`, cfg.SessionID, cfg.AutoPauseOnErrorThreshold, toBoolInt(cfg.RequireApprovalForDestructive),
			toBoolInt(cfg.RequireApprovalForSchemaChanges), toBoolInt(cfg.RequireApprovalForExternalTools),
			cfg.ApprovalTimeoutMinutes, toBoolInt(cfg.NotifyOnPause),
			time.Now())
	}

	return fmt.Errorf("hitl: invalid scope: %q", cfg.Scope)
}

// GetEffectiveConfiguration returns the effective HITL config for a session.
// Session-scoped config takes precedence over global defaults.
func (m *Manager) GetEffectiveConfiguration(ctx context.Context, sessionID string) (Configuration, error) {
	// Try session-scoped first
	if sessionID != "" {
		rows, err := m.database.Query(ctx, `
			SELECT auto_pause_on_error_threshold, require_approval_for_destructive, require_approval_for_schema_changes, require_approval_for_external_tools, approval_timeout_minutes, notify_on_pause
			FROM hitl_configuration WHERE scope = 'session' AND session_id = $1
		`, sessionID)
		if err == nil && len(rows) > 0 {
			return rowToConfig(ScopeSession, sessionID, rows[0]), nil
		}
	}

	// Fall back to global
	rows, err := m.database.Query(ctx, `
		SELECT auto_pause_on_error_threshold, require_approval_for_destructive, require_approval_for_schema_changes, require_approval_for_external_tools, approval_timeout_minutes, notify_on_pause
		FROM hitl_configuration WHERE scope = 'global' LIMIT 1
	`)
	if err != nil || len(rows) == 0 {
		return DefaultConfiguration(), nil
	}

	return rowToConfig(ScopeGlobal, "", rows[0]), nil
}

// ============================================================================
// AC-HITL-05: Approval expiry cron
// ============================================================================

// StartExpiryCron spawns a goroutine that periodically expires pending approvals.
// stopCh is closed when the server shuts down.
func (m *Manager) StartExpiryCron(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				count, err := m.ExpirePendingApprovals(ctx)
				if err != nil {
					slog.Error("hitl: expiry cron error", "error", err)
					continue
				}
				if count > 0 {
					slog.Info("hitl: expired pending approvals", "count", count)
				}
			}
		}
	}()
}

// ============================================================================
// AC-HITL-06: Notification channels
// ============================================================================

// Notification represents a HITL notification to be sent to a human reviewer.
type Notification struct {
	SessionID    string `json:"session_id"`
	ApprovalType string `json:"approval_type"`
	RiskLevel    string `json:"risk_level"`
	Description  string `json:"description"`
	Channel      string `json:"channel,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// notificationCallback is a function that receives HITL notifications.
type notificationCallback func(Notification)

var notificationCallbacks []notificationCallback
var notifyMu sync.Mutex

// RegisterNotificationCallback registers a callback for HITL notifications.
// This is used by external systems (SSE, Slack, email) to receive real-time alerts.
func RegisterNotificationCallback(cb notificationCallback) {
	notifyMu.Lock()
	defer notifyMu.Unlock()
	notificationCallbacks = append(notificationCallbacks, cb)
}

func (m *Manager) notify(ctx context.Context, n Notification) {
	n.Timestamp = time.Now()

	// Log to notification_log
	_ = m.database.Exec(ctx, `
		INSERT INTO notification_log (approval_id, channel, recipient, sent_at, delivered)
		VALUES ($1, $2, $3, $4, $5)
	`, n.SessionID, "dashboard", "admin", n.Timestamp, true)

	// Fire registered callbacks
	notifyMu.Lock()
	defer notifyMu.Unlock()
	for _, cb := range notificationCallbacks {
		cb(n)
	}
}

// ============================================================================
// Session Management
// ============================================================================

// PauseSession transitions a session to paused state.
func (m *Manager) PauseSession(ctx context.Context, sessionID string) error {
	current, err := m.getSessionStatus(ctx, sessionID)
	if err != nil {
		return err
	}

	if !current.IsPausable() {
		return fmt.Errorf("hitl: cannot pause session %q with status %q", sessionID, current)
	}

	return m.setSessionStatus(ctx, sessionID, session.StatusPaused)
}

// ResumeSession transitions a session back to idle from paused.
func (m *Manager) ResumeSession(ctx context.Context, sessionID string) error {
	current, err := m.getSessionStatus(ctx, sessionID)
	if err != nil {
		return err
	}

	if current != session.StatusPaused {
		return fmt.Errorf("hitl: cannot resume non-paused session %q (current: %q)", sessionID, current)
	}

	return m.setSessionStatus(ctx, sessionID, session.StatusIdle)
}

// ============================================================================
// List / Query
// ============================================================================

// ListPendingApprovals returns all pending (not yet reviewed) approval requests.
func (m *Manager) ListPendingApprovals(ctx context.Context) ([]ApprovalRequest, error) {
	return m.ListApprovalsByStatus(ctx, ApprovalStatusPending)
}

// ListApprovalsByStatus returns approval requests filtered by status.
func (m *Manager) ListApprovalsByStatus(ctx context.Context, status ApprovalStatus) ([]ApprovalRequest, error) {
	rows, err := m.database.Query(ctx, `
		SELECT id, session_id, iteration, request_type, description, risk_level, context, target_tool, target_sql, status, reviewer_id, review_notes, modified_sql, created_at, reviewed_at, expires_at
		FROM approval_requests WHERE status = $1
		ORDER BY
		  CASE risk_level
		    WHEN 'critical' THEN 0 WHEN 'high' THEN 1
		    WHEN 'medium' THEN 2 WHEN 'low' THEN 3
		  END,
		  created_at ASC
	`, string(status))
	if err != nil {
		return nil, fmt.Errorf("hitl: list approvals: %w", err)
	}

	var result []ApprovalRequest
	for _, row := range rows {
		req, err := rowToApproval(row)
		if err != nil {
			continue
		}
		result = append(result, *req)
	}
	return result, nil
}

// ListPendingForSession returns pending approvals for a specific session,
// ordered by risk level (highest first) then creation time.
func (m *Manager) ListPendingForSession(ctx context.Context, sessionID string) ([]ApprovalRequest, error) {
	rows, err := m.database.Query(ctx, `
		SELECT id, session_id, iteration, request_type, description, risk_level, context, target_tool, target_sql, status, reviewer_id, review_notes, modified_sql, created_at, reviewed_at, expires_at
		FROM approval_requests WHERE session_id = $1 AND status = 'pending'
		ORDER BY
		  CASE risk_level
		    WHEN 'critical' THEN 0 WHEN 'high' THEN 1
		    WHEN 'medium' THEN 2 WHEN 'low' THEN 3
		  END,
		  created_at ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("hitl: list session approvals: %w", err)
	}

	var result []ApprovalRequest
	for _, row := range rows {
		req, err := rowToApproval(row)
		if err != nil {
			continue
		}
		result = append(result, *req)
	}
	return result, nil
}

// ListSessionApprovals returns all approvals for a session (any status),
// ordered by creation time descending.
func (m *Manager) ListSessionApprovals(ctx context.Context, sessionID string) ([]ApprovalRequest, error) {
	rows, err := m.database.Query(ctx, `
		SELECT id, session_id, iteration, request_type, description, risk_level, context, target_tool, target_sql, status, reviewer_id, review_notes, modified_sql, created_at, reviewed_at, expires_at
		FROM approval_requests WHERE session_id = $1
		ORDER BY created_at DESC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("hitl: list session approvals: %w", err)
	}

	var result []ApprovalRequest
	for _, row := range rows {
		req, err := rowToApproval(row)
		if err != nil {
			continue
		}
		result = append(result, *req)
	}
	return result, nil
}

// ListSessionApprovalsByStatus returns approvals for a session filtered by a specific status.
func (m *Manager) ListSessionApprovalsByStatus(ctx context.Context, sessionID string, status ApprovalStatus) ([]ApprovalRequest, error) {
	rows, err := m.database.Query(ctx, `
		SELECT id, session_id, iteration, request_type, description, risk_level, context, target_tool, target_sql, status, reviewer_id, review_notes, modified_sql, created_at, reviewed_at, expires_at
		FROM approval_requests WHERE session_id = $1 AND status = $2
		ORDER BY created_at DESC
	`, sessionID, string(status))
	if err != nil {
		return nil, fmt.Errorf("hitl: list session approvals by status: %w", err)
	}

	var result []ApprovalRequest
	for _, row := range rows {
		req, err := rowToApproval(row)
		if err != nil {
			continue
		}
		result = append(result, *req)
	}
	return result, nil
}

// GetApproval retrieves a single approval request by ID.
func (m *Manager) GetApproval(ctx context.Context, id string) (*ApprovalRequest, error) {
	rows, err := m.database.Query(ctx, `
		SELECT id, session_id, iteration, request_type, description, risk_level, context, target_tool, target_sql, status, reviewer_id, review_notes, modified_sql, created_at, reviewed_at, expires_at
		FROM approval_requests WHERE id = $1
	`, id)
	if err != nil {
		return nil, fmt.Errorf("hitl: get approval: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("hitl: approval %q not found", id)
	}
	return rowToApproval(rows[0])
}

// ============================================================================
// Internal Helpers
// ============================================================================

func (m *Manager) getSessionStatus(ctx context.Context, sessionID string) (session.Status, error) {
	rows, err := m.database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		return "", fmt.Errorf("hitl: get session status: %w", err)
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("hitl: session %q not found", sessionID)
	}
	status := rows[0]["status"].(string)
	return session.Status(status), nil
}

func (m *Manager) setSessionStatus(ctx context.Context, sessionID string, status session.Status) error {
	now := time.Now()
	if status.IsTerminal() {
		return m.database.Exec(ctx, `UPDATE sessions SET status = $1, heartbeat_at = $2, completed_at = $2 WHERE id = $3`, string(status), now, sessionID)
	}
	return m.database.Exec(ctx, `UPDATE sessions SET status = $1, heartbeat_at = $2 WHERE id = $3`, string(status), now, sessionID)
}

func isValidDecision(d Decision) bool {
	switch d {
	case DecisionApproved, DecisionRejected, DecisionModified:
		return true
	}
	return false
}

func rowToConfig(scope Scope, sessionID string, row db.Row) Configuration {
	cfg := Configuration{
		Scope:     scope,
		SessionID: sessionID,
	}

	if v, ok := row["auto_pause_on_error_threshold"]; ok {
		cfg.AutoPauseOnErrorThreshold = toInt(v)
	}
	if v, ok := row["require_approval_for_destructive"]; ok {
		cfg.RequireApprovalForDestructive = toBool(v)
	}
	if v, ok := row["require_approval_for_schema_changes"]; ok {
		cfg.RequireApprovalForSchemaChanges = toBool(v)
	}
	if v, ok := row["require_approval_for_external_tools"]; ok {
		cfg.RequireApprovalForExternalTools = toBool(v)
	}
	if v, ok := row["approval_timeout_minutes"]; ok {
		cfg.ApprovalTimeoutMinutes = toInt(v)
	}
	if v, ok := row["notify_on_pause"]; ok {
		cfg.NotifyOnPause = toBool(v)
	}

	return cfg
}

func rowToApproval(row db.Row) (*ApprovalRequest, error) {
	req := &ApprovalRequest{}

	if v, ok := row["id"].(string); ok {
		req.ID = v
	}
	if v, ok := row["session_id"].(string); ok {
		req.SessionID = v
	}
	if v, ok := row["iteration"]; ok {
		req.Iteration = int64(toInt(v))
	}
	if v, ok := row["request_type"].(string); ok {
		req.RequestType = RequestType(v)
	}
	if v, ok := row["description"].(string); ok {
		req.Description = v
	}
	if v, ok := row["risk_level"].(string); ok {
		req.RiskLevel = RiskLevel(v)
	}
	if v, ok := row["context"].(string); ok {
		req.Context = v
	}
	if v, ok := row["target_tool"].(string); ok {
		req.TargetTool = v
	}
	if v, ok := row["target_sql"].(string); ok {
		req.TargetSQL = v
	}
	if v, ok := row["status"].(string); ok {
		req.Status = ApprovalStatus(v)
	}
	if v, ok := row["reviewer_id"].(string); ok {
		req.ReviewerID = v
	}
	if v, ok := row["review_notes"].(string); ok {
		req.ReviewNotes = v
	}
	if v, ok := row["modified_sql"].(string); ok {
		req.ModifiedSQL = v
	}
	if v, ok := row["created_at"].(string); ok {
		req.CreatedAt, _ = time.Parse(time.RFC3339, v)
	}
	if v, ok := row["reviewed_at"].(string); ok && v != "" {
		t, _ := time.Parse(time.RFC3339, v)
		req.ReviewedAt = &t
	}
	if v, ok := row["expires_at"].(string); ok {
		req.ExpiresAt, _ = time.Parse(time.RFC3339, v)
	}

	return req, nil
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}

func toBool(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case int64:
		return val > 0
	case float64:
		return val > 0
	case string:
		return val == "true" || val == "1"
	default:
		return false
	}
}

// toBoolInt converts a Go bool to 0/1 for portable boolean storage in SQLite.
func toBoolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
