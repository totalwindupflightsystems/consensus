// Package api: memory & context endpoint handlers (SPEC-015 §3.2).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-2/step-2-2-1 impl=internal/api/memory.go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// GET /api/v1/sessions/{id}/memory — List memory events for session
// ============================================================================

func (s *Server) handleListMemory(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !s.checkSessionAccess(w, r, sessionID) {
		return
	}

	ctx := r.Context()

	// Optional query params: type filter, limit
	typeFilter := r.URL.Query().Get("type")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	var rows []db.Row
	var err error

	if typeFilter != "" {
		rows, err = s.db.Query(ctx,
			`SELECT me.id, me.type, me.content, me.summary_text, me.session_id, me.iteration_created, me.created_at,
			        COALESCE(dm.mode, 'full') AS display_mode
			 FROM memory_events me
			 LEFT JOIN display_modes dm ON dm.memory_id = me.id
			 WHERE me.session_id = $1 AND me.type = $2
			 ORDER BY me.id DESC
			 LIMIT $3`,
			sessionID, typeFilter, limit,
		)
	} else {
		rows, err = s.db.Query(ctx,
			`SELECT me.id, me.type, me.content, me.summary_text, me.session_id, me.iteration_created, me.created_at,
			        COALESCE(dm.mode, 'full') AS display_mode
			 FROM memory_events me
			 LEFT JOIN display_modes dm ON dm.memory_id = me.id
			 WHERE me.session_id = $1
			 ORDER BY me.id DESC
			 LIMIT $2`,
			sessionID, limit,
		)
	}

	if err != nil {
		slog.Error("api: failed to list memory", "error", err, "session", sessionID)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list memory events")
		return
	}

	results := make([]MemoryEventResponse, 0, len(rows))
	for _, row := range rows {
		results = append(results, rowToMemoryEventResponse(row))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// ============================================================================
// GET /api/v1/sessions/{id}/memory/{mid} — Get single memory event
// ============================================================================

func (s *Server) handleGetMemoryEvent(w http.ResponseWriter, r *http.Request, sessionID, memoryID string) {
	if !s.checkSessionAccess(w, r, sessionID) {
		return
	}

	ctx := r.Context()

	row, err := s.db.QueryRow(ctx,
		`SELECT me.id, me.type, me.content, me.summary_text, me.session_id, me.iteration_created, me.created_at,
		        COALESCE(dm.mode, 'full') AS display_mode
		 FROM memory_events me
		 LEFT JOIN display_modes dm ON dm.memory_id = me.id
		 WHERE me.id = $1 AND me.session_id = $2`,
		memoryID, sessionID,
	)
	if err != nil || row == nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "memory event not found")
		return
	}

	writeJSON(w, rowToMemoryEventResponse(row))
}

// ============================================================================
// GET /api/v1/sessions/{id}/context — Get active context (rendered)
// ============================================================================

