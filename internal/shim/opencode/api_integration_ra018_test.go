package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (openapi-1.18.33.json, ROUTE-ADD-018 / SHIM-DRIFT-005):
// GET /api/integration/{integrationID}, operation v2.integration.get, one
// required string path parameter plus the same optional deepObject `location`
// selector v2.integration.list declares, and responses 200, 400 and 401.
// ROUTE-ADD-018 requires the list operation's truthful collection shape for the
// success payload: {location: LocationInfo, data: IntegrationInfo[]}. Consensus
// has no integration registry, so data must be an empty, non-null array.
type apiIntegrationRA018Response struct {
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

func TestApiIntegrationRA018ServesTruthfulDeclared200(t *testing.T) {
	s, srv := newTestServer(&mockDB{})
	s.workdir = t.TempDir()
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/integration/github")
	if status != http.StatusOK {
		t.Fatalf("GET /api/integration/github: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, raw, "location", "data")

	var got apiIntegrationRA018Response
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the required {location, data} response: %v (%s)", err, body)
	}
	if got.Location.Directory != s.workdir {
		t.Errorf("location.directory = %q, want the real selected workspace %q", got.Location.Directory, s.workdir)
	}
	if got.Location.Project.ID != instanceID(s.workdir) || got.Location.Project.Directory != s.workdir {
		t.Errorf("location.project = %+v, want the selected workspace's real id and directory", got.Location.Project)
	}
	if got.Data == nil || len(got.Data) != 0 {
		t.Fatalf("data = %s, want an empty non-null array: the runtime has no integration store", body)
	}
	if strings.Contains(string(body), "github") {
		t.Errorf("response fabricated an IntegrationInfo row from the requested id: %s", body)
	}
}

func TestApiIntegrationRA018ValidatesDeclaredLocation(t *testing.T) {
	s, srv := newTestServer(&mockDB{})
	s.workdir = t.TempDir()
	defer srv.Close()

	selected := t.TempDir()
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/integration/github?location%5Bdirectory%5D="+selected+"&location%5Bworkspace%5D=wrk_ra018")
	if status != http.StatusOK {
		t.Fatalf("GET with declared location selector: got %d, want 200. Body: %s", status, body)
	}
	var got apiIntegrationRA018Response
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode selected-location response: %v (%s)", err, body)
	}
	if got.Location.Directory != selected || got.Location.WorkspaceID != "wrk_ra018" {
		t.Errorf("location = %+v, want selected directory %q and workspace wrk_ra018", got.Location, selected)
	}

	for _, path := range []string{
		"/api/integration/github?location%5Bworkspace%5D=",
		"/api/integration/github?location%5Bbogus%5D=x",
		"/api/integration/github?location%5Bworkspace%5D=not-a-workspace",
	} {
		status, _, body = doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusBadRequest {
			t.Errorf("GET %s: got %d, want declared 400. Body: %s", path, status, body)
			continue
		}
		var errBody map[string]any
		if err := json.Unmarshal(body, &errBody); err != nil {
			t.Errorf("GET %s: 400 body is not JSON: %v (%s)", path, err, body)
			continue
		}
		if errBody["_tag"] != "InvalidRequestError" || errBody["message"] == "" {
			t.Errorf("GET %s: 400 body = %v, want declared InvalidRequestError", path, errBody)
		}
	}
}

func TestApiIntegrationRA018MethodAndMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/integration/{integrationID}" {
			mounted = true
		}
	}
	if !mounted {
		t.Fatal("/api/integration/{integrationID} is not registered in MountPatterns — a parent chi mount would 404 ROUTE-ADD-018")
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/integration/github", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/integration/github: got %d, want 405. Body: %s", method, rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), "METHOD_NOT_ALLOWED") {
			t.Errorf("%s /api/integration/github: body %s does not carry the sibling METHOD_NOT_ALLOWED envelope", method, rec.Body)
		}
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/integration/github")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/integration/github: got %d, want 200. Body: %s", status, body)
	}
	// ROUTE-ADD-019 now registers this exact deeper sibling path for POST;
	// GET therefore reaches that operation's method guard instead of the old
	// net/http default 404. This remains exact-route wiring, not a subtree
	// catch-all.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/integration/github/connect/key")
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET connect/key sibling path: got %d, want 405 after ROUTE-ADD-019 registration", status)
	}
	if !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
		t.Errorf("GET connect/key sibling path body %s does not carry METHOD_NOT_ALLOWED", body)
	}
}

func TestApiIntegrationRA018ServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/integration/github")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET: got %d, want 401. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, got, "_tag", "message")
	if got["_tag"] != "UnauthorizedError" || got["message"] == "" {
		t.Errorf("401 body = %v, want declared UnauthorizedError", got)
	}
}
