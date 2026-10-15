package opencode

import (
	"net/http"
)

// handleApiProviderList serves GET /api/provider — upstream v2.provider.list
// (ROUTE-ADD-026, declared responses: 200 {location: LocationInfo, data:
// ProviderV2Info[]} "Retrieve active AI providers so clients can show provider
// availability and configuration.", 400 InvalidRequestError, 401
// UnauthorizedError, 503 ServiceUnavailableError). Previously the path had no
// shim route at all, so net/http answered its default 404 — the
// declared-vs-served artifact's NOT-SERVED class, which a client cannot tell
// apart from "no such opencode operation". The v1 compatibility route
// /provider (upstream provider.list) is a separate declared operation with its
// own body and is untouched.
//
// ch:trace row=ROUTE-ADD-026 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.provider.list test=internal/shim/opencode/api_provider_list_test.go evidence=docs/evidence/ROUTE-ADD-026-live-probe.md witness=none:unattended-worker-session
//
// The declared 200 envelope is {location: LocationInfo, data:
// ProviderV2Info[]}, with `location` derived from the REAL selected workspace
// exactly like its siblings: requestWorkspaceDir (the x-opencode-directory
// header or the configured workdir) overridden by the declared deepObject
// location[directory], project.id = instanceID(directory). The declared
// deepObject `location` selector is validated by the shared
// apiV2LocationSelector, so malformed input answers the declared
// InvalidRequestError 400; undeclared methods keep the sibling
// METHOD_NOT_ALLOWED 405 and the declared UnauthorizedError 401 is served by
// the /api/* auth wall's v2 envelope (server.go v2EnvelopeRoutePaths).
//
// Why `data` is the declared empty, non-null collection: a ProviderV2Info
// requires four members — id, name, api and request — and this runtime can
// source at most the first two.
//
//	id      <- model_registry.provider_id (migration 020; the models.dev
//	           provider column the runtime's own routing catalog carries and
//	           `consensus models sync` writes). This handler READS that store,
//	           and the read is what decides the two declared arms.
//	name    — no store holds a provider display name; only the id exists.
//	api     <- NOT SOURCED. ProviderApi is anyOf ProviderAISDK
//	           {type:"aisdk", package} | ProviderNative {type:"native",
//	           settings}; nothing in this runtime records which protocol a
//	           provider is reached with, nor an AI-SDK package name (this is a
//	           Go runtime; its clients speak OpenAI-compatible and Anthropic
//	           HTTP directly — internal/llm), nor a settings document. Emitting
//	           either arm would invent the member that tells a client HOW to
//	           reach the provider.
//	request <- NOT SOURCED. ProviderRequest is {headers: object<string,string>,
//	           body: object}; the runtime's outbound provider configuration
//	           lives in the harness LLM config (internal/llm/config: base URL,
//	           API key, model), which is not a provider-request descriptor and
//	           must never be copied into a response body — its headers are
//	           where the provider credential goes.
//
// The only other provider-shaped evidence this runtime holds is the stored
// credential rows PUT /auth/{providerID} writes into system_settings
// (auth.<providerID>.<field>, the registry storedProviderAuthMethods derives
// its methods from): those name a provider id and carry no api/request
// descriptor either, so they cannot rescue the two unsourced members. Serving
// an entry built from model_registry.provider_id would therefore invent the api
// and request members — the two that describe provider reachability — which is
// exactly the fabrication this shim must not do (the handleApiModelList /
// handleQuestionList reshape-refusal precedent, recorded in
// api_v2_lists.go for v2.model.list, whose declared response set this
// operation matches: 200, 400, 401, 503).
//
// The read split, mirroring handleApiModelList:
//
//	readable   -> the declared 200 with data: [] (see above)
//	unreadable -> the declared 503 ServiceUnavailableError, service
//	              "model_registry": this instance genuinely cannot name the
//	              providers its own catalog routes to, so the provider registry
//	              is unavailable, not empty. That is the difference the declared
//	              503 exists to express.
func (s *Server) handleApiProviderList(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}
	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}

	// The runtime's real provider evidence: the distinct providers its routing
	// catalog holds enabled models for. The read decides the declared arms — an
	// unreadable catalog cannot name them (503), a readable one names providers
	// whose api/request descriptors no store holds (empty data above).
	providerRows, err := s.db.Query(r.Context(),
		`SELECT DISTINCT provider_id FROM model_registry
		 WHERE enabled = true AND provider_id IS NOT NULL AND provider_id <> ''
		 ORDER BY provider_id`)
	if err != nil {
		writeV2ServiceUnavailable(w,
			"provider registry unavailable: "+err.Error(), "model_registry")
		return
	}
	// The catalog is readable — providerRows holds this runtime's real provider
	// ids — but no ProviderV2Info can be built from one (see the doc comment):
	// the declared 200 answers the empty collection, never an entry with an
	// invented api descriptor or request configuration.
	_ = providerRows

	writeJSON(w, map[string]any{
		"location": location.payload,
		"data":     []any{},
	})
}
