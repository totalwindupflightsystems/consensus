package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

const apiSessionRevertClearID = "9b1c4d2e-0000-4000-8000-abcdefabcdef"

func TestApiSessionRevertClearKnownSessionAnswersDeclared204(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": apiSessionRevertClearID})})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost,
		"/api/session/"+apiSessionRevertClearID+"/revert/clear")
	if status != http.StatusNoContent {
		t.Fatalf("known session: got %d, want 204. Body: %s", status, body)
	}
	if len(body) != 0 {
		t.Fatalf("204 body = %q, want empty", body)
	}
	if contentType := header.Get("Content-Type"); contentType != "" {
		t.Fatalf("204 Content-Type = %q, want none", contentType)
	}
}

func TestApiSessionRevertClearUnknownSessionAnswersDeclared404(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: errors.New("sqlite: no rows in result")})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost,
		"/api/session/unknown/revert/clear")
	if status != http.StatusNotFound {
		t.Fatalf("unknown session: got %d, want 404. Body: %s", status, body)
	}
	if !strings.HasPrefix(header.Get("Content-Type"), "application/json") {
		t.Fatalf("404 Content-Type = %q, want application/json", header.Get("Content-Type"))
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode 404 body: %v (%s)", err, body)
	}
	if got["_tag"] != "SessionNotFoundError" || got["sessionID"] != "unknown" {
		t.Fatalf("404 body = %v, want SessionNotFoundError for unknown", got)
	}
}

func TestApiSessionRevertClearMissingAuthAnswersDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	_, srv := newTestServerWithServer(t, s)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost,
		"/api/session/"+apiSessionRevertClearID+"/revert/clear")
	if status != http.StatusUnauthorized {
		t.Fatalf("missing auth: got %d, want 401. Body: %s", status, body)
	}
	if !strings.HasPrefix(header.Get("Content-Type"), "application/json") {
		t.Fatalf("401 Content-Type = %q, want application/json", header.Get("Content-Type"))
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode 401 body: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Fatalf("401 body = %v, want UnauthorizedError", got)
	}
}

func TestApiSessionRevertClearBlankSessionAnswersDeclared400(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodPost,
		"/api/session/%20/revert/clear")
	if status != http.StatusBadRequest {
		t.Fatalf("blank session: got %d, want 400. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode 400 body: %v (%s)", err, body)
	}
	if got["_tag"] != "InvalidRequestError" || got["kind"] != "Path" {
		t.Fatalf("400 body = %v, want InvalidRequestError kind Path", got)
	}
}
