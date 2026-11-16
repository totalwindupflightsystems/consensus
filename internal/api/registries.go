package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// DF-CONSENSUS-67: registration verbs for stores that already had production
// read paths but no production writer. These handlers deliberately insert into
// the same tables consumed by the native API, OpenCode shim, webhook router,
// and harness prompt builder; they do not maintain parallel API-only state.

func registrationEnabled(requested *bool) bool {
	return requested == nil || *requested
}

func registrationAdminOnly(w http.ResponseWriter, r *http.Request) bool {
	if GetAuthScope(r) == "admin" {
		return true
	}
	writeError(w, r, http.StatusForbidden, "FORBIDDEN", "admin scope required")
	return false
}

func writeCreatedJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(payload)
}

func isRegistrationUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}

func nullableRegistrationString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Server) registrationSessionExists(r *http.Request, sessionID string) (bool, error) {
	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1 AND deleted_at IS NULL`, sessionID)
	return err == nil && row != nil && toString(row["id"]) != "", err
}

// int64RegistrationArray uses a PostgreSQL array on PostgreSQL and the JSON
// representation used by the SQLite mirror everywhere else.
func (s *Server) int64RegistrationArray(values []int64) any {
	if values == nil {
		values = []int64{}
	}
	if s.db.Backend() == db.BackendPostgres {
		return values
	}
	raw, _ := json.Marshal(values)
	return string(raw)
}

func (s *Server) stringRegistrationArray(values []string) any {
	if values == nil {
		values = []string{}
	}
	if s.db.Backend() == db.BackendPostgres {
		return values
	}
	raw, _ := json.Marshal(values)
	return string(raw)
}

// handleCreateSkill serves POST /api/v1/skills.
func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	if !registrationAdminOnly(w, r) {
		return
	}
	var req CreateSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Instructions = strings.TrimSpace(req.Instructions)
	if req.Name == "" || len(req.Name) > 128 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "name is required and must be at most 128 characters")
		return
	}
	if req.Instructions == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "instructions is required")
		return
	}
	linkedToolIDs := req.LinkedToolIDs
	if linkedToolIDs == nil {
		linkedToolIDs = []string{}
	}
	metadata := map[string]any{}
	if description := strings.TrimSpace(req.Description); description != "" {
		metadata["description"] = description
	}
	metadataJSON, _ := json.Marshal(metadata)
	linkedArg := s.stringRegistrationArray(linkedToolIDs)
	id := newUUID()
	now := time.Now().UTC()
	enabled := registrationEnabled(req.Enabled)

	err := s.admindb.Exec(r.Context(),
		`INSERT INTO skills_registry (id, name, metadata, instructions, linked_tool_ids, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, req.Name, string(metadataJSON), req.Instructions, linkedArg, enabled, now, now)
	if err != nil {
		if isRegistrationUniqueViolation(err) {
			writeError(w, r, http.StatusConflict, "CONFLICT", "a skill with this name already exists")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to register skill: "+err.Error())
		return
	}
	writeCreatedJSON(w, map[string]any{
		"id": id, "name": req.Name, "description": req.Description,
		"instructions": req.Instructions, "linked_tool_ids": linkedToolIDs,
		"enabled": enabled, "created_at": now,
	})
}

// handleCreateProject serves POST /api/v1/projects.
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if !registrationAdminOnly(w, r) {
		return
	}
	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "name is required and must be at most 200 characters")
		return
	}
	id := newUUID()
	created := time.Now().Unix()
	var createdArg any = created
	if s.db.Backend() == db.BackendPostgres {
		createdArg = time.Unix(created, 0).UTC()
	}
	if err := s.admindb.Exec(r.Context(),
		`INSERT INTO projects (id, name, created_at) VALUES ($1, $2, $3)`,
		id, req.Name, createdArg); err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to register project: "+err.Error())
		return
	}
	writeCreatedJSON(w, map[string]any{"id": id, "name": req.Name, "created_at": created})
}

