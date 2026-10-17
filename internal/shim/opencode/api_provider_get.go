package opencode

import (
	"net/http"
	"strings"
)

// handleApiProviderGet serves GET /api/provider/{providerID} — upstream
// v2.provider.get (ROUTE-ADD-027; the board row's source item SHIM-DRIFT-023 is
// POSITIONAL — in the committed artifact that id is POST
// /api/session/{sessionID}/permission and v2.provider.get carries
// SHIM-DRIFT-001; the operation, path and declared response set named in the
// brief — 200, 400, 401, 404, 503 — all identify this row). Declared responses:
// 200 {location: LocationInfo, data: ProviderV2Info} "Retrieve a single AI
// provider so clients can inspect its availability and endpoint settings",
// 400 InvalidRequestError, 401 UnauthorizedError, 404 ProviderNotFoundError,
// 503 ServiceUnavailableError. Before this route was registered the path had no
// shim route at all, so net/http answered its default 404 — the
// declared-vs-served artifact's NOT-SERVED class, indistinguishable from "no
// such opencode operation".
//
// ch:trace row=ROUTE-ADD-027 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.provider.get test=internal/shim/opencode/api_provider_get_test.go evidence=docs/evidence/ROUTE-ADD-027-live-probe.md witness=none:unattended-worker-session
//
// The declared 200 envelope is {location: LocationInfo, data: ProviderV2Info},
// with `location` derived from the REAL selected workspace exactly like its
// siblings: requestWorkspaceDir (the x-opencode-directory header or the
// configured workdir) overridden by the declared deepObject
// location[directory], project.id = instanceID(directory). The declared
// deepObject `location` selector is validated by the shared
// apiV2LocationSelector, so malformed input answers the declared
// InvalidRequestError 400; undeclared methods keep the sibling
// METHOD_NOT_ALLOWED 405 and the declared UnauthorizedError 401 is served by
// the /api/* auth wall's v2 envelope (server.go isV2EnvelopeRoutePath admits
// the exact one-segment /api/provider/{providerID} shape).
//
// WHICH PROVIDERS EXIST (the 200 vs 404 decision) is taken from the two real
// provider stores this runtime holds — never from a probe, a guess or a
// hard-coded provider list:
//
//	model_registry.provider_id (migration 020) — the models.dev provider
//	  column the runtime's own routing catalog carries and `consensus models
//	  sync` writes. The sibling list operation reads the same store.
//	system_settings keys auth.<providerID>.<field> — the credential rows
//	  PUT /auth/{providerID} writes (SPEC-017 §3.2), the same derivation
//	  storedProviderAuthMethods reports through GET /provider/auth.
//
// A providerID that appears in NEITHER store is not something this instance can
// name, so it answers the declared 404 ProviderNotFoundError (the
// writeOpencodeNotFoundError typed body: {_tag, providerID, message}, the
// document's exact required member set with additionalProperties:false).
//
// WHY THE 200 CAN BE SERVED TRUTHFULLY (and what each member means). The pinned
// schema requires id, name, api and request on ProviderV2Info. This runtime
// sources each of them from its own state, and says exactly what it does not
// have instead of inventing a value:
//
//	id      — the resolved {providerID}, matched against a real store row above.
//	name    — the providerID itself. The runtime holds no display-name store
//	          (no table, migration or config field carries one); the only name
//	          it has for a provider is its catalog id, so the honest answer is
//	          that id echoed, not a fabricated marketing label.
//	api     — {"type":"native","settings":{}}. ProviderApi is anyOf
//	          ProviderAISDK {type:"aisdk", package} | ProviderNative
//	          {type:"native", settings}. This is a Go runtime that reaches
//	          every provider through its own HTTP clients (internal/llm), not
//	          through an AI-SDK package — so "native" is the true arm and the
//	          AI-SDK arm (which would require naming a JS package) would be
//	          the fabrication. `settings` is empty because no store holds a
//	          provider-specific settings document.
//	request — {"headers":{},"body":{}}. ProviderRequest declares a
//	          client-facing request descriptor. This runtime configures no
//	          per-provider outbound request template: its provider
//	          configuration (base URL, model, API key) lives in the harness LLM
//	          config, and its headers are exactly where the provider credential
//	          goes — that configuration is never copied into a response body
//	          (the ROUTE-ADD-026 rule). Empty objects state "none configured";
//	          they leak nothing and claim nothing.
//	disabled — emitted only when the provider carries catalog rows, and then it
//	          is the real catalog state: true when NO enabled model_registry row
//	          routes to that provider. A credential-only provider has no
//	          catalog state to report, so the optional member is omitted rather
//	          than guessed.
//
// No member is copied from the harness LLM config and no credential, key or
// header value ever reaches the response.
//
// The two store reads are the declared 503 arms: an unreadable model_registry
// names service "model_registry", an unreadable credential store names
// "system_settings" — this instance genuinely cannot answer whether the
// provider exists, which is the difference the declared 503 exists to express
// (the handleApiModelList / handleApiProviderList precedent). The selector and
// the path parameter are validated BEFORE any store is read, so a malformed
// request is never masked by an unavailable store.
func (s *Server) handleApiProviderGet(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}

	providerID := strings.TrimSpace(r.PathValue("providerID"))
	if providerID == "" {
		// The pinned document declares providerID as a plain required string
		// with NO pattern, so the only malformed id this handler can see is a
		// present-but-blank one (a whitespace-only segment such as %20); an
		// actually-empty segment cannot match the {providerID} route pattern
		// and keeps net/http's default 404. Inventing a shape constraint would
		// fabricate one the contract does not state.
		writeV2InvalidRequest(w, "Path", "providerID", "providerID must not be blank")
		return
	}

	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}

	// Provider evidence 1: the routing catalog. The read decides the 503 arm before
	// the 404 arm — an unreadable catalog cannot prove a provider is absent.
	//
	// The query carries NO placeholder: model_registry is the runtime's small
	// routing catalog (handleConfigProviders reads it whole as well), and the
	// provider-id match is done in Go. That keeps ONE query valid on both
	// backends — the shim runs on PostgreSQL and on SQLite, whose pure-Go
	// driver treats "$1" as a NAMED parameter ("missing named argument")
	// rather than a positional ordinal.
	catalogRows, err := s.db.Query(r.Context(),
		`SELECT provider_id, enabled FROM model_registry
		 WHERE provider_id IS NOT NULL AND provider_id <> ''`)
	if err != nil {
		writeV2ServiceUnavailable(w,
			"provider registry unavailable: "+err.Error(), "model_registry")
		return
	}

	cataloged := false
	catalogEnabled := false
	for _, row := range catalogRows {
		if toString(row["provider_id"]) != providerID {
			continue
		}
		cataloged = true
		if toBool(row["enabled"]) {
			catalogEnabled = true
			break
		}
	}

	if !cataloged {
		// Provider evidence 2: the stored credential rows PUT /auth/{providerID}
		// keeps, derived by the same helper GET /provider/auth advertises from so
		// the two surfaces cannot disagree about which providers exist.
		methods, err := s.storedProviderAuthMethods(r.Context())
		if err != nil {
			writeV2ServiceUnavailable(w,
				"provider credential store unavailable: "+err.Error(), "system_settings")
			return
		}
		if _, registered := methods[providerID]; !registered {
			writeOpencodeNotFoundError(w, "ProviderNotFoundError", "providerID", providerID,
				"Provider not found: "+providerID)
			return
		}
	}

	data := map[string]any{
		"id":   providerID,
		"name": providerID,
		"api": map[string]any{
			"type":     "native",
			"settings": map[string]any{},
		},
		"request": map[string]any{
			"headers": map[string]any{},
			"body":    map[string]any{},
		},
	}
	if cataloged {
		// The real catalog state: no enabled model routes to this provider.
		data["disabled"] = !catalogEnabled
	}

	writeJSON(w, map[string]any{
		"location": location.payload,
		"data":     data,
	})
}
