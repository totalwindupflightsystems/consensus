// Package api: service layer — shared business logic for HTTP handlers and shims.
//
// The service methods contain the core business logic for sessions, messages, config,
// tools, and skills. Both the REST API HTTP handlers and protocol shims (opencode,
// MCP) use the same service — no duplicated database access or business logic.
//
// SPEC-017 §2: "The opencode shim does NOT bypass the native API. It calls it."
// The service layer makes this possible: the shim calls the same Go service methods
// that the HTTP handlers call, instead of writing its own raw SQL.
//
// axiom:trace work_item=spec-017-hardening-01 spec=specs/017-ui-adapter-layer.md plan=phase-1/task-1/step-1 impl=internal/api/service.go
package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/modelsync"
)

// ============================================================================
// Service — aggregates all sub-services
// ============================================================================

// Service groups all business-logic services together.
// Both the REST API server and protocol shims use this.
type Service struct {
	Sessions *SessionService
	Messages *MessageService
	Config   *ConfigService
	Tools    *ToolsService
	Metrics  *MetricsService
}

// NewService creates the service layer with all sub-services.
func NewService(dbase db.DB, events *EventBus) *Service {
	return &Service{
		Sessions: &SessionService{db: dbase, events: events},
		Messages: &MessageService{db: dbase, events: events},
		Config:   &ConfigService{db: dbase},
		Tools:    &ToolsService{db: dbase},
		Metrics:  &MetricsService{db: dbase},
	}
}

// ============================================================================
// SessionService — session lifecycle operations
// ============================================================================

// SessionService handles session CRUD and lifecycle management.
type SessionService struct {
	db     db.DB
	events *EventBus

	// modelSyncer optionally enables auto-registration of unrecognized models
	// from models.dev during session creation. nil = disabled.
	modelSyncer *modelsync.Syncer
}

// SetModelSyncer configures the SessionService to auto-register unknown models
// from models.dev during session creation.
func (svc *SessionService) SetModelSyncer(s *modelsync.Syncer) {
	svc.modelSyncer = s
}

// CreateSessionInput is the data needed to create a new agent session.
type CreateSessionInput struct {
	AgentName     string
	Goal          string
	ModelID       string
	ContextBudget int
	ProjectID     string // empty = Global scope; non-empty = Project scope
}

// CreateSessionOutput is the result of creating a session (includes the generated API key).
type CreateSessionOutput struct {
	SessionID string
	Status    string
	APIKey    string
	ModelID   string
	ProjectID string // empty = Global scope
	CreatedAt string
}

