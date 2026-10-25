package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ch:trace row=ROUTE-ADD-034 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.pty.connectToken test=internal/shim/opencode/api_pty_connect_token_test.go::TestApiPtyConnectTokenContract doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=none:self-verified-in-worktree witness=none:self-verified-in-worktree
func TestApiPtyConnectTokenContract(t *testing.T) {
	s, srv := newTestServer(&mockDB{})
	s.skipAuth = true
	t.Cleanup(srv.Close)

	cases := []struct {
		name, path string
		want       int
		wantTag    string
	}{
		{"unknown pty", "/api/pty/pty_abc/connect-token", http.StatusNotFound, "PtyNotFoundError"},
		{"bad pty id", "/api/pty/nope/connect-token", http.StatusBadRequest, "BadRequest"},
		{"blank selector", "/api/pty/pty_abc/connect-token?directory=", http.StatusBadRequest, "BadRequest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, http.MethodPost, tc.path)
			if status != tc.want {
				t.Fatalf("POST %s: got status %d want %d body=%s", tc.path, status, tc.want, body)
			}
			if !strings.Contains(string(body), tc.wantTag) {
				t.Fatalf("POST %s: body=%q missing %q", tc.path, body, tc.wantTag)
			}
		})
	}
}

func TestApiPtyConnectTokenServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptestNewServer(t, s.Handler())

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost, "/api/pty/pty_abc/connect-token")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST /api/pty/pty_abc/connect-token: got %d want 401 body=%s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type=%q want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("_tag=%v want UnauthorizedError body=%s", got["_tag"], body)
	}
}

// httptestNewServer keeps this test's server cleanup local without duplicating
// the package's request helper setup.
func httptestNewServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}
