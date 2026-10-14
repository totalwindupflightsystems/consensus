package opencode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ============================================================================
// ROUTE-ADD-021 — the declared v2 read-only list surface
// ============================================================================
//
// Seven operations the pinned upstream document declares and the shim never
// routed (the declared-vs-served artifact's NOT-SERVED class — no shim route at
// all, so net/http answered the default 404, which a client cannot tell apart
// from "no such opencode operation"):
//
//	GET /api/location           v2.location.get
//	GET /api/model              v2.model.list                (declares 503 too)
//	GET /api/reference          v2.reference.list
//	GET /api/skill              v2.skill.list
//	GET /api/permission/request v2.permission.request.list
//	GET /api/permission/saved   v2.permission.saved.list
//	GET /api/question/request   v2.question.request.list
//
// Six of the seven declare the v2 SDK's deepObject `location` selector
// (location[directory] / location[workspace], additionalProperties:false in the
// pinned document); v2.permission.saved.list declares a plain `projectID`
// string instead. Every handler enforces the declared GET (405 with the sibling
// METHOD_NOT_ALLOWED envelope otherwise — the ROUTE-ADD-005 convention),
// answers the declared InvalidRequestError 400 for malformed input
// (writeV2InvalidRequest, the handleFSFind/handleApiAgentList spelling), and
// relies on the existing /api/* auth wall for the declared UnauthorizedError
// 401.
//
// Truthfulness rule (the same one handleQuestionList/handleSkill/handleFormatter
// answer with): a NON-EMPTY payload may only carry rows a real runtime store
// holds in a shape the declared schema can actually express. Where no such store
// exists, or where a real store's rows cannot be reshaped without inventing
// required members, the handler answers the declared 200 with an EMPTY array and
// says so in its own doc comment. Nothing here fabricates a row.

// apiV2Location is the selected workspace a v2 operation answers with: the
// absolute root the handler resolved plus the declared LocationInfo payload
// (directory, project{id,directory}, optional workspaceID).
type apiV2Location struct {
	directory string
	payload   map[string]any
}

// apiV2RequireGet enforces the declared GET method, writing the sibling
// METHOD_NOT_ALLOWED envelope and reporting false for every other method. The
// declared response sets of these operations do not include 405, but a
// registered route may not lie with a 404/405-free empty success either: the
// shim's method-guard convention (handleApiAgentList, handleFSFind,
// handleFormatter) keeps the honest 405 instead of serving a GET-only payload
// to a POST.
func apiV2RequireGet(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return false
	}
	return true
}

// apiV2LocationSelector validates the declared deepObject `location` query
// selector and resolves the workspace the operation answers for. It mirrors
// handleApiAgentList/handleFSFind exactly (same declared spelling, same error
// arms); it is factored out here because seven operations share that contract.
//
// Malformed input — an unknown location[...] field (the pinned schema sets
// additionalProperties:false, so an undeclared field is a contract violation,
// not an extension point), a present-but-blank value, an unreadable path, a
// path that is not a directory, or a workspace id that does not match the
// declared ^wrk pattern — answers the declared InvalidRequestError 400
// (writeV2InvalidRequest) and reports false.
//
// The resolved directory is the real selected workspace: the declared
// location[directory] override, else the request's fixed-workspace directory
// (x-opencode-directory header or the configured workdir), the same derivation
// /path, /instance and handleFSFind use. project.id is instanceID(dir) — the
// singleton instance id derived from that directory — and project.directory is
// the directory itself; no LocationInfo member is invented.
func (s *Server) apiV2LocationSelector(w http.ResponseWriter, r *http.Request) (apiV2Location, bool) {
	q := r.URL.Query()
	for key := range q {
		if strings.HasPrefix(key, "location[") && key != "location[directory]" && key != "location[workspace]" {
			writeV2InvalidRequest(w, "Query", key, fmt.Sprintf("unknown location field %q", key))
			return apiV2Location{}, false
		}
	}
	for _, key := range []string{"location[directory]", "location[workspace]"} {
		if values, ok := q[key]; ok && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeV2InvalidRequest(w, "Query", key, fmt.Sprintf("query parameter %q must not be blank", key))
			return apiV2Location{}, false
		}
	}

	workspaceID := strings.TrimSpace(q.Get("location[workspace]"))
	if workspaceID != "" && !strings.HasPrefix(workspaceID, "wrk") {
		writeV2InvalidRequest(w, "Query", "location[workspace]", "location[workspace] must start with wrk")
		return apiV2Location{}, false
	}

	root := strings.TrimSpace(q.Get("location[directory]"))
	if root == "" {
		root = s.requestWorkspaceDir(r)
	}
	absoluteRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		writeV2InvalidRequest(w, "Query", "location[directory]", "location directory is invalid")
		return apiV2Location{}, false
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		writeV2InvalidRequest(w, "Query", "location[directory]",
			fmt.Sprintf("location directory %q cannot be read: %v", absoluteRoot, err))
		return apiV2Location{}, false
	}
	if !info.IsDir() {
		writeV2InvalidRequest(w, "Query", "location[directory]",
			fmt.Sprintf("location directory %q is not a directory", absoluteRoot))
		return apiV2Location{}, false
	}

	payload := map[string]any{
		"directory": absoluteRoot,
		"project": map[string]any{
			"id":        instanceID(absoluteRoot),
			"directory": absoluteRoot,
		},
	}
	if workspaceID != "" {
		payload["workspaceID"] = workspaceID
	}
	return apiV2Location{directory: absoluteRoot, payload: payload}, true
}