// CreateSession creates a new agent session with a bound API key.
func (svc *SessionService) CreateSession(ctx context.Context, input CreateSessionInput) (*CreateSessionOutput, error) {
	if input.AgentName == "" {
		return nil, fmt.Errorf("agent_name is required")
	}
	if input.Goal == "" {
		return nil, fmt.Errorf("goal is required")
	}

	sessionID := newUUID()
	modelID := input.ModelID
	if modelID == "" {
		// Prefer a chat-capable model. Exclude embedding-only models
		// (e.g. text-embedding-3-small) which are registered by the
		// compression worker and would otherwise win on cost.
		rows, err := svc.db.Query(ctx, `SELECT model_id FROM model_registry
			WHERE enabled = true AND model_id NOT LIKE 'text-embedding%'
			ORDER BY tier ASC, cost_per_m_in ASC LIMIT 1`)
		if err == nil && len(rows) > 0 {
			modelID = toString(rows[0]["model_id"])
		}
		if modelID == "" {
			// Fall back to config default via the harness LLM client;
			// "default" is resolved to cfg.LLM.DefaultModel at call time.
			modelID = "default"
		}
	}

	// Auto-register unrecognized models from models.dev if syncer is wired.
	if svc.modelSyncer != nil && modelID != "default" {
		if err := svc.modelSyncer.RegisterIfMissing(ctx, modelID); err != nil {
			// Non-fatal: session creation proceeds even if registration fails.
		}
	}

	contextBudget := input.ContextBudget
	if contextBudget <= 0 {
		contextBudget = 128000
	}

	apiKey := generateAPIKey()
	keyHash := sha256Hash(apiKey)
	keyPrefix := apiKey[:8]

	now := time.Now().UTC().Format(time.RFC3339)

	// Insert session (with optional project_id for scope)
	var projectIDArg any
	if input.ProjectID != "" {
		projectIDArg = input.ProjectID
	}
	err := svc.db.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, context_budget, project_id, heartbeat_at, created_at)
		 VALUES ($1, $2, $3, 'booting', $4, $5, $6, $7, $8)`,
		sessionID, input.AgentName, modelID, input.Goal, contextBudget, projectIDArg, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Insert API key
	keyID := newUUID()
	err = svc.db.Exec(ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at)
		 VALUES ($1, $2, $3, 'session', $4, $5)`,
		keyID, keyHash, keyPrefix, sessionID, now,
	)
	if err != nil {
		svc.db.Exec(ctx, `UPDATE sessions SET status = 'failed' WHERE id = $1`, sessionID)
		return nil, fmt.Errorf("failed to create api key: %w", err)
	}

	// Publish event
	if svc.events != nil {
		svc.events.PublishSessionUpdate(sessionID, "booting", 0)
	}

	return &CreateSessionOutput{
		SessionID: sessionID,
		Status:    "booting",
		APIKey:    apiKey,
		ModelID:   modelID,
		ProjectID: input.ProjectID,
		CreatedAt: now,
	}, nil
}

// ListSessions returns session summaries, optionally filtered by status.
// If sessionID is non-empty and scope is "session", only that session is returned.
func (svc *SessionService) ListSessions(ctx context.Context, statusFilter string, sessionID string, scope string) ([]SessionResponse, error) {
	if scope == "session" && sessionID != "" {
		rows, err := svc.db.Query(ctx,
			`SELECT id, agent_name, model_id, status, goal, iteration, tokens_used_in, tokens_used_out, project_id, heartbeat_at, created_at
			 FROM sessions WHERE id = $1`, sessionID)
		if err != nil {
			return nil, err
		}
		return rowsToSessionResponses(rows), nil
	}

	var rows []db.Row
	var err error

	if statusFilter != "" {
		statuses := strings.Split(statusFilter, ",")
		placeholders := make([]string, len(statuses))
		args := make([]any, len(statuses))
		for i, s := range statuses {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = strings.TrimSpace(s)
		}
		query := fmt.Sprintf(
			`SELECT id, agent_name, model_id, status, goal, iteration, tokens_used_in, tokens_used_out, project_id, heartbeat_at, created_at
			 FROM sessions WHERE status IN (%s) ORDER BY created_at DESC LIMIT 50`,
			strings.Join(placeholders, ","))
		rows, err = svc.db.Query(ctx, query, args...)
	} else {
		rows, err = svc.db.Query(ctx,
			`SELECT id, agent_name, model_id, status, goal, iteration, tokens_used_in, tokens_used_out, project_id, heartbeat_at, created_at
			 FROM sessions ORDER BY created_at DESC LIMIT 50`)
	}
	if err != nil {
		return nil, err
	}

	return rowsToSessionResponses(rows), nil
}

