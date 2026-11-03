package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/project/{projectID}/copy/generate-name".post):
// POST /experimental/project/{projectID}/copy/generate-name, operationId
// experimental.projectCopy.generateName, required plain-string path parameter
// `projectID` (no pattern), optional plain-string query selectors `directory`
// and `workspace`, an optional requestBody {context?: string}
// (additionalProperties:false), and exactly two declared responses: 200
// {name*: string} (additionalProperties:false) and 400 BadRequestError.
//
// ROUTE-ADD-071 serves that exact contract. Before the fix every method on the
// path fell through the mux to net/http's default 404 — the declared-vs-served
// comparison's NOT-SERVED class (source item SHIM-DRIFT-002 in the committed
// artifact).
//
// RED-pin: TestExperimentalProjectCopyGenerateNameServesDeclared200 fails with
// 404 against the pre-fix tree (the route does not exist) and passes once the
// exact mux entry + handler land. The rest of the battery pins the declared 400
// arms, the method guard, the shim's own 401 envelope (this is a v1
// experimental.* operation, not v2.*) and the absence of any
// /experimental/project/* catch-all.

// generateNameSlugPattern is the kebab-case shape every answered name must
// carry: lowercase alphanumeric tokens joined by single hyphens.
var generateNameSlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const generateNamePath = "/experimental/project/prj-copy-0001/copy/generate-name"

func newGenerateNameServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// doGenerateNameRequest performs POST with an optional JSON body against the
// declared path (body == "" sends no body at all, the optional-body arm).
func doGenerateNameRequest(t *testing.T, base, method, path, body string) (int, []byte) {
	t.Helper()
	status, _, data := doCopyRequest(t, base, method, path, body)
	return status, data
}

