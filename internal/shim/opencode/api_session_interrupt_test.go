package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

const apiSessionInterruptID = "9b1c4d2e-0000-4000-8000-abcdefabcdef"

func TestSessionInterrupt_MissingAuth(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	_, srv := newTestServerWithServer(t, s)
	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, "/api/session/"+apiSessionInterruptID+"/interrupt")
	if status != http.StatusUnauthorized {
		t.Fatalf("missing auth: got %d, want 401. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 401 body: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Fatalf("401 body = %v, want UnauthorizedError", got)
	}
}

func TestSessionInterrupt_ForeignSessionKey(t *testing.T) {
	db := &mockDB{
		queryResults: []db.Row{rowOf(map[string]any{"session_id": "other-session"})},
		queryRow:     rowOf(map[string]any{"id": apiSessionInterruptID}),
	}
	s := NewServer(db, "test-key", nil, nil)
	s.workdir = t.TempDir()
	_, srv := newTestServerWithServer(t, s)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/session/"+apiSessionInterruptID+"/interrupt", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+"cs_sk_foreign-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign session key: got %d, want 403", resp.StatusCode)
	}
}

func TestSessionInterrupt_BlankSessionID(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, "/api/session/%20/interrupt")
	if status != http.StatusBadRequest {
		t.Fatalf("blank sessionID: got %d, want 400. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["_tag"] != "InvalidRequestError" || got["kind"] != "Path" {
		t.Fatalf("400 body = %v, want InvalidRequestError kind Path", got)
	}
}

func TestSessionInterrupt_MultiSegmentSessionID(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, "/api/session/a%2Fb/interrupt")
	if status != http.StatusBadRequest {
		t.Fatalf("multi-segment sessionID: got %d, want 400. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["_tag"] != "InvalidRequestError" || got["kind"] != "Path" {
		t.Fatalf("400 body = %v, want InvalidRequestError kind Path", got)
	}
}

func TestSessionInterrupt_UnknownSession(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: errors.New("sqlite: no rows in result")})
	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, "/api/session/unknown/interrupt")
	if status != http.StatusNotFound {
		t.Fatalf("unknown session: got %d, want 404. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["_tag"] != "SessionNotFoundError" || got["sessionID"] != "unknown" {
		t.Fatalf("404 body = %v, want SessionNotFoundError for unknown", got)
	}
}

func TestSessionInterrupt_HappyPath(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": apiSessionInterruptID})})
	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, "/api/session/"+apiSessionInterruptID+"/interrupt")
	if status != http.StatusNoContent {
		t.Fatalf("known session: got %d, want 204. Body: %s", status, body)
	}
	if len(body) != 0 {
		t.Fatalf("204 body = %q, want empty", body)
	}
}

func TestSessionInterrupt_WrongMethod(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	_, srv := newTestServerWithServer(t, s)
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/"+apiSessionInterruptID+"/interrupt")
	if status != http.StatusNotFound || !strings.Contains(string(body), "404 page not found") {
		t.Fatalf("wrong method: got %d %q, want default 404", status, body)
	}

}

func newTestServerWithServer(t *testing.T, s *Server) (*Server, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv
}