// GetSession returns a single session by ID.
func (svc *SessionService) GetSession(ctx context.Context, id string) (*SessionResponse, error) {
	row, err := svc.db.QueryRow(ctx,
		`SELECT id, parent_id, agent_name, model_id, status, goal, context_budget,
		        tokens_used_in, tokens_used_out, iteration, project_id, heartbeat_at, created_at, completed_at
		 FROM sessions WHERE id = $1`, id)
	if err != nil || row == nil {
		return nil, fmt.Errorf("session not found")
	}
	resp := rowToSessionResponse(row)

	// Surface the most recent audit_logs error for failed sessions so a
	// wrong/expired API key (or any harness failure) is actionable (DOGFOOD-004).
	if resp.Status == "failed" {
		errRow, qerr := svc.db.QueryRow(ctx,
			`SELECT error_message FROM audit_logs
			 WHERE session_id = $1 AND error_message IS NOT NULL AND error_message != ''
			 ORDER BY id DESC LIMIT 1`, id)
		if qerr == nil && errRow != nil {
			if em := toString(errRow["error_message"]); em != "" {
				resp.LastError = &em
			}
		}
	}

	return &resp, nil
}

// GetSessionStatus returns just the status and iteration for a session (lightweight).
func (svc *SessionService) GetSessionStatus(ctx context.Context, id string) (string, int64, error) {
	row, err := svc.db.QueryRow(ctx, `SELECT status, iteration FROM sessions WHERE id = $1`, id)
	if err != nil || row == nil {
		return "", 0, fmt.Errorf("session not found")
	}
	return toString(row["status"]), toInt64(row["iteration"]), nil
}