// writeV2ServiceUnavailable writes the declared ServiceUnavailableError body
// ({_tag, message, optional service}) — the error arm only v2.model.list
// declares among the ROUTE-ADD-021 operations. service names the runtime
// component that is unavailable, so the client learns what could not be read.
func writeV2ServiceUnavailable(w http.ResponseWriter, message, service string) {
	body := map[string]any{
		"_tag":    "ServiceUnavailableError",
		"message": message,
	}
	if service != "" {
		body["service"] = service
	}
	writeJSONStatus(w, http.StatusServiceUnavailable, body)
}

// handleApiLocation serves GET /api/location — upstream v2.location.get
// (ROUTE-ADD-021, declared responses: 200 LocationInfo, 400
// InvalidRequestError, 401 UnauthorizedError). Previously the path had no shim
// route and net/http answered the default 404 (the artifact's NOT-SERVED
// class).
//
// ch:trace row=ROUTE-ADD-021 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.location.get test=internal/shim/opencode/api_v2_lists_test.go witness=none:unattended-worker-session
//
// The declared 200 IS the LocationInfo object (this operation, alone in the
// family, does not wrap it in a {location, data} envelope): directory, project
// {id, directory} and the optional ^wrk workspaceID. It is derived from the
// REAL selected workspace — requestWorkspaceDir (the x-opencode-directory
// header or the configured workdir) overridden by location[directory], with
// project.id = instanceID(dir), the same derivation /path, /instance,
// handleProjectCurrent and handleApiAgentList use. No member is invented.
func (s *Server) handleApiLocation(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}
	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}
	writeJSON(w, location.payload)
}

