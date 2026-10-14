package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// Upstream contract (openapi-1.18.33.json paths, ROUTE-ADD-021): seven declared
// v2 read-only list operations that had no shim route at all before this change
// and answered net/http's default 404 (the declared-vs-served artifact's
// NOT-SERVED class):
//
//	GET /api/location           v2.location.get           200 LocationInfo | 400 | 401
//	GET /api/model              v2.model.list             200 {location,data} | 400 | 401 | 503
//	GET /api/reference          v2.reference.list         200 {location,data} | 400 | 401
//	GET /api/skill              v2.skill.list             200 {location,data} | 400 | 401
//	GET /api/permission/request v2.permission.request.list 200 {location,data} | 400 | 401
//	GET /api/permission/saved   v2.permission.saved.list  200 {data} | 400 | 401
//	GET /api/question/request   v2.question.request.list  200 {location,data} | 400 | 401
//
// Every 200 schema sets additionalProperties:false, so the tests assert the
// exact member set (an undeclared member is a contract violation, not an
// enhancement). Six operations declare the v2 SDK's deepObject `location`
// selector (location[directory] / location[workspace]); v2.permission.saved.list
// declares a plain-string `projectID` instead — both malformed-input arms are
// covered below.

// apiV2ListResponse is the declared {location: LocationInfo, data: T[]} envelope
// six of the seven operations answer with.
type apiV2ListResponse struct {
	Location struct {
		Directory   string `json:"directory"`
		WorkspaceID string `json:"workspaceID,omitempty"`
		Project     struct {
			ID        string `json:"id"`
			Directory string `json:"directory"`
		} `json:"project"`
	} `json:"location"`
	Data []json.RawMessage `json:"data"`
}

// apiV2LocationResponse is the declared LocationInfo body v2.location.get
// answers with (this operation alone does not wrap it in a data envelope).
type apiV2LocationResponse struct {
	Directory   string `json:"directory"`
	WorkspaceID string `json:"workspaceID,omitempty"`
	Project     struct {
		ID        string `json:"id"`
		Directory string `json:"directory"`
	} `json:"project"`
}

// apiV2WrapperOperation is one of the six operations that answer the
// {location, data} envelope.
type apiV2WrapperOperation struct {
	path      string
	operation string
	// storeBacked reports whether the handler reads a real runtime store (and
	// therefore answers a declared 400 on a store failure).
	storeBacked bool
}

// apiV2WrapperOperations is every ROUTE-ADD-021 operation with the {location,
// data} envelope. The count is asserted so the table cannot silently shrink and
// leave a declared operation unserved.
func apiV2WrapperOperations() []apiV2WrapperOperation {
	return []apiV2WrapperOperation{
		{path: "/api/model", operation: "v2.model.list", storeBacked: true},
		{path: "/api/reference", operation: "v2.reference.list"},
		{path: "/api/skill", operation: "v2.skill.list", storeBacked: true},
		{path: "/api/permission/request", operation: "v2.permission.request.list", storeBacked: true},
		{path: "/api/question/request", operation: "v2.question.request.list", storeBacked: true},
	}
}

// apiV2AllPaths is every path ROUTE-ADD-021 routes (the six above plus the
// envelope-less v2.location.get and v2.permission.saved.list).
func apiV2AllPaths() []string {
	paths := []string{"/api/location", "/api/permission/saved"}
	for _, op := range apiV2WrapperOperations() {
		paths = append(paths, op.path)
	}
	sort.Strings(paths)
	return paths
}

