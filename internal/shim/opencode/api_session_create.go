package opencode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// handleApiSessionCreate serves POST /api/session — upstream
// v2.session.create (ROUTE-ADD-038, declared responses 200, 400, 401).
// It delegates creation to the runtime API service so the returned id is the
// id actually stored by Consensus; malformed input and runtime creation errors
// use the operation's declared InvalidRequestError arm.
func (s *Server) handleApiSessionCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var body map[string]json.RawMessage
	if err := decoder.Decode(&body); err != nil {
		writeV2InvalidRequest(w, "Body", "body", "request body must be a JSON object: "+err.Error())
		return
	}
	if body == nil {
		writeV2InvalidRequest(w, "Body", "body", "request body must be a JSON object")
		return
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		writeV2InvalidRequest(w, "Body", "body", "request body must contain exactly one JSON object: "+err.Error())
		return
	}

	var req struct {
		AgentName string `json:"agent_name"`
		Goal      string `json:"goal"`
		ModelID   string `json:"model_id"`
	}
	if err := json.Unmarshal(mustMarshalRawObject(body), &req); err != nil {
		writeV2InvalidRequest(w, "Body", "body", "request body contains invalid fields: "+err.Error())
		return
	}
	if strings.TrimSpace(req.AgentName) == "" {
		writeV2InvalidRequest(w, "Body", "agent_name", `required field "agent_name" is missing or blank`)
		return
	}
	if strings.TrimSpace(req.Goal) == "" {
		writeV2InvalidRequest(w, "Body", "goal", `required field "goal" is missing or blank`)
		return
	}
	if s.svc == nil {
		writeV2InvalidRequest(w, "Shim", "sessions", "session creation requires the runtime service")
		return
	}

	result, err := s.svc.CreateSession(r.Context(), SessionCreateInput{
		AgentName:     req.AgentName,
		Goal:          req.Goal,
		ModelID:       req.ModelID,
		ContextBudget: 128000,
	})
	if err != nil || result == nil || strings.TrimSpace(result.SessionID) == "" {
		message := "session creation returned no session id"
		if err != nil {
			message = "failed to create session: " + err.Error()
		}
		writeV2InvalidRequest(w, "Shim", "sessions", message)
		return
	}

	// The v2 response is a data envelope. Return the actual runtime identifier
	// and only creation fields the service supplies or the request defines.
	writeJSON(w, map[string]any{
		"data": map[string]any{
			"id":         result.SessionID,
			"agent_name": req.AgentName,
			"goal":       req.Goal,
			"status":     result.Status,
		},
	})
}

func mustMarshalRawObject(body map[string]json.RawMessage) []byte {
	data, _ := json.Marshal(body)
	return data
}
