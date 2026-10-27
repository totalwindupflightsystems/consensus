package opencode

import (
	"testing"
)

// ============================================================================
// ROUTE-ADD-049 focused contract tests: POST /api/session/{sessionID}/model
// ============================================================================

func TestApiSessionModelSwitch_SessionNotFound(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, "POST", "/api/session/nonexistent/model")
	if status != 404 {
		t.Fatalf("POST /api/session/nonexistent/model: got %d, want 404. Body: %s", status, body)
	}
}

func TestApiSessionModelSwitch_UndeclaredMethod(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, _ := doShimRequest(t, srv.URL, "GET", "/api/session/ses123abc/model")
	if status == 204 {
		t.Fatalf("GET /api/session/ses123abc/model: got 204, should not succeed for unknown session")
	}
}
