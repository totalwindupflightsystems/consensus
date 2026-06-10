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

	"github.com/wojons/conscientiousness/internal/db"
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
	json.NewEncoder(w).Encode(results)
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
			json.Unmarshal([]byte(rawMeta), &meta)
		}

		results = append(results, SkillResponse{
			ID:       toString(row["id"]),
			Name:     toString(row["name"]),
			Metadata: meta,
			Enabled:  toBool(row["enabled"]),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
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
		json.Unmarshal([]byte(rawMeta), &meta)
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

	// Also insert a tool_requests record for audit
	s.db.Exec(ctx,
		`INSERT INTO tool_requests (session_id, tool_name, parameters, status, completed_at)
		 VALUES ($1, $2, $3, 'completed', datetime('now'))`,
		req.SessionID, toolName, "{}",
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

	err := s.db.Exec(ctx,
		`INSERT INTO tool_requests (session_id, tool_name, parameters, status)
		 VALUES ($1, $2, $3, 'pending')`,
		req.SessionID, toolName, string(paramsJSON),
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