// UpdateSession changes a session's status (pause, resume, cancel).
func (svc *SessionService) UpdateSession(ctx context.Context, id string, action string) error {
	// Get current status
	row, err := svc.db.QueryRow(ctx, `SELECT status FROM sessions WHERE id = $1`, id)
	if err != nil || row == nil {
		return fmt.Errorf("session not found")
	}
	currentStatus := toString(row["status"])

	var targetStatus string

	switch action {
	case "pause":
		allowed := []string{"idle", "thinking", "planning", "tool_exec", "executing", "waiting_sub"}
		if !contains(allowed, currentStatus) {
			return fmt.Errorf("cannot pause session in status %q", currentStatus)
		}
		targetStatus = "paused"

	case "resume":
		if currentStatus != "paused" {
			return fmt.Errorf("can only resume paused sessions, current status is %q", currentStatus)
		}
		targetStatus = "idle"

	case "cancel":
		targetStatus = "failed"

	case "title":
		targetStatus = currentStatus // no status change, title only handled by caller
		return nil

	default:
		return fmt.Errorf("unknown session action: %q (use pause, resume, or cancel)", action)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var execErr error
	if targetStatus == "failed" || targetStatus == "completed" {
		execErr = svc.db.Exec(ctx,
			`UPDATE sessions SET status = $1, heartbeat_at = $2, completed_at = $2 WHERE id = $3`,
			targetStatus, now, id)
	} else {
		execErr = svc.db.Exec(ctx,
			`UPDATE sessions SET status = $1, heartbeat_at = $2 WHERE id = $3`,
			targetStatus, now, id)
	}

	if execErr != nil {
		return fmt.Errorf("failed to update session: %w", execErr)
	}

	// Publish event
	if svc.events != nil {
		svc.events.PublishSessionUpdate(id, targetStatus, 0)
	}

	return nil
}

// UpdateSessionFields updates arbitrary session fields (title, goal, status, etc.).
func (svc *SessionService) UpdateSessionFields(ctx context.Context, id string, fields map[string]string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	sets := []string{}
	args := []any{}
	idx := 0

	if v, ok := fields["agent_name"]; ok && v != "" {
		idx++
		sets = append(sets, fmt.Sprintf("agent_name = $%d", idx))
		args = append(args, v)
	}
	if v, ok := fields["status"]; ok && v != "" {
		idx++
		sets = append(sets, fmt.Sprintf("status = $%d", idx))
		args = append(args, v)
	}
	if v, ok := fields["goal"]; ok && v != "" {
		idx++
		sets = append(sets, fmt.Sprintf("goal = $%d", idx))
		args = append(args, v)
	}

	if len(sets) == 0 {
		return fmt.Errorf("no fields to update")
	}

	idx++
	sets = append(sets, fmt.Sprintf("heartbeat_at = $%d", idx))
	args = append(args, now)

	idx++
	args = append(args, id)

	query := fmt.Sprintf("UPDATE sessions SET %s WHERE id = $%d", strings.Join(sets, ", "), idx)
	return svc.db.Exec(ctx, query, args...)
}

// DeleteSession marks a session as failed (soft delete).
func (svc *SessionService) DeleteSession(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return svc.db.Exec(ctx,
		`UPDATE sessions SET status = 'failed', completed_at = $1, heartbeat_at = $1 WHERE id = $2`,
		now, id)
}

// AbortSession is an alias for DeleteSession that sets status to 'failed'.
func (svc *SessionService) AbortSession(ctx context.Context, id string) error {
	return svc.DeleteSession(ctx, id)
}

// ListChildren returns child sessions of the given parent session.
func (svc *SessionService) ListChildren(ctx context.Context, parentID string) ([]SessionResponse, error) {
	rows, err := svc.db.Query(ctx,
		`SELECT id, agent_name, model_id, status, goal, iteration, tokens_used_in, tokens_used_out, project_id, heartbeat_at, created_at
		 FROM sessions WHERE parent_id = $1 ORDER BY created_at DESC LIMIT 50`,
		parentID,
	)
	if err != nil {
		return nil, err
	}
	return rowsToSessionResponses(rows), nil
}

// ============================================================================
// MessageService — message operations
// ============================================================================

// MessageService handles agent message sending and retrieval.
type MessageService struct {
	db     db.DB
	events *EventBus
}

// SendMessageInput is the data needed to send a message to a session.
type SendMessageInput struct {
	SessionID string
	Content   string
	MsgType   string // defaults to "user_instruction"
}

// SendMessage sends a user message to an agent session.
func (svc *MessageService) SendMessage(ctx context.Context, input SendMessageInput) error {
	if input.Content == "" {
		return fmt.Errorf("content is required")
	}

	msgType := input.MsgType
	if msgType == "" {
		msgType = "user_instruction"
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Check session status
	status, iteration, err := svc.getSessionStatus(ctx, input.SessionID)
	if err != nil {
		return err
	}

	// Insert message into memory_events
	err = svc.db.Exec(ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, $3, $4)`,
		input.Content, input.SessionID, iteration+1, now)
	if err != nil {
		return fmt.Errorf("failed to store message: %w", err)
	}

	// Transition session to thinking if idle or booting.
	// Sessions are born 'booting' (CreateSession); a message must wake them
	// so the heartbeat loop claims them. (Native HTTP handler + shim both
	// route through here via Service.SendMessage.)
	if status == "idle" || status == "booting" {
		svc.db.Exec(ctx,
			`UPDATE sessions SET status = 'thinking', heartbeat_at = $1, iteration = iteration + 1 WHERE id = $2`,
			now, input.SessionID)
		if svc.events != nil {
			svc.events.PublishSessionUpdate(input.SessionID, "thinking", iteration+1)
		}
	}

	return nil
}

func (svc *MessageService) getSessionStatus(ctx context.Context, sessionID string) (string, int64, error) {
	row, err := svc.db.QueryRow(ctx, `SELECT status, iteration FROM sessions WHERE id = $1`, sessionID)
	if err != nil || row == nil {
		return "", 0, fmt.Errorf("session not found")
	}
	return toString(row["status"]), toInt64(row["iteration"]), nil
}

// MemoryEvent is a raw memory event row.
type MemoryEvent struct {
	ID               int64
	Type             string
	Content          string
	SessionID        string
	IterationCreated int64
	CreatedAt        string
}

// ListMessages returns recent memory events for a session.
func (svc *MessageService) ListMessages(ctx context.Context, sessionID string, limit int) ([]MemoryEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := svc.db.Query(ctx,
		`SELECT me.id, me.type, me.content, me.session_id, me.iteration_created, me.created_at
		 FROM memory_events me
		 WHERE me.session_id = $1
		 ORDER BY me.id DESC LIMIT $2`,
		sessionID, limit,
	)
	if err != nil {
		return nil, err
	}

	events := make([]MemoryEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, MemoryEvent{
			ID:               toInt64(row["id"]),
			Type:             toString(row["type"]),
			Content:          toString(row["content"]),
			SessionID:        toString(row["session_id"]),
			IterationCreated: toInt64(row["iteration_created"]),
			CreatedAt:        toString(row["created_at"]),
		})
	}
	return events, nil
}

// GetMessage returns a single memory event by ID.
func (svc *MessageService) GetMessage(ctx context.Context, sessionID, messageID string) (*MemoryEvent, error) {
	// messageID may be "msg-N" format; strip prefix
	trimmed := strings.TrimPrefix(messageID, "msg-")
	row, err := svc.db.QueryRow(ctx,
		`SELECT id, type, content, session_id, iteration_created, created_at
		 FROM memory_events WHERE session_id = $1 AND CAST(id AS TEXT) = $2 LIMIT 1`,
		sessionID, trimmed,
	)
	if err != nil || row == nil {
		// Fallback: prefix match
		row, err = svc.db.QueryRow(ctx,
			`SELECT id, type, content, session_id, iteration_created, created_at
			 FROM memory_events WHERE session_id = $1 AND CAST(id AS TEXT) LIKE $2 LIMIT 1`,
			sessionID, trimmed+"%",
		)
	}
	if err != nil || row == nil {
		return nil, fmt.Errorf("message not found")
	}

	return &MemoryEvent{
		ID:               toInt64(row["id"]),
		Type:             toString(row["type"]),
		Content:          toString(row["content"]),
		SessionID:        toString(row["session_id"]),
		IterationCreated: toInt64(row["iteration_created"]),
		CreatedAt:        toString(row["created_at"]),
	}, nil
}

// GetLastUserMessage returns the most recent user_message for a session.
func (svc *MessageService) GetLastUserMessage(ctx context.Context, sessionID string) (*MemoryEvent, error) {
	row, err := svc.db.QueryRow(ctx,
		`SELECT id, type, content, session_id, iteration_created, created_at
		 FROM memory_events WHERE session_id = $1 AND type = 'user_message'
		 ORDER BY id DESC LIMIT 1`,
		sessionID,
	)
	if err != nil || row == nil {
		return nil, fmt.Errorf("no messages found")
	}
	return &MemoryEvent{
		ID:               toInt64(row["id"]),
		Type:             toString(row["type"]),
		Content:          toString(row["content"]),
		SessionID:        toString(row["session_id"]),
		IterationCreated: toInt64(row["iteration_created"]),
		CreatedAt:        toString(row["created_at"]),
	}, nil
}

// ============================================================================
// ConfigService — configuration operations
// ============================================================================

// ConfigService handles system configuration.
type ConfigService struct {
	db db.DB
}

// GetConfig returns all system settings as a map.
func (svc *ConfigService) GetConfig(ctx context.Context) (map[string]string, error) {
	rows, err := svc.db.Query(ctx, `SELECT key, value FROM system_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	settings := make(map[string]string, len(rows))
	for _, row := range rows {
		settings[toString(row["key"])] = toString(row["value"])
	}
	return settings, nil
}

// UpdateConfig sets multiple system settings.
func (svc *ConfigService) UpdateConfig(ctx context.Context, settings map[string]string) error {
	for k, v := range settings {
		err := svc.db.Exec(ctx,
			`INSERT INTO system_settings (key, value) VALUES ($1, $2)
			 ON CONFLICT (key) DO UPDATE SET value = $2`,
			k, v,
		)
		if err != nil {
			return fmt.Errorf("failed to set %s: %w", k, err)
		}
	}
	return nil
}

// SetConfigKey sets a single system setting.
func (svc *ConfigService) SetConfigKey(ctx context.Context, key, value string) error {
	return svc.db.Exec(ctx,
		`INSERT INTO system_settings (key, value) VALUES ($1, $2)
		 ON CONFLICT (key) DO UPDATE SET value = $2`,
		key, value,
	)
}

// GetConfigKey returns a single system setting value.
func (svc *ConfigService) GetConfigKey(ctx context.Context, key string) (string, error) {
	row, err := svc.db.QueryRow(ctx, `SELECT value FROM system_settings WHERE key = $1`, key)
	if err != nil || row == nil {
		return "", fmt.Errorf("config key %q not found", key)
	}
	return toString(row["value"]), nil
}

// ============================================================================
// ToolsService — tools and skills discovery
// ============================================================================

// ToolsService handles tool and skill registry queries.
type ToolsService struct {
	db db.DB
}

// ToolInfo is a lightweight tool descriptor.
type ToolInfo struct {
	ID               string
	Name             string
	Description      string
	Hemisphere       string
	HandlerType      string
	Status           string
	Enabled          bool
	RequiresApproval bool
}

// ListTools returns all tools, optionally filtered.
func (svc *ToolsService) ListTools(ctx context.Context, statusFilter, hemisphereFilter string) ([]ToolInfo, error) {
	var rows []db.Row
	var err error

	switch {
	case statusFilter != "" && hemisphereFilter != "":
		rows, err = svc.db.Query(ctx,
			`SELECT id, name, description, hemisphere, handler_type, status, enabled, requires_approval
			 FROM tools_registry WHERE status = $1 AND hemisphere = $2 ORDER BY name`,
			statusFilter, hemisphereFilter)
	case statusFilter != "":
		rows, err = svc.db.Query(ctx,
			`SELECT id, name, description, hemisphere, handler_type, status, enabled, requires_approval
			 FROM tools_registry WHERE status = $1 ORDER BY name`,
			statusFilter)
	case hemisphereFilter != "":
		rows, err = svc.db.Query(ctx,
			`SELECT id, name, description, hemisphere, handler_type, status, enabled, requires_approval
			 FROM tools_registry WHERE hemisphere = $1 ORDER BY name`,
			hemisphereFilter)
	default:
		rows, err = svc.db.Query(ctx,
			`SELECT id, name, description, hemisphere, handler_type, status, enabled, requires_approval
			 FROM tools_registry ORDER BY name`)
	}
	if err != nil {
		return nil, err
	}

	tools := make([]ToolInfo, 0, len(rows))
	for _, row := range rows {
		tools = append(tools, ToolInfo{
			ID:               toString(row["id"]),
			Name:             toString(row["name"]),
			Description:      toString(row["description"]),
			Hemisphere:       toString(row["hemisphere"]),
			HandlerType:      toString(row["handler_type"]),
			Status:           toString(row["status"]),
			Enabled:          toBool(row["enabled"]),
			RequiresApproval: toBool(row["requires_approval"]),
		})
	}
	return tools, nil
}

// ListToolIDs returns just tool names.
func (svc *ToolsService) ListToolIDs(ctx context.Context) ([]string, error) {
	rows, err := svc.db.Query(ctx, `SELECT name FROM tools_registry ORDER BY name`)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, toString(row["name"]))
	}
	return ids, nil
}

