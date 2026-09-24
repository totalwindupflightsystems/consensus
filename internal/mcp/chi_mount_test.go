package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Regression test for C-GAP-005: the MCP handler was mounted with
// apiMux.Handle("/mcp/", mcpSrv.Handler()) — chi v5 treats a bare
// trailing-slash pattern as an EXACT match, so every subpath request
// (/mcp/message, /mcp/sse) 404'd before reaching the MCP mux.
//
// The fix (mirrored here) mounts with "/mcp/*" so the inner stdlib mux,
// which registers absolute paths (/mcp/sse, /mcp/message), is reached.
func TestHandler_MountedUnderChiWildcard(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	srv.sessions["test-sess"] = &mcpSession{id: "test-sess"}

	// Mirror cmd/consensus/main.go: apiMux.Handle("/mcp/*", mcpSrv.Handler())
	r := chi.NewRouter()
	r.Handle("/mcp/*", srv.Handler())

	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "tools/list", "id": 1})
	req := httptest.NewRequest(http.MethodPost, "/mcp/message?sessionId=test-sess", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatal("POST /mcp/message returned 404 — chi mount pattern does not reach the MCP mux (C-GAP-005)")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v (body=%q)", err, w.Body.String())
	}
	if _, ok := resp["jsonrpc"]; !ok {
		t.Errorf("expected JSON-RPC response with jsonrpc field, got %v", resp)
	}
	if resp["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %v", resp["jsonrpc"])
	}
}

// The old mount pattern must stay broken-by-test so nobody regresses it back.
func TestHandler_MountedUnderChiTrailingSlash(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	srv.sessions["test-sess"] = &mcpSession{id: "test-sess"}

	r := chi.NewRouter()
	r.Handle("/mcp/", srv.Handler()) // the buggy pattern

	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "tools/list", "id": 1})
	req := httptest.NewRequest(http.MethodPost, "/mcp/message?sessionId=test-sess", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for the buggy trailing-slash mount (documents why /mcp/* is required), got %d", w.Code)
	}
}
