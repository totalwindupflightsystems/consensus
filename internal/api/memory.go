// Package api: memory & context endpoint handlers (SPEC-015 §3.2).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-2/step-2-2-1 impl=internal/api/memory.go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
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
	_ = json.NewEncoder(w).Encode(results)
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
	_ = json.NewEncoder(w).Encode(results)
}

// ============================================================================
// GET /api/v1/sessions/{id}/iterations — List per-iteration history
// ============================================================================

// listIterationsLimitMax bounds the `limit` query parameter (declared maximum
// in specs/openapi/paths/memory.yaml).
const listIterationsLimitMax = 200

func (s *Server) handleListIterations(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !s.checkSessionAccess(w, r, sessionID) {
		return
	}

	ctx := r.Context()
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= listIterationsLimitMax {
			limit = n
		}
	}

	// Two stores hold per-iteration state for a session, and they have
	// DIFFERENT writers — reading only the first is DF-CONSENSUS-073:
	//
	//   - iteration_commits: written by the SQL-executor loop via
	//     harness.FinalizeIteration -> WriteIterationSnapshot
	//     (internal/harness/executor.go:214 is its only production caller).
	//     It carries active memory pointers, display rules, the LLM response
	//     and the SQL executed.
	//   - agent_billing: written once per LLM call by the conversational
	//     planning loop (harness.flushPendingUsage, internal/harness/planning.go),
	//     which advances sessions.iteration but NEVER writes an
	//     iteration_commits row. It carries the token/cost accounting, keyed by
	//     the iteration number.
	//
	// The planning loop is the path modern agent sessions take, so those
	// sessions used to answer `[]` here while GET /sessions/{id} reported
	// iteration >= 4 and agent_billing held a row per iteration. We now merge
	// both, one entry per iteration number, so the endpoint reports what the
	// store actually recorded instead of pretending the session has no
	// iterations.
	commitRows, err := s.db.Query(ctx,
		`SELECT iteration_id, session_id, active_pointers, display_rules,
		        llm_response, sql_executed, rows_affected, created_at
		 FROM iteration_commits
		 WHERE session_id = $1
		 ORDER BY iteration_id DESC
		 LIMIT $2`,
		sessionID, limit,
	)
	if err != nil {
		slog.Error("api: failed to list iteration commits", "error", err, "session", sessionID)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list iterations")
		return
	}

	// Per-iteration billing aggregate. SUMs stay in SQL so a session with many
	// LLM calls per iteration returns one row per iteration, not one per call.
	// MIN(model_id)/COUNT(DISTINCT model_id) let a caller tell a single-model
	// iteration from a mixed one without a second request.
	billingRows, err := s.db.Query(ctx,
		`SELECT iteration,
		        COUNT(*)                        AS llm_calls,
		        SUM(prompt_tokens)              AS prompt_tokens,
		        SUM(completion_tokens)          AS completion_tokens,
		        SUM(cache_read_tokens)          AS cache_read_tokens,
		        SUM(cache_write_tokens)         AS cache_write_tokens,
		        SUM(cost_usd)                   AS cost_usd,
		        MIN(model_id)                   AS model_id,
		        COUNT(DISTINCT model_id)        AS model_count,
		        MIN(recorded_at)                AS first_recorded_at
		 FROM agent_billing
		 WHERE session_id = $1
		 GROUP BY iteration
		 ORDER BY iteration DESC
		 LIMIT $2`,
		sessionID, limit,
	)
	if err != nil {
		slog.Error("api: failed to list iteration billing", "error", err, "session", sessionID)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list iterations")
		return
	}

	merged := make(map[int64]*IterationCommitResponse, len(commitRows)+len(billingRows))
	order := make([]int64, 0, len(commitRows)+len(billingRows))
	entry := func(iteration int64) *IterationCommitResponse {
		if resp, ok := merged[iteration]; ok {
			return resp
		}
		resp := &IterationCommitResponse{
			IterationID:    iteration,
			Iteration:      iteration,
			SessionID:      sessionID,
			Sources:        make([]string, 0, 2),
			ActivePointers: []int64{},
		}
		merged[iteration] = resp
		order = append(order, iteration)
		return resp
	}

	// Snapshots first: a snapshot owns the structural fields, so billing
	// aggregates are layered onto it rather than overwriting it.
	for _, row := range commitRows {
		resp := rowToIterationCommitResponse(row)
		iteration := resp.IterationID
		target := entry(iteration)
		*target = resp
		target.Iteration = iteration
		target.Sources = []string{"iteration_commit"}
	}

	for _, row := range billingRows {
		iteration := toInt64(row["iteration"])
		resp := entry(iteration)
		resp.Sources = append(resp.Sources, "agent_billing")
		resp.LLMCalls = toInt(row["llm_calls"])
		resp.PromptTokens = toInt64(row["prompt_tokens"])
		resp.CompletionTokens = toInt64(row["completion_tokens"])
		resp.CacheReadTokens = toInt64(row["cache_read_tokens"])
		resp.CacheWriteTokens = toInt64(row["cache_write_tokens"])
		resp.CostUSD = toFloat64(row["cost_usd"])
		// Only name a model when the whole iteration used exactly one; a
		// mixed-model iteration must not be labelled with an arbitrary pick.
		if toInt64(row["model_count"]) == 1 {
			resp.ModelID = toString(row["model_id"])
		}
		// created_at means "when this iteration's record started": the
		// snapshot's creation time when there is one, otherwise the first
		// billed call of the iteration.
		if resp.CreatedAt == "" {
			resp.CreatedAt = toString(row["first_recorded_at"])
		}
	}

	sort.Slice(order, func(i, j int) bool { return order[i] > order[j] })
	if len(order) > limit {
		order = order[:limit]
	}

	results := make([]IterationCommitResponse, 0, len(order))
	for _, iteration := range order {
		results = append(results, *merged[iteration])
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
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