// GetTool returns a single tool by name, or nil if not found.
func (svc *ToolsService) GetTool(ctx context.Context, name string) (*ToolInfo, error) {
	row, err := svc.db.QueryRow(ctx,
		`SELECT id, name, description, hemisphere, handler_type, status, enabled, requires_approval
		 FROM tools_registry WHERE name = $1`, name)
	if err != nil || row == nil {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	return &ToolInfo{
		ID:               toString(row["id"]),
		Name:             toString(row["name"]),
		Description:      toString(row["description"]),
		Hemisphere:       toString(row["hemisphere"]),
		HandlerType:      toString(row["handler_type"]),
		Status:           toString(row["status"]),
		Enabled:          toBool(row["enabled"]),
		RequiresApproval: toBool(row["requires_approval"]),
	}, nil
}

// ModelInfo is a model registry entry.
type ModelInfo struct {
	ModelID     string
	Tier        int
	MaxContext  int64
	CostPerMIn  float64
	CostPerMOut float64
	Enabled     bool
}

// ListModels returns all enabled model registry entries.
func (svc *ToolsService) ListModels(ctx context.Context) ([]ModelInfo, error) {
	rows, err := svc.db.Query(ctx,
		`SELECT model_id, tier, max_context, cost_per_m_in, cost_per_m_out, enabled
		 FROM model_registry ORDER BY tier ASC, cost_per_m_in ASC`)
	if err != nil {
		return nil, err
	}
	models := make([]ModelInfo, 0, len(rows))
	for _, row := range rows {
		models = append(models, ModelInfo{
			ModelID:     toString(row["model_id"]),
			Tier:        toInt(row["tier"]),
			MaxContext:  toInt64(row["max_context"]),
			CostPerMIn:  toFloat64(row["cost_per_m_in"]),
			CostPerMOut: toFloat64(row["cost_per_m_out"]),
			Enabled:     toBool(row["enabled"]),
		})
	}
	return models, nil
}

// GetDefaultModel returns the cheapest enabled model.
func (svc *ToolsService) GetDefaultModel(ctx context.Context) (*ModelInfo, error) {
	row, err := svc.db.QueryRow(ctx,
		`SELECT model_id, tier, max_context, cost_per_m_in, cost_per_m_out, enabled
		 FROM model_registry WHERE enabled = true ORDER BY tier ASC, cost_per_m_in ASC LIMIT 1`)
	if err != nil || row == nil {
		return nil, fmt.Errorf("no enabled models found")
	}
	return &ModelInfo{
		ModelID:     toString(row["model_id"]),
		Tier:        toInt(row["tier"]),
		MaxContext:  toInt64(row["max_context"]),
		CostPerMIn:  toFloat64(row["cost_per_m_in"]),
		CostPerMOut: toFloat64(row["cost_per_m_out"]),
		Enabled:     toBool(row["enabled"]),
	}, nil
}

// ============================================================================
// MetricsService — system metrics
// ============================================================================

// MetricsService provides system-wide stats.
type MetricsService struct {
	db db.DB
}

// MetricsResult holds system metric counts.
type MetricsResult struct {
	ActiveSessions   int
	PendingTasks     int
	PendingApprovals int
	TotalSessions    int
}

// GetMetrics returns system metrics.
func (svc *MetricsService) GetMetrics(ctx context.Context) (*MetricsResult, error) {
	var r MetricsResult

	row, err := svc.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM sessions WHERE status IN ('idle', 'thinking', 'planning', 'tool_exec', 'executing', 'waiting_sub', 'booting')`)
	if err == nil && row != nil {
		r.ActiveSessions = toInt(row["count"])
	}

	row, err = svc.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM tasks WHERE status = 'pending'`)
	if err == nil && row != nil {
		r.PendingTasks = toInt(row["count"])
	}

	row, err = svc.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM approval_requests WHERE status = 'pending'`)
	if err == nil && row != nil {
		r.PendingApprovals = toInt(row["count"])
	}

	row, err = svc.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM sessions`)
	if err == nil && row != nil {
		r.TotalSessions = toInt(row["count"])
	}

	return &r, nil
}

// ============================================================================
// Helpers
// ============================================================================

func rowsToSessionResponses(rows []db.Row) []SessionResponse {
	results := make([]SessionResponse, 0, len(rows))
	for _, row := range rows {
		results = append(results, rowToSessionResponse(row))
	}
	return results
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// toFloat64 is defined in billing.go (shared across all api files)
