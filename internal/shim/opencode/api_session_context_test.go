package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const contextFixtureSessionID = "9b1c4d2e-0000-4000-8000-abcdefabcdef"

func TestApiSessionContextKnownSessionAnswersDeclared200(t *testing.T) {
	_, srv, cleanup := newHistoryFixtureServer(t)
	defer cleanup()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+contextFixtureSessionID+"/context")
	if status != http.StatusOK {
		t.Fatalf("known session: got %d, want 200. Body: %s", status, body)
	}
	var got struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 200 body: %v (%s)", err, body)
	}
	if len(got.Data) != 2 {
		t.Fatalf("context data = %d entries, want 2 visible entries: %s", len(got.Data), body)
	}
	if got.Data[0]["memory_event_id"].(float64) != 1 || got.Data[1]["memory_event_id"].(float64) != 2 {
		t.Errorf("context event ids = %v, want 1 and 2", got.Data)
	}
	if got.Data[1]["display_mode"] != "compressed" || got.Data[1]["summary_text"] == "" || got.Data[1]["content"] != nil {
		t.Errorf("compressed context entry must expose summary and withhold content: %v", got.Data[1])
	}
}

func TestApiSessionContextUnknownSessionAnswersDeclared404(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: errors.New("sqlite: no rows in result")})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+contextFixtureSessionID+"/context")
	if status != http.StatusNotFound {
		t.Fatalf("unknown session: got %d, want 404. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 404 body: %v (%s)", err, body)
	}
	if got["_tag"] != "SessionNotFoundError" || got["sessionID"] != contextFixtureSessionID {
		t.Errorf("404 body = %v, want SessionNotFoundError with sessionID", got)
	}
}

func TestApiSessionContextBlankSegmentAnswersDeclared400(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/%20/context")
	if status != http.StatusBadRequest {
		t.Fatalf("blank sessionID: got %d, want 400. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 400 body: %v (%s)", err, body)
	}
	if got["_tag"] != "InvalidRequestError" || got["kind"] != "Path" || got["field"] != "sessionID" {
		t.Errorf("400 body = %v, want InvalidRequestError Path/sessionID", got)
	}
}

func TestApiSessionContextServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+contextFixtureSessionID+"/context")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated context: got %d, want 401. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 401 body: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("401 body = %v, want UnauthorizedError", got)
	}
}

func TestApiSessionContextStoreFailureAnswersDeclared500(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: errors.New("disk I/O error")})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+contextFixtureSessionID+"/context")
	if status != http.StatusInternalServerError {
		t.Fatalf("context store failure: got %d, want 500. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 500 body: %v (%s)", err, body)
	}
	if got["_tag"] != "UnknownError" {
		t.Errorf("500 body = %v, want UnknownError", got)
	}
}

func TestApiSessionContextMountAndEnvelopeShape(t *testing.T) {
	if !isApiSessionContextPath("/api/session/abc/context") {
		t.Fatal("context route shape was not recognized")
	}
	for _, path := range []string{"/api/session/abc", "/api/session/abc/history", "/api/session/abc/context/deeper", "/api/session//context"} {
		if isApiSessionContextPath(path) {
			t.Errorf("isApiSessionContextPath(%q) = true, want false", path)
		}
	}
	if !isV2EnvelopeRoutePath("/api/session/abc/context") {
		t.Fatal("context route is not admitted to v2 UnauthorizedError envelope")
	}
	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/session/{sessionID}/context" {
			mounted = true
		}
	}
	if !mounted {
		t.Fatal("context route missing from MountPatterns")
	}
}

func TestApiSessionContextUndeclaredMethodAnswers405(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodPost,
		"/api/session/"+contextFixtureSessionID+"/context")
	if status != http.StatusMethodNotAllowed || !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
		t.Fatalf("POST context: got %d %q, want 405 METHOD_NOT_ALLOWED", status, body)
	}
}