// newApiV2ListsServer builds the ROUTE-ADD-021 fixture: a real SQLite database
// carrying the runtime stores the handlers read (skills_registry, model_registry,
// approval_requests) with auth bypassed, so the 200/400 arms are exercised
// against a real store rather than a mock.
func newApiV2ListsServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()

	ctx := context.Background()
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + filepath.Join(t.TempDir(), "api-v2-list.db"),
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open api v2 list test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, stmt := range []string{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			iteration INTEGER NOT NULL DEFAULT 0
		)`,
		// migration 001 §4.5 / 011 (SQLite mirror) — the real skill store.
		`CREATE TABLE skills_registry (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			metadata TEXT NOT NULL DEFAULT '{}',
			instructions TEXT NOT NULL DEFAULT '',
			linked_tool_ids TEXT NOT NULL DEFAULT '[]',
			enabled INTEGER NOT NULL DEFAULT 1
		)`,
		// migration 001 §3.5 (+ 020 sync columns, omitted here: this handler
		// reads only the tier-catalog columns the shim's /config/providers
		// route already reads).
		`CREATE TABLE model_registry (
			model_id TEXT PRIMARY KEY,
			tier INTEGER NOT NULL,
			max_context INTEGER NOT NULL,
			cost_per_m_in REAL NOT NULL,
			cost_per_m_out REAL NOT NULL,
			classifier_tags TEXT,
			enabled INTEGER NOT NULL DEFAULT 1
		)`,
		// migration 008 (HITL) — the runtime's real human-input store.
		`CREATE TABLE approval_requests (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			request_type TEXT NOT NULL,
			risk_level TEXT NOT NULL DEFAULT 'medium',
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending'
		)`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare api v2 list test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}

// assertExactMembers fails when body carries any member outside want (the
// pinned 200 schemas set additionalProperties:false) or misses one of them.
func assertExactMembers(t *testing.T, body map[string]any, want ...string) {
	t.Helper()
	allowed := map[string]bool{}
	for _, key := range want {
		allowed[key] = true
		if _, ok := body[key]; !ok {
			t.Errorf("response is missing required member %q (got %v)", key, sortedKeys(body))
		}
	}
	for _, key := range sortedKeys(body) {
		if !allowed[key] {
			t.Errorf("response carries undeclared member %q — the pinned schema sets additionalProperties:false (got %v)", key, sortedKeys(body))
		}
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ---------------------------------------------------------------------------
// 200 — the declared success shape
// ---------------------------------------------------------------------------

// TestApiV2ListsServeDeclared200 pins that each of the seven operations answers
// its declared 200 shape on the exact declared path, instead of the pre-fix
// net/http default 404 (class NOT-SERVED: no shim route at all).
func TestApiV2ListsServeDeclared200(t *testing.T) {
	if len(apiV2WrapperOperations()) != 5 {
		t.Fatalf("apiV2WrapperOperations has %d entries, want the 5 ROUTE-ADD-021 envelope operations",
			len(apiV2WrapperOperations()))
	}
	s, srv, _ := newApiV2ListsServer(t)

	for _, op := range apiV2WrapperOperations() {
		t.Run(op.operation, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, http.MethodGet, op.path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: got %d, want 200. Body: %s", op.path, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			var raw map[string]any
			if err := json.Unmarshal(body, &raw); err != nil {
				t.Fatalf("GET %s body is not a JSON object: %v (%s)", op.path, err, body)
			}
			assertExactMembers(t, raw, "location", "data")

			var got apiV2ListResponse
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("GET %s body is not the declared {location, data} envelope: %v (%s)", op.path, err, body)
			}
			if got.Location.Directory == "" {
				t.Errorf("location.directory is empty, want the selected workspace directory")
			}
			if got.Location.Directory != s.workdir {
				t.Errorf("location.directory = %q, want the real selected workspace %q", got.Location.Directory, s.workdir)
			}
			if got.Location.Project.ID == "" || got.Location.Project.Directory == "" {
				t.Errorf("location.project = %+v, want id and directory (LocationInfo.project required fields)", got.Location.Project)
			}
			if got.Location.Project.Directory != got.Location.Directory {
				t.Errorf("location.project.directory = %q, want the selected directory %q",
					got.Location.Project.Directory, got.Location.Directory)
			}
			if got.Data == nil {
				t.Fatalf("data = null, want an array (the declared schema types data as an array)")
			}
		})
	}
}

// TestApiLocationServesDeclared200 pins v2.location.get: the declared 200 IS the
// LocationInfo object (no {location, data} wrapper), derived from the real
// selected workspace — directory from the request workspace, project.id from
// instanceID(directory), and the optional ^wrk workspaceID echoed back.
func TestApiLocationServesDeclared200(t *testing.T) {
	s, srv, _ := newApiV2ListsServer(t)

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/location")
	if status != http.StatusOK {
		t.Fatalf("GET /api/location: got %d, want 200. Body: %s", status, body)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, raw, "directory", "project")

	var got apiV2LocationResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared LocationInfo: %v (%s)", err, body)
	}
	if got.Directory != s.workdir {
		t.Errorf("directory = %q, want the real selected workspace %q", got.Directory, s.workdir)
	}
	if got.Project.ID != instanceID(s.workdir) {
		t.Errorf("project.id = %q, want instanceID(directory) = %q", got.Project.ID, instanceID(s.workdir))
	}
	if got.Project.Directory != got.Directory {
		t.Errorf("project.directory = %q, want %q", got.Project.Directory, got.Directory)
	}
	if got.WorkspaceID != "" {
		t.Errorf("workspaceID = %q, want it absent when no location[workspace] was requested", got.WorkspaceID)
	}

	// The declared deepObject selector selects a different real workspace and
	// the optional workspaceID is echoed.
	other := t.TempDir()
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet,
		"/api/location?location%5Bdirectory%5D="+other+"&location%5Bworkspace%5D=wrk_abc123")
	if status != http.StatusOK {
		t.Fatalf("GET /api/location with location selectors: got %d, want 200. Body: %s", status, body)
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, raw, "directory", "project", "workspaceID")
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared LocationInfo: %v (%s)", err, body)
	}
	if got.Directory != other {
		t.Errorf("directory = %q, want the requested %q", got.Directory, other)
	}
	if got.WorkspaceID != "wrk_abc123" {
		t.Errorf("workspaceID = %q, want the requested wrk_abc123", got.WorkspaceID)
	}
}

// TestApiSkillListServesRealRegistryRows is the anti-fabrication arm with a
// payload: /api/skill answers rows the runtime's REAL skill store holds
// (skills_registry — the store internal/harness/prompt.go readSkillsMetadata and
// the native API's GET /api/v1/skills read), and only those. A disabled row is
// absent (the declared schema has no member that could express "disabled"), and
// every emitted member is a declared SkillV2Info member.
func TestApiSkillListServesRealRegistryRows(t *testing.T) {
	_, srv, conn := newApiV2ListsServer(t)
	ctx := context.Background()

	for _, stmt := range []string{
		`INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		 VALUES ('skill-1', 'code-review',
		         '{"description":"Review a change for defects"}',
		         'Read the diff, then report findings.', 1)`,
		`INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		 VALUES ('skill-2', 'retired-skill', '{}', 'legacy bundle', 0)`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("seed skills_registry: %v", err)
		}
	}

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/skill")
	if status != http.StatusOK {
		t.Fatalf("GET /api/skill: got %d, want 200. Body: %s", status, body)
	}

	var got apiV2ListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared envelope: %v (%s)", err, body)
	}
	if len(got.Data) != 1 {
		t.Fatalf("data has %d entries, want exactly the 1 enabled skills_registry row: %s", len(got.Data), body)
	}

	var entry map[string]any
	if err := json.Unmarshal(got.Data[0], &entry); err != nil {
		t.Fatalf("skill entry is not a JSON object: %v", err)
	}
	assertExactMembers(t, entry, "name", "location", "content", "description")
	if entry["name"] != "code-review" {
		t.Errorf("name = %v, want the skills_registry row name code-review", entry["name"])
	}
	if entry["content"] != "Read the diff, then report findings." {
		t.Errorf("content = %v, want the row's instructions", entry["content"])
	}
	if entry["location"] != "skills_registry/code-review" {
		t.Errorf("location = %v, want the store+name that really holds the bundle (skills_registry/code-review)", entry["location"])
	}
	if entry["description"] != "Review a change for defects" {
		t.Errorf("description = %v, want the row's metadata.description", entry["description"])
	}
	if _, ok := entry["slash"]; ok {
		t.Errorf("slash was emitted (%v) — the runtime has no slash binding for a skill, so the member must stay absent", entry["slash"])
	}
	if strings.Contains(string(got.Data[0]), "retired-skill") {
		t.Errorf("the disabled row leaked into data: %s", body)
	}
}

// TestApiSkillListEmptyStoreAnswersEmptyArray pins the truthful empty registry:
// the store is readable and holds no enabled skill, so the declared 200 carries
// an empty (never null) array.
func TestApiSkillListEmptyStoreAnswersEmptyArray(t *testing.T) {
	_, srv, _ := newApiV2ListsServer(t)

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/skill")
	if status != http.StatusOK {
		t.Fatalf("GET /api/skill: got %d, want 200. Body: %s", status, body)
	}
	var got apiV2ListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared envelope: %v (%s)", err, body)
	}
	if got.Data == nil || len(got.Data) != 0 {
		t.Fatalf("data = %v, want an empty non-null array for an empty skills_registry", got.Data)
	}
}

// TestApiModelListNeverFabricatesRows pins the model surgery: the runtime's
// model catalog (model_registry) IS read — a seeded row proves the store is
// reachable — but a routing-tier registry row cannot be truthfully reshaped into
// the declared ModelV2Info (twelve required members the registry does not hold),
// so data stays empty rather than carrying an invented model.
func TestApiModelListNeverFabricatesRows(t *testing.T) {
	_, srv, conn := newApiV2ListsServer(t)
	ctx := context.Background()
	if err := conn.Exec(ctx,
		`INSERT INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out, classifier_tags, enabled)
		 VALUES ('gpt-4o-mini', 1, 128000, 0.15, 0.60, '["chat"]', 1)`); err != nil {
		t.Fatalf("seed model_registry: %v", err)
	}

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/model")
	if status != http.StatusOK {
		t.Fatalf("GET /api/model with a seeded catalog: got %d, want 200. Body: %s", status, body)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, raw, "location", "data")

	var got apiV2ListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared envelope: %v (%s)", err, body)
	}
	if len(got.Data) != 0 {
		t.Fatalf("data = %s, want the empty array: no model_registry row can be truthfully reshaped into the declared ModelV2Info (provider api descriptor, request config, variants, release time, cache pricing and output limit are not held by any runtime store)", body)
	}
	if got.Location.Directory == "" || got.Location.Project.ID == "" {
		t.Errorf("location = %+v, want the real selected workspace", got.Location)
	}
}

// TestApiModelListUnreadableCatalogAnswersDeclared503 pins the declared 503 arm:
// when this instance genuinely cannot read its model catalog, the handler says
// so with the declared ServiceUnavailableError body instead of claiming an empty
// catalog.
func TestApiModelListUnreadableCatalogAnswersDeclared503(t *testing.T) {
	s := NewServer(&mockDB{queryErr: errors.New("no such table: model_registry")}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/model")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("GET /api/model with an unreadable catalog: got %d, want the declared 503. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("503 body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, got, "_tag", "message", "service")
	if got["_tag"] != "ServiceUnavailableError" {
		t.Errorf("_tag = %v, want ServiceUnavailableError (the declared 503 body)", got["_tag"])
	}
	if got["service"] != "model_registry" {
		t.Errorf("service = %v, want the unavailable store named (model_registry)", got["service"])
	}
	if msg, _ := got["message"].(string); !strings.Contains(msg, "model catalog unavailable") {
		t.Errorf("message = %q, want it to say what could not be read", msg)
	}
}

// TestApiReferencePermissionQuestionAnswerEmptyArrays pins the three operations
// whose real store either does not exist (reference) or cannot be truthfully
// reshaped into the declared shape (approval_requests -> PermissionV2Request's
// ^per/^ses identifiers and action/resources vocabulary; -> QuestionV2Info's
// ^que/^ses identifiers, headers and labelled options): each answers the declared
// 200 with an empty, non-null data array and the real selected location.
func TestApiReferencePermissionQuestionAnswerEmptyArrays(t *testing.T) {
	_, srv, conn := newApiV2ListsServer(t)
	ctx := context.Background()
	// A pending approval exists in the runtime's real human-input store; it must
	// NOT surface as an invented permission/question row.
	if err := conn.Exec(ctx,
		`INSERT INTO approval_requests (id, session_id, request_type, risk_level, description, status)
		 VALUES ('11111111-2222-3333-4444-555555555555', 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee',
		         'destructive_action', 'high', 'drop a table', 'pending')`); err != nil {
		t.Fatalf("seed approval_requests: %v", err)
	}

	for _, path := range []string{"/api/reference", "/api/permission/request", "/api/question/request"} {
		t.Run(path, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: got %d, want 200. Body: %s", path, status, body)
			}
			var raw map[string]any
			if err := json.Unmarshal(body, &raw); err != nil {
				t.Fatalf("body is not JSON: %v (%s)", err, body)
			}
			assertExactMembers(t, raw, "location", "data")

			var got apiV2ListResponse
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("body is not the declared envelope: %v (%s)", err, body)
			}
			if got.Data == nil || len(got.Data) != 0 {
				t.Fatalf("data = %s, want an empty non-null array: no runtime row can be truthfully expressed in the declared shape", body)
			}
			if got.Location.Directory == "" || got.Location.Project.ID == "" {
				t.Errorf("location = %+v, want the real selected workspace", got.Location)
			}
		})
	}
}

// TestApiPermissionSavedAnswersEmptyArray pins v2.permission.saved.list: no
// saved-permission store exists in this runtime (permission.reply records
// "always" as the resolved approval's review note, not as a reusable rule), so
// the declared 200 carries an empty, non-null array and the declared 200 schema's
// only member is data.
func TestApiPermissionSavedAnswersEmptyArray(t *testing.T) {
	_, srv, _ := newApiV2ListsServer(t)

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/permission/saved")
	if status != http.StatusOK {
		t.Fatalf("GET /api/permission/saved: got %d, want 200. Body: %s", status, body)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, raw, "data")
	var got struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared {data} envelope: %v (%s)", err, body)
	}
	if got.Data == nil || len(got.Data) != 0 {
		t.Fatalf("data = %s, want an empty non-null array", body)
	}
}

// ---------------------------------------------------------------------------
// 400 — malformed declared selectors
// ---------------------------------------------------------------------------

// TestApiV2ListsDeclared400MalformedSelector pins the declared InvalidRequestError
// 400 arm: the six operations declaring the deepObject `location` selector refuse
// an unknown location field, a present-but-blank value, a workspace id outside
// the declared ^wrk pattern and an unreadable directory; v2.permission.saved.list
// refuses its present-but-blank `projectID` string parameter.
func TestApiV2ListsDeclared400MalformedSelector(t *testing.T) {
	s, srv, _ := newApiV2ListsServer(t)

	locationCases := []struct {
		name  string
		query string
		field string
	}{
		{"blank directory", "?location%5Bdirectory%5D=", "location[directory]"},
		{"blank workspace", "?location%5Bworkspace%5D=", "location[workspace]"},
		{"unknown field", "?location%5Bbogus%5D=x", "location[bogus]"},
		{"workspace without wrk prefix", "?location%5Bworkspace%5D=ws-1", "location[workspace]"},
		{"unreadable directory", "?location%5Bdirectory%5D=" + filepath.Join(s.workdir, "does-not-exist"), "location[directory]"},
	}

	for _, op := range apiV2WrapperOperations() {
		for _, tc := range locationCases {
			t.Run(op.operation+" "+tc.name, func(t *testing.T) {
				status, header, body := doShimRequest(t, srv.URL, http.MethodGet, op.path+tc.query)
				if status != http.StatusBadRequest {
					t.Fatalf("GET %s%s: got %d, want the declared 400. Body: %s", op.path, tc.query, status, body)
				}
				if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
					t.Errorf("Content-Type = %q, want application/json", ct)
				}
				var got map[string]any
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("400 body is not JSON: %v (%s)", err, body)
				}
				if got["_tag"] != "InvalidRequestError" {
					t.Errorf("_tag = %v, want InvalidRequestError (the declared 400 body)", got["_tag"])
				}
				if got["message"] == "" {
					t.Error("400 body carries no message")
				}
				if field, _ := got["field"].(string); field == "" {
					t.Errorf("400 body names no field (got %v), want the malformed selector named", got)
				}
			})
		}
	}

	// v2.location.get declares the same location selector.
	for _, tc := range locationCases {
		t.Run("v2.location.get "+tc.name, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/location"+tc.query)
			if status != http.StatusBadRequest {
				t.Fatalf("GET /api/location%s: got %d, want 400. Body: %s", tc.query, status, body)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("400 body is not JSON: %v (%s)", err, body)
			}
			if got["_tag"] != "InvalidRequestError" {
				t.Errorf("_tag = %v, want InvalidRequestError", got["_tag"])
			}
		})
	}

	// v2.permission.saved.list declares a plain-string projectID, not a location
	// deepObject: a present-but-blank value is the malformed-input arm, and a
	// blank projectID must not be answered as a successful empty list.
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/permission/saved?projectID=")
	if status != http.StatusBadRequest {
		t.Fatalf("GET /api/permission/saved?projectID= : got %d, want 400. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("400 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "InvalidRequestError" || got["field"] != "projectID" {
		t.Errorf("400 body = %v, want InvalidRequestError naming field projectID", got)
	}
}

// TestApiV2ListsStoreFailureAnswersDeclaredError pins the error arm of the four
// store-backed operations: an unreadable store is answered with a DECLARED code —
// the InvalidRequestError 400 for the operations whose declared set has no 5xx
// arm (the handleQuestionList convention) and the declared 503 for v2.model.list —
// never a fabricated empty success.
func TestApiV2ListsStoreFailureAnswersDeclaredError(t *testing.T) {
	s := NewServer(&mockDB{queryErr: errors.New("store unavailable")}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	want := map[string]int{
		"/api/model":              http.StatusServiceUnavailable,
		"/api/skill":              http.StatusBadRequest,
		"/api/permission/request": http.StatusBadRequest,
		"/api/question/request":   http.StatusBadRequest,
	}
	// Every store-backed operation in the table must have an expected arm here,
	// so a new store-backed handler cannot be added without covering its failure.
	for _, op := range apiV2WrapperOperations() {
		if !op.storeBacked {
			continue
		}
		if _, ok := want[op.path]; !ok {
			t.Errorf("%s reads a store (%s) but this battery has no expected failure arm for it", op.path, op.operation)
		}
	}
	for path, code := range want {
		t.Run(path, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
			if status != code {
				t.Fatalf("GET %s with an unreadable store: got %d, want %d. Body: %s", path, status, code, body)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("error body is not JSON: %v (%s)", err, body)
			}
			if got["message"] == "" {
				t.Errorf("error body = %v, want a message naming the failure", got)
			}
			if code == http.StatusBadRequest && got["_tag"] != "InvalidRequestError" {
				t.Errorf("_tag = %v, want InvalidRequestError", got["_tag"])
			}
			if code == http.StatusServiceUnavailable && got["_tag"] != "ServiceUnavailableError" {
				t.Errorf("_tag = %v, want ServiceUnavailableError", got["_tag"])
			}
		})
	}

	// The store-less operations must NOT be affected by an unreadable database:
	// they answer the declared 200 from the workspace alone.
	for _, path := range []string{"/api/location", "/api/reference", "/api/permission/saved"} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusOK {
			t.Errorf("GET %s with an unreadable store: got %d, want 200 (no store is read). Body: %s", path, status, body)
		}
	}
}

// ---------------------------------------------------------------------------
// 405 / 401 / mount wiring
// ---------------------------------------------------------------------------

// TestApiV2ListsMethodNotAllowed pins the sibling METHOD_NOT_ALLOWED envelope for
// every method outside the declared GET, so a registered route never serves a
// GET-only payload to another method.
func TestApiV2ListsMethodNotAllowed(t *testing.T) {
	_, srv, _ := newApiV2ListsServer(t)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	paths := apiV2AllPaths()
	if len(paths) != 7 {
		t.Fatalf("apiV2AllPaths() has %d paths, want all 7 ROUTE-ADD-021 operations", len(paths))
	}
	for _, path := range paths {
		for _, method := range methods {
			status, _, body := doShimRequest(t, srv.URL, method, path)
			if status != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: got %d, want 405. Body: %s", method, path, status, body)
				continue
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Errorf("%s %s 405 body is not JSON: %v (%s)", method, path, err, body)
			}
		}
	}
}

// TestApiV2ListsServeDeclared401 pins the declared UnauthorizedError arm for all
// seven operations: an unauthenticated request answers the v2 envelope
// ({_tag: "UnauthorizedError", message}) — never the shim's UNAUTHENTICATED body,
// which is what an unregistered /api/* path keeps.
func TestApiV2ListsServeDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, path := range apiV2AllPaths() {
		t.Run(path, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, http.MethodGet, path)
			if status != http.StatusUnauthorized {
				t.Fatalf("unauthenticated GET %s: got %d, want 401. Body: %s", path, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("401 body is not JSON: %v (%s)", err, body)
			}
			assertExactMembers(t, got, "_tag", "message")
			if got["_tag"] != "UnauthorizedError" {
				t.Errorf("_tag = %v, want UnauthorizedError (the declared 401 body)", got["_tag"])
			}
		})
	}
}

// TestApiV2ListsMountWiring pins the chi mount contract: every path is listed in
// MountPatterns (cmd/consensus/main.go mounts the shim by these patterns —
// without the entry a parent chi router answers before the shim sees the
// request), each route is reachable through that mount, and no /api/* catch-all
// was introduced (an unclaimed sub-path keeps the 404 it is classified with).
func TestApiV2ListsMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := map[string]bool{}
	for _, pattern := range MountPatterns {
		mounted[pattern] = true
	}
	for _, path := range apiV2AllPaths() {
		if !mounted[path] {
			t.Errorf("%s is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-021)", path)
		}
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	for _, path := range apiV2AllPaths() {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusOK {
			t.Errorf("chi-mounted GET %s: got %d, want 200 (the mount must reach the shim route). Body: %s", path, status, body)
		}
	}

	for _, path := range []string{"/api/location/bogus", "/api/model/bogus", "/api/skill/bogus", "/api/permission/request/bogus"} {
		status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status == http.StatusOK {
			t.Errorf("GET %s: got 200, want 404 — ROUTE-ADD-021 registers exact patterns, never an /api/* catch-all", path)
		}
	}

	// The v2 envelope list is exact-path: an unregistered /api/* sub-path keeps
	// the shim's own 401 envelope, so the two families stay distinguishable.
	if isV2EnvelopeRoutePath("/api/model/bogus") {
		t.Error("/api/model/bogus is treated as a v2-envelope route; membership must be exact-path")
	}
	for _, path := range apiV2AllPaths() {
		if !isV2EnvelopeRoutePath(path) {
			t.Errorf("%s is not listed in v2EnvelopeRoutePaths — its declared 401 body is the v2 UnauthorizedError envelope", path)
		}
	}
	if !isV2EnvelopeRoutePath("/api/agent") || !isV2EnvelopeRoutePath("/api/fs/find") {
		t.Error("the pre-existing v2 routes /api/agent and /api/fs/find lost their v2 401 envelope")
	}
}

// TestApiV2ListsTraceRows pins the ch:trace declarations for all seven handlers:
// each names this row, its own operationId, this test file and the unattended
// witness, so the traceability link cannot silently drift from the served
// surface.
func TestApiV2ListsTraceRows(t *testing.T) {
	// The handlers live in api_v2_lists.go; the assertion reads it through the
	// same route-table contract the artifact records.
	raw, err := readAPIV2ListsSource()
	if err != nil {
		t.Skipf("api_v2_lists.go not readable from the test working directory: %v", err)
	}
	for _, op := range []string{
		"v2.location.get", "v2.model.list", "v2.reference.list", "v2.skill.list",
		"v2.permission.request.list", "v2.permission.saved.list", "v2.question.request.list",
	} {
		want := fmt.Sprintf("row=ROUTE-ADD-021 spec=specs/openapi/upstream/openapi-1.18.33.json#%s test=internal/shim/opencode/api_v2_lists_test.go witness=none:unattended-worker-session", op)
		if !strings.Contains(raw, want) {
			t.Errorf("api_v2_lists.go carries no ch:trace line for %s (want %q)", op, want)
		}
	}
}

func readAPIV2ListsSource() (string, error) {
	// The Go test binary runs in the package directory.
	data, err := os.ReadFile("api_v2_lists.go")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