// handleApiModelList serves GET /api/model — upstream v2.model.list
// (ROUTE-ADD-021, declared responses: 200 {location: LocationInfo, data:
// ModelV2Info[]} "Success", 400 InvalidRequestError, 401 UnauthorizedError, 503
// ServiceUnavailableError). Previously the path had no shim route and net/http
// answered the default 404 (the artifact's NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-021 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.model.list test=internal/shim/opencode/api_v2_lists_test.go witness=none:unattended-worker-session
//
// The runtime's real model catalog is `model_registry` (migration
// 001_initial_schema.sql, extended by 020_model_registry_sync_source.sql; written
// by `consensus models sync` from models.dev, read by the shim's own /provider
// and /config/providers routes, by the harness's model selection and by the
// compression worker). This handler READS it — the read is what decides the two
// declared arms:
//
//	readable   -> the declared 200 with data: [] (see the reshape note below)
//	unreadable -> the declared 503 ServiceUnavailableError, service
//	              "model_registry": this instance genuinely cannot name its
//	              models, so the catalog is unavailable, not empty. That is the
//	              difference the declared 503 exists to express, and this is the
//	              arm the sibling handlers (/skill, /question) have no declared
//	              code for.
//
// Why no row is served: `data` must carry ModelV2Info entries, and the pinned
// schema requires twelve members — id, providerID, name, api, capabilities,
// request, variants, time{released}, cost[]{input, output, cache{read, write}},
// status (alpha|beta|deprecated|active), enabled, limit{context, output}. A
// model_registry row holds the runtime's own routing-tier catalog (model_id,
// tier, max_context, cost_per_m_in/out, classifier_tags, enabled, plus the
// models.dev sync columns) and nothing about the upstream provider catalog: no
// api descriptor, no request configuration, no variants, no release time, no
// cache pricing and no output limit. Emitting a ModelV2Info from it would invent
// at least seven of the twelve required members, which is exactly the fabrication
// this shim must not do (the handleQuestionList precedent: a real row that cannot
// be truthfully reshaped into the declared shape answers the declared empty
// collection, never an invented entry). The instance's configured LLM
// (system_settings llm.provider / llm.default_model, read by handleConfig) names
// at most one model and is not a catalog either — it cannot satisfy those
// required members any better, so it is not presented as one. The truthful
// answer for a readable catalog this runtime keeps no upstream-shaped entries
// for is therefore an empty, non-null array.
func (s *Server) handleApiModelList(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}
	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}

	catalogRows, err := s.db.Query(r.Context(),
		`SELECT model_id, tier, max_context, cost_per_m_in, cost_per_m_out, enabled
		 FROM model_registry WHERE enabled = true
		 ORDER BY tier ASC, cost_per_m_in ASC`)
	if err != nil {
		writeV2ServiceUnavailable(w,
			"model catalog unavailable: "+err.Error(), "model_registry")
		return
	}
	// The catalog is readable — catalogRows holds this runtime's real registry
	// entries — but no entry can be truthfully reshaped into the declared
	// ModelV2Info (see the doc comment above): the declared 200 answers the empty
	// catalog, never an invented ModelV2Info.
	_ = catalogRows

	writeJSON(w, map[string]any{
		"location": location.payload,
		"data":     []any{},
	})
}

// handleApiReferenceList serves GET /api/reference — upstream v2.reference.list
// (ROUTE-ADD-021, declared responses: 200 {location: LocationInfo, data:
// ReferenceInfo[]} "Success", 400 InvalidRequestError, 401 UnauthorizedError).
// Previously the path had no shim route and net/http answered the default 404
// (the artifact's NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-021 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.reference.list test=internal/shim/opencode/api_v2_lists_test.go witness=none:unattended-worker-session
//
// The runtime keeps NO reference store. The declared ReferenceInfo (name, path,
// source{type: local|git, path|repository}) describes the upstream's read-only
// documentation/config repositories an agent may consult; nothing in this
// runtime registers, resolves or reads such a reference — there is no reference
// table in the migration set (001..026), and the ConfigV2 document's own
// `reference`/`references` blocks are response-only in the shim
// (globalConfigDeclaredKeys in server.go; GET /global/config answers {} for
// them). The truthful payload is therefore the declared 200 with an empty,
// non-null array — never an invented entry.
func (s *Server) handleApiReferenceList(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}
	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}
	writeJSON(w, map[string]any{
		"location": location.payload,
		"data":     []any{},
	})
}

// skillRegistryDescription extracts the optional SkillV2Info.description from a
// skills_registry.metadata JSON document. The column is JSONB on PostgreSQL and
// TEXT on SQLite (migrations 001/011) and may legitimately be '{}' or hold no
// description key; anything that is not a JSON object with a string
// "description" yields "" so the member is omitted rather than invented.
func skillRegistryDescription(raw any) string {
	text := strings.TrimSpace(toString(raw))
	if text == "" {
		return ""
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(text), &meta); err != nil {
		return ""
	}
	description, _ := meta["description"].(string)
	return strings.TrimSpace(description)
}

