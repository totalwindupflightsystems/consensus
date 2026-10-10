// Package api: tool request visibility handlers (SPEC-015 §3.4).
//
// axiom:trace work_item=DF-CONSENSUS-49 spec=specs/015-api-and-mcp.md impl=internal/api/tool_requests.go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"

	"github.com/wojons/consensus/internal/db"
)

const toolRequestListLimit = 100

const toolRequestSelect = `SELECT
	tr.id, tr.session_id, tr.iteration_id, tr.tool_name, tr.parameters,
	tr.status, tr.timeout_ms, tr.approval_request_id, tr.created_at,
	tr.executed_at, tr.completed_at,
	tres.id AS result_id, tres.request_id AS result_request_id,
	tres.output AS result_output, tres.is_error AS result_is_error,
	tres.error_code AS result_error_code, tres.exit_code AS result_exit_code,
	tres.duration_ms AS result_duration_ms, tres.token_count AS result_token_count,
	tres.created_at AS result_created_at
	FROM tool_requests tr
	LEFT JOIN tool_results tres ON tres.id = (
		SELECT latest.id FROM tool_results latest
		WHERE latest.request_id = tr.id
		ORDER BY latest.created_at DESC, latest.id DESC
		LIMIT 1
	)`

// handleListToolRequests returns at most 100 executor requests, oldest first.
func (s *Server) handleListToolRequests(w http.ResponseWriter, r *http.Request) {
	sessionFilter := r.URL.Query().Get("session_id")
	statusFilter := r.URL.Query().Get("status")

	if GetAuthScope(r) == "session" {
		authSessionID := GetAuthSessionID(r)
		if sessionFilter != "" && !s.checkSessionAccess(w, r, sessionFilter) {
			return
		}
		sessionFilter = authSessionID
	}

	query := toolRequestSelect
	args := make([]any, 0, 2)
	switch {
	case sessionFilter != "" && statusFilter != "":
		query += " WHERE tr.session_id = $1 AND tr.status = $2"
		args = append(args, sessionFilter, statusFilter)
	case sessionFilter != "":
		query += " WHERE tr.session_id = $1"
		args = append(args, sessionFilter)
	case statusFilter != "":
		query += " WHERE tr.status = $1"
		args = append(args, statusFilter)
	}
	query += " ORDER BY tr.created_at ASC, tr.id ASC LIMIT 100"

	rows, err := s.db.Query(r.Context(), query, args...)
	if err != nil {
		slog.Error("api: failed to list tool requests", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list tool requests")
		return
	}

	results := make([]ToolRequestResponse, 0, len(rows))
	for _, row := range rows {
		results = append(results, rowToToolRequestResponse(row))
	}
	writeJSON(w, results)
}

// handleGetToolRequest returns one executor request and its latest result.
func (s *Server) handleGetToolRequest(w http.ResponseWriter, r *http.Request, requestID string) {
	id, err := strconv.ParseInt(requestID, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "tool request not found")
		return
	}

	row, err := s.db.QueryRow(r.Context(), toolRequestSelect+" WHERE tr.id = $1", id)
	if err != nil || row == nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "tool request not found")
		return
	}
	if !s.checkSessionAccess(w, r, toString(row["session_id"])) {
		return
	}
	writeJSON(w, rowToToolRequestResponse(row))
}

// handleSessionToolCalls merges executor requests with planning tool-call refs.
func (s *Server) handleSessionToolCalls(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !s.checkSessionAccess(w, r, sessionID) {
		return
	}

	requestRows, err := s.db.Query(r.Context(),
		toolRequestSelect+" WHERE tr.session_id = $1 ORDER BY tr.created_at ASC, tr.id ASC LIMIT 100",
		sessionID)
	if err != nil {
		slog.Error("api: failed to list session tool requests", "session_id", sessionID, "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list session tool calls")
		return
	}

	stagingRows, err := s.db.Query(r.Context(), `SELECT
		id, session_id, iteration, turn, seq, payload, description,
		executed, result, status, created_at, executed_at
		FROM staging_buffer
		WHERE session_id = $1 AND cmd_type = 'tool_call_ref'
		ORDER BY created_at ASC, id ASC
		LIMIT 100`, sessionID)
	if err != nil {
		slog.Error("api: failed to list session staging tool calls", "session_id", sessionID, "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list session tool calls")
		return
	}

	results := make([]SessionToolCallResponse, 0, len(requestRows)+len(stagingRows))
	for _, row := range requestRows {
		request := rowToToolRequestResponse(row)
		results = append(results, SessionToolCallResponse{
			Source:      "tool_request",
			ID:          request.ID,
			SessionID:   request.SessionID,
			Iteration:   request.IterationID,
			ToolName:    request.ToolName,
			Parameters:  request.Parameters,
			Status:      request.Status,
			Executed:    request.ExecutedAt != nil,
			Result:      request.Result,
			CreatedAt:   request.CreatedAt,
			ExecutedAt:  request.ExecutedAt,
			CompletedAt: request.CompletedAt,
		})
	}
	for _, row := range stagingRows {
		results = append(results, rowToStagingToolCallResponse(row))
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].CreatedAt != results[j].CreatedAt {
			return results[i].CreatedAt < results[j].CreatedAt
		}
		if results[i].Source != results[j].Source {
			return results[i].Source < results[j].Source
		}
		return results[i].ID < results[j].ID
	})
	if len(results) > toolRequestListLimit {
		results = results[:toolRequestListLimit]
	}
	writeJSON(w, results)
}

