package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// doShimRequestBody is doShimRequest with a request body: the /sync family
// is the first shim surface whose declared contract is POST-with-body, and
// the body IS the input under test.
func doShimRequestBody(t *testing.T, base, method, path, body string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, path, err)
	}
	return resp.StatusCode, resp.Header, data
}

// assertInvalidRequest pins the requested 400 envelope. Existing sibling
// handlers pass an HTTP status and keep the shim INVALID_REQUEST shape;
// /sync/history passes Body or Payload and expects the upstream v2 SDK
// NamedError BadRequest shape.
func assertInvalidRequest(t *testing.T, path, body string, envelope any, data []byte) {
	t.Helper()
	if wantKind, ok := envelope.(string); ok {
		var got struct {
			Name string `json:"name"`
			Data struct {
				Kind    string `json:"kind"`
				Message string `json:"message"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("POST %s: body not the NamedError envelope: %v (%s)", path, err, body)
		}
		if got.Name != "BadRequest" {
			t.Errorf("POST %s: name = %q, want BadRequest (body %s)", path, got.Name, body)
		}
		if got.Data.Kind != wantKind {
			t.Errorf("POST %s: data.kind = %q, want %q matching ^(Body|Payload)$ (body %s)", path, got.Data.Kind, wantKind, body)
		}
		if got.Data.Message == "" {
			t.Errorf("POST %s: data.message empty, want the offending input named (body %s)", path, body)
		}
		return
	}

	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("POST %s: body not the error envelope: %v (%s)", path, err, body)
	}
	if got.Error.Code != "INVALID_REQUEST" {
		t.Errorf("POST %s: error.code = %q, want INVALID_REQUEST (body %s)", path, got.Error.Code, body)
	}
	if got.Error.Message == "" {
		t.Errorf("POST %s: error.message empty, want the offending input named (body %s)", path, body)
	}
}

// TestSyncHistoryAnswersDeclaredList answers upstream sync.history.list
// (ROUTE-ADD-112, SHIM-DRIFT-126, declared responses: 200 SyncEvent[],
// 400 BadRequest | InvalidRequestError): POST /sync/history must return 200
// with a JSON array — the shim keeps no sync event store, so the truthful
// payload is an empty array (never null, never an error) — instead of the
// pre-fix net/http default 404. An absent body is well-formed (the document
// leaves requestBody optional), and so is a body with unknown aggregate
// keys: an unlisted aggregate's full history is empty.
func TestSyncHistoryAnswersDeclaredList(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct {
		name, path, body string
	}{
		{"empty object body", "/sync/history", "{}"},
		{"no body at all", "/sync/history", ""},
		{"unknown aggregate cursor", "/sync/history", `{"other-agg": 3}`},
		{"valued declared params", "/sync/history?directory=/tmp/ws&workspace=ws1", "{}"},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
		if status != http.StatusOK {
			t.Fatalf("%s: POST %s: got %d, want 200. Body: %s", tc.name, tc.path, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: POST %s: Content-Type = %q, want application/json", tc.name, tc.path, ct)
		}
		var events []map[string]any
		if err := json.Unmarshal(body, &events); err != nil {
			t.Fatalf("%s: POST %s body must be a JSON array, got %q: %v", tc.name, tc.path, body, err)
		}
		if events == nil {
			t.Errorf("%s: POST %s body = %s, want an array (never null)", tc.name, tc.path, body)
		}
		if len(events) != 0 {
			t.Errorf("%s: POST %s = %v, want empty (the shim keeps no sync event store)", tc.name, tc.path, events)
		}
	}
}

// TestSyncHistoryMalformedInputIsBadRequest pins the declared 400 arm:
// malformed input must answer the upstream NamedError BadRequest body, not 200
// and not the pre-fix 404. Covered classes: a seq value that violates the
// declared integer>=0 item schema (negative, fractional, non-number), a body
// that is not valid JSON, a body that is JSON but not an object, and a
// present-but-blank declared query param (directory, workspace — the sibling
// handleSkill convention).
func TestSyncHistoryMalformedInputIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct {
		name, path, body, kind string
	}{
		{"negative seq", "/sync/history", `{"aggregate": -1}`, "Payload"},
		{"fractional seq", "/sync/history", `{"aggregate": 1.5}`, "Payload"},
		{"string seq", "/sync/history", `{"aggregate": "3"}`, "Payload"},
		{"not json", "/sync/history", `not json`, "Body"},
		{"json array body", "/sync/history", `[1,2]`, "Body"},
		{"json null body", "/sync/history", `null`, "Body"},
		{"blank directory param", "/sync/history?directory=", "{}", "Payload"},
		{"blank workspace param", "/sync/history?workspace=", "{}", "Payload"},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
		if status != http.StatusBadRequest {
			t.Fatalf("%s: POST %s (%q): got %d, want 400. Body: %s", tc.name, tc.path, tc.body, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: POST %s: Content-Type = %q, want application/json", tc.name, tc.path, ct)
		}
		assertInvalidRequest(t, tc.path, tc.body, tc.kind, body)
	}
}

// TestSyncHistoryMethodGuard pins the non-POST behaviour: /sync/history is a
// registered route, so a non-POST request reaches the handler and answers the
// sibling METHOD_NOT_ALLOWED envelope — never the net/http default 404 the
// pre-fix tree answered, and never 404 from the handler itself.
func TestSyncHistoryMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/sync/history")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /sync/history: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestSyncHistoryNeighboursUntouched is the non-vacuity control: serving
// /sync/history must not disturb the neighbouring /sync/* family — /sync/steal
// is served too now (ROUTE-ADD-115) and keeps answering its declared 200 for a
// well-formed body — and there is no /sync/* catch-all: a deeper sub-path stays
// 404. The sibling registry route GET /agent keeps answering 200.
func TestSyncHistoryNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/steal", syncStealBody(syncStealValidSessionID))
	if id := decodeSyncStealResponse(t, "/sync/steal", status, header, body); id != syncStealValidSessionID {
		t.Errorf("POST /sync/steal: sessionID = %q, want %q (sibling route intact)", id, syncStealValidSessionID)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/sync/history/sub")
	if status != http.StatusNotFound {
		t.Errorf("GET /sync/history/sub: got %d, want 404 (no /sync/* catch-all). Body: %s", status, body)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/agent")
	if status != http.StatusOK {
		t.Errorf("GET /agent: got %d, want 200 (sibling registry route). Body: %s", status, body)
	}
}

// TestSyncHistoryChiMount is the BUG-009 regression for the new route:
// /sync/history must be listed in MountPatterns so a parent chi router mount
// reaches the shim (a shim-produced 200 proves the wiring; chi's 404 would
// mean the mount lost the route). /sync/steal gained its own exact pattern
// with ROUTE-ADD-115 and reaches the shim through the same mount —
// MountPatterns gains exact paths, never a /sync/* catch-all that would
// silently reclassify the unimplemented siblings (/sync/replay gained its own
// exact pattern with ROUTE-ADD-113 and /sync/start with ROUTE-ADD-114).
func TestSyncHistoryChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/history", "{}")
	if status != http.StatusOK {
		t.Fatalf("POST /sync/history via chi mount: got %d, want 200 — /sync/history must be registered in MountPatterns. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("chi-mounted /sync/history: Content-Type = %q, want application/json", ct)
	}
	var events []map[string]any
	if err := json.Unmarshal(body, &events); err != nil {
		t.Fatalf("chi-mounted /sync/history body must be a JSON array: %v", err)
	}
	if events == nil {
		t.Errorf("chi-mounted /sync/history body = null, want an array (never null)")
	}

	status, stealHeader, stealBody := doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/steal", syncStealBody(syncStealValidSessionID))
	if id := decodeSyncStealResponse(t, "/sync/steal via chi mount", status, stealHeader, stealBody); id != syncStealValidSessionID {
		t.Errorf("chi-mounted /sync/steal: sessionID = %q, want %q", id, syncStealValidSessionID)
	}
}
