package opencode

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// ch:trace row=ROUTE-ADD-033 spec=specs/024-shim-support-matrix.md wave=consensus-2026-10-24-23-58-46#serial test=internal/shim/opencode/api_pty_connect_test.go::TestApiPtyConnectContract doc=specs/openapi/upstream/consensus-shim-served-surface.yaml prompt=/tmp/consensus-2026-10-24-23-58-46-ROUTE-ADD-033-brief.txt evidence=none:pending witness=none:self-verified-in-worktree verdict=none:pending commit=none:pending memory=none:pending
func TestApiPtyConnectContract(t *testing.T) {
	s, srv := newTestServer(&mockDB{})
	s.skipAuth = true
	defer srv.Close()
	cases := []struct {
		name, key, path string
		want            int
		wantTag         string
	}{
		{"mounted route", "", "/api/pty/pty_abc/connect", http.StatusNotFound, "NotFoundError"},
		{"missing pty", "", "/api/pty/pty_abc/connect", http.StatusNotFound, "NotFoundError"},
		{"bad id", "", "/api/pty/nope/connect", http.StatusNotFound, "NotFoundError"},
		{"blank selector", "", "/api/pty/pty_abc/connect?directory=", http.StatusForbidden, "Forbidden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, srv.URL+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.key != "" {
				req.Header.Set("Authorization", "Bearer "+tc.key)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			body := string(raw)
			if resp.StatusCode != tc.want {
				t.Fatalf("status=%d want %d body=%s", resp.StatusCode, tc.want, body)
			}
			if !strings.Contains(body, tc.wantTag) {
				t.Fatalf("body=%q missing %q", body, tc.wantTag)
			}
		})
	}
}