// handleCreateRoutingRule serves POST /api/v1/routing-rules.
func (s *Server) handleCreateRoutingRule(w http.ResponseWriter, r *http.Request) {
	if !registrationAdminOnly(w, r) {
		return
	}
	var req CreateRoutingRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.SourcePattern = strings.TrimSpace(req.SourcePattern)
	req.EventTypePattern = strings.TrimSpace(req.EventTypePattern)
	req.PayloadPattern = strings.TrimSpace(req.PayloadPattern)
	req.TargetSessionID = strings.TrimSpace(req.TargetSessionID)
	req.TargetWorkflowID = strings.TrimSpace(req.TargetWorkflowID)
	if req.Name == "" || len(req.Name) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "name is required and must be at most 200 characters")
		return
	}
	if req.SourcePattern == "" && req.EventTypePattern == "" && req.PayloadPattern == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "at least one source_pattern, event_type_pattern or payload_pattern is required")
		return
	}
	if req.TargetSessionID == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "target_session_id is required")
		return
	}
	exists, err := s.registrationSessionExists(r, req.TargetSessionID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to validate target session")
		return
	}
	if !exists {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "target_session_id does not name an active session")
		return
	}
	id := "rule_" + newUUID()
	enabled := registrationEnabled(req.Enabled)
	now := time.Now().UTC()
	if err := s.admindb.Exec(r.Context(),
		`INSERT INTO routing_rules (id, name, source_pattern, event_type_pattern, payload_pattern, target_session_id, target_workflow_id, priority, enabled, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, req.Name, req.SourcePattern, req.EventTypePattern, req.PayloadPattern, req.TargetSessionID, nullableRegistrationString(req.TargetWorkflowID), req.Priority, enabled, now); err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to register routing rule: "+err.Error())
		return
	}
	writeCreatedJSON(w, map[string]any{
		"id": id, "name": req.Name, "source_pattern": req.SourcePattern,
		"event_type_pattern": req.EventTypePattern, "payload_pattern": req.PayloadPattern,
		"target_session_id": req.TargetSessionID, "target_workflow_id": req.TargetWorkflowID,
		"priority": req.Priority, "enabled": enabled, "created_at": now,
	})
}

// handleCreateMemoryPage serves POST /api/v1/memory-pages.
func (s *Server) handleCreateMemoryPage(w http.ResponseWriter, r *http.Request) {
	if !registrationAdminOnly(w, r) {
		return
	}
	var req CreateMemoryPageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Name = strings.TrimSpace(req.Name)
	if req.SessionID == "" || req.Name == "" || len(req.Name) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "session_id and a name of at most 200 characters are required")
		return
	}
	if len(req.TargetIDs) > 1000 || len(req.LinkedPageIDs) > 1000 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "target_ids and linked_page_ids each accept at most 1000 entries")
		return
	}
	exists, err := s.registrationSessionExists(r, req.SessionID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to validate session")
		return
	}
	if !exists {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "session_id does not name an active session")
		return
	}
	for _, targetID := range req.TargetIDs {
		if targetID <= 0 {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "target_ids must contain positive memory event ids")
			return
		}
		row, queryErr := s.db.QueryRow(r.Context(),
			`SELECT id FROM memory_events WHERE id = $1 AND session_id = $2`, targetID, req.SessionID)
		if queryErr != nil || row == nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "every target_ids entry must name a memory event in session_id")
			return
		}
	}
	for _, pageID := range req.LinkedPageIDs {
		if pageID <= 0 {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "linked_page_ids must contain positive memory page ids")
			return
		}
	}

	now := time.Now().UTC()
	if req.TargetIDs == nil {
		req.TargetIDs = []int64{}
	}
	if req.LinkedPageIDs == nil {
		req.LinkedPageIDs = []int64{}
	}
	row, err := s.admindb.QueryRow(r.Context(),
		`INSERT INTO memory_pages (name, target_ids, linked_page_ids, session_id, created_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		req.Name, s.int64RegistrationArray(req.TargetIDs), s.int64RegistrationArray(req.LinkedPageIDs), req.SessionID, now)
	if err != nil {
		if isRegistrationUniqueViolation(err) {
			writeError(w, r, http.StatusConflict, "CONFLICT", "a memory page with this name already exists in the session")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to register memory page: "+err.Error())
		return
	}
	var id int64
	if row != nil {
		id = toInt64(row["id"])
	}
	writeCreatedJSON(w, map[string]any{
		"id": id, "session_id": req.SessionID, "name": req.Name,
		"target_ids": req.TargetIDs, "linked_page_ids": req.LinkedPageIDs, "created_at": now,
	})
}
