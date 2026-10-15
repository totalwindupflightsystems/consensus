package opencode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	apiIntegrationPathPrefix         = "/api/integration/"
	apiIntegrationConnectOauthSuffix = "/connect/oauth"
)

// handleApiIntegrationConnectOauth serves POST
// /api/integration/{integrationID}/connect/oauth — upstream
// v2.integration.connect.oauth (ROUTE-ADD-020, declared responses 200, 400
// and 401). Previously the path had no shim route and net/http answered its
// default 404.
//
// ch:trace row=ROUTE-ADD-020 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.connect.oauth test=internal/shim/opencode/api_integration_connect_oauth_test.go evidence=docs/evidence/ROUTE-ADD-020-live-probe.md witness=none:unattended-worker-session
//
// The required additionalProperties:false request body is
// {methodID: string, inputs: object<string,string>, label?: string}. A blank
// integrationID, missing or blank methodID, missing or non-object inputs,
// malformed JSON, undeclared body members, and malformed deepObject location
// selectors answer the declared InvalidRequestError 400.
//
// Consensus keeps no integration registry, OAuth provider runtime, or
// IntegrationAttempt store. The truthful declared 200 therefore carries the
// real selected LocationInfo and every required IntegrationAttempt property at
// its empty/zero value. It does not echo the request into an attempt, invent an
// authorization URL or attempt ID, or claim that an OAuth flow was started.
func (s *Server) handleApiIntegrationConnectOauth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	integrationID := strings.TrimSpace(r.PathValue("integrationID"))
	if integrationID == "" {
		writeV2InvalidRequest(w, "Path", "integrationID", "integrationID must not be blank")
		return
	}

	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}
	if _, ok := decodeIntegrationConnectOauthBody(w, r); !ok {
		return
	}

	writeJSON(w, map[string]any{
		"location": location.payload,
		"data": map[string]any{
			"attemptID":    "",
			"url":          "",
			"instructions": "",
			"mode":         "",
			"time": map[string]any{
				"created": 0,
				"expires": 0,
			},
		},
	})
}

// decodeIntegrationConnectOauthBody validates presence separately from value
// so an empty inputs object is accepted while a missing or null inputs member
// is rejected.
func decodeIntegrationConnectOauthBody(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var body map[string]json.RawMessage
	if err := decoder.Decode(&body); err != nil {
		writeV2InvalidRequest(w, "Body", "body", "request body must be a JSON object: "+err.Error())
		return nil, false
	}
	if body == nil {
		writeV2InvalidRequest(w, "Body", "body", "request body must be a JSON object")
		return nil, false
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		writeV2InvalidRequest(w, "Body", "body", "request body must contain exactly one JSON object: "+err.Error())
		return nil, false
	}

	for field := range body {
		switch field {
		case "methodID", "inputs", "label":
		default:
			writeV2InvalidRequest(w, "Body", field, fmt.Sprintf("unknown field %q", field))
			return nil, false
		}
	}

	rawMethodID, exists := body["methodID"]
	if !exists {
		writeV2InvalidRequest(w, "Body", "methodID", `required field "methodID" is missing`)
		return nil, false
	}
	var methodID string
	if err := json.Unmarshal(rawMethodID, &methodID); err != nil || strings.TrimSpace(methodID) == "" {
		writeV2InvalidRequest(w, "Body", "methodID", `field "methodID" must be a non-blank string`)
		return nil, false
	}

	rawInputs, exists := body["inputs"]
	if !exists {
		writeV2InvalidRequest(w, "Body", "inputs", `required field "inputs" is missing`)
		return nil, false
	}
	var inputs map[string]string
	if err := json.Unmarshal(rawInputs, &inputs); err != nil || inputs == nil {
		writeV2InvalidRequest(w, "Body", "inputs", `field "inputs" must be an object whose values are strings`)
		return nil, false
	}

	if rawLabel, exists := body["label"]; exists {
		var label string
		if err := json.Unmarshal(rawLabel, &label); err != nil {
			writeV2InvalidRequest(w, "Body", "label", `field "label" must be a string`)
			return nil, false
		}
	}
	return body, true
}

// isApiIntegrationConnectOauthPath matches only the declared path shape. The
// auth wall uses it to select the v2 UnauthorizedError envelope without
// claiming the rest of /api/integration/*.
func isApiIntegrationConnectOauthPath(path string) bool {
	if !strings.HasPrefix(path, apiIntegrationPathPrefix) || !strings.HasSuffix(path, apiIntegrationConnectOauthSuffix) {
		return false
	}
	integrationID := strings.TrimSuffix(strings.TrimPrefix(path, apiIntegrationPathPrefix), apiIntegrationConnectOauthSuffix)
	return integrationID != "" && !strings.Contains(integrationID, "/")
}
