package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (openapi-1.18.33.json
// paths."/api/permission/saved/{id}".delete):
// DELETE /api/permission/saved/{id}, operationId v2.permission.saved.remove,
// required plain-string path parameter `id` (no pattern, no query selector),
// and responses 204 No Content, 400 InvalidRequestError, 401
// UnauthorizedError. ROUTE-ADD-025 serves that exact contract instead of the
// pre-fix net/http default 404 recorded as SHIM-DRIFT-001 in the committed
// artifact's positional ids.
//
// The sibling GET /api/permission/saved (v2.permission.saved.list,
// ROUTE-ADD-021) is a DIFFERENT operation on its own exact path; the DELETE
// route must not swallow it.

func newApiPermissionSavedRemoveServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// TestApiPermissionSavedRemoveServesDeclared204 pins the declared success
// arm: the runtime keeps no saved-permission store, so removal of any
// well-formed id is an idempotent no-op — an empty 204 body that fabricates
// neither a stored rule nor a deletion claim. The second call pins the
// idempotence (removing what nothing holds twice answers the same way).
func TestApiPermissionSavedRemoveServesDeclared204(t *testing.T) {
	srv := newApiPermissionSavedRemoveServer(t)

	for i := 0; i < 2; i++ {
		status, header, body := doShimRequest(t, srv.URL, http.MethodDelete,
			"/api/permission/saved/per-0001")
		if status != http.StatusNoContent {
			t.Fatalf("call %d: DELETE saved permission: got %d, want 204. Body: %s", i+1, status, body)
		}
		if len(body) != 0 {
			t.Errorf("call %d: 204 body = %q, want empty", i+1, body)
		}
		if ct := header.Get("Content-Type"); ct != "" {
			t.Errorf("call %d: 204 Content-Type = %q, want none", i+1, ct)
		}
	}
}

// TestApiPermissionSavedRemoveBlankIDAnswersDeclared400 pins the declared
// InvalidRequestError arm for the only malformed id the pinned document
// allows a handler to see: a present-but-blank (whitespace-only) {id}
// segment, percent-encoded so it still matches the route pattern. The pinned
// schema declares `id` as a plain required string with NO pattern and the
// operation declares no requestBody and no query selector, so inventing
// further malformed-id constraints would fabricate rules the document does
// not state (the credentialRemove precedent).
func TestApiPermissionSavedRemoveBlankIDAnswersDeclared400(t *testing.T) {
	srv := newApiPermissionSavedRemoveServer(t)

	status, header, body := doShimRequest(t, srv.URL, http.MethodDelete,
		"/api/permission/saved/%20")
	if status != http.StatusBadRequest {
		t.Fatalf("DELETE blank id: got %d, want 400. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("400 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "InvalidRequestError" {
		t.Errorf("_tag = %v, want InvalidRequestError", got["_tag"])
	}
	if got["field"] != "id" {
		t.Errorf("field = %v, want \"id\"", got["field"])
	}
	if kind, _ := got["kind"].(string); kind != "Path" {
		t.Errorf("kind = %v, want \"Path\"", got["kind"])
	}
	if msg, _ := got["message"].(string); strings.TrimSpace(msg) == "" {
		t.Errorf("message = %q, want a non-empty reason", msg)
	}
}

// TestApiPermissionSavedRemoveMethodAndMountWiring pins the exact chi mount,
// the sibling METHOD_NOT_ALLOWED envelope for every non-DELETE method, the
// untouched sibling list route, and the absence of an /api/* catch-all.
func TestApiPermissionSavedRemoveMethodAndMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/permission/saved/{id}" {
			mounted = true
			break
		}
	}
	if !mounted {
		t.Fatal("/api/permission/saved/{id} is not registered in MountPatterns")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodDelete,
		"/api/permission/saved/per-wired")
	if status != http.StatusNoContent {
		t.Fatalf("chi-mounted DELETE: got %d, want 204. Body: %s", status, body)
	}

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch} {
		status, _, body := doShimRequest(t, srv.URL, method,
			"/api/permission/saved/per-wired")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s saved permission: got %d, want 405. Body: %s", method, status, body)
			continue
		}
		var got struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s 405 body is not JSON: %v (%s)", method, err, body)
			continue
		}
		if got.Error.Code != "METHOD_NOT_ALLOWED" || strings.TrimSpace(got.Error.Message) == "" {
			t.Errorf("%s 405 body = %s, want METHOD_NOT_ALLOWED with message", method, body)
		}
	}

	// The sibling list operation keeps its own exact route: GET on the bare
	// path answers the declared 200 collection, not the DELETE handler.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/permission/saved")
	if status != http.StatusOK {
		t.Fatalf("sibling GET /api/permission/saved: got %d, want 200. Body: %s", status, body)
	}
	var listed struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatalf("sibling GET body is not JSON: %v (%s)", err, body)
	}
	if listed.Data == nil || len(listed.Data) != 0 {
		t.Errorf("sibling GET data = %v, want the declared empty non-null array", listed.Data)
	}

	// No catch-all: unregistered /api/permission/saved* shapes keep the
	// net/http default 404 they are classified with.
	for _, path := range []string{
		"/api/permission/saved/per-wired/deeper",
		"/api/permission/saved/per-wired/sub/more",
	} {
		status, _, _ := doShimRequest(t, srv.URL, http.MethodDelete, path)
		if status != http.StatusNotFound {
			t.Errorf("DELETE %s: got %d, want 404 (exact declared path only)", path, status)
		}
	}
}

// TestApiPermissionSavedRemoveServesDeclared401 pins the v2 UnauthorizedError
// envelope supplied by the existing /api/* authentication wall through the
// one-segment v2 envelope match.
func TestApiPermissionSavedRemoveServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodDelete,
		"/api/permission/saved/per-0001")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated DELETE: got %d, want 401. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" || strings.TrimSpace(toString(got["message"])) == "" {
		t.Errorf("401 body = %v, want UnauthorizedError with message", got)
	}
	if len(got) != 2 {
		t.Errorf("401 body = %v, want only declared _tag and message", got)
	}
}