// decodeGeneratedName decodes the declared 200 body {name: string} and refuses
// any member the schema does not declare.
func decodeGeneratedName(t *testing.T, label string, status int, body []byte) string {
	t.Helper()
	if status != http.StatusOK {
		t.Fatalf("%s: got %d, want the declared 200. Body: %s", label, status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%s: 200 body is not JSON: %v (%s)", label, err, body)
	}
	if len(got) != 1 {
		t.Fatalf("%s: 200 body carries %d members (%s), want exactly the declared {name}", label, len(got), body)
	}
	name, ok := got["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		t.Fatalf("%s: name = %v, want the declared non-empty string", label, got["name"])
	}
	if !generateNameSlugPattern.MatchString(name) {
		t.Fatalf("%s: name = %q, want a kebab-case slug", label, name)
	}
	return name
}

// TestExperimentalProjectCopyGenerateNameServesDeclared200 pins the declared
// success arm: the runtime owes a NAME and the handler derives one. The body is
// optional (no requestBody.required in the pinned document), so the absent-body
// arm must answer the declared 200 as well.
func TestExperimentalProjectCopyGenerateNameServesDeclared200(t *testing.T) {
	srv := newGenerateNameServer(t)

	status, body := doGenerateNameRequest(t, srv.URL, http.MethodPost, generateNamePath, "")
	name := decodeGeneratedName(t, "absent body", status, body)

	// Two-word generator shape (upstream Slug.create): the absent-body arm has
	// no context to derive from, so it must answer the sibling copy handler's
	// adjective-noun slug rather than an empty or single-letter name.
	if !strings.Contains(name, "-") {
		t.Fatalf("absent-body name = %q, want the two-word generated slug", name)
	}

	// An explicit empty body and an empty object are the same well-formed arm.
	for _, arm := range []struct{ label, body string }{
		{"empty body", " "},
		{"empty object", "{}"},
	} {
		status, body = doGenerateNameRequest(t, srv.URL, http.MethodPost, generateNamePath, arm.body)
		if got := decodeGeneratedName(t, arm.label, status, body); got == "" {
			t.Fatalf("%s: answered an empty name", arm.label)
		}
	}

	// A name is a name: nothing is created, so the path stays a pure generator
	// and the entrypoint directory the test server started in is untouched.
	if status, body = doGenerateNameRequest(t, srv.URL, http.MethodPost, generateNamePath, `{"context":"fix the flaky login test"}`); status != http.StatusOK {
		t.Fatalf("context body: got %d, want 200; body=%s", status, body)
	}
}

// TestExperimentalProjectCopyGenerateNameDerivesFromContext pins the operation's
// own description — "generate a short name for a project copy from task context"
// — to the served behaviour: the words of `context` are what the name is made
// of, not a value unrelated to the request.
func TestExperimentalProjectCopyGenerateNameDerivesFromContext(t *testing.T) {
	srv := newGenerateNameServer(t)

	cases := []struct {
		context string
		want    string
	}{
		{`{"context":"Fix the flaky billing migration runner"}`, "flaky-billing"},
		{`{"context":"Add websocket reconnect backoff"}`, "websocket-reconnect"},
		{`{"context":"UPDATE docs for the CLI flags"}`, "docs-cli"},
	}
	for _, tc := range cases {
		status, body := doGenerateNameRequest(t, srv.URL, http.MethodPost, generateNamePath, tc.context)
		name := decodeGeneratedName(t, tc.context, status, body)
		if name != tc.want {
			t.Errorf("context %s: name = %q, want %q", tc.context, name, tc.want)
		}
	}

	// Punctuation-only and stop-word-only contexts carry no usable word, so the
	// two-word generator answers instead of an empty name.
	for _, context := range []string{`{"context":"the and to for"}`, `{"context":"--- ... ***"}`, `{"context":""}`} {
		status, body := doGenerateNameRequest(t, srv.URL, http.MethodPost, generateNamePath, context)
		name := decodeGeneratedName(t, context, status, body)
		if !strings.Contains(name, "-") {
			t.Errorf("context %s: name = %q, want the two-word fallback slug", context, name)
		}
	}
}

// TestExperimentalProjectCopyGenerateNameDeclared400 pins every declared 400 arm
// against the declared BadRequestError schema ({name: "BadRequest",
// data: {message*, kind?}} with kind in Params|Headers|Query|Body|Payload).
func TestExperimentalProjectCopyGenerateNameDeclared400(t *testing.T) {
	srv := newGenerateNameServer(t)

	cases := []struct {
		label    string
		path     string
		body     string
		wantKind string
	}{
		// A present-but-blank {projectID}: the only ill-shaped id the contract
		// itself defines (the parameter is a plain required string, no pattern).
		{"blank projectID", "/experimental/project/%20/copy/generate-name", "", "Params"},
		// Present-but-blank declared query selectors.
		{"blank directory selector", generateNamePath + "?directory=", "", "Query"},
		{"blank workspace selector", generateNamePath + "?workspace=%20", "", "Query"},
		// Request bodies that are not exactly one JSON object of declared fields.
		{"unknown field", generateNamePath, `{"context":"ok","extra":1}`, "Body"},
		{"non-object body", generateNamePath, `["context"]`, "Body"},
		{"scalar body", generateNamePath, `"context"`, "Body"},
		{"json null body", generateNamePath, `null`, "Body"},
		{"two JSON values", generateNamePath, `{"context":"a"} {"context":"b"}`, "Body"},
		{"malformed JSON", generateNamePath, `{"context":`, "Body"},
		// A declared member of the wrong type.
		{"non-string context", generateNamePath, `{"context":123}`, "Payload"},
		{"null context", generateNamePath, `{"context":null}`, "Payload"},
	}

	for _, tc := range cases {
		status, body := doGenerateNameRequest(t, srv.URL, http.MethodPost, tc.path, tc.body)
		if status != http.StatusBadRequest {
			t.Fatalf("%s: got %d, want the declared 400. Body: %s", tc.label, status, body)
		}
		var got struct {
			Name string `json:"name"`
			Data struct {
				Message string `json:"message"`
				Kind    string `json:"kind"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s: 400 body is not JSON: %v (%s)", tc.label, err, body)
		}
		if got.Name != "BadRequest" {
			t.Errorf("%s: name = %q, want BadRequest (the declared 400 arm)", tc.label, got.Name)
		}
		if strings.TrimSpace(got.Data.Message) == "" {
			t.Errorf("%s: data.message is empty, want a reason", tc.label)
		}
		switch got.Data.Kind {
		case "", "Params", "Headers", "Query", "Body", "Payload":
		default:
			t.Errorf("%s: data.kind = %q, not in the declared enum", tc.label, got.Data.Kind)
		}
		if tc.wantKind != "" && got.Data.Kind != tc.wantKind {
			t.Errorf("%s: data.kind = %q, want %q", tc.label, got.Data.Kind, tc.wantKind)
		}
	}

	// A well-formed valued selector is NOT an error arm: the selectors are
	// optional inputs this operation does not consume.
	status, body := doGenerateNameRequest(t, srv.URL, http.MethodPost,
		generateNamePath+"?directory=%2Ftmp&workspace=ws-1", "")
	if status != http.StatusOK {
		t.Fatalf("valued selectors: got %d, want 200; body=%s", status, body)
	}
}

// TestExperimentalProjectCopyGenerateNameMethodGuard pins that the exact
// registration serves POST and answers the sibling METHOD_NOT_ALLOWED envelope
// for every other method (the path is now routed, so these are 405s, not the
// net/http default 404 the path answered before ROUTE-ADD-071).
func TestExperimentalProjectCopyGenerateNameMethodGuard(t *testing.T) {
	srv := newGenerateNameServer(t)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, body := doGenerateNameRequest(t, srv.URL, method, generateNamePath, "")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s: got %d, want 405 METHOD_NOT_ALLOWED. Body: %s", method, status, body)
			continue
		}
		if !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
			t.Errorf("%s: body = %s, want the METHOD_NOT_ALLOWED envelope", method, body)
		}
	}
}

// TestExperimentalProjectCopyGenerateNameRequiresAuth pins the auth wall.
// experimental.projectCopy.generateName is a v1 (experimental.*) operation, so
// the unauthenticated answer is the shim's own UNAUTHENTICATED envelope — NOT
// the v2 UnauthorizedError the sibling /copy rows carry.
func TestExperimentalProjectCopyGenerateNameRequiresAuth(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, body := doGenerateNameRequest(t, srv.URL, http.MethodPost, generateNamePath, `{"context":"x"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST: got %d, want 401. Body: %s", status, body)
	}
	if !strings.Contains(string(body), "UNAUTHENTICATED") {
		t.Errorf("unauthenticated body = %s, want the shim UNAUTHENTICATED envelope", body)
	}
	if strings.Contains(string(body), "UnauthorizedError") {
		t.Errorf("unauthenticated body = %s, want the v1 envelope (this operation is not v2.*)", body)
	}
}

// TestExperimentalProjectCopyGenerateNameNoCatchAll pins the exact
// registration: the sibling /copy operations keep the behaviour their own
// contract rows own, and undeclared deeper shapes keep the net/http default
// 404 they are classified with.
func TestExperimentalProjectCopyGenerateNameNoCatchAll(t *testing.T) {
	srv := newGenerateNameServer(t)

	// The sibling DELETE keeps its declared 204 no-op.
	status, _, body := doCopyRequest(t, srv.URL, http.MethodDelete, "/experimental/project/prj-copy-0001/copy", "")
	if status != http.StatusNoContent {
		t.Errorf("sibling DELETE /copy: got %d, want the declared 204. Body: %s", status, body)
	}

	// Declared siblings that own their own (still NOT-SERVED) contract rows and
	// undeclared shapes keep the default 404.
	for _, path := range []string{
		"/experimental/project/prj-copy-0001/copy/refresh",
		"/experimental/project/prj-copy-0001/copy/generate-name/deeper",
		"/experimental/project/prj-copy-0001/copyx",
	} {
		status, body := doGenerateNameRequest(t, srv.URL, http.MethodPost, path, "")
		if status != http.StatusNotFound {
			t.Errorf("POST %s: got %d, want 404 (exact declared path only). Body: %s", path, status, body)
		}
	}
}