// handleApiSkillList serves GET /api/skill — upstream v2.skill.list
// (ROUTE-ADD-021, declared responses: 200 {location: LocationInfo, data:
// SkillV2Info[]} "Success", 400 InvalidRequestError, 401 UnauthorizedError).
// Previously the path had no shim route and net/http answered the default 404
// (the artifact's NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-021 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.skill.list test=internal/shim/opencode/api_v2_lists_test.go witness=none:unattended-worker-session
//
// This handler serves REAL rows: the runtime's skill store is `skills_registry`
// (migration 001_initial_schema.sql §4.5, mirrored for SQLite by
// 011_sqlite_missing_tables.sql) — the same table the harness reads when it
// builds an agent prompt (internal/harness/prompt.go readSkillsMetadata) and the
// native API serves as GET /api/v1/skills (internal/api/tools.go
// handleListSkills / handleGetSkill). `data` is built only from rows that table
// returns; nothing is synthesized.
//
// Member mapping (SkillV2Info requires name, location, content; description and
// slash are optional):
//
//	name     <- skills_registry.name (NOT NULL UNIQUE — the row's real id)
//	content  <- skills_registry.instructions (the bundle text the harness
//	            injects; the native API's SkillDetailResponse.Instructions)
//	location <- "skills_registry/" + name. The runtime keeps no filesystem skill
//	            directory to name, so location identifies the store that really
//	            holds the bundle (the table plus the row's unique name) instead of
//	            inventing a path or a URL.
//	description <- metadata.description when the JSON document carries one
//	            (optional member; omitted otherwise, never invented).
//
// `slash` is omitted: the runtime has no slash-command binding for a skill, and
// asserting one would be a fabricated capability. Rows are filtered to
// enabled = true — the same filter readSkillsMetadata applies — because the
// declared schema has no member that could express a disabled skill, so listing
// one would claim a skill this runtime does not actually offer its agents.
// A store read failure answers the declared InvalidRequestError 400 (the
// handleQuestionList convention for an unreadable store on an operation whose
// declared set has no 5xx arm).
func (s *Server) handleApiSkillList(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}
	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}

	rows, err := s.db.Query(r.Context(),
		`SELECT name, metadata, instructions
		 FROM skills_registry WHERE enabled = true
		 ORDER BY name`)
	if err != nil {
		writeV2InvalidRequest(w, "Shim", "skills_registry",
			"failed to read the skills registry: "+err.Error())
		return
	}

	skills := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(toString(row["name"]))
		if name == "" {
			// name is NOT NULL UNIQUE in the store, so this cannot happen for a
			// real row; skipping keeps a malformed row from becoming a
			// SkillV2Info with an invented name.
			continue
		}
		entry := map[string]any{
			"name":     name,
			"location": "skills_registry/" + name,
			"content":  toString(row["instructions"]),
		}
		if description := skillRegistryDescription(row["metadata"]); description != "" {
			entry["description"] = description
		}
		skills = append(skills, entry)
	}
	writeJSON(w, map[string]any{
		"location": location.payload,
		"data":     skills,
	})
}

// handleApiPermissionRequestList serves GET /api/permission/request — upstream
// v2.permission.request.list (ROUTE-ADD-021, declared responses: 200
// {location: LocationInfo, data: PermissionV2Request[]} "Success", 400
// InvalidRequestError, 401 UnauthorizedError). Previously the path had no shim
// route and net/http answered the default 404 (the artifact's NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-021 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.permission.request.list test=internal/shim/opencode/api_v2_lists_test.go witness=none:unattended-worker-session
//
// The runtime DOES keep a human-input store — `approval_requests`
// (migrations/008_hitl_tables.sql, the canonical SPEC-014 schema; the store the
// shim's own /permission routes and permission.reply read and write) — and this
// handler reads it (a store failure answers the declared InvalidRequestError
// 400, the handleQuestionList convention). But no approval row can be truthfully
// reshaped into the declared PermissionV2Request, whose required members are
// fixed by the pinned schema:
//
//	id (pattern ^per)        — approval_requests.id is a UUID (gen_random_uuid /
//	                           randomblob), never ^per; rewriting it would
//	                           fabricate the very identifier the client uses to
//	                           answer the request.
//	sessionID (pattern ^ses) — approval_requests.session_id is the native
//	                           session UUID; it does not match ^ses either.
//	action / resources       — the declared action+resources vocabulary
//	                           (permission actions and the resource strings they
//	                           apply to) has no counterpart: an approval row
//	                           carries request_type ('tool_execution',
//	                           'destructive_action', ...), risk_level and
//	                           target_tool/target_sql. Mapping those onto
//	                           action/resources would invent both a vocabulary
//	                           and its values.
//
// The declared 200 therefore answers with an empty, non-null array — the same
// refusal handleQuestionList records for its own un-reshapable store — and never
// an invented permission request. (The v1 sibling /permission keeps its own
// documented translation; this is the v2 surface.)
func (s *Server) handleApiPermissionRequestList(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}
	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}

	if _, err := s.db.Query(r.Context(),
		`SELECT ar.id, ar.session_id, ar.request_type, ar.risk_level, ar.status
		 FROM approval_requests ar WHERE ar.status = 'pending'`); err != nil {
		writeV2InvalidRequest(w, "Shim", "approval_requests",
			"failed to list permission requests: "+err.Error())
		return
	}

	// The store read succeeded: no pending approval row can be truthfully
	// expressed as a PermissionV2Request (see the doc comment), so the truthful
	// answer is the declared empty collection.
	writeJSON(w, map[string]any{
		"location": location.payload,
		"data":     []any{},
	})
}