func rowToToolRequestResponse(row db.Row) ToolRequestResponse {
	response := ToolRequestResponse{
		ID:                toInt64(row["id"]),
		SessionID:         toString(row["session_id"]),
		IterationID:       toInt64(row["iteration_id"]),
		ToolName:          toString(row["tool_name"]),
		Parameters:        parseJSONObject(toString(row["parameters"])),
		Status:            toString(row["status"]),
		TimeoutMS:         toInt(row["timeout_ms"]),
		ApprovalRequestID: optionalString(row["approval_request_id"]),
		CreatedAt:         toString(row["created_at"]),
		ExecutedAt:        optionalString(row["executed_at"]),
		CompletedAt:       optionalString(row["completed_at"]),
	}
	if row["result_id"] != nil {
		response.Result = &ToolResultResponse{
			ID:         toInt64(row["result_id"]),
			RequestID:  toInt64(row["result_request_id"]),
			Output:     toString(row["result_output"]),
			IsError:    toBool(row["result_is_error"]),
			ErrorCode:  optionalString(row["result_error_code"]),
			ExitCode:   optionalInt(row["result_exit_code"]),
			DurationMS: optionalInt64(row["result_duration_ms"]),
			TokenCount: optionalInt(row["result_token_count"]),
			CreatedAt:  toString(row["result_created_at"]),
		}
	}
	return response
}

func rowToStagingToolCallResponse(row db.Row) SessionToolCallResponse {
	status := toString(row["status"])
	var payload struct {
		ToolName   string         `json:"tool_name"`
		Parameters map[string]any `json:"parameters"`
	}
	rawPayload := []byte(toString(row["payload"]))
	if err := json.Unmarshal(rawPayload, &payload); err != nil || payload.ToolName == "" {
		// Staging persistence encodes entry.Payload as a JSON string, so the
		// durable tool_call_ref shape is commonly JSON nested inside JSON.
		var inner string
		if json.Unmarshal(rawPayload, &inner) == nil {
			_ = json.Unmarshal([]byte(inner), &payload)
		}
	}
	if payload.Parameters == nil {
		payload.Parameters = map[string]any{}
	}
	return SessionToolCallResponse{
		Source:      "staging_buffer",
		ID:          toInt64(row["id"]),
		SessionID:   toString(row["session_id"]),
		Iteration:   toInt64(row["iteration"]),
		Turn:        optionalInt(row["turn"]),
		Seq:         optionalInt(row["seq"]),
		ToolName:    payload.ToolName,
		Parameters:  payload.Parameters,
		Status:      status,
		Description: toString(row["description"]),
		Executed:    toBool(row["executed"]) || status == "executed",
		Result:      parseJSONRaw(toString(row["result"])),
		CreatedAt:   toString(row["created_at"]),
		ExecutedAt:  optionalString(row["executed_at"]),
	}
}

func parseJSONObject(raw string) map[string]any {
	result := map[string]any{}
	_ = json.Unmarshal([]byte(raw), &result)
	return result
}

func optionalString(value any) *string {
	if value == nil {
		return nil
	}
	result := toString(value)
	return &result
}

func optionalInt(value any) *int {
	if value == nil {
		return nil
	}
	result := toInt(value)
	return &result
}

func optionalInt64(value any) *int64 {
	if value == nil {
		return nil
	}
	result := toInt64(value)
	return &result
}
