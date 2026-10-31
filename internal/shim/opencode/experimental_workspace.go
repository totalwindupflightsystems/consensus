package opencode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ROUTE-ADD-076 — GET /experimental/workspace (experimental.workspace.list)
// ROUTE-ADD-077 — POST /experimental/workspace (experimental.workspace.create)
//
// The pinned upstream runtime owns a workspace adapter and persistent workspace
// registry. Consensus owns neither, so the list operation truthfully reports an
// empty collection and the create operation reports the declared
// WorkspaceCreateError instead of inventing a workspace or claiming that an
// adapter was started.
func (s *Server) handleExperimentalWorkspace(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleExperimentalWorkspaceList(w, r)
	case http.MethodPost:
		s.handleExperimentalWorkspaceCreate(w, r)
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
	}
}

// handleExperimentalWorkspaceList serves GET /experimental/workspace — the
// declared 200 Workspace[] or 400 BadRequestError. No Consensus workspace
// registry exists; [] is the truthful state, rather than a synthetic Workspace
// object derived from the process directory.
func validateExperimentalWorkspaceSelectors(w http.ResponseWriter, r *http.Request) bool {
	q := r.URL.Query()
	for _, param := range []string{"directory", "workspace"} {
		if values, present := q[param]; present && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeOpencodeNamedError(w, r, http.StatusBadRequest, "BadRequest",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return false
		}
	}
	return true
}

func (s *Server) handleExperimentalWorkspaceList(w http.ResponseWriter, r *http.Request) {
	if !validateExperimentalWorkspaceSelectors(w, r) {
		return
	}
	writeJSON(w, []any{})
}

// handleExperimentalWorkspaceCreate serves POST /experimental/workspace — the
// declared 200 Workspace or declared 400 errors. The request is validated
// against the pinned WorkspaceCreateInput before the runtime limitation is
// reported as WorkspaceCreateError.
func (s *Server) handleExperimentalWorkspaceCreate(w http.ResponseWriter, r *http.Request) {
	if !validateExperimentalWorkspaceSelectors(w, r) {
		return
	}
	if !decodeExperimentalWorkspaceCreateBody(w, r) {
		return
	}
	writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorkspaceCreateError",
		"workspace adapters and the persistent workspace registry are not available in this runtime")
}

// decodeExperimentalWorkspaceCreateBody validates the pinned request body:
// {id?: ^wrk string, type: string, branch?: string|null, extra?: any|null}.
// The body itself is optional in the OpenAPI document; when supplied, its
// required type member and additionalProperties:false schema still apply.
func decodeExperimentalWorkspaceCreateBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil {
		return true
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		writeV2InvalidRequest(w, "Body", "", "request body must be one JSON object")
		return false
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeV2InvalidRequest(w, "Body", "", "request body must contain exactly one JSON object")
		return false
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		writeV2InvalidRequest(w, "Body", "", "request body must be a JSON object")
		return false
	}
	for field := range fields {
		if field != "id" && field != "type" && field != "branch" && field != "extra" {
			writeV2InvalidRequest(w, "Body", field, fmt.Sprintf("unknown field %q", field))
			return false
		}
	}
	if value, ok := fields["type"]; !ok {
		writeV2InvalidRequest(w, "Payload", "type", `field "type" is required`)
		return false
	} else {
		var workspaceType string
		if err := json.Unmarshal(value, &workspaceType); err != nil || strings.TrimSpace(workspaceType) == "" {
			writeV2InvalidRequest(w, "Payload", "type", `field "type" must be a non-blank string`)
			return false
		}
	}
	if value, ok := fields["id"]; ok {
		var id string
		if err := json.Unmarshal(value, &id); err != nil || !strings.HasPrefix(id, "wrk") {
			writeV2InvalidRequest(w, "Payload", "id", `field "id" must be a string matching ^wrk`)
			return false
		}
	}
	if value, ok := fields["branch"]; ok && string(value) != "null" {
		var branch string
		if err := json.Unmarshal(value, &branch); err != nil {
			writeV2InvalidRequest(w, "Payload", "branch", `field "branch" must be a string or null`)
			return false
		}
	}
	return true
}
