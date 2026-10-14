package opencode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ============================================================================
// ROUTE-ADD-008 — PATCH /api/credential/{credentialID} (v2.credential.update)
//
// ch:trace row=ROUTE-ADD-008 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.credential.update test=internal/shim/opencode/api_credential_update_test.go witness=none:unattended-worker-session
// ============================================================================

// handleApiCredential dispatches the /api/credential/{credentialID} path — the
// declared v2 credential family (upstream operationIds v2.credential.update and
// v2.credential.remove; declared responses per operation: 204, 400
// InvalidRequestError, 401 UnauthorizedError). Before ROUTE-ADD-008 the path had
// no shim route at all, so net/http answered the default 404 (the drift
// artifact's NOT-SERVED class, source item SHIM-DRIFT-002 for PATCH).
//
// Method split: PATCH serves v2.credential.update (credentialUpdate, this
// wave). DELETE is v2.credential.remove and is ROUTE-ADD-007's scope — the
// sibling worker serves it on its own branch; until the foreman's merge lands
// that dispatch, DELETE keeps the honest 405 METHOD_NOT_ALLOWED rather than
// pretending the removal happened. Every other declared-or-not method keeps the
// sibling method-guard envelope (handleApiAgentList convention).

// credentialUpdate serves PATCH /api/credential/{credentialID} — upstream
// v2.credential.update ("Update a stored credential label."; declared
// responses: 204 <No Content>, 400 InvalidRequestError, 401 UnauthorizedError;
// specs/openapi/upstream/openapi-1.18.33.json
// paths."/api/credential/{credentialID}".patch).
//
// Declared input: the required path parameter credentialID (schema type
// string — the pinned document declares NO pattern for it, unlike ^pty / ^ses,
// so the only ill-shape the contract itself defines is an absent or blank
// value); the v2 SDK's deepObject `location` selector
// (location[directory] / location[workspace], additionalProperties:false); and
// the required requestBody {label: string} (additionalProperties:false, `label`
// required).
//
// TRUTHFULNESS — why the declared 204 is NOT served. Upstream's 204 asserts
// that a stored credential's label WAS updated. The Consensus runtime keeps no
// credential store at all: no credentials table exists in the migration set
// (001..026), the auth material the shim does hold (api_keys) is never exposed
// through this declared path, and the provider OAuth surface it serves
// (provider.auth / provider.{providerID}/oauth/callback) STORES nothing for a
// caller to relabel. Serving 204 would echo an update that never happened —
// the exact fabrication the ROUTE-FIX-037 part.update precedent forbids
// ("declared 404 for every well-formed partID — never fabricate a 200"), whose
// narrowing-rationale entry in
// specs/openapi/upstream/consensus-shim-served-surface.yaml this arm set
// mirrors. The truthful answer to a well-formed update request is therefore
// the DECLARED 400 InvalidRequestError naming the missing store — 400 is in
// the declared response set, 204 is reachable only through state this runtime
// does not have, and no undeclared code is invented. A future credential
// store widens this handler to the full contract (the narrowing is the honest
// fallback while the state the declared success describes does not exist).
//
// Validation order (the pty.connectToken / credential sibling convention):
//
//  1. blank credentialID -> declared 400 kind Path;
//  2. a requestBody that violates the declared schema (not an object, missing
//     or non-string `label`, an undeclared key) -> declared 400 kind Body;
//  3. a malformed location selector (unknown location[...] field,
//     present-but-blank value, unreadable/non-directory location[directory],
//     non-^wrk location[workspace]) -> declared 400 kind Query, via the shared
//     apiV2LocationSelector (the ROUTE-ADD-021 spelling);
//  4. no credential store -> the declared 400 InvalidRequestError naming the
//     credentialID, never a fabricated 204.
//
// The declared 401 arm is answered by the /api/* auth wall (the
// /api/credential/ prefix joins the v2-envelope branch of authMiddleware, so
// the body is the declared {_tag: "UnauthorizedError", message}).
func (s *Server) credentialUpdate(w http.ResponseWriter, r *http.Request) {
	// 1. credentialID is a required path parameter; the pinned document
	// declares no pattern beyond type: string, so a blank value is the one
	// shape the contract itself calls ill-formed. The route registers the
	// prefix "/api/credential/" (one dispatch entry per path family), so the
	// id is the URL suffix after it — the declared shape is a single segment.
	credentialID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/credential/"))
	if credentialID == "" {
		writeV2InvalidRequest(w, "Path", "credentialID",
			`required path parameter "credentialID" is missing`)
		return
	}

	// 2. The declared requestBody {label*: string} additionalProperties:false —
	// a body violating the schema is the declared 400 (the ptyUpdate strict
	// body-validation convention; upstream's sdk marks the body required). The
	// strict shape is checked on a generic object so an undeclared key is
	// refused rather than silently dropped.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeV2InvalidRequest(w, "Body", "label",
			fmt.Sprintf("request body must be a JSON object with a string \"label\": %v", err))
		return
	}
	labelRaw, ok := raw["label"]
	if !ok {
		writeV2InvalidRequest(w, "Body", "label", `required field "label" is missing`)
		return
	}
	var label string
	if !json.Valid(labelRaw) || json.Unmarshal(labelRaw, &label) != nil {
		writeV2InvalidRequest(w, "Body", "label", `field "label" must be a string`)
		return
	}
	_ = label // validated, not applied: no credential store exists to apply it to
	for key := range raw {
		if key != "label" {
			writeV2InvalidRequest(w, "Body", key,
				fmt.Sprintf("unknown request body field %q (the declared schema is additionalProperties: false)", key))
			return
		}
	}

	// 3. The declared deepObject location selector — same validation arms as
	// the sibling v2 operations (unknown field, blank value, unreadable or
	// non-directory location[directory], non-^wrk location[workspace]).
	if _, ok := s.apiV2LocationSelector(w, r); !ok {
		return
	}

	// 4. No credential store exists (see the truthfulness note above): there is
	// no stored credential with this id to relabel, so the declared 204 can
	// only lie. The declared 400 InvalidRequestError is the truthful answer —
	// never a fabricated success.
	writeV2InvalidRequest(w, "", "", fmt.Sprintf(
		"credential %q not found: this runtime keeps no credential store, so no stored credential can be updated (the declared 204 is never fabricated)",
		credentialID))
}
