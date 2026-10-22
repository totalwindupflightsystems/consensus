// Package api: tools & skills endpoint handlers (SPEC-015 §3.4).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-4/step-2-4-1 impl=internal/api/tools.go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// GET /api/v1/tools — List available tools
// ============================================================================

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Optional query filters
	statusFilter := r.URL.Query().Get("status")
	hemisphereFilter := r.URL.Query().Get("hemisphere")

	var rows []db.Row
	var err error

	switch {
	case statusFilter != "" && hemisphereFilter != "":
		rows, err = s.db.Query(ctx,
			`SELECT id, name, description, hemisphere, handler_type, status, enabled, requires_approval
			 FROM tools_registry
			 WHERE status = $1 AND hemisphere = $2
			 ORDER BY name`, statusFilter, hemisphereFilter)
	case statusFilter != "":
		rows, err = s.db.Query(ctx,
			`SELECT id, name, description, hemisphere, handler_type, status, enabled, requires_approval
			 FROM tools_registry
			 WHERE status = $1
			 ORDER BY name`, statusFilter)
	case hemisphereFilter != "":
		rows, err = s.db.Query(ctx,
			`SELECT id, name, description, hemisphere, handler_type, status, enabled, requires_approval
			 FROM tools_registry
			 WHERE hemisphere = $1
			 ORDER BY name`, hemisphereFilter)
	default:
		rows, err = s.db.Query(ctx,
			`SELECT id, name, description, hemisphere, handler_type, status, enabled, requires_approval
			 FROM tools_registry
			 ORDER BY name`)
	}

	if err != nil {
		slog.Error("api: failed to list tools", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list tools")
		return
	}

	results := make([]ToolResponse, 0, len(rows))
	for _, row := range rows {
		results = append(results, ToolResponse{
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

// ============================================================================
// POST /api/v1/tools — Register a tool in tools_registry
// ============================================================================

// handleCreateTool registers a tool row (DF-CONSENSUS-62).
//
// Before this handler there was NO user-facing on-ramp to tools_registry: a
// fresh install shipped the table empty, the CLI `tool` verb was inspect-only,
// and the only documented way to reach the HITL approval gate was a direct
// INSERT. Registration is an admin-plane write because the row decides what
// executable capability the runtime will dispatch (handler_type/handler_ref),
// so it requires an admin-scoped key — the same bar as API-key minting.
func (s *Server) handleCreateTool(w http.ResponseWriter, r *http.Request) {
	if GetAuthScope(r) != "admin" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "admin scope required")
		return
	}

	var req CreateToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body: "+err.Error())
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "name is required")
		return
	}
	if len(name) > 128 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "name must be 128 characters or fewer")
		return
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "description is required")
		return
	}
	handlerRef := strings.TrimSpace(req.HandlerRef)
	if handlerRef == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "handler_ref is required")
		return
	}

	// Defaults mirror the registry's column defaults so a caller can register
	// a row with the minimum three required fields.
	hemisphere := strings.TrimSpace(req.Hemisphere)
	if hemisphere == "" {
		hemisphere = "internal"
	}
	switch hemisphere {
	case "internal", "external":
	default:
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"hemisphere must be one of: internal, external")
		return
	}

	handlerType := strings.TrimSpace(req.HandlerType)
	if handlerType == "" {
		handlerType = "sql_function"
	}
	switch handlerType {
	case "sql_function", "http_endpoint", "go_native", "subprocess":
	default:
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"handler_type must be one of: sql_function, http_endpoint, go_native, subprocess")
		return
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "active"
	}
	switch status {
	case "active", "testing", "deprecated", "disabled":
	default:
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"status must be one of: active, testing, deprecated, disabled")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	parameterSchema := "{}"
	if len(req.ParameterSchema) > 0 && string(req.ParameterSchema) != "null" {
		var probe map[string]any
		if err := json.Unmarshal(req.ParameterSchema, &probe); err != nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				"parameter_schema must be a JSON object")
			return
		}
		parameterSchema = string(req.ParameterSchema)
	}

	ctx := r.Context()

	// A precise 409 beats a dialect-specific unique-violation error string, and
	// it also tells the caller the name is taken before any write is attempted.
	existing, err := s.db.Query(ctx, `SELECT id FROM tools_registry WHERE name = $1`, name)
	if err != nil {
		slog.Error("api: failed to check tool name", "tool", name, "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to register tool")
		return
	}
	if len(existing) > 0 {
		writeError(w, r, http.StatusConflict, "CONFLICT", fmt.Sprintf("tool %q already exists", name))
		return
	}

	// The agent role holds only SELECT on tools_registry (migration 001 §10 —
	// the registry is shared, read-mostly). Write through the admin handle so
	// the insert works on Postgres as well as SQLite; admindb falls back to the
	// agent pool when no separate pool is configured (SQLite, tests).
	var rateLimit *int
	if req.RateLimitPerMin != nil && *req.RateLimitPerMin > 0 {
		rateLimit = req.RateLimitPerMin
	}

	// Generate the id in Go instead of relying on the column default: the
	// defaults differ by dialect (gen_random_uuid on Postgres, randomblob on
	// SQLite) and not every schema that carries this table declares one.
	toolID := newUUID()

	if err := s.admindb.Exec(ctx,
		`INSERT INTO tools_registry
			(id, name, description, hemisphere, parameter_schema, handler_type, handler_ref,
			 status, enabled, requires_approval, rate_limit_per_min)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		toolID, name, description, hemisphere, parameterSchema, handlerType, handlerRef,
		status, enabled, req.RequiresApproval, rateLimit,
	); err != nil {
		slog.Error("api: failed to register tool", "tool", name, "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to register tool")
		return
	}

	row, err := s.db.QueryRow(ctx,
		`SELECT id, name, description, hemisphere, handler_type, status, enabled, requires_approval
		 FROM tools_registry
		 WHERE name = $1`, name)
	if err != nil || row == nil {
		// The row was written; report it rather than a misleading 500-free 200
		// with no body.
		slog.Error("api: tool registered but read-back failed", "tool", name, "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to read back registered tool")
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, ToolResponse{
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

// ============================================================================
// GET /api/v1/skills — List skill metadata (progressive disclosure)
// ============================================================================

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.Query(ctx,
		`SELECT id, name, metadata, enabled
		 FROM skills_registry
		 ORDER BY name`)
	if err != nil {
		slog.Error("api: failed to list skills", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list skills")
		return
	}

	results := make([]SkillResponse, 0, len(rows))
	for _, row := range rows {
		var meta any
		rawMeta := toString(row["metadata"])
		if rawMeta != "" {
			_ = json.Unmarshal([]byte(rawMeta), &meta)
		}

		results = append(results, SkillResponse{
			ID:       toString(row["id"]),
			Name:     toString(row["name"]),
			Metadata: meta,
			Enabled:  toBool(row["enabled"]),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

// ============================================================================
// GET /api/v1/skills/:name — Get full skill instructions (progressive disclosure)
// ============================================================================

func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request, skillName string) {
	ctx := r.Context()

	row, err := s.db.QueryRow(ctx,
		`SELECT id, name, metadata, instructions, linked_tool_ids, enabled
		 FROM skills_registry
		 WHERE name = $1`, skillName)
	if err != nil || row == nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("skill %q not found", skillName))
		return
	}

	var meta any
	rawMeta := toString(row["metadata"])
	if rawMeta != "" {
		_ = json.Unmarshal([]byte(rawMeta), &meta)
	}

	resp := SkillDetailResponse{
		ID:           toString(row["id"]),
		Name:         toString(row["name"]),
		Metadata:     meta,
		Instructions: toString(row["instructions"]),
		Enabled:      toBool(row["enabled"]),
	}

	// Parse linked_tool_ids from JSON array string
	rawTools := toString(row["linked_tool_ids"])
	resp.LinkedToolIDs = parseStringArray(rawTools)

	writeJSON(w, resp)
}

// ============================================================================
// POST /api/v1/tools/:name/execute — Execute an internal tool
// ============================================================================

func (s *Server) handleExecuteTool(w http.ResponseWriter, r *http.Request, toolName string) {
	ctx := r.Context()

	var req ExecuteToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body: "+err.Error())
		return
	}

	if req.SessionID == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "session_id is required")
		return
	}

	if !s.checkSessionAccess(w, r, req.SessionID) {
		return
	}

	// Look up the tool in the registry
	toolRow, err := s.db.QueryRow(ctx,
		`SELECT id, name, hemisphere, handler_type, handler_ref, status, enabled, parameter_schema
		 FROM tools_registry
		 WHERE name = $1`, toolName)
	if err != nil || toolRow == nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("tool %q not found", toolName))
		return
	}

	if !toBool(toolRow["enabled"]) {
		writeError(w, r, http.StatusConflict, "CONFLICT", fmt.Sprintf("tool %q is disabled", toolName))
		return
	}

	status := toString(toolRow["status"])
	if status != "active" {
		writeError(w, r, http.StatusConflict, "CONFLICT", fmt.Sprintf("tool %q status is %q, must be active to execute", toolName, status))
		return
	}

	handlerType := toString(toolRow["handler_type"])
	switch handlerType {
	case "sql_function":
		s.executeSQLFunctionTool(w, r, ctx, toolRow, req)
	case "go_native":
		s.executeGoNativeTool(w, r, toolName, req)
	case "http_endpoint", "subprocess":
		// External tools are dispatched via tool_requests, not directly executed
		s.enqueueExternalTool(w, r, ctx, toolRow, req)
	default:
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			fmt.Sprintf("unsupported handler_type %q for direct execution", handlerType))
	}
}

// executeSQLFunctionTool runs a sql_function tool: SELECT * FROM handler_ref(params).
func (s *Server) executeSQLFunctionTool(w http.ResponseWriter, r *http.Request, ctx context.Context, toolRow db.Row, req ExecuteToolRequest) {
	handlerRef := toString(toolRow["handler_ref"])
	toolName := toString(toolRow["name"])

	// Build the SQL call: SELECT * FROM <handler_ref>(params...)
	// For simplicity, we pass params as positional args if the function name
	// matches a known pattern, or call without args.
	sql := fmt.Sprintf("SELECT * FROM %s()", handlerRef)

	rows, err := s.db.Query(ctx, sql)
	if err != nil {
		slog.Error("api: tool execution failed", "tool", toolName, "error", err)
		resp := ExecuteToolResponse{
			ToolName: toolName,
			IsError:  true,
			Error:    err.Error(),
		}
		writeJSON(w, resp)
		return
	}

	var result any
	if len(rows) > 0 {
		// Return first row's content or a count
		if content := rows[0]["content"]; content != nil {
			result = toString(content)
		} else {
			result = map[string]any{"row_count": len(rows)}
		}
	} else {
		result = map[string]any{"row_count": 0}
	}

	// Also insert a tool_requests record for audit (iteration_id is NOT NULL
	// with no default — see enqueueExternalTool).
	_ = s.db.Exec(ctx,
		`INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status, completed_at)
		 VALUES ($1, $2, $3, $4, 'completed', datetime('now'))`,
		req.SessionID, s.sessionIteration(ctx, req.SessionID), toolName, "{}",
	)

	resp := ExecuteToolResponse{
		ToolName:     toolName,
		Result:       result,
		RowsAffected: int64(len(rows)),
		IsError:      false,
	}
	writeJSON(w, resp)
}

// executeGoNativeTool calls a Go handler function registered as go_native.
func (s *Server) executeGoNativeTool(w http.ResponseWriter, r *http.Request, toolName string, req ExecuteToolRequest) {
	// Go native tools are harness-level utilities. For now, we support a few built-ins.
	var result any
	var isError bool
	var errMsg string

	switch toolName {
	case "list_sessions":
		ctx := r.Context()
		rows, err := s.db.Query(ctx,
			`SELECT id, agent_name, status, iteration FROM sessions ORDER BY created_at DESC LIMIT 50`)
		if err != nil {
			isError = true
			errMsg = err.Error()
		} else {
			result = rows
		}
	default:
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			fmt.Sprintf("unknown go_native tool %q", toolName))
		return
	}

	resp := ExecuteToolResponse{
		ToolName: toolName,
		Result:   result,
		IsError:  isError,
		Error:    errMsg,
	}
	writeJSON(w, resp)
}

// enqueueExternalTool writes a pending tool_requests row for async execution.
func (s *Server) enqueueExternalTool(w http.ResponseWriter, r *http.Request, ctx context.Context, toolRow db.Row, req ExecuteToolRequest) {
	toolName := toString(toolRow["name"])
	paramsJSON, _ := json.Marshal(req.Parameters)

	// tool_requests.iteration_id is NOT NULL with no column default on both
	// backends (001 §4.2 Postgres, 031 SQLite rebuild). Omitting it made this
	// path fail with `NOT NULL constraint failed: tool_requests.iteration_id`
	// (500 INTERNAL_ERROR) on a fresh install — i.e. the documented execute
	// route could not create the request the approval gate watches (DF-CONSENSUS-62).
	err := s.db.Exec(ctx,
		`INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status)
		 VALUES ($1, $2, $3, $4, 'pending')`,
		req.SessionID, s.sessionIteration(ctx, req.SessionID), toolName, string(paramsJSON),
	)
	if err != nil {
		slog.Error("api: failed to enqueue tool", "tool", toolName, "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to enqueue tool")
		return
	}

	resp := ExecuteToolResponse{
		ToolName: toolName,
		Result:   map[string]string{"status": "pending", "message": "tool execution enqueued"},
		IsError:  false,
	}
	writeJSON(w, resp)
}

// sessionIteration returns the session's current iteration number, which every
// tool_requests row must carry (the column is NOT NULL with no default).
// A missing session or a read error yields 0: the enqueue must not fail on a
// bookkeeping lookup, and 0 is the migration-legacy value for "not yet in an
// iteration".
func (s *Server) sessionIteration(ctx context.Context, sessionID string) int64 {
	row, err := s.db.QueryRow(ctx, `SELECT iteration FROM sessions WHERE id = $1`, sessionID)
	if err != nil || row == nil {
		return 0
	}
	return toInt64(row["iteration"])
}

// ============================================================================
// Helpers
// ============================================================================

func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int64:
		return b != 0
	case float64:
		return b != 0
	case string:
		return strings.EqualFold(b, "true") || b == "1"
	default:
		return false
	}
}

// toInt64 is defined in sessions.go (shared across all api handler files)
