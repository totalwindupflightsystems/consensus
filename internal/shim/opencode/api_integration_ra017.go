package opencode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	apiIntegrationAttemptPrefix  = "/api/integration/attempt/"
	apiIntegrationCompleteSuffix = "/complete"
)

// handleApiIntegrationRA017 serves POST
// /api/integration/attempt/{attemptID}/complete — upstream
// v2.integration.attempt.complete (ROUTE-ADD-017, SHIM-DRIFT-004, declared
// responses: 204 No Content, 400 InvalidRequestError, 401 UnauthorizedError).
// Before this route was registered, net/http answered its default 404.
//
// ch:trace row=ROUTE-ADD-017 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.attempt.complete test=internal/shim/opencode/api_integration_ra017_test.go witness=none:unattended-worker-session
//
// The declared request body is a required object with one optional string
// property, code, and additionalProperties:false. The optional location query
// parameter is the same deepObject selector v2.integration.list declares and
// is validated by apiV2LocationSelector. Malformed input answers the declared
// 400 through writeV2InvalidRequest; authentication remains the existing
// /api/* wall and its declared v2 UnauthorizedError envelope.
//
// Truthful 204: this runtime keeps no integration-attempt store and advertises
// no OAuth integration-attempt producer, so there is no pending attempt or
// resulting integration credential to mutate. A well-formed completion is
// therefore an idempotent no-op, like v2.credential.remove on the same
// store-less surface: it acknowledges completion with the declared empty 204
// but never fabricates an attempt, credential, or response row.
func (s *Server) handleApiIntegrationRA017(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	if _, ok := s.apiV2LocationSelector(w, r); !ok {
		return
	}

	body, ok := decodeIntegrationCompleteBody(w, r)
	if !ok {
		return
	}
	// The optional code was validated against the declared string shape. There
	// is no integration-attempt store to receive it; retaining or exchanging it
	// would invent a credential flow this runtime does not have.
	_ = body

	w.WriteHeader(http.StatusNoContent)
}

// decodeIntegrationCompleteBody validates the operation's required
// additionalProperties:false object without inventing constraints on code: an
// empty string is valid because the declared schema has no minLength.
func decodeIntegrationCompleteBody(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, bool) {
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
		if field != "code" {
			writeV2InvalidRequest(w, "Body", field,
				fmt.Sprintf("unknown field %q: the declared body schema allows only code", field))
			return nil, false
		}
	}
	if raw, exists := body["code"]; exists {
		var code any
		if err := json.Unmarshal(raw, &code); err != nil {
			writeV2InvalidRequest(w, "Body", "code", `field "code" must be a string`)
			return nil, false
		}
		if _, isString := code.(string); !isString {
			writeV2InvalidRequest(w, "Body", "code", `field "code" must be a string`)
			return nil, false
		}
	}
	return body, true
}

// isApiIntegrationCompletePath matches exactly the one declared parameterized
// route shape. It is shared with the auth wall so concrete attempt IDs receive
// the v2 UnauthorizedError envelope without turning /api/integration/* into a
// catch-all.
func isApiIntegrationCompletePath(path string) bool {
	if !strings.HasPrefix(path, apiIntegrationAttemptPrefix) || !strings.HasSuffix(path, apiIntegrationCompleteSuffix) {
		return false
	}
	attemptID := strings.TrimSuffix(strings.TrimPrefix(path, apiIntegrationAttemptPrefix), apiIntegrationCompleteSuffix)
	return attemptID != "" && !strings.Contains(attemptID, "/")
}