func (s *Server) handleGetActiveContext(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !s.checkSessionAccess(w, r, sessionID) {
		return
	}

	ctx := r.Context()
	limitStr := r.URL.Query().Get("limit")
	limit := 200
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	// Build the active context from memory_events + display_modes.
	// The spec defines active_context_view but it's not materialized in SQLite.
	// We emulate it: query memory_events with display_modes joined and render content.
	rows, err := s.db.Query(ctx,
		`SELECT me.id, me.iteration_created, me.type,
		        COALESCE(dm.mode, 'full') AS display_mode,
		        CASE
		            WHEN COALESCE(dm.mode, 'full') = 'compressed' AND me.summary_text IS NOT NULL
		                THEN me.summary_text
		            WHEN COALESCE(dm.mode, 'full') = 'hidden'
		                THEN NULL
		            ELSE me.content
		        END AS rendered_text
		 FROM memory_events me
		 LEFT JOIN display_modes dm ON dm.memory_id = me.id
		 WHERE me.session_id = $1
		   AND COALESCE(dm.mode, 'full') != 'hidden'
		 ORDER BY me.iteration_created, me.id
		 LIMIT $2`,
		sessionID, limit,
	)
	if err != nil {
		slog.Error("api: failed to get active context", "error", err, "session", sessionID)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get active context")
		return
	}

	results := make([]ActiveContextResponse, 0, len(rows))
	for _, row := range rows {
		resp := ActiveContextResponse{
			ID:               toInt64(row["id"]),
			IterationCreated: toInt64(row["iteration_created"]),
			Type:             toString(row["type"]),
			DisplayMode:      toString(row["display_mode"]),
		}
		if rt := row["rendered_text"]; rt != nil {
			s := toString(rt)
			resp.RenderedText = &s
		}
		results = append(results, resp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// ============================================================================
// GET /api/v1/sessions/{id}/iterations — List iteration commits
// ============================================================================

func (s *Server) handleListIterations(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !s.checkSessionAccess(w, r, sessionID) {
		return
	}

	ctx := r.Context()
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	rows, err := s.db.Query(ctx,
		`SELECT iteration_id, session_id, active_pointers, display_rules,
		        llm_response, sql_executed, rows_affected, created_at
		 FROM iteration_commits
		 WHERE session_id = $1
		 ORDER BY iteration_id DESC
		 LIMIT $2`,
		sessionID, limit,
	)
	if err != nil {
		slog.Error("api: failed to list iterations", "error", err, "session", sessionID)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list iterations")
		return
	}

	results := make([]IterationCommitResponse, 0, len(rows))
	for _, row := range rows {
		results = append(results, rowToIterationCommitResponse(row))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// ============================================================================
// Row Mappers
// ============================================================================

func rowToMemoryEventResponse(row db.Row) MemoryEventResponse {
	resp := MemoryEventResponse{
		ID:               toInt64(row["id"]),
		Type:             toString(row["type"]),
		Content:          toString(row["content"]),
		SessionID:        toString(row["session_id"]),
		IterationCreated: toInt64(row["iteration_created"]),
		DisplayMode:      toString(row["display_mode"]),
		CreatedAt:        toString(row["created_at"]),
	}
	if st := row["summary_text"]; st != nil {
		s := toString(st)
		resp.SummaryText = &s
	}
	return resp
}

func rowToIterationCommitResponse(row db.Row) IterationCommitResponse {
	resp := IterationCommitResponse{
		IterationID:  toInt64(row["iteration_id"]),
		SessionID:    toString(row["session_id"]),
		CreatedAt:    toString(row["created_at"]),
		RowsAffected: toInt(row["rows_affected"]),
	}

	// Parse JSON arrays
	if ap := row["active_pointers"]; ap != nil {
		resp.ActivePointers = parseInt64Array(toString(ap))
	}
	if dr := row["display_rules"]; dr != nil {
		var rules any
		if err := json.Unmarshal([]byte(toString(dr)), &rules); err == nil {
			resp.DisplayRules = rules
		}
	}
	if lr := row["llm_response"]; lr != nil {
		var response any
		if err := json.Unmarshal([]byte(toString(lr)), &response); err == nil {
			resp.LLMResponse = response
		}
	}
	if se := row["sql_executed"]; se != nil {
		resp.SQLExecuted = parseStringArray(toString(se))
	}

	return resp
}

// parseInt64Array parses a JSON array of integers from a string.
func parseInt64Array(s string) []int64 {
	var arr []int64
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		// Try with float64 fallback (JSON numbers unmarshal as float64 by default)
		var floatArr []float64
		if err2 := json.Unmarshal([]byte(s), &floatArr); err2 == nil {
			result := make([]int64, len(floatArr))
			for i, f := range floatArr {
				result[i] = int64(f)
			}
			return result
		}
		return []int64{}
	}
	return arr
}

// parseStringArray parses a JSON array of strings from a string.
func parseStringArray(s string) []string {
	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return []string{}
	}
	return arr
}
