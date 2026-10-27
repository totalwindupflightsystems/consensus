package opencode

import (
	"testing"
)

// ============================================================================
// ROUTE-ADD-040 focused contract tests: GET /api/session/{sessionID}
// ============================================================================

func TestApiSessionGet_SessionNotFound(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, "GET", "/api/session/nonexistent123")
	if status != 404 {
		t.Fatalf("GET /api/session/nonexistent123: got %d, want 404. Body: %s", status, body)
	}
}

func TestApiSessionGet_UndeclaredMethod(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, _ := doShimRequest(t, srv.URL, "POST", "/api/session/ses123abc")
	if status == 200 {
		t.Fatalf("POST /api/session/ses123abc: got 200, should not succeed — only GET is declared")
	}
}
