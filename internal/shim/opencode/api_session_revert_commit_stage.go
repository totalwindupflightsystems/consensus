package opencode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// handleSessionRevertCommit serves POST /api/session/{sessionID}/revert/commit
// (v2.session.revert.commit). Consensus has no upstream revert engine, so the
// declared bodyless success is an honest idempotent no-op after session lookup.
func (s *Server) handleSessionRevertCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if !s.resolveRevertSession(w, r) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSessionRevertStage serves POST /api/session/{sessionID}/revert/stage
// (v2.session.revert.stage). Consensus exposes the declared minimal
// RevertState envelope using the requested messageID; no file or snapshot
// claims are added when the runtime has no corresponding state.
func (s *Server) handleSessionRevertStage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" || strings.Contains(sessionID, "/") {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must be one non-blank path segment")
		return
	}
	messageID, ok := validateRevertStageBody(w, r)
	if !ok {
		return
	}
	if !s.resolveRevertSession(w, r) {
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]any{
		"data": map[string]any{"messageID": messageID},
	})
}

func (s *Server) resolveRevertSession(w http.ResponseWriter, r *http.Request) bool {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" || strings.Contains(sessionID, "/") {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must be one non-blank path segment")
		return false
	}
	if !s.skipAuth {
		keySessionID, ok := s.validateAuth(r)
		if !ok {
			writeV2Unauthorized(w, "invalid credentials")
			return false
		}
		if keySessionID != "" && keySessionID != sessionID {
			writeOpencodeError(w, r, http.StatusForbidden, "FORBIDDEN", "session key is not authorized for this session")
			return false
		}
	}
	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: the shim has no session store")
		return false
	}
	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: nothing to revert", sessionID))
			return false
		}
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: "+err.Error())
		return false
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: nothing to revert", sessionID))
		return false
	}
	return true
}

func validateRevertStageBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r.Body == nil {
		writeV2InvalidRequest(w, "Body", "messageID", "request body must contain messageID")
		return "", false
	}
	var body struct {
		MessageID *string `json:"messageID"`
		PartID    *string `json:"partID"`
		Files     *bool   `json:"files"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeV2InvalidRequest(w, "Body", "messageID", "request body must be a JSON object containing messageID")
		return "", false
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeV2InvalidRequest(w, "Body", "messageID", "request body must contain exactly one JSON object")
		return "", false
	}
	if body.MessageID == nil || !strings.HasPrefix(*body.MessageID, "msg_") {
		writeV2InvalidRequest(w, "Body", "messageID", "messageID must be a string matching ^msg_")
		return "", false
	}
	return *body.MessageID, true
}

func isApiSessionRevertCommitPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	return ok && sessionID != "" && tail == "revert/commit"
}

func isApiSessionRevertStagePath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	return ok && sessionID != "" && tail == "revert/stage"
}
