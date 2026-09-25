package opencode

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleMCPEndpoint_ImplementedRequestsDelegate is the MCP-DIRECT-001
// RED→GREEN pair for the shim /mcp stub (SPEC-017 §3.9): the stub answer
// stays for plain browsers, but MCP requests (JSON-RPC bodies / SSE
// negotiation) must delegate to the injected MCP handler — the mount a client
// reaches when the shim owns bare /mcp. A 501 for an initialize POST is a
// failed attach.
func TestHandleMCPEndpoint_ImplementedRequestsDelegate(t *testing.T) {
	srv := NewServer(&mockDB{}, "", nil, nil)
	delegated := false
	srv.SetMCPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delegated = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("mcp-handler"))
	}))

	// A JSON-RPC POST body means "an MCP client is speaking to /mcp" —
	// the shim must delegate, not answer its 501 stub.
	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleMCPEndpoint(w, req)

	if w.Code == http.StatusNotImplemented {
		t.Fatal("POST /mcp with a JSON-RPC body answered the 501 stub — MCP clients cannot attach through the shim mount (MCP-DIRECT-001)")
	}
	if !delegated {
		t.Fatalf("POST /mcp with a JSON-RPC body was not delegated to the MCP handler (status %d, body %q)", w.Code, w.Body.String())
	}

	// An Accept: text/event-stream GET means "an MCP client wants the SSE
	// stream" — same delegation rule.
	req2 := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req2.Header.Set("Accept", "text/event-stream")
	w2 := httptest.NewRecorder()
	srv.handleMCPEndpoint(w2, req2)
	if w2.Code == http.StatusNotImplemented {
		t.Fatal("GET /mcp with SSE negotiation answered the 501 stub — MCP clients cannot attach through the shim mount (MCP-DIRECT-001)")
	}
	if delegated != true {
		t.Fatal("GET /mcp with SSE negotiation was not delegated")
	}
}

// TestHandleMCPEndpoint_StubAnswerStillExists pins the retained half of the
// contract: a bare GET without SSE negotiation still answers the documented
// 501 NOT_IMPLEMENTED stub (SPEC-017 §3.9 — the OpenCode contract tests hit
// these unauthenticated and expect 501). This holds whether or not an MCP
// handler is injected: the delegation probes only fire on MCP client shapes.
func TestHandleMCPEndpoint_StubAnswerStillExists(t *testing.T) {
	for _, withHandler := range []bool{false, true} {
		srv := NewServer(&mockDB{}, "", nil, nil)
		if withHandler {
			srv.SetMCPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("bare GET /mcp must not be delegated to the MCP handler")
			}))
		}

		req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		w := httptest.NewRecorder()
		srv.handleMCPEndpoint(w, req)

		if w.Code != http.StatusNotImplemented {
			t.Fatalf("bare GET /mcp (injected handler: %v): expected 501 stub answer, got %d", withHandler, w.Code)
		}
	}
}