// handleApiPermissionSavedList serves GET /api/permission/saved — upstream
// v2.permission.saved.list (ROUTE-ADD-021, declared responses: 200 {data:
// PermissionSavedInfo[]} "Success", 400 InvalidRequestError, 401
// UnauthorizedError). Previously the path had no shim route and net/http
// answered the default 404 (the artifact's NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-021 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.permission.saved.list test=internal/shim/opencode/api_v2_lists_test.go witness=none:unattended-worker-session
//
// The runtime keeps NO saved-permission (reusable allow/deny rule) store. This
// operation is the only one in the family that does NOT declare the deepObject
// `location` selector: it declares a single optional plain-string `projectID`
// query parameter, so the declared 400 arm is answered for that parameter — a
// present-but-blank projectID is malformed input (the sibling present-but-blank
// selector rule).
//
// No row is served because there is nothing to read: the migration set (001..026)
// has no saved-permission table, and permission.reply's "always" answer is
// recorded as the resolved approval's review_notes on the approval_requests row
// (server.go permissionReply: `note = "granted (always)"`), not as a reusable
// rule. PermissionSavedInfo requires id, projectID, action and resource — none of
// which any runtime row holds for this purpose — so the declared 200 answers an
// empty, non-null array rather than an invented rule.
func (s *Server) handleApiPermissionSavedList(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}
	q := r.URL.Query()
	if values, ok := q["projectID"]; ok && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
		writeV2InvalidRequest(w, "Query", "projectID",
			`query parameter "projectID" must not be blank`)
		return
	}

	writeJSON(w, map[string]any{
		"data": []any{},
	})
}

// handleApiQuestionRequestList serves GET /api/question/request — upstream
// v2.question.request.list (ROUTE-ADD-021, declared responses: 200 {location:
// LocationInfo, data: QuestionV2Info[]} "Success", 400 InvalidRequestError, 401
// UnauthorizedError). Previously the path had no shim route and net/http
// answered the default 404 (the artifact's NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-021 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.question.request.list test=internal/shim/opencode/api_v2_lists_test.go witness=none:unattended-worker-session
//
// This is the v2 spelling of the question surface handleQuestionList already
// serves at /question (question.list, ROUTE-ADD-110): the declared
// QuestionV2Info requires id (pattern ^que), sessionID (pattern ^ses) and a
// questions[] array of {question, header, options[]} entries. The runtime keeps
// no question producer — its human-input surface is approval_requests
// (request_type / risk_level, SPEC-014), and nothing in the shim or the native
// API writes a question row, so no ^que id can exist and no approval row can be
// truthfully reshaped into a QuestionV2Info (no header, no labelled options).
// The handler reads the same pending-approval store handleQuestionList reads
// (a store failure answers the declared InvalidRequestError 400) and answers the
// declared empty, non-null array — never an invented question.
func (s *Server) handleApiQuestionRequestList(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}
	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}

	if _, err := s.db.Query(r.Context(),
		`SELECT ar.id, ar.session_id, ar.request_type, ar.status
		 FROM approval_requests ar WHERE ar.status = 'pending'`); err != nil {
		writeV2InvalidRequest(w, "Shim", "approval_requests",
			"failed to list question requests: "+err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"location": location.payload,
		"data":     []any{},
	})
}
