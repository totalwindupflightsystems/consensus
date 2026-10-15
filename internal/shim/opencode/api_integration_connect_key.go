package opencode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const apiIntegrationConnectKeySuffix = "/connect/key"

// handleApiIntegrationConnectKey serves POST
// /api/integration/{integrationID}/connect/key — upstream
// v2.integration.connect.key (ROUTE-ADD-019, formerly SHIM-DRIFT-015,
// declared responses: 204 No Content, 400 InvalidRequestError, 401
// UnauthorizedError). Before this exact route was registered, net/http
// answered its default 404.
//
// ch:trace row=ROUTE-ADD-019 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.connect.key test=internal/shim/opencode/api_integration_connect_key_test.go evidence=docs/evidence/ROUTE-ADD-019-live-probe.md witness=none:unattended-worker-session
//
// The required request body is the additionalProperties:false object
// {key: string, label?: string}; ROUTE-ADD-019 additionally rejects an empty or
// whitespace-only key. The optional deepObject location selector is validated
// by apiV2LocationSelector. Every malformed input answers the declared
// InvalidRequestError 400 through writeV2InvalidRequest.
//
// Truthful 204: Consensus keeps no integration registry and no
// credential-connect runtime. A well-formed request is accepted as an
// idempotent no-op with the declared empty response; the key is never retained,
// no IntegrationAttempt or credential row is fabricated, and integrationID is
// never echoed as a created object. The data-bearing success shape belongs to
// v2.integration.connect.oauth, not this operation.
func (s *Server) handleApiIntegrationConnectKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	integrationID := strings.TrimSpace(r.PathValue("integrationID"))
	if integrationID == "" {
		writeV2InvalidRequest(w, "Path", "integrationID", "integrationID must not be blank")
		return
	}

	if _, ok := s.apiV2LocationSelector(w, r); !ok {
		return
	}

	if !decodeApiIntegrationConnectKeyBody(w, r) {
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// decodeApiIntegrationConnectKeyBody validates the operation's required body
// without retaining its credential material. It accepts an optional string
// label (including the empty string, because the schema declares no minLength)
// and rejects every undeclared field.
func decodeApiIntegrationConnectKeyBody(w http.ResponseWriter, r *http.Request) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var body map[string]json.RawMessage
	if err := decoder.Decode(&body); err != nil {
		writeV2InvalidRequest(w, "Body", "body", "request body must be a JSON object: "+err.Error())
		return false
	}
	if body == nil {
		writeV2InvalidRequest(w, "Body", "body", "request body must be a JSON object")
		return false
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		writeV2InvalidRequest(w, "Body", "body", "request body must contain exactly one JSON object: "+err.Error())
		return false
	}

	for field := range body {
		if field != "key" && field != "label" {
			writeV2InvalidRequest(w, "Body", field,
				fmt.Sprintf("unknown field %q: the declared body schema allows only key and label", field))
			return false
		}
	}

	keyRaw, exists := body["key"]
	if !exists {
		writeV2InvalidRequest(w, "Body", "key", `required field "key" is missing`)
		return false
	}
	var keyValue any
	if err := json.Unmarshal(keyRaw, &keyValue); err != nil {
		writeV2InvalidRequest(w, "Body", "key", `field "key" must be a string`)
		return false
	}
	key, isString := keyValue.(string)
	if !isString {
		writeV2InvalidRequest(w, "Body", "key", `field "key" must be a string`)
		return false
	}
	if strings.TrimSpace(key) == "" {
		writeV2InvalidRequest(w, "Body", "key", `field "key" must not be blank`)
		return false
	}

	if labelRaw, exists := body["label"]; exists {
		var labelValue any
		if err := json.Unmarshal(labelRaw, &labelValue); err != nil {
			writeV2InvalidRequest(w, "Body", "label", `field "label" must be a string`)
			return false
		}
		if _, isString := labelValue.(string); !isString {
			writeV2InvalidRequest(w, "Body", "label", `field "label" must be a string`)
			return false
		}
	}

	return true
}

// isApiIntegrationConnectKeyPath admits only the declared concrete route shape
// to the v2 UnauthorizedError auth branch. Deeper unregistered integration
// paths keep the shim's own envelope.
func isApiIntegrationConnectKeyPath(path string) bool {
	const prefix = "/api/integration/"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, apiIntegrationConnectKeySuffix) {
		return false
	}
	integrationID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), apiIntegrationConnectKeySuffix)
	return integrationID != "" && !strings.Contains(integrationID, "/")
}
